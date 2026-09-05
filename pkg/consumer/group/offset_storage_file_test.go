package group

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestNewFileOffsetStorage_RequiresDir(t *testing.T) {
	if _, err := NewFileOffsetStorage(""); err == nil {
		t.Error("expected error for empty directory")
	}
}

func TestNewFileOffsetStorage_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "offsets")

	storage, err := NewFileOffsetStorage(dir)
	if err != nil {
		t.Fatalf("NewFileOffsetStorage failed: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected directory to be created: %v", err)
	}

	offsets, err := storage.FetchOffsets("missing-group")
	if err != nil {
		t.Fatalf("FetchOffsets failed: %v", err)
	}
	if len(offsets.Offsets) != 0 {
		t.Errorf("expected no offsets for a fresh store, got %d topics", len(offsets.Offsets))
	}
}

func TestFileOffsetStorage_StoreAndFetch(t *testing.T) {
	dir := t.TempDir()

	storage, err := NewFileOffsetStorage(dir)
	if err != nil {
		t.Fatalf("NewFileOffsetStorage failed: %v", err)
	}

	want := &OffsetAndMetadata{
		Offset:      42,
		Metadata:    "some metadata",
		CommitTime:  time.Now().Truncate(time.Second),
		ExpireTime:  time.Now().Add(time.Hour).Truncate(time.Second),
		LeaderEpoch: 3,
	}

	if err := storage.StoreOffset("group-a", "topic-a", 0, want); err != nil {
		t.Fatalf("StoreOffset failed: %v", err)
	}

	got, err := storage.FetchOffset("group-a", "topic-a", 0)
	if err != nil {
		t.Fatalf("FetchOffset failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected a stored offset, got nil")
	}
	if got.Offset != want.Offset || got.Metadata != want.Metadata || got.LeaderEpoch != want.LeaderEpoch {
		t.Errorf("FetchOffset = %+v, want %+v", got, want)
	}
	if !got.CommitTime.Equal(want.CommitTime) {
		t.Errorf("CommitTime = %v, want %v", got.CommitTime, want.CommitTime)
	}
	if !got.ExpireTime.Equal(want.ExpireTime) {
		t.Errorf("ExpireTime = %v, want %v", got.ExpireTime, want.ExpireTime)
	}

	// FetchOffset must return a copy: mutating it must not corrupt the store.
	got.Offset = 999
	got2, err := storage.FetchOffset("group-a", "topic-a", 0)
	if err != nil {
		t.Fatalf("FetchOffset failed: %v", err)
	}
	if got2.Offset != want.Offset {
		t.Errorf("FetchOffset leaked a mutable reference: Offset = %d, want %d", got2.Offset, want.Offset)
	}
}

func TestFileOffsetStorage_FetchOffset_Missing(t *testing.T) {
	storage, err := NewFileOffsetStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileOffsetStorage failed: %v", err)
	}

	offset, err := storage.FetchOffset("no-such-group", "topic", 0)
	if err != nil {
		t.Fatalf("FetchOffset failed: %v", err)
	}
	if offset != nil {
		t.Errorf("expected nil offset for missing entry, got %+v", offset)
	}
}

func TestFileOffsetStorage_MultipleGroupsTopicsPartitions(t *testing.T) {
	storage, err := NewFileOffsetStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileOffsetStorage failed: %v", err)
	}

	type entry struct {
		group, topic string
		partition    int32
		offset       int64
	}
	entries := []entry{
		{"group-a", "topic-1", 0, 10},
		{"group-a", "topic-1", 1, 11},
		{"group-a", "topic-2", 0, 20},
		{"group-b", "topic-1", 0, 100},
	}

	for _, e := range entries {
		om := &OffsetAndMetadata{Offset: e.offset, CommitTime: time.Now()}
		if err := storage.StoreOffset(e.group, e.topic, e.partition, om); err != nil {
			t.Fatalf("StoreOffset(%+v) failed: %v", e, err)
		}
	}

	for _, e := range entries {
		got, err := storage.FetchOffset(e.group, e.topic, e.partition)
		if err != nil {
			t.Fatalf("FetchOffset(%+v) failed: %v", e, err)
		}
		if got == nil || got.Offset != e.offset {
			t.Errorf("FetchOffset(%+v) = %+v, want offset %d", e, got, e.offset)
		}
	}

	groupAOffsets, err := storage.FetchOffsets("group-a")
	if err != nil {
		t.Fatalf("FetchOffsets failed: %v", err)
	}
	if len(groupAOffsets.Offsets) != 2 {
		t.Fatalf("expected 2 topics for group-a, got %d", len(groupAOffsets.Offsets))
	}
	if len(groupAOffsets.Offsets["topic-1"]) != 2 {
		t.Errorf("expected 2 partitions for group-a/topic-1, got %d", len(groupAOffsets.Offsets["topic-1"]))
	}

	// group-b must not see group-a's data.
	groupBOffsets, err := storage.FetchOffsets("group-b")
	if err != nil {
		t.Fatalf("FetchOffsets failed: %v", err)
	}
	if len(groupBOffsets.Offsets) != 1 {
		t.Fatalf("expected 1 topic for group-b, got %d", len(groupBOffsets.Offsets))
	}
}

func TestFileOffsetStorage_DeleteOffsets(t *testing.T) {
	storage, err := NewFileOffsetStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileOffsetStorage failed: %v", err)
	}

	if err := storage.StoreOffset("group-a", "topic-a", 0, &OffsetAndMetadata{Offset: 1, CommitTime: time.Now()}); err != nil {
		t.Fatalf("StoreOffset failed: %v", err)
	}
	if err := storage.StoreOffset("group-b", "topic-a", 0, &OffsetAndMetadata{Offset: 2, CommitTime: time.Now()}); err != nil {
		t.Fatalf("StoreOffset failed: %v", err)
	}

	if err := storage.DeleteOffsets("group-a"); err != nil {
		t.Fatalf("DeleteOffsets failed: %v", err)
	}

	got, err := storage.FetchOffset("group-a", "topic-a", 0)
	if err != nil {
		t.Fatalf("FetchOffset failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected group-a offsets to be gone, got %+v", got)
	}

	// group-b must survive deleting group-a.
	got, err = storage.FetchOffset("group-b", "topic-a", 0)
	if err != nil {
		t.Fatalf("FetchOffset failed: %v", err)
	}
	if got == nil || got.Offset != 2 {
		t.Errorf("expected group-b offset to survive, got %+v", got)
	}

	// Deleting a group that never existed is a no-op, not an error.
	if err := storage.DeleteOffsets("never-existed"); err != nil {
		t.Errorf("DeleteOffsets on missing group returned error: %v", err)
	}
}

// TestFileOffsetStorage_Reopen verifies the whole point of this storage: a
// fresh instance opened against the same directory sees everything a prior
// instance committed, i.e. offsets survive a broker restart.
func TestFileOffsetStorage_Reopen(t *testing.T) {
	dir := t.TempDir()

	first, err := NewFileOffsetStorage(dir)
	if err != nil {
		t.Fatalf("NewFileOffsetStorage failed: %v", err)
	}

	committed := &OffsetAndMetadata{
		Offset:      55,
		Metadata:    "reopen-test",
		CommitTime:  time.Now().Truncate(time.Second),
		ExpireTime:  time.Now().Add(24 * time.Hour).Truncate(time.Second),
		LeaderEpoch: 7,
	}
	if err := first.StoreOffset("group-a", "topic-a", 2, committed); err != nil {
		t.Fatalf("StoreOffset failed: %v", err)
	}
	if err := first.StoreOffset("group-b", "topic-b", 0, &OffsetAndMetadata{Offset: 1, CommitTime: time.Now()}); err != nil {
		t.Fatalf("StoreOffset failed: %v", err)
	}
	if err := first.DeleteOffsets("group-b"); err != nil {
		t.Fatalf("DeleteOffsets failed: %v", err)
	}

	second, err := NewFileOffsetStorage(dir)
	if err != nil {
		t.Fatalf("reopening storage failed: %v", err)
	}

	got, err := second.FetchOffset("group-a", "topic-a", 2)
	if err != nil {
		t.Fatalf("FetchOffset failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected offset to survive reopen")
	}
	if got.Offset != committed.Offset || got.Metadata != committed.Metadata || got.LeaderEpoch != committed.LeaderEpoch {
		t.Errorf("reopened offset = %+v, want %+v", got, committed)
	}
	if !got.CommitTime.Equal(committed.CommitTime) {
		t.Errorf("CommitTime = %v, want %v", got.CommitTime, committed.CommitTime)
	}
	if !got.ExpireTime.Equal(committed.ExpireTime) {
		t.Errorf("ExpireTime = %v, want %v", got.ExpireTime, committed.ExpireTime)
	}

	// The deleted group must not reappear after reopening.
	deletedGroup, err := second.FetchOffsets("group-b")
	if err != nil {
		t.Fatalf("FetchOffsets failed: %v", err)
	}
	if len(deletedGroup.Offsets) != 0 {
		t.Errorf("expected group-b to remain deleted after reopen, got %+v", deletedGroup.Offsets)
	}
}

func TestFileOffsetStorage_MissingFile(t *testing.T) {
	dir := t.TempDir()

	// A directory with nothing in it yet must open cleanly with no offsets,
	// matching the "no offsets committed yet" case documented on load().
	storage, err := NewFileOffsetStorage(dir)
	if err != nil {
		t.Fatalf("NewFileOffsetStorage on empty dir failed: %v", err)
	}
	offsets, err := storage.FetchOffsets("any-group")
	if err != nil {
		t.Fatalf("FetchOffsets failed: %v", err)
	}
	if len(offsets.Offsets) != 0 {
		t.Errorf("expected no offsets, got %+v", offsets.Offsets)
	}
}

func TestFileOffsetStorage_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "consumer-offsets.json")
	if err := os.WriteFile(path, nil, 0o640); err != nil {
		t.Fatalf("writing empty snapshot failed: %v", err)
	}

	storage, err := NewFileOffsetStorage(dir)
	if err != nil {
		t.Fatalf("NewFileOffsetStorage on empty file failed: %v", err)
	}
	offsets, err := storage.FetchOffsets("any-group")
	if err != nil {
		t.Fatalf("FetchOffsets failed: %v", err)
	}
	if len(offsets.Offsets) != 0 {
		t.Errorf("expected no offsets from an empty file, got %+v", offsets.Offsets)
	}
}

func TestFileOffsetStorage_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "consumer-offsets.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o640); err != nil {
		t.Fatalf("writing corrupt snapshot failed: %v", err)
	}

	if _, err := NewFileOffsetStorage(dir); err == nil {
		t.Error("expected an error opening storage backed by a corrupt snapshot")
	}
}

func TestFileOffsetStorage_UnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "consumer-offsets.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"offsets":[]}`), 0o640); err != nil {
		t.Fatalf("writing snapshot failed: %v", err)
	}

	if _, err := NewFileOffsetStorage(dir); err == nil {
		t.Error("expected an error opening storage backed by an unsupported snapshot version")
	}
}

// TestFileOffsetStorage_FailedFlushKeepsMemoryConsistent verifies that when a
// flush cannot even create its temporary file (so nothing was written to
// disk), StoreOffset rolls back in-memory state to match. This is the
// "did not silently diverge from disk" guarantee the rollback path exists for.
func TestFileOffsetStorage_FailedFlushKeepsMemoryConsistent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission bits are not enforced the same way on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}

	dir := t.TempDir()
	storage, err := NewFileOffsetStorage(dir)
	if err != nil {
		t.Fatalf("NewFileOffsetStorage failed: %v", err)
	}

	// Establish a baseline value that a failed StoreOffset must not disturb.
	baseline := &OffsetAndMetadata{Offset: 1, CommitTime: time.Now()}
	if err := storage.StoreOffset("group-a", "topic-a", 0, baseline); err != nil {
		t.Fatalf("StoreOffset failed: %v", err)
	}

	// Remove write permission on the directory so creating the temp file for
	// the next flush fails before anything is written to disk.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	defer func() { _ = os.Chmod(dir, 0o750) }()

	bad := &OffsetAndMetadata{Offset: 2, CommitTime: time.Now()}
	if err := storage.StoreOffset("group-a", "topic-a", 0, bad); err == nil {
		t.Fatal("expected StoreOffset to fail when the directory is not writable")
	}

	// In-memory state must have rolled back to the baseline, since the
	// failed write never reached disk.
	got, err := storage.FetchOffset("group-a", "topic-a", 0)
	if err != nil {
		t.Fatalf("FetchOffset failed: %v", err)
	}
	if got == nil || got.Offset != baseline.Offset {
		t.Errorf("FetchOffset after failed flush = %+v, want rollback to %+v", got, baseline)
	}

	// Restore permissions and confirm a reopened store agrees with what
	// FetchOffset reported: memory and disk were never allowed to diverge.
	if err := os.Chmod(dir, 0o750); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	reopened, err := NewFileOffsetStorage(dir)
	if err != nil {
		t.Fatalf("reopening storage failed: %v", err)
	}
	got, err = reopened.FetchOffset("group-a", "topic-a", 0)
	if err != nil {
		t.Fatalf("FetchOffset failed: %v", err)
	}
	if got == nil || got.Offset != baseline.Offset {
		t.Errorf("on-disk offset after failed flush = %+v, want %+v", got, baseline)
	}
}

func TestFileOffsetStorage_ConcurrentAccess(t *testing.T) {
	storage, err := NewFileOffsetStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileOffsetStorage failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			groupID := "group-a"
			topic := "topic-a"
			partition := int32(i % 3)
			om := &OffsetAndMetadata{Offset: int64(i), CommitTime: time.Now()}
			if err := storage.StoreOffset(groupID, topic, partition, om); err != nil {
				t.Errorf("StoreOffset failed: %v", err)
			}
			if _, err := storage.FetchOffset(groupID, topic, partition); err != nil {
				t.Errorf("FetchOffset failed: %v", err)
			}
			if _, err := storage.FetchOffsets(groupID); err != nil {
				t.Errorf("FetchOffsets failed: %v", err)
			}
		}(i)
	}
	wg.Wait()
}
