package storage

import (
	"testing"
	"time"
)

func TestLog_AppendAndRead(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Create a batch of messages
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key1"), Value: []byte("value1")},
			{Key: []byte("key2"), Value: []byte("value2")},
			{Key: []byte("key3"), Value: []byte("value3")},
		},
	}

	// Append the batch
	offsets, err := log.Append(batch)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if len(offsets) != 3 {
		t.Fatalf("Expected 3 offsets, got %d", len(offsets))
	}

	// Verify offsets are sequential
	for i := 0; i < len(offsets); i++ {
		if offsets[i] != Offset(i) {
			t.Errorf("Expected offset %d, got %d", i, offsets[i])
		}
	}

	// Read the messages back
	messages, err := log.Read(0, 1024)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(messages))
	}

	// Verify message contents
	expectedKeys := []string{"key1", "key2", "key3"}
	expectedValues := []string{"value1", "value2", "value3"}

	for i, msg := range messages {
		if string(msg.Key) != expectedKeys[i] {
			t.Errorf("Message %d: expected key %s, got %s", i, expectedKeys[i], string(msg.Key))
		}
		if string(msg.Value) != expectedValues[i] {
			t.Errorf("Message %d: expected value %s, got %s", i, expectedValues[i], string(msg.Value))
		}
	}

	// Verify high water mark
	hwm := log.HighWaterMark()
	if hwm != 3 {
		t.Errorf("Expected high water mark 3, got %d", hwm)
	}
}

func TestLog_ReadRange(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Append 10 messages
	for i := 0; i < 10; i++ {
		batch := &MessageBatch{
			Messages: []Message{
				{Key: []byte("key"), Value: []byte("value")},
			},
		}
		if _, err := log.Append(batch); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Read a range
	messages, err := log.ReadRange(3, 7)
	if err != nil {
		t.Fatalf("ReadRange failed: %v", err)
	}

	if len(messages) != 4 {
		t.Errorf("Expected 4 messages, got %d", len(messages))
	}
}

func TestLog_MemTableRotation(t *testing.T) {
	dir := t.TempDir()

	// Small max size to trigger rotation
	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncAlways, // Ensure data is fsynced
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      100, // Very small to trigger rotation
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Append many messages to ensure rotation happens multiple times
	numMessages := 20
	for i := 0; i < numMessages; i++ {
		batch := &MessageBatch{
			Messages: []Message{
				{
					Key:   []byte("key"),
					Value: make([]byte, 50), // Large enough to trigger rotation
				},
			},
		}
		if _, err := log.Append(batch); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// All messages should still be readable (even those rotated out of memtables)
	messages, err := log.Read(0, 100000)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// We should be able to read all messages via WAL fallback
	if len(messages) != numMessages {
		t.Errorf("Expected %d messages after rotation, got %d", numMessages, len(messages))
	}
}

func TestLog_HighWaterMark(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Initial high water mark should be 0
	if hwm := log.HighWaterMark(); hwm != 0 {
		t.Errorf("Expected initial high water mark 0, got %d", hwm)
	}

	// Append some messages
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key1"), Value: []byte("value1")},
			{Key: []byte("key2"), Value: []byte("value2")},
		},
	}

	if _, err := log.Append(batch); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// High water mark should now be 2
	if hwm := log.HighWaterMark(); hwm != 2 {
		t.Errorf("Expected high water mark 2, got %d", hwm)
	}
}

func TestLog_Offsets(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Initial offsets
	if start := log.StartOffset(); start != 0 {
		t.Errorf("Expected start offset 0, got %d", start)
	}
	if end := log.EndOffset(); end != 0 {
		t.Errorf("Expected end offset 0, got %d", end)
	}

	// Append messages
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key1"), Value: []byte("value1")},
			{Key: []byte("key2"), Value: []byte("value2")},
			{Key: []byte("key3"), Value: []byte("value3")},
		},
	}

	if _, err := log.Append(batch); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Check offsets after append
	if start := log.StartOffset(); start != 0 {
		t.Errorf("Expected start offset 0, got %d", start)
	}
	if end := log.EndOffset(); end != 3 {
		t.Errorf("Expected end offset 3, got %d", end)
	}
}

func TestLog_Flush(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Append messages
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key1"), Value: []byte("value1")},
		},
	}

	if _, err := log.Append(batch); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Flush should succeed
	if err := log.Flush(); err != nil {
		t.Errorf("Flush failed: %v", err)
	}
}

func TestLog_Delete(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Append messages
	for i := 0; i < 10; i++ {
		batch := &MessageBatch{
			Messages: []Message{
				{Key: []byte("key"), Value: []byte("value")},
			},
		}
		if _, err := log.Append(batch); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Delete messages before offset 5
	if err := log.Delete(5); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Start offset should now be 5
	if start := log.StartOffset(); start != 5 {
		t.Errorf("Expected start offset 5, got %d", start)
	}

	// Reading offset < 5 should fail
	_, err = log.Read(3, 1024)
	if err != ErrOffsetOutOfRange {
		t.Errorf("Expected ErrOffsetOutOfRange, got %v", err)
	}

	// Reading offset >= 5 should succeed
	messages, err := log.Read(5, 1024)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}
}

func TestLog_Close(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	// Append messages
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key1"), Value: []byte("value1")},
		},
	}

	if _, err := log.Append(batch); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Close the log
	if err := log.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Operations after close should fail
	_, err = log.Append(batch)
	if err != ErrLogClosed {
		t.Errorf("Expected ErrLogClosed, got %v", err)
	}

	_, err = log.Read(0, 1024)
	if err != ErrLogClosed {
		t.Errorf("Expected ErrLogClosed, got %v", err)
	}
}

func TestLog_Reopen(t *testing.T) {
	dir := t.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncAlways,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}

	// Create and write to log
	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key1"), Value: []byte("value1")},
			{Key: []byte("key2"), Value: []byte("value2")},
			{Key: []byte("key3"), Value: []byte("value3")},
		},
	}

	if _, err := log.Append(batch); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if err := log.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reopen the log
	log2, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to reopen log: %v", err)
	}
	defer log2.Close()

	// Data should still be readable
	messages, err := log2.Read(0, 1024)
	if err != nil {
		t.Fatalf("Read after reopen failed: %v", err)
	}

	if len(messages) != 3 {
		t.Errorf("Expected 3 messages after reopen, got %d", len(messages))
	}
}

// Benchmarks

func BenchmarkLog_Append(b *testing.B) {
	dir := b.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024 * 100,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024 * 100,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		b.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key"), Value: []byte("value")},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := log.Append(batch); err != nil {
			b.Fatalf("Append failed: %v", err)
		}
	}
}

func BenchmarkLog_Read(b *testing.B) {
	dir := b.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024 * 100,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024 * 100,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		b.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	// Pre-populate with messages
	for i := 0; i < 1000; i++ {
		batch := &MessageBatch{
			Messages: []Message{
				{Key: []byte("key"), Value: []byte("value")},
			},
		}
		if _, err := log.Append(batch); err != nil {
			b.Fatalf("Append failed: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		offset := Offset(i % 1000)
		if _, err := log.Read(offset, 1024); err != nil {
			b.Fatalf("Read failed: %v", err)
		}
	}
}

func BenchmarkLog_AppendBatch(b *testing.B) {
	dir := b.TempDir()

	config := Config{
		DataDir: dir,
		WAL: WALConfig{
			SegmentSize:   1024 * 1024 * 100,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024 * 100,
			NumImmutable: 2,
		},
	}

	log, err := NewLog(dir, config)
	if err != nil {
		b.Fatalf("Failed to create log: %v", err)
	}
	defer log.Close()

	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key1"), Value: []byte("value1")},
			{Key: []byte("key2"), Value: []byte("value2")},
			{Key: []byte("key3"), Value: []byte("value3")},
			{Key: []byte("key4"), Value: []byte("value4")},
			{Key: []byte("key5"), Value: []byte("value5")},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := log.Append(batch); err != nil {
			b.Fatalf("Append failed: %v", err)
		}
	}
}
