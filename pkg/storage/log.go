package storage

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/gstreamio/streambus/pkg/logger"
	"go.uber.org/zap"
)

// logImpl implements the Log interface
type logImpl struct {
	config Config
	dir    string

	mu sync.RWMutex

	// Write-ahead log for durability
	wal WAL

	// Active memtable for writes
	activeMemTable MemTable

	// Immutable memtables being flushed
	immutableMemTables []MemTable

	// Offset tracking
	nextOffset      int64
	highWaterMark   int64
	logStartOffset  int64

	// Flush coordination
	flushInProgress atomic.Bool //nolint:unused // Reserved for future use in async flush coordination
	flushChan       chan struct{}

	closed bool
}

// NewLog creates a new partition log
func NewLog(dir string, config Config) (Log, error) {
	// Create WAL
	walDir := filepath.Join(dir, "wal")
	wal, err := NewWAL(walDir, config.WAL)
	if err != nil {
		return nil, fmt.Errorf("failed to create WAL: %w", err)
	}

	log := &logImpl{
		config:             config,
		dir:                dir,
		wal:                wal,
		activeMemTable:     NewMemTable(),
		immutableMemTables: make([]MemTable, 0, config.MemTable.NumImmutable),
		flushChan:          make(chan struct{}, 1),
	}

	// Recover from WAL
	if err := log.recover(); err != nil {
		return nil, fmt.Errorf("failed to recover from WAL: %w", err)
	}

	return log, nil
}

func (l *logImpl) Append(batch *MessageBatch) ([]Offset, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil, ErrLogClosed
	}

	// Assign offsets to messages and write them individually
	offsets := make([]Offset, len(batch.Messages))
	currentOffset := atomic.LoadInt64(&l.nextOffset)

	for i := range batch.Messages {
		offsets[i] = Offset(currentOffset)
		batch.Messages[i].Offset = Offset(currentOffset)

		// Serialize individual message for WAL
		msgData := l.serializeMessage(&batch.Messages[i])

		// Append to WAL for durability (one entry per message)
		if _, err := l.wal.Append(msgData); err != nil {
			return nil, fmt.Errorf("WAL append failed: %w", err)
		}

		// Add to active memtable
		key := offsetToKey(batch.Messages[i].Offset)
		if err := l.activeMemTable.Put(key, msgData); err != nil {
			return nil, fmt.Errorf("memtable put failed: %w", err)
		}

		currentOffset++
	}

	// Update next offset and high water mark
	atomic.StoreInt64(&l.nextOffset, currentOffset)
	atomic.StoreInt64(&l.highWaterMark, currentOffset)

	// Check if we need to flush memtable
	if l.activeMemTable.Size() >= l.config.MemTable.MaxSize {
		l.rotateMemTable()
	}

	return offsets, nil
}

func (l *logImpl) Read(offset Offset, maxBytes int) ([]*Message, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, ErrLogClosed
	}

	logStart := Offset(atomic.LoadInt64(&l.logStartOffset))
	hwm := Offset(atomic.LoadInt64(&l.highWaterMark))

	// Log at debug level for troubleshooting
	logger.Debug("read request",
		zap.Int64("offset", int64(offset)),
		zap.Int64("logStart", int64(logStart)),
		zap.Int64("highWaterMark", int64(hwm)))

	if offset < logStart {
		logger.Debug("offset out of range: offset < logStart",
			zap.Int64("offset", int64(offset)),
			zap.Int64("logStart", int64(logStart)))
		return nil, ErrOffsetOutOfRange
	}

	if offset >= hwm {
		logger.Debug("offset out of range: offset >= highWaterMark",
			zap.Int64("offset", int64(offset)),
			zap.Int64("highWaterMark", int64(hwm)))
		return nil, ErrOffsetOutOfRange
	}

	messages := make([]*Message, 0)
	bytesRead := 0
	currentOffset := offset

	// Try reading from active memtable first
	for bytesRead < maxBytes && currentOffset < Offset(atomic.LoadInt64(&l.highWaterMark)) {
		key := offsetToKey(currentOffset)

		// Check active memtable
		value, found, err := l.activeMemTable.Get(key)
		if err != nil {
			return nil, err
		}

		if !found {
			// Check immutable memtables
			for _, mt := range l.immutableMemTables {
				value, found, err = mt.Get(key)
				if err != nil {
					return nil, err
				}
				if found {
					break
				}
			}
		}

		if found {
			msg := l.deserializeMessage(value)
			msg.Offset = currentOffset
			messages = append(messages, msg)
			bytesRead += len(msg.Value) + len(msg.Key)
			currentOffset++
		} else {
			// Not in memtable, try reading from WAL
			logger.Debug("offset not in memtable, trying WAL", zap.Int64("offset", int64(currentOffset)))
			walData, err := l.wal.Read(currentOffset)
			if err != nil {
				// If not in WAL either, skip this offset
				logger.Debug("WAL read error", zap.Int64("offset", int64(currentOffset)), zap.Error(err))
				currentOffset++
				continue
			}

			// Deserialize the message from WAL
			logger.Debug("found in WAL", zap.Int64("offset", int64(currentOffset)), zap.Int("bytes", len(walData)))
			msg := l.deserializeMessage(walData)
			msg.Offset = currentOffset
			messages = append(messages, msg)
			bytesRead += len(msg.Value) + len(msg.Key)
			currentOffset++
		}
	}

	return messages, nil
}

func (l *logImpl) ReadRange(startOffset, endOffset Offset) ([]*Message, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, ErrLogClosed
	}

	messages := make([]*Message, 0)

	for offset := startOffset; offset < endOffset; offset++ {
		key := offsetToKey(offset)

		// Check active memtable
		value, found, err := l.activeMemTable.Get(key)
		if err != nil {
			return nil, err
		}

		if !found {
			// Check immutable memtables
			for _, mt := range l.immutableMemTables {
				value, found, err = mt.Get(key)
				if err != nil {
					return nil, err
				}
				if found {
					break
				}
			}
		}

		if found {
			msg := l.deserializeMessage(value)
			messages = append(messages, msg)
		}
	}

	return messages, nil
}

func (l *logImpl) HighWaterMark() Offset {
	return Offset(atomic.LoadInt64(&l.highWaterMark))
}

func (l *logImpl) StartOffset() Offset {
	return Offset(atomic.LoadInt64(&l.logStartOffset))
}

func (l *logImpl) EndOffset() Offset {
	return Offset(atomic.LoadInt64(&l.nextOffset))
}

func (l *logImpl) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrLogClosed
	}

	// Sync WAL
	if err := l.wal.Sync(); err != nil {
		return err
	}

	// TODO: Flush immutable memtables to SSTables
	// For now, we just sync the WAL

	return nil
}

func (l *logImpl) Compact() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrLogClosed
	}

	// TODO: Implement compaction
	// For now, this is a no-op

	return nil
}

func (l *logImpl) Delete(beforeOffset Offset) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrLogClosed
	}

	// Update log start offset
	atomic.StoreInt64(&l.logStartOffset, int64(beforeOffset))

	// Truncate WAL
	if err := l.wal.Truncate(beforeOffset); err != nil {
		return err
	}

	// TODO: Delete old SSTables

	return nil
}

func (l *logImpl) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	l.closed = true

	// Close WAL
	if err := l.wal.Close(); err != nil {
		return err
	}

	// TODO: Flush any pending data

	return nil
}

// rotateMemTable moves the active memtable to immutable list
// Caller must hold write lock
func (l *logImpl) rotateMemTable() {
	// Move active to immutable
	l.immutableMemTables = append(l.immutableMemTables, l.activeMemTable)

	// Create new active memtable
	l.activeMemTable = NewMemTable()

	// Trim immutable list if needed
	if len(l.immutableMemTables) > l.config.MemTable.NumImmutable {
		// In production, we'd flush the oldest to SSTable before removing
		l.immutableMemTables = l.immutableMemTables[1:]
	}

	// Signal flush goroutine (non-blocking)
	select {
	case l.flushChan <- struct{}{}:
	default:
	}
}

// serializeBatch serializes a message batch for WAL
func (l *logImpl) serializeBatch(batch *MessageBatch) ([]byte, error) {
	// Simple serialization: [NumMessages:4][Message1][Message2]...
	// Each message: [OffsetLen:4][Offset:8][KeyLen:4][Key:n][ValueLen:4][Value:n]

	size := 4 // NumMessages
	for _, msg := range batch.Messages {
		size += 8 + 4 + len(msg.Key) + 4 + len(msg.Value)
	}

	buf := make([]byte, size)
	offset := 0

	// Write number of messages
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(batch.Messages)))
	offset += 4

	// Write each message
	for _, msg := range batch.Messages {
		// Offset
		binary.BigEndian.PutUint64(buf[offset:], uint64(msg.Offset))
		offset += 8

		// Key
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Key)))
		offset += 4
		copy(buf[offset:], msg.Key)
		offset += len(msg.Key)

		// Value
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Value)))
		offset += 4
		copy(buf[offset:], msg.Value)
		offset += len(msg.Value)
	}

	return buf, nil
}

// deserializeBatch deserializes a message batch from WAL
func (l *logImpl) deserializeBatch(data []byte) (*MessageBatch, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("invalid batch data: too short")
	}

	offset := 0

	// Read number of messages
	numMessages := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	batch := &MessageBatch{
		Messages: make([]Message, numMessages),
	}

	// Read each message
	for i := uint32(0); i < numMessages; i++ {
		if offset+16 > len(data) {
			return nil, fmt.Errorf("invalid batch data: unexpected end")
		}

		// Offset
		msgOffset := Offset(binary.BigEndian.Uint64(data[offset:]))
		offset += 8

		// Key
		keyLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4
		if offset+int(keyLen) > len(data) {
			return nil, fmt.Errorf("invalid batch data: key overflow")
		}
		key := make([]byte, keyLen)
		copy(key, data[offset:offset+int(keyLen)])
		offset += int(keyLen)

		// Value
		if offset+4 > len(data) {
			return nil, fmt.Errorf("invalid batch data: unexpected end")
		}
		valueLen := binary.BigEndian.Uint32(data[offset:])
		offset += 4
		if offset+int(valueLen) > len(data) {
			return nil, fmt.Errorf("invalid batch data: value overflow")
		}
		value := make([]byte, valueLen)
		copy(value, data[offset:offset+int(valueLen)])
		offset += int(valueLen)

		batch.Messages[i] = Message{
			Offset: msgOffset,
			Key:    key,
			Value:  value,
		}
	}

	return batch, nil
}

// serializeMessage serializes a single message
func (l *logImpl) serializeMessage(msg *Message) []byte {
	// [KeyLen:4][Key:n][ValueLen:4][Value:n]
	size := 4 + len(msg.Key) + 4 + len(msg.Value)
	buf := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Key)))
	offset += 4
	copy(buf[offset:], msg.Key)
	offset += len(msg.Key)

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Value)))
	offset += 4
	copy(buf[offset:], msg.Value)

	return buf
}

// deserializeMessage deserializes a single message
func (l *logImpl) deserializeMessage(data []byte) *Message {
	offset := 0

	// Key
	keyLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	key := make([]byte, keyLen)
	copy(key, data[offset:offset+int(keyLen)])
	offset += int(keyLen)

	// Value
	valueLen := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	value := make([]byte, valueLen)
	copy(value, data[offset:offset+int(valueLen)])

	return &Message{
		Key:   key,
		Value: value,
	}
}

// offsetToKey converts an offset to a memtable key
func offsetToKey(offset Offset) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(offset))
	return key
}

// recover rebuilds state from WAL on startup
func (l *logImpl) recover() error {
	// Get the current offset from WAL
	nextOffset := l.wal.NextOffset()

	logger.Info("starting recovery", zap.Int64("nextOffset", int64(nextOffset)))

	// Set our offsets based on WAL state
	atomic.StoreInt64(&l.nextOffset, int64(nextOffset))
	atomic.StoreInt64(&l.highWaterMark, int64(nextOffset))
	atomic.StoreInt64(&l.logStartOffset, 0)

	// Rebuild memtable from WAL for better performance
	// Try to recover last N messages into memtable (where N is based on memtable size)
	// We'll recover from the end backwards to get the most recent messages

	if nextOffset == 0 {
		// No data to recover
		logger.Debug("no data to recover")
		return nil
	}

	// Calculate how many messages we should try to recover
	// We want to fill the memtable as much as possible to avoid WAL lookups
	// Default memtable size is 64MB, estimate ~2KB per message average
	maxMemTableSize := l.config.MemTable.MaxSize
	if maxMemTableSize == 0 {
		maxMemTableSize = 64 * 1024 * 1024 // Default 64MB
	}

	// Estimate we can fit roughly maxMemTableSize/2KB messages
	// But cap at all available messages
	estimatedCapacity := Offset(maxMemTableSize / 2048)
	maxToRecover := estimatedCapacity
	if nextOffset < estimatedCapacity {
		maxToRecover = nextOffset
	}

	startOffset := nextOffset - maxToRecover
	if startOffset < 0 {
		startOffset = 0
	}

	logger.Info("recovering messages from WAL",
		zap.Int64("startOffset", int64(startOffset)),
		zap.Int64("endOffset", int64(nextOffset)),
		zap.Int64("memTableSize", maxMemTableSize))

	recovered := 0
	skipped := 0
	totalSize := int64(0)

	for offset := Offset(startOffset); offset < nextOffset; offset++ {
		// Try to read from WAL
		walData, err := l.wal.Read(offset)
		if err != nil {
			// Skip missing offsets (could be truncated or not exist)
			skipped++
			continue
		}

		// Check if we're approaching memtable size limit
		messageSize := int64(len(walData))
		if totalSize+messageSize >= maxMemTableSize && recovered > 0 {
			// We've filled the memtable, but let's try to rotate and continue
			logger.Debug("memtable full, rotating",
				zap.Int64("offset", int64(offset)),
				zap.Int64("size", totalSize),
				zap.Int("messages", recovered))
			l.rotateMemTable()

			// Check if we can continue with more immutable memtables
			if len(l.immutableMemTables) >= l.config.MemTable.NumImmutable {
				logger.Debug("max immutable memtables reached, stopping recovery",
					zap.Int("maxImmutable", l.config.MemTable.NumImmutable))
				break
			}

			// Reset counters for new memtable
			totalSize = 0
		}

		// Add to active memtable
		key := offsetToKey(offset)
		if err := l.activeMemTable.Put(key, walData); err != nil {
			logger.Warn("memtable put failed during recovery",
				zap.Int64("offset", int64(offset)),
				zap.Error(err))
			break
		}
		recovered++
		totalSize += messageSize
	}

	logger.Info("recovery complete",
		zap.Int("recovered", recovered),
		zap.Int("skipped", skipped),
		zap.Int64("totalSize", totalSize),
		zap.Int("activeMemtables", 1),
		zap.Int("immutableMemtables", len(l.immutableMemTables)))
	return nil
}
