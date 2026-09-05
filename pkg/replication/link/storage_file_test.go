package link

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestNewFileStorage_RequiresDir(t *testing.T) {
	if _, err := NewFileStorage(""); err == nil {
		t.Error("expected error for empty directory")
	}
}

func TestNewFileStorage_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "replication")

	storage, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected directory to be created: %v", err)
	}

	links, err := storage.ListLinks()
	if err != nil {
		t.Fatalf("ListLinks failed: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected no links for a fresh store, got %d", len(links))
	}
}

func TestFileStorage_SaveLoadLink(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	l := createTestLink("save-test", "Save Test")
	l.FailoverConfig = DefaultFailoverConfig()
	l.Metrics = &ReplicationMetrics{PartitionMetrics: map[string]*PartitionReplicationMetrics{
		"test-topic-0": {Topic: "test-topic", Partition: 0, SourceOffset: 10, TargetOffset: 9},
	}}
	l.Health = &ReplicationHealth{Status: "healthy", Issues: []string{"warn1"}}

	if err := storage.SaveLink(l); err != nil {
		t.Fatalf("SaveLink failed: %v", err)
	}

	loaded, err := storage.LoadLink("save-test")
	if err != nil {
		t.Fatalf("LoadLink failed: %v", err)
	}
	if loaded.ID != l.ID || loaded.Name != l.Name {
		t.Errorf("LoadLink = %+v, want ID/Name %s/%s", loaded, l.ID, l.Name)
	}
	if loaded.FailoverConfig == nil || loaded.FailoverConfig.MaxConsecutiveFailures != l.FailoverConfig.MaxConsecutiveFailures {
		t.Errorf("FailoverConfig not round-tripped: %+v", loaded.FailoverConfig)
	}
	if loaded.Metrics == nil || loaded.Metrics.PartitionMetrics["test-topic-0"].SourceOffset != 10 {
		t.Errorf("Metrics not round-tripped: %+v", loaded.Metrics)
	}
	if loaded.Health == nil || loaded.Health.Status != "healthy" || len(loaded.Health.Issues) != 1 {
		t.Errorf("Health not round-tripped: %+v", loaded.Health)
	}

	// LoadLink must return a copy: mutating it must not corrupt the store.
	loaded.Name = "mutated"
	loaded.Health.Issues[0] = "mutated"
	again, err := storage.LoadLink("save-test")
	if err != nil {
		t.Fatalf("LoadLink failed: %v", err)
	}
	if again.Name != l.Name {
		t.Errorf("LoadLink leaked a mutable reference: Name = %s, want %s", again.Name, l.Name)
	}
	if again.Health.Issues[0] != "warn1" {
		t.Errorf("LoadLink leaked a mutable Health reference: Issues = %v", again.Health.Issues)
	}
}

// TestFileStorage_SaveLoadLink_FullyPopulated exercises every optional,
// nested field on ReplicationLink -- Filter, Transform, TopicConfig,
// FailoverConfig, plus Metrics/Health with their own nested maps -- through
// a save and a reload from a fresh storage instance (i.e. a simulated
// restart, not just an in-process read-back). ReplicationLink has no JSON
// struct tags and carries time.Time fields and *Metrics/*Health pointers,
// so this is checking that encoding/json's default struct handling is
// actually sufficient, not assuming it.
func TestFileStorage_SaveLoadLink_FullyPopulated(t *testing.T) {
	dir := t.TempDir()

	minTS := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	maxTS := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	l := createTestLink("full-link", "Full Link")
	l.TopicPrefix = "replicated."
	l.TopicConfig = map[string]*TopicReplicationConfig{
		"test-topic": {
			SourceTopic:            "test-topic",
			TargetTopic:            "replicated.test-topic",
			Enabled:                true,
			ReplicateDeletes:       true,
			PreservePartitionCount: true,
			PreserveTimestamps:     true,
			CompressionType:        "zstd",
			Priority:               5,
		},
	}
	l.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"^orders-.*"},
		ExcludePatterns: []string{"^orders-test-.*"},
		FilterByHeader:  map[string]string{"env": "prod"},
		MinTimestamp:    minTS,
		MaxTimestamp:    maxTS,
	}
	l.Transform = &TransformConfig{
		Enabled:          true,
		HeaderTransforms: map[string]string{"trace-id": "uuid"},
		KeyTransform:     "upper(key)",
		ValueTransform:   "redact(value)",
		AddHeaders:       map[string]string{"replicated-by": "streambus"},
		RemoveHeaders:    []string{"internal-only"},
	}
	l.FailoverConfig = &FailoverConfig{
		Enabled:                true,
		FailoverThreshold:      50000,
		FailoverTimeoutMs:      30000,
		MaxConsecutiveFailures: 5,
		AutoFailback:           true,
		FailbackDelayMs:        120000,
		NotificationWebhook:    "https://hooks.example.com/failover",
		NotificationEmail:      "ops@example.com",
	}
	l.Metrics = &ReplicationMetrics{
		TotalMessagesReplicated: 1000,
		TotalBytesReplicated:    2048,
		ReplicationLag:          150,
		ConsecutiveFailures:     2,
		PartitionMetrics: map[string]*PartitionReplicationMetrics{
			"test-topic-0": {
				Topic: "test-topic", Partition: 0,
				SourceOffset: 500, TargetOffset: 498, Lag: 2,
				MessagesReplicated: 500, BytesReplicated: 1024,
				LastReplicatedAt: minTS, Errors: 1,
			},
		},
	}
	l.Health = &ReplicationHealth{
		Status:                 "degraded",
		LastHealthCheck:        maxTS,
		SourceClusterReachable: true,
		TargetClusterReachable: false,
		ReplicationLagHealthy:  false,
		ErrorRateHealthy:       true,
		CheckpointHealthy:      true,
		Issues:                 []string{"target cluster unreachable"},
		Warnings:               []string{"lag above baseline"},
	}

	first, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	if err := first.SaveLink(l); err != nil {
		t.Fatalf("SaveLink failed: %v", err)
	}

	// Reopen against the same directory to force a real decode from disk,
	// not just a read-back of the in-memory clone SaveLink kept.
	second, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("reopening storage failed: %v", err)
	}
	got, err := second.LoadLink("full-link")
	if err != nil {
		t.Fatalf("LoadLink after reopen failed: %v", err)
	}

	if got.TopicPrefix != l.TopicPrefix {
		t.Errorf("TopicPrefix = %q, want %q", got.TopicPrefix, l.TopicPrefix)
	}

	tc, ok := got.TopicConfig["test-topic"]
	if !ok {
		t.Fatal("TopicConfig[test-topic] missing after reopen")
	}
	if tc.TargetTopic != "replicated.test-topic" || tc.CompressionType != "zstd" || tc.Priority != 5 || !tc.ReplicateDeletes {
		t.Errorf("TopicConfig round-trip = %+v", tc)
	}

	if got.Filter == nil {
		t.Fatal("Filter lost across reopen")
	}
	if len(got.Filter.IncludePatterns) != 1 || got.Filter.IncludePatterns[0] != "^orders-.*" {
		t.Errorf("Filter.IncludePatterns = %v", got.Filter.IncludePatterns)
	}
	if len(got.Filter.ExcludePatterns) != 1 || got.Filter.ExcludePatterns[0] != "^orders-test-.*" {
		t.Errorf("Filter.ExcludePatterns = %v", got.Filter.ExcludePatterns)
	}
	if got.Filter.FilterByHeader["env"] != "prod" {
		t.Errorf("Filter.FilterByHeader = %v", got.Filter.FilterByHeader)
	}
	if !got.Filter.MinTimestamp.Equal(minTS) || !got.Filter.MaxTimestamp.Equal(maxTS) {
		t.Errorf("Filter timestamps = %v / %v, want %v / %v",
			got.Filter.MinTimestamp, got.Filter.MaxTimestamp, minTS, maxTS)
	}

	if got.Transform == nil {
		t.Fatal("Transform lost across reopen")
	}
	if got.Transform.HeaderTransforms["trace-id"] != "uuid" {
		t.Errorf("Transform.HeaderTransforms = %v", got.Transform.HeaderTransforms)
	}
	if got.Transform.AddHeaders["replicated-by"] != "streambus" {
		t.Errorf("Transform.AddHeaders = %v", got.Transform.AddHeaders)
	}
	if len(got.Transform.RemoveHeaders) != 1 || got.Transform.RemoveHeaders[0] != "internal-only" {
		t.Errorf("Transform.RemoveHeaders = %v", got.Transform.RemoveHeaders)
	}
	if got.Transform.KeyTransform != "upper(key)" || got.Transform.ValueTransform != "redact(value)" {
		t.Errorf("Transform key/value transforms = %q / %q", got.Transform.KeyTransform, got.Transform.ValueTransform)
	}

	if got.FailoverConfig == nil {
		t.Fatal("FailoverConfig lost across reopen")
	}
	if got.FailoverConfig.MaxConsecutiveFailures != 5 || !got.FailoverConfig.AutoFailback ||
		got.FailoverConfig.NotificationWebhook != "https://hooks.example.com/failover" ||
		got.FailoverConfig.NotificationEmail != "ops@example.com" {
		t.Errorf("FailoverConfig round-trip = %+v", got.FailoverConfig)
	}

	if got.Metrics == nil {
		t.Fatal("Metrics lost across reopen")
	}
	if got.Metrics.TotalMessagesReplicated != 1000 || got.Metrics.ReplicationLag != 150 {
		t.Errorf("Metrics round-trip = %+v", got.Metrics)
	}
	pm, ok := got.Metrics.PartitionMetrics["test-topic-0"]
	if !ok {
		t.Fatal("Metrics.PartitionMetrics[test-topic-0] missing after reopen")
	}
	if pm.SourceOffset != 500 || pm.TargetOffset != 498 || pm.Errors != 1 {
		t.Errorf("PartitionMetrics round-trip = %+v", pm)
	}
	if !pm.LastReplicatedAt.Equal(minTS) {
		t.Errorf("PartitionMetrics.LastReplicatedAt = %v, want %v", pm.LastReplicatedAt, minTS)
	}

	if got.Health == nil {
		t.Fatal("Health lost across reopen")
	}
	if got.Health.Status != "degraded" || got.Health.SourceClusterReachable != true || got.Health.TargetClusterReachable != false {
		t.Errorf("Health round-trip = %+v", got.Health)
	}
	if !got.Health.LastHealthCheck.Equal(maxTS) {
		t.Errorf("Health.LastHealthCheck = %v, want %v", got.Health.LastHealthCheck, maxTS)
	}
	if len(got.Health.Issues) != 1 || got.Health.Issues[0] != "target cluster unreachable" {
		t.Errorf("Health.Issues = %v", got.Health.Issues)
	}
	if len(got.Health.Warnings) != 1 || got.Health.Warnings[0] != "lag above baseline" {
		t.Errorf("Health.Warnings = %v", got.Health.Warnings)
	}
}

func TestFileStorage_LoadLink_NotFound(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	if _, err := storage.LoadLink("no-such-link"); err == nil {
		t.Error("expected error when loading non-existent link")
	}
}

func TestFileStorage_SaveLink_Nil(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	if err := storage.SaveLink(nil); err == nil {
		t.Error("expected error when saving nil link")
	}
}

func TestFileStorage_ListLinks(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	for _, id := range []string{"link-a", "link-b", "link-c"} {
		if err := storage.SaveLink(createTestLink(id, id)); err != nil {
			t.Fatalf("SaveLink(%s) failed: %v", id, err)
		}
	}

	links, err := storage.ListLinks()
	if err != nil {
		t.Fatalf("ListLinks failed: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d", len(links))
	}
}

func TestFileStorage_DeleteLink_CleansUpAll(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	l := createTestLink("cleanup-test", "Cleanup Test")
	if err := storage.SaveLink(l); err != nil {
		t.Fatalf("SaveLink failed: %v", err)
	}

	checkpoint := &Checkpoint{
		LinkID: "cleanup-test", Topic: "test-topic", Partition: 0,
		SourceOffset: 100, TargetOffset: 95, Timestamp: time.Now(),
	}
	if err := storage.SaveCheckpoint(checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	mapping := &OffsetMapping{
		LinkID: "cleanup-test", Topic: "test-topic", Partition: 0,
		Mappings: map[int64]int64{100: 200}, LastUpdated: time.Now(),
	}
	if err := storage.SaveOffsetMapping(mapping); err != nil {
		t.Fatalf("SaveOffsetMapping failed: %v", err)
	}

	if err := storage.DeleteLink("cleanup-test"); err != nil {
		t.Fatalf("DeleteLink failed: %v", err)
	}

	if _, err := storage.LoadLink("cleanup-test"); err == nil {
		t.Error("link should not exist after delete")
	}
	if _, err := storage.LoadCheckpoint("cleanup-test", "test-topic", 0); err == nil {
		t.Error("checkpoint should not exist after delete")
	}
	if _, err := storage.LoadOffsetMapping("cleanup-test", "test-topic", 0); err == nil {
		t.Error("offset mapping should not exist after delete")
	}
}

func TestFileStorage_SaveLoadCheckpoint(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	checkpoint := &Checkpoint{
		LinkID: "cp-test", Topic: "test-topic", Partition: 0,
		SourceOffset: 100, TargetOffset: 95, Timestamp: time.Now().Truncate(time.Second),
		Metadata: map[string]string{"key1": "value1"},
	}
	if err := storage.SaveCheckpoint(checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	loaded, err := storage.LoadCheckpoint("cp-test", "test-topic", 0)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded.SourceOffset != 100 || loaded.TargetOffset != 95 {
		t.Errorf("LoadCheckpoint = %+v", loaded)
	}
	if loaded.Metadata["key1"] != "value1" {
		t.Errorf("Metadata not round-tripped: %+v", loaded.Metadata)
	}
	if !loaded.Timestamp.Equal(checkpoint.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", loaded.Timestamp, checkpoint.Timestamp)
	}

	// LoadCheckpoint must return a copy.
	loaded.Metadata["key1"] = "mutated"
	again, err := storage.LoadCheckpoint("cp-test", "test-topic", 0)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if again.Metadata["key1"] != "value1" {
		t.Errorf("LoadCheckpoint leaked a mutable reference: %v", again.Metadata)
	}
}

func TestFileStorage_LoadCheckpoint_NotFound(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	if _, err := storage.LoadCheckpoint("no-link", "topic", 0); err == nil {
		t.Error("expected error for missing link")
	}

	checkpoint := &Checkpoint{LinkID: "link-x", Topic: "topic-x", Partition: 0, Timestamp: time.Now()}
	if err := storage.SaveCheckpoint(checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	if _, err := storage.LoadCheckpoint("link-x", "other-topic", 0); err == nil {
		t.Error("expected error for missing topic")
	}
	if _, err := storage.LoadCheckpoint("link-x", "topic-x", 999); err == nil {
		t.Error("expected error for missing partition")
	}
}

func TestFileStorage_SaveCheckpoint_Nil(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	if err := storage.SaveCheckpoint(nil); err == nil {
		t.Error("expected error when saving nil checkpoint")
	}
}

func TestFileStorage_SaveLoadOffsetMapping(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	mapping := &OffsetMapping{
		LinkID: "mapping-test", Topic: "test-topic", Partition: 0,
		Mappings:    map[int64]int64{100: 200, 101: 201},
		LastUpdated: time.Now().Truncate(time.Second),
	}
	if err := storage.SaveOffsetMapping(mapping); err != nil {
		t.Fatalf("SaveOffsetMapping failed: %v", err)
	}

	loaded, err := storage.LoadOffsetMapping("mapping-test", "test-topic", 0)
	if err != nil {
		t.Fatalf("LoadOffsetMapping failed: %v", err)
	}
	if len(loaded.Mappings) != 2 || loaded.Mappings[100] != 200 || loaded.Mappings[101] != 201 {
		t.Errorf("Mappings not round-tripped: %+v", loaded.Mappings)
	}
	if !loaded.LastUpdated.Equal(mapping.LastUpdated) {
		t.Errorf("LastUpdated = %v, want %v", loaded.LastUpdated, mapping.LastUpdated)
	}
}

func TestFileStorage_LoadOffsetMapping_NotFound(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	if _, err := storage.LoadOffsetMapping("no-link", "topic", 0); err == nil {
		t.Error("expected error for missing link")
	}

	mapping := &OffsetMapping{LinkID: "link-x", Topic: "topic-x", Partition: 0, Mappings: map[int64]int64{1: 2}}
	if err := storage.SaveOffsetMapping(mapping); err != nil {
		t.Fatalf("SaveOffsetMapping failed: %v", err)
	}

	if _, err := storage.LoadOffsetMapping("link-x", "other-topic", 0); err == nil {
		t.Error("expected error for missing topic")
	}
	if _, err := storage.LoadOffsetMapping("link-x", "topic-x", 999); err == nil {
		t.Error("expected error for missing partition")
	}
}

func TestFileStorage_SaveOffsetMapping_Nil(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	if err := storage.SaveOffsetMapping(nil); err == nil {
		t.Error("expected error when saving nil offset mapping")
	}
}

// TestFileStorage_Reopen verifies that links, checkpoints and offset
// mappings all survive a broker restart: a fresh storage instance opened
// against the same directory must see everything a prior instance saved.
func TestFileStorage_Reopen(t *testing.T) {
	dir := t.TempDir()

	first, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	l := createTestLink("reopen-link", "Reopen Link")
	l.FailoverConfig = DefaultFailoverConfig()
	l.Status = ReplicationStatusPaused
	if err := first.SaveLink(l); err != nil {
		t.Fatalf("SaveLink failed: %v", err)
	}

	checkpoint := &Checkpoint{
		LinkID: "reopen-link", Topic: "test-topic", Partition: 0,
		SourceOffset: 42, TargetOffset: 40, Timestamp: time.Now().Truncate(time.Second),
		Metadata: map[string]string{"k": "v"},
	}
	if err := first.SaveCheckpoint(checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	mapping := &OffsetMapping{
		LinkID: "reopen-link", Topic: "test-topic", Partition: 0,
		Mappings: map[int64]int64{42: 40}, LastUpdated: time.Now().Truncate(time.Second),
	}
	if err := first.SaveOffsetMapping(mapping); err != nil {
		t.Fatalf("SaveOffsetMapping failed: %v", err)
	}

	second, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("reopening storage failed: %v", err)
	}

	loadedLink, err := second.LoadLink("reopen-link")
	if err != nil {
		t.Fatalf("LoadLink after reopen failed: %v", err)
	}
	if loadedLink.Status != ReplicationStatusPaused {
		t.Errorf("Status = %v, want %v", loadedLink.Status, ReplicationStatusPaused)
	}
	if loadedLink.FailoverConfig == nil {
		t.Error("FailoverConfig lost across reopen")
	}

	loadedCheckpoint, err := second.LoadCheckpoint("reopen-link", "test-topic", 0)
	if err != nil {
		t.Fatalf("LoadCheckpoint after reopen failed: %v", err)
	}
	if loadedCheckpoint.SourceOffset != 42 || loadedCheckpoint.TargetOffset != 40 {
		t.Errorf("checkpoint after reopen = %+v", loadedCheckpoint)
	}

	loadedMapping, err := second.LoadOffsetMapping("reopen-link", "test-topic", 0)
	if err != nil {
		t.Fatalf("LoadOffsetMapping after reopen failed: %v", err)
	}
	if loadedMapping.Mappings[42] != 40 {
		t.Errorf("offset mapping after reopen = %+v", loadedMapping.Mappings)
	}
}

// TestFileStorage_ManagerReloadsLinksOnRestart is the end-to-end version of
// TestFileStorage_Reopen: it exercises the actual path a broker restart
// takes, through NewManager -> loadLinksFromStorage, rather than calling
// Storage methods directly.
func TestFileStorage_ManagerReloadsLinksOnRestart(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	mgr := NewManager(storage)
	l := createTestLink("restart-link", "Restart Link")
	if err := mgr.CreateLink(l); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Simulate a restart: open a brand new storage instance against the same
	// directory, and a brand new manager on top of it.
	reopenedStorage, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("reopening storage failed: %v", err)
	}
	reopenedMgr := NewManager(reopenedStorage)
	defer func() { _ = reopenedMgr.Close() }()

	got, err := reopenedMgr.GetLink("restart-link")
	if err != nil {
		t.Fatalf("GetLink after restart failed: %v", err)
	}
	if got.Name != "Restart Link" {
		t.Errorf("GetLink after restart = %+v, want Name Restart Link", got)
	}
}

func TestFileStorage_MissingFile(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage on empty dir failed: %v", err)
	}
	links, err := storage.ListLinks()
	if err != nil {
		t.Fatalf("ListLinks failed: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected no links, got %d", len(links))
	}
}

func TestFileStorage_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replication-links.json")
	if err := os.WriteFile(path, nil, 0o640); err != nil {
		t.Fatalf("writing empty snapshot failed: %v", err)
	}

	storage, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("NewFileStorage on empty file failed: %v", err)
	}
	links, err := storage.ListLinks()
	if err != nil {
		t.Fatalf("ListLinks failed: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected no links from an empty file, got %d", len(links))
	}
}

func TestFileStorage_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replication-links.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o640); err != nil {
		t.Fatalf("writing corrupt snapshot failed: %v", err)
	}

	if _, err := NewFileStorage(dir); err == nil {
		t.Error("expected an error opening storage backed by a corrupt snapshot")
	}
}

func TestFileStorage_UnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replication-links.json")
	if err := os.WriteFile(path, []byte(`{"version":99}`), 0o640); err != nil {
		t.Fatalf("writing snapshot failed: %v", err)
	}

	if _, err := NewFileStorage(dir); err == nil {
		t.Error("expected an error opening storage backed by an unsupported snapshot version")
	}
}

// TestFileStorage_FailedFlushKeepsMemoryConsistent mirrors the equivalent
// group.FileOffsetStorage test: when the temp-file write cannot even start
// (directory not writable), the mutation must roll back rather than leaving
// memory ahead of what is on disk.
func TestFileStorage_FailedFlushKeepsMemoryConsistent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission bits are not enforced the same way on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}

	dir := t.TempDir()
	storage, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	baseline := createTestLink("perm-test", "Baseline")
	if err := storage.SaveLink(baseline); err != nil {
		t.Fatalf("SaveLink failed: %v", err)
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	defer func() { _ = os.Chmod(dir, 0o750) }()

	updated := createTestLink("perm-test", "Updated")
	if err := storage.SaveLink(updated); err == nil {
		t.Fatal("expected SaveLink to fail when the directory is not writable")
	}

	got, err := storage.LoadLink("perm-test")
	if err != nil {
		t.Fatalf("LoadLink failed: %v", err)
	}
	if got.Name != "Baseline" {
		t.Errorf("LoadLink after failed flush = %+v, want rollback to Baseline", got)
	}

	if err := os.Chmod(dir, 0o750); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	reopened, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("reopening storage failed: %v", err)
	}
	got, err = reopened.LoadLink("perm-test")
	if err != nil {
		t.Fatalf("LoadLink failed: %v", err)
	}
	if got.Name != "Baseline" {
		t.Errorf("on-disk link after failed flush = %+v, want Baseline", got)
	}
}

func TestFileStorage_ConcurrentAccess(t *testing.T) {
	storage, err := NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l := createTestLink("concurrent-test", "Concurrent Test")
			_ = storage.SaveLink(l)
			_, _ = storage.LoadLink("concurrent-test")
		}(i)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = storage.ListLinks()
		}(i)
	}
	wg.Wait()
}
