package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndex_AddAndLookup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.idx")

	idx, err := NewIndex(path)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	// Add entries
	entries := []struct {
		offset   Offset
		position int64
	}{
		{0, 0},
		{100, 1000},
		{200, 2000},
		{300, 3000},
	}

	for _, e := range entries {
		if err := idx.Add(e.offset, e.position); err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	// Lookup exact matches
	for _, e := range entries {
		pos, err := idx.Lookup(e.offset)
		if err != nil {
			t.Fatalf("Lookup failed for offset %d: %v", e.offset, err)
		}
		if pos != e.position {
			t.Fatalf("Position mismatch for offset %d: got %d, want %d", e.offset, pos, e.position)
		}
	}

	// Lookup offsets between entries (should return previous entry)
	pos, err := idx.Lookup(150)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if pos != 1000 {
		t.Fatalf("Expected position 1000 for offset 150, got %d", pos)
	}

	pos, err = idx.Lookup(250)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if pos != 2000 {
		t.Fatalf("Expected position 2000 for offset 250, got %d", pos)
	}
}

func TestIndex_Sync(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.idx")

	// Create index and add entries
	idx, err := NewIndex(path)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	idx.Add(0, 0)
	idx.Add(100, 1000)
	idx.Add(200, 2000)

	// Sync to disk
	if err := idx.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	idx.Close()

	// Reopen and verify entries are persisted
	idx2, err := NewIndex(path)
	if err != nil {
		t.Fatalf("Failed to reopen index: %v", err)
	}
	defer idx2.Close()

	pos, err := idx2.Lookup(100)
	if err != nil {
		t.Fatalf("Lookup failed after reopen: %v", err)
	}
	if pos != 1000 {
		t.Fatalf("Position mismatch after reopen: got %d, want 1000", pos)
	}
}

func TestIndex_Truncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.idx")

	idx, err := NewIndex(path)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	// Add entries
	for i := 0; i < 10; i++ {
		idx.Add(Offset(i*100), int64(i*1000))
	}

	// Truncate before offset 500
	if err := idx.Truncate(500); err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Verify old entries are gone
	_, err = idx.Lookup(400)
	if err != ErrOffsetOutOfRange {
		t.Fatalf("Expected ErrOffsetOutOfRange for old offset, got %v", err)
	}

	// Verify new entries remain
	pos, err := idx.Lookup(600)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	// Offset 600 maps to position 6000
	if pos != 6000 {
		t.Fatalf("Position mismatch after truncate: got %d, want 6000", pos)
	}

	// Check offset 500 (first remaining entry)
	pos, err = idx.Lookup(500)
	if err != nil {
		t.Fatalf("Lookup failed for offset 500: %v", err)
	}
	if pos != 5000 {
		t.Fatalf("Position mismatch for offset 500: got %d, want 5000", pos)
	}
}

func TestIndex_EmptyLookup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.idx")

	idx, err := NewIndex(path)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	// Lookup in empty index
	_, err = idx.Lookup(0)
	if err != ErrOffsetOutOfRange {
		t.Fatalf("Expected ErrOffsetOutOfRange for empty index, got %v", err)
	}
}

func TestIndex_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.idx")

	idx, err := NewIndex(path)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	idx.Add(100, 1000)
	idx.Add(200, 2000)

	// Lookup before first entry
	_, err = idx.Lookup(50)
	if err != ErrOffsetOutOfRange {
		t.Fatalf("Expected ErrOffsetOutOfRange for offset before first entry, got %v", err)
	}
}

func TestIndex_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.idx")

	// Create and populate index
	idx, err := NewIndex(path)
	if err != nil {
		t.Fatalf("Failed to create index: %v", err)
	}

	for i := 0; i < 1000; i++ {
		idx.Add(Offset(i*10), int64(i*100))
	}

	if err := idx.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	idx.Close()

	// Verify file exists and has correct size
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Index file not found: %v", err)
	}

	expectedSize := int64(1000 * indexEntrySize)
	if stat.Size() != expectedSize {
		t.Fatalf("Index file size mismatch: got %d, want %d", stat.Size(), expectedSize)
	}

	// Reopen and verify all entries
	idx2, err := NewIndex(path)
	if err != nil {
		t.Fatalf("Failed to reopen index: %v", err)
	}
	defer idx2.Close()

	for i := 0; i < 1000; i++ {
		pos, err := idx2.Lookup(Offset(i * 10))
		if err != nil {
			t.Fatalf("Lookup failed for offset %d: %v", i*10, err)
		}
		expectedPos := int64(i * 100)
		if pos != expectedPos {
			t.Fatalf("Position mismatch at offset %d: got %d, want %d", i*10, pos, expectedPos)
		}
	}
}

func BenchmarkIndex_Add(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.idx")

	idx, err := NewIndex(path)
	if err != nil {
		b.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx.Add(Offset(i), int64(i*100))
	}
}

func BenchmarkIndex_Lookup(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.idx")

	idx, err := NewIndex(path)
	if err != nil {
		b.Fatalf("Failed to create index: %v", err)
	}
	defer idx.Close()

	// Populate index
	for i := 0; i < 10000; i++ {
		idx.Add(Offset(i*10), int64(i*100))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := idx.Lookup(Offset((i % 10000) * 10))
		if err != nil {
			b.Fatalf("Lookup failed: %v", err)
		}
	}
}
