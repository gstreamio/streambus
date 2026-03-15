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

func TestLog_Compact(t *testing.T) {
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

	// Compact should not error (even though it's a no-op)
	err = log.Compact()
	if err != nil {
		t.Errorf("Compact failed: %v", err)
	}
}

func TestLog_Compact_Closed(t *testing.T) {
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

	log.Close()

	// Compact on closed log should return error
	err = log.Compact()
	if err != ErrLogClosed {
		t.Errorf("Expected ErrLogClosed, got %v", err)
	}
}

func TestLogImpl_SerializeBatch(t *testing.T) {
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

	logInst, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer logInst.Close()

	// Cast to implementation to access private method
	impl := logInst.(*logImpl)

	// Create a batch with messages
	batch := &MessageBatch{
		Messages: []Message{
			{
				Offset: 100,
				Key:    []byte("key1"),
				Value:  []byte("value1"),
			},
			{
				Offset: 101,
				Key:    []byte("key2"),
				Value:  []byte("value2"),
			},
		},
	}

	// Serialize
	data, err := impl.serializeBatch(batch)
	if err != nil {
		t.Fatalf("serializeBatch failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected non-empty serialized data")
	}
}

func TestLogImpl_DeserializeBatch(t *testing.T) {
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

	logInst, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer logInst.Close()

	impl := logInst.(*logImpl)

	// Create a batch
	originalBatch := &MessageBatch{
		Messages: []Message{
			{
				Offset: 200,
				Key:    []byte("testkey"),
				Value:  []byte("testvalue"),
			},
		},
	}

	// Serialize
	data, err := impl.serializeBatch(originalBatch)
	if err != nil {
		t.Fatalf("serializeBatch failed: %v", err)
	}

	// Deserialize
	deserializedBatch, err := impl.deserializeBatch(data)
	if err != nil {
		t.Fatalf("deserializeBatch failed: %v", err)
	}

	if len(deserializedBatch.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(deserializedBatch.Messages))
	}

	msg := deserializedBatch.Messages[0]
	if msg.Offset != 200 {
		t.Errorf("Offset = %d, want 200", msg.Offset)
	}

	if string(msg.Key) != "testkey" {
		t.Errorf("Key = %s, want testkey", msg.Key)
	}

	if string(msg.Value) != "testvalue" {
		t.Errorf("Value = %s, want testvalue", msg.Value)
	}
}

func TestLogImpl_DeserializeBatch_InvalidData(t *testing.T) {
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

	logInst, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	defer logInst.Close()

	impl := logInst.(*logImpl)

	// Test with too-short data
	_, err = impl.deserializeBatch([]byte{0, 1})
	if err == nil {
		t.Error("Expected error for invalid data, got nil")
	}
}

func TestLog_FindOffsetByTimestamp(t *testing.T) {
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

	// Create messages with known timestamps
	baseTime := time.Now()
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key1"), Value: []byte("value1"), Timestamp: baseTime},
			{Key: []byte("key2"), Value: []byte("value2"), Timestamp: baseTime.Add(time.Second)},
			{Key: []byte("key3"), Value: []byte("value3"), Timestamp: baseTime.Add(2 * time.Second)},
			{Key: []byte("key4"), Value: []byte("value4"), Timestamp: baseTime.Add(3 * time.Second)},
			{Key: []byte("key5"), Value: []byte("value5"), Timestamp: baseTime.Add(4 * time.Second)},
		},
	}

	// Append the batch
	_, err = log.Append(batch)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Test finding offset by exact timestamp
	offset, ts, err := log.FindOffsetByTimestamp(baseTime.Add(2 * time.Second).UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	if offset != 2 {
		t.Errorf("Expected offset 2 for exact timestamp, got %d", offset)
	}
	if ts != baseTime.Add(2*time.Second).UnixNano() {
		t.Errorf("Expected timestamp %d, got %d", baseTime.Add(2*time.Second).UnixNano(), ts)
	}

	// Test finding offset for timestamp between messages (should return next message)
	offset, _, err = log.FindOffsetByTimestamp(baseTime.Add(1500 * time.Millisecond).UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	if offset != 2 {
		t.Errorf("Expected offset 2 for in-between timestamp, got %d", offset)
	}

	// Test finding offset for timestamp before all messages
	offset, _, err = log.FindOffsetByTimestamp(baseTime.Add(-time.Hour).UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	if offset != 0 {
		t.Errorf("Expected offset 0 for early timestamp, got %d", offset)
	}

	// Test finding offset for timestamp after all messages (should return end offset)
	offset, _, err = log.FindOffsetByTimestamp(baseTime.Add(time.Hour).UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	if offset != 5 {
		t.Errorf("Expected offset 5 (end offset) for late timestamp, got %d", offset)
	}
}

func TestLog_FindOffsetByTimestamp_EmptyLog(t *testing.T) {
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

	// Test on empty log
	offset, ts, err := log.FindOffsetByTimestamp(time.Now().UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp on empty log failed: %v", err)
	}
	if offset != 0 {
		t.Errorf("Expected offset 0 for empty log, got %d", offset)
	}
	if ts != 0 {
		t.Errorf("Expected timestamp 0 for empty log, got %d", ts)
	}
}

func TestLog_MessageTimestampSerialization(t *testing.T) {
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

	// Create message with specific timestamp
	expectedTimestamp := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key1"), Value: []byte("value1"), Timestamp: expectedTimestamp},
		},
	}

	// Append the batch
	_, err = log.Append(batch)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Read the message back
	messages, err := log.Read(0, 1024)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	// Verify timestamp was preserved
	if !messages[0].Timestamp.Equal(expectedTimestamp) {
		t.Errorf("Timestamp not preserved: expected %v, got %v", expectedTimestamp, messages[0].Timestamp)
	}
}

func TestLog_FindOffsetByTimestamp_BoundaryConditions(t *testing.T) {
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

	// Create messages with specific timestamps
	baseTime := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key0"), Value: []byte("value0"), Timestamp: baseTime},
			{Key: []byte("key1"), Value: []byte("value1"), Timestamp: baseTime.Add(10 * time.Second)},
			{Key: []byte("key2"), Value: []byte("value2"), Timestamp: baseTime.Add(20 * time.Second)},
		},
	}

	_, err = log.Append(batch)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Test: Exact first message timestamp
	offset, ts, err := log.FindOffsetByTimestamp(baseTime.UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	if offset != 0 {
		t.Errorf("Expected offset 0 for first timestamp, got %d", offset)
	}
	if ts != baseTime.UnixNano() {
		t.Errorf("Expected timestamp %d, got %d", baseTime.UnixNano(), ts)
	}

	// Test: Exact last message timestamp
	lastTime := baseTime.Add(20 * time.Second)
	offset, _, err = log.FindOffsetByTimestamp(lastTime.UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	if offset != 2 {
		t.Errorf("Expected offset 2 for last timestamp, got %d", offset)
	}

	// Test: Timestamp between messages (should return next message)
	midTime := baseTime.Add(15 * time.Second) // Between msg 1 and 2
	offset, _, err = log.FindOffsetByTimestamp(midTime.UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	// Should find the message at or after the target time
	if offset < 1 || offset > 2 {
		t.Errorf("Expected offset 1 or 2 for mid timestamp, got %d", offset)
	}
}

func TestLog_FindOffsetByTimestamp_SingleMessage(t *testing.T) {
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

	// Create single message
	msgTime := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("only-key"), Value: []byte("only-value"), Timestamp: msgTime},
		},
	}

	_, err = log.Append(batch)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Test exact match
	offset, ts, err := log.FindOffsetByTimestamp(msgTime.UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	if offset != 0 {
		t.Errorf("Expected offset 0, got %d", offset)
	}
	if ts != msgTime.UnixNano() {
		t.Errorf("Timestamp mismatch: expected %d, got %d", msgTime.UnixNano(), ts)
	}

	// Test before message time (should return first message)
	offset, _, err = log.FindOffsetByTimestamp(msgTime.Add(-time.Hour).UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	if offset != 0 {
		t.Errorf("Expected offset 0 for early timestamp, got %d", offset)
	}

	// Test after message time (should return end offset)
	offset, _, err = log.FindOffsetByTimestamp(msgTime.Add(time.Hour).UnixNano())
	if err != nil {
		t.Fatalf("FindOffsetByTimestamp failed: %v", err)
	}
	if offset != 1 {
		t.Errorf("Expected offset 1 (end offset) for late timestamp, got %d", offset)
	}
}

func TestMessageBatch_LeaderEpochField(t *testing.T) {
	batch := MessageBatch{
		Messages: []Message{
			{Key: []byte("k"), Value: []byte("v")},
		},
		BaseOffset:    100,
		Compression:   CompressionNone,
		Timestamp:     time.Now(),
		ProducerID:    12345,
		ProducerEpoch: 1,
		LeaderEpoch:   5,
	}

	if batch.LeaderEpoch != 5 {
		t.Errorf("LeaderEpoch = %d, want 5", batch.LeaderEpoch)
	}
	if batch.ProducerID != 12345 {
		t.Errorf("ProducerID = %d, want 12345", batch.ProducerID)
	}
	if batch.BaseOffset != 100 {
		t.Errorf("BaseOffset = %d, want 100", batch.BaseOffset)
	}
}

func TestLog_TimestampZeroValue(t *testing.T) {
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

	// Create message with zero timestamp (should default to current time during append)
	batch := &MessageBatch{
		Messages: []Message{
			{Key: []byte("key"), Value: []byte("value")}, // No timestamp set
		},
	}

	_, err = log.Append(batch)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Read the message back
	messages, err := log.Read(0, 1024)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	// Timestamp should be set (not zero)
	if messages[0].Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp for message appended without explicit timestamp")
	}
}
