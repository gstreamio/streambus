package link

import (
	"testing"
	"time"
)

func TestNewStreamHandler(t *testing.T) {
	link := createTestLink("stream-test", "Stream Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}

	if handler.link != link {
		t.Error("Expected link to be set")
	}

	if handler.checkpointStore != storage {
		t.Error("Expected checkpoint store to be set")
	}

	if handler.metrics == nil {
		t.Error("Expected metrics to be initialized")
	}

	if handler.health == nil {
		t.Error("Expected health to be initialized")
	}

	if handler.health.Status != "initializing" {
		t.Errorf("Expected status 'initializing', got %s", handler.health.Status)
	}
}

func TestNewStreamHandler_InvalidLink(t *testing.T) {
	link := &ReplicationLink{
		ID: "", // Invalid: empty ID
	}
	storage := NewMemoryStorage()

	_, err := NewStreamHandler(link, storage)
	if err == nil {
		t.Error("Expected error for invalid link")
	}
}

func TestNewStreamHandler_WithFilter(t *testing.T) {
	link := createTestLink("filter-test", "Filter Test")
	link.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"test-.*", "prod-.*"},
		ExcludePatterns: []string{".*-temp"},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler with filter failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Verify filter patterns were compiled
	if len(handler.filterPatterns.include) != 2 {
		t.Errorf("Expected 2 include patterns, got %d", len(handler.filterPatterns.include))
	}

	if len(handler.filterPatterns.exclude) != 1 {
		t.Errorf("Expected 1 exclude pattern, got %d", len(handler.filterPatterns.exclude))
	}
}

func TestNewStreamHandler_InvalidFilterPattern(t *testing.T) {
	link := createTestLink("invalid-filter", "Invalid Filter")
	link.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"[invalid(pattern"},
	}

	storage := NewMemoryStorage()

	_, err := NewStreamHandler(link, storage)
	if err == nil {
		t.Error("Expected error for invalid filter pattern")
	}
}

func TestStreamHandler_Stop_NotStarted(t *testing.T) {
	link := createTestLink("stop-test", "Stop Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}

	// Stop should not error even if not started
	err = handler.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestStreamHandler_GetTopicsToReplicate_SpecificTopics(t *testing.T) {
	link := createTestLink("topics-test", "Topics Test")
	link.Topics = []string{"topic1", "topic2", "topic3"}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	topics, err := handler.getTopicsToReplicate()
	if err != nil {
		t.Fatalf("getTopicsToReplicate failed: %v", err)
	}

	if len(topics) != 3 {
		t.Errorf("Expected 3 topics, got %d", len(topics))
	}

	expectedTopics := map[string]bool{
		"topic1": true,
		"topic2": true,
		"topic3": true,
	}

	for _, topic := range topics {
		if !expectedTopics[topic] {
			t.Errorf("Unexpected topic: %s", topic)
		}
	}
}

func TestStreamHandler_StartTopicReplication(t *testing.T) {
	link := createTestLink("start-repl-test", "Start Replication Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start replication for a topic
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	// Verify worker was created
	workerKey := "test-topic-0"
	worker, exists := handler.partitionWorkers[workerKey]
	if !exists {
		t.Error("Expected partition worker to be created")
	}

	if worker.topic != "test-topic" {
		t.Errorf("Expected topic 'test-topic', got %s", worker.topic)
	}

	if worker.partition != 0 {
		t.Errorf("Expected partition 0, got %d", worker.partition)
	}

	// Cancel the context to stop the worker
	worker.cancel()
}

func TestStreamHandler_StartTopicReplication_WithCheckpoint(t *testing.T) {
	link := createTestLink("checkpoint-test", "Checkpoint Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Save a checkpoint first
	checkpoint := &Checkpoint{
		LinkID:       link.ID,
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 100,
		TargetOffset: 95,
		Timestamp:    time.Now(),
		Metadata:     make(map[string]string),
	}
	err = storage.SaveCheckpoint(checkpoint)
	if err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Start replication
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	// Verify worker loaded checkpoint
	workerKey := "test-topic-0"
	worker := handler.partitionWorkers[workerKey]
	if worker.sourceOffset != 100 {
		t.Errorf("Expected source offset 100, got %d", worker.sourceOffset)
	}
	if worker.targetOffset != 95 {
		t.Errorf("Expected target offset 95, got %d", worker.targetOffset)
	}

	worker.cancel()
}

func TestPartitionWorker_SaveCheckpoint(t *testing.T) {
	link := createTestLink("save-checkpoint-test", "Save Checkpoint Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start replication to create a worker
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	workerKey := "test-topic-0"
	worker := handler.partitionWorkers[workerKey]
	defer worker.cancel()

	// Update worker offsets
	worker.sourceOffset = 50
	worker.targetOffset = 45

	// Save checkpoint
	worker.saveCheckpoint()

	// Verify checkpoint was saved
	checkpoint, err := storage.LoadCheckpoint(link.ID, "test-topic", 0)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	if checkpoint.SourceOffset != 50 {
		t.Errorf("Expected source offset 50, got %d", checkpoint.SourceOffset)
	}

	if checkpoint.TargetOffset != 45 {
		t.Errorf("Expected target offset 45, got %d", checkpoint.TargetOffset)
	}
}

func TestPartitionWorker_SaveCheckpoint_NoStorage(t *testing.T) {
	link := createTestLink("no-storage-test", "No Storage Test")

	handler, err := NewStreamHandler(link, nil) // No storage
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start replication to create a worker
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	workerKey := "test-topic-0"
	worker := handler.partitionWorkers[workerKey]
	defer worker.cancel()

	// saveCheckpoint should not panic when no storage
	worker.saveCheckpoint()
}

func TestStreamHandler_PerformHealthCheck(t *testing.T) {
	link := createTestLink("health-check-test", "Health Check Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Perform health check
	handler.performHealthCheck()

	// Verify health was updated
	if handler.health.LastHealthCheck.IsZero() {
		t.Error("Expected LastHealthCheck to be set")
	}

	if !handler.health.SourceClusterReachable {
		t.Error("Expected SourceClusterReachable to be true")
	}

	if !handler.health.TargetClusterReachable {
		t.Error("Expected TargetClusterReachable to be true")
	}
}

func TestStreamHandler_PerformHealthCheck_HighReplicationLag(t *testing.T) {
	link := createTestLink("high-lag-test", "High Lag Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Set high replication lag
	handler.metrics.ReplicationLag = 120000 // 120 seconds

	// Perform health check
	handler.performHealthCheck()

	// Verify health status
	if handler.health.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %s", handler.health.Status)
	}

	if handler.health.ReplicationLagHealthy {
		t.Error("Expected ReplicationLagHealthy to be false")
	}

	if len(handler.health.Issues) == 0 {
		t.Error("Expected health issues to be reported")
	}
}

func TestStreamHandler_PerformHealthCheck_HighErrorRate(t *testing.T) {
	link := createTestLink("high-error-test", "High Error Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Set high error rate
	handler.metrics.ErrorsPerSecond = 15.0

	// Perform health check
	handler.performHealthCheck()

	// Verify health status
	if handler.health.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %s", handler.health.Status)
	}

	if handler.health.ErrorRateHealthy {
		t.Error("Expected ErrorRateHealthy to be false")
	}
}

func TestStreamHandler_PerformHealthCheck_NoRecentCheckpoint(t *testing.T) {
	link := createTestLink("no-checkpoint-test", "No Checkpoint Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Set old checkpoint time
	handler.metrics.LastCheckpoint = time.Now().Add(-10 * time.Minute)

	// Perform health check
	handler.performHealthCheck()

	// Verify health status
	if handler.health.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %s", handler.health.Status)
	}

	if handler.health.CheckpointHealthy {
		t.Error("Expected CheckpointHealthy to be false")
	}
}

func TestStreamHandler_CompileFilterPatterns(t *testing.T) {
	link := createTestLink("compile-test", "Compile Test")
	link.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"^test-"},
		ExcludePatterns: []string{"-temp$"},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Patterns should already be compiled during NewStreamHandler
	if len(handler.filterPatterns.include) == 0 {
		t.Error("Expected include patterns to be compiled")
	}

	if len(handler.filterPatterns.exclude) == 0 {
		t.Error("Expected exclude patterns to be compiled")
	}
}

func TestStreamHandler_CompileFilterPatterns_NoFilter(t *testing.T) {
	link := createTestLink("no-filter-test", "No Filter Test")
	link.Filter = nil

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Should not error with no filter
	err = handler.compileFilterPatterns()
	if err != nil {
		t.Errorf("compileFilterPatterns should not error: %v", err)
	}
}

func TestStreamHandler_ShouldFilterMessage_NoFilter(t *testing.T) {
	link := createTestLink("no-filter-msg-test", "No Filter Message Test")
	link.Filter = nil

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Should not filter when no filter is configured
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message to not be filtered when no filter is configured")
	}
}

func TestStreamHandler_ShouldFilterMessage_TimestampFilter(t *testing.T) {
	link := createTestLink("timestamp-filter-test", "Timestamp Filter Test")
	minTime := time.Now().Add(-1 * time.Hour)
	maxTime := time.Now().Add(1 * time.Hour)

	link.Filter = &FilterConfig{
		Enabled:      true,
		MinTimestamp: minTime,
		MaxTimestamp: maxTime,
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Message within range should not be filtered
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message within timestamp range to not be filtered")
	}

	// Message before min timestamp should be filtered
	oldTime := minTime.Add(-1 * time.Minute)
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		oldTime,
	)

	if !filtered {
		t.Error("Expected message before min timestamp to be filtered")
	}

	// Message after max timestamp should be filtered
	futureTime := maxTime.Add(1 * time.Minute)
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		futureTime,
	)

	if !filtered {
		t.Error("Expected message after max timestamp to be filtered")
	}
}

func TestStreamHandler_ShouldFilterMessage_IncludePattern(t *testing.T) {
	link := createTestLink("include-pattern-test", "Include Pattern Test")
	link.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"important"},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Message matching include pattern should not be filtered
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("important message"),
		nil,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message matching include pattern to not be filtered")
	}

	// Message not matching include pattern should be filtered
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("regular message"),
		nil,
		time.Now(),
	)

	if !filtered {
		t.Error("Expected message not matching include pattern to be filtered")
	}
}

func TestStreamHandler_ShouldFilterMessage_ExcludePattern(t *testing.T) {
	link := createTestLink("exclude-pattern-test", "Exclude Pattern Test")
	link.Filter = &FilterConfig{
		Enabled:         true,
		ExcludePatterns: []string{"debug"},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Message matching exclude pattern should be filtered
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("debug message"),
		nil,
		time.Now(),
	)

	if !filtered {
		t.Error("Expected message matching exclude pattern to be filtered")
	}

	// Message not matching exclude pattern should not be filtered
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("info message"),
		nil,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message not matching exclude pattern to not be filtered")
	}
}

func TestStreamHandler_ShouldFilterMessage_HeaderFilter(t *testing.T) {
	link := createTestLink("header-filter-test", "Header Filter Test")
	link.Filter = &FilterConfig{
		Enabled: true,
		FilterByHeader: map[string]string{
			"priority": "high",
		},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Message with matching header should not be filtered
	headers := map[string][]byte{
		"priority": []byte("high"),
	}
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		headers,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message with matching header to not be filtered")
	}

	// Message without matching header should be filtered
	headers = map[string][]byte{
		"priority": []byte("low"),
	}
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		headers,
		time.Now(),
	)

	if !filtered {
		t.Error("Expected message without matching header to be filtered")
	}

	// Message with missing header should be filtered
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		time.Now(),
	)

	if !filtered {
		t.Error("Expected message with missing header to be filtered")
	}
}

func TestPartitionWorker_ReplicateBatch(t *testing.T) {
	link := createTestLink("replicate-batch-test", "Replicate Batch Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start replication to create a worker
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	workerKey := "test-topic-0"
	worker := handler.partitionWorkers[workerKey]
	defer worker.cancel()

	// Test replicateBatch - currently just a placeholder that sleeps
	err = worker.replicateBatch()
	if err != nil {
		t.Errorf("replicateBatch failed: %v", err)
	}
}
