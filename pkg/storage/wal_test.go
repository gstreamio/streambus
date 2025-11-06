package storage

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestWAL_AppendAndRead(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		SegmentSize:   1024 * 1024, // 1MB
		FsyncPolicy:   FsyncAlways,
		FsyncInterval: time.Second,
	}

	wal, err := NewWAL(dir, config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()

	// Append data
	testData := [][]byte{
		[]byte("data1"),
		[]byte("data2"),
		[]byte("data3"),
	}

	offsets := make([]Offset, 0)
	for _, data := range testData {
		offset, err := wal.Append(data)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
		offsets = append(offsets, offset)
	}

	// Read data
	for i, offset := range offsets {
		data, err := wal.Read(offset)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if !bytes.Equal(data, testData[i]) {
			t.Fatalf("Data mismatch at offset %d: got %s, want %s", offset, data, testData[i])
		}
	}
}

func TestWAL_Sync(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		SegmentSize:   1024 * 1024,
		FsyncPolicy:   FsyncNever,
		FsyncInterval: time.Second,
	}

	wal, err := NewWAL(dir, config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}

	// Append without fsync
	offset, err := wal.Append([]byte("test data"))
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Explicit sync
	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	wal.Close()

	// Reopen and verify data is persisted
	wal2, err := NewWAL(dir, config)
	if err != nil {
		t.Fatalf("Failed to reopen WAL: %v", err)
	}
	defer wal2.Close()

	data, err := wal2.Read(offset)
	if err != nil {
		t.Fatalf("Read failed after reopen: %v", err)
	}

	if !bytes.Equal(data, []byte("test data")) {
		t.Fatalf("Data mismatch after reopen")
	}
}

func TestWAL_SegmentRoll(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		SegmentSize:   100, // Small segment to force rolling
		FsyncPolicy:   FsyncAlways,
		FsyncInterval: time.Second,
	}

	wal, err := NewWAL(dir, config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()

	// Append enough data to force segment roll
	for i := 0; i < 10; i++ {
		data := []byte(fmt.Sprintf("this is a longer message to force segment rolling %d", i))
		_, err := wal.Append(data)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Check that multiple segment files were created
	files, err := filepath.Glob(filepath.Join(dir, "*.wal"))
	if err != nil {
		t.Fatalf("Failed to list segment files: %v", err)
	}

	if len(files) < 2 {
		t.Fatalf("Expected at least 2 segment files, got %d", len(files))
	}
}

func TestWAL_Truncate(t *testing.T) {
	dir := t.TempDir()

	// Use small segment size to force multiple segments
	config := WALConfig{
		SegmentSize:   200, // Small to force multiple segments
		FsyncPolicy:   FsyncAlways,
		FsyncInterval: time.Second,
	}

	wal, err := NewWAL(dir, config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()

	// Append data to create multiple segments
	for i := 0; i < 100; i++ {
		_, err := wal.Append([]byte(fmt.Sprintf("data-message-number-%d-with-padding", i)))
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Count segments before truncate
	files1, _ := filepath.Glob(filepath.Join(dir, "*.wal"))
	t.Logf("Segments before truncate: %d", len(files1))

	// Truncate before offset 50
	if err := wal.Truncate(50); err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Count segments after truncate
	files2, _ := filepath.Glob(filepath.Join(dir, "*.wal"))
	t.Logf("Segments after truncate: %d", len(files2))

	// Should have fewer segments now
	if len(files2) >= len(files1) {
		t.Fatalf("Expected fewer segments after truncate")
	}

	// Verify we can't read old data (in removed segments)
	_, err = wal.Read(0)
	if err != ErrOffsetOutOfRange {
		t.Fatalf("Expected ErrOffsetOutOfRange for old data, got %v", err)
	}

	// Verify we can read data from remaining segments
	data, err := wal.Read(90)
	if err != nil {
		t.Fatalf("Read failed for offset 90: %v", err)
	}

	if !bytes.Equal(data, []byte("data-message-number-90-with-padding")) {
		t.Fatalf("Data mismatch after truncate")
	}
}

func TestWAL_Reopen(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		SegmentSize:   1024 * 1024,
		FsyncPolicy:   FsyncAlways,
		FsyncInterval: time.Second,
	}

	// Create WAL and append data
	wal, err := NewWAL(dir, config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}

	testData := [][]byte{
		[]byte("data1"),
		[]byte("data2"),
		[]byte("data3"),
	}

	for _, data := range testData {
		_, err := wal.Append(data)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	wal.Close()

	// Reopen WAL
	wal2, err := NewWAL(dir, config)
	if err != nil {
		t.Fatalf("Failed to reopen WAL: %v", err)
	}
	defer wal2.Close()

	// Verify data
	for i := 0; i < len(testData); i++ {
		data, err := wal2.Read(Offset(i))
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if !bytes.Equal(data, testData[i]) {
			t.Fatalf("Data mismatch at offset %d", i)
		}
	}

	// Append more data
	offset, err := wal2.Append([]byte("data4"))
	if err != nil {
		t.Fatalf("Append after reopen failed: %v", err)
	}

	if offset != 3 {
		t.Fatalf("Expected offset 3, got %d", offset)
	}
}

func BenchmarkWAL_Append(b *testing.B) {
	dir := b.TempDir()

	config := WALConfig{
		SegmentSize:   1024 * 1024 * 1024, // 1GB
		FsyncPolicy:   FsyncNever,
		FsyncInterval: time.Second,
	}

	wal, err := NewWAL(dir, config)
	if err != nil {
		b.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()

	data := []byte("this is a test message for benchmarking")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := wal.Append(data)
		if err != nil {
			b.Fatalf("Append failed: %v", err)
		}
	}
}

func BenchmarkWAL_AppendWithSync(b *testing.B) {
	dir := b.TempDir()

	config := WALConfig{
		SegmentSize:   1024 * 1024 * 1024, // 1GB
		FsyncPolicy:   FsyncAlways,
		FsyncInterval: time.Second,
	}

	wal, err := NewWAL(dir, config)
	if err != nil {
		b.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()

	data := []byte("this is a test message for benchmarking")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := wal.Append(data)
		if err != nil {
			b.Fatalf("Append failed: %v", err)
		}
	}
}

func BenchmarkWAL_Read(b *testing.B) {
	dir := b.TempDir()

	config := WALConfig{
		SegmentSize:   1024 * 1024 * 1024,
		FsyncPolicy:   FsyncAlways, // Need to sync for reads to work
		FsyncInterval: time.Second,
	}

	wal, err := NewWAL(dir, config)
	if err != nil {
		b.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()

	// Prepare data
	numRecords := 1000
	for i := 0; i < numRecords; i++ {
		_, err := wal.Append([]byte(fmt.Sprintf("data%d", i)))
		if err != nil {
			b.Fatalf("Append failed: %v", err)
		}
	}

	// Ensure all data is written
	if err := wal.Sync(); err != nil {
		b.Fatalf("Sync failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := wal.Read(Offset(i % numRecords))
		if err != nil {
			b.Fatalf("Read failed at offset %d: %v", i%numRecords, err)
		}
	}
}
