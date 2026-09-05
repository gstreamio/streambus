package storage

import (
	"sort"

	"encoding/binary"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

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
	nextOffset     int64
	highWaterMark  int64
	logStartOffset int64

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
		// Genuinely invalid: before retention start, or a negative/unresolved
		// sentinel offset (e.g. -1) sent to the broker literally instead of
		// being resolved client-side first. This must propagate as an error.
		logger.Debug("offset out of range: offset < logStart",
			zap.Int64("offset", int64(offset)),
			zap.Int64("logStart", int64(logStart)))
		return nil, ErrOffsetOutOfRange
	}

	if offset >= hwm {
		// Not an error: this is the normal steady state for a caught-up
		// consumer polling for new messages. Returning ErrOffsetOutOfRange
		// here previously got silently swallowed by handleFetch into an
		// empty-but-successful response - which happened to look right for
		// this specific case, but meant a genuinely invalid offset (the
		// branch above) was indistinguishable from "no new messages yet".
		logger.Debug("caught up: offset >= highWaterMark",
			zap.Int64("offset", int64(offset)),
			zap.Int64("highWaterMark", int64(hwm)))
		return []*Message{}, nil
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

// lookupOffset returns the raw serialized message stored at an offset,
// checking the active memtable, then the immutable memtables, then the WAL.
// The bool reports whether the offset was found anywhere.
//
// Callers must hold at least a read lock on l.mu.
func (l *logImpl) lookupOffset(offset Offset) ([]byte, bool, error) {
	key := offsetToKey(offset)

	value, found, err := l.activeMemTable.Get(key)
	if err != nil {
		return nil, false, err
	}
	if found {
		return value, true, nil
	}

	for _, mt := range l.immutableMemTables {
		value, found, err = mt.Get(key)
		if err != nil {
			return nil, false, err
		}
		if found {
			return value, true, nil
		}
	}

	// Not in any memtable - the message may have been flushed, so fall back
	// to the WAL. A read error here means the offset simply isn't present.
	walData, err := l.wal.Read(offset)
	if err != nil {
		logger.Debug("offset not found in memtables or WAL",
			zap.Int64("offset", int64(offset)), zap.Error(err))
		return nil, false, nil
	}

	return walData, true, nil
}

func (l *logImpl) ReadRange(startOffset, endOffset Offset) ([]*Message, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, ErrLogClosed
	}

	messages := make([]*Message, 0)

	for offset := startOffset; offset < endOffset; offset++ {
		value, found, err := l.lookupOffset(offset)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}

		// Stamp the offset the message was read from. The serialized form
		// does not carry it, so without this every message in the range comes
		// back reporting offset 0.
		msg := l.deserializeMessage(value)
		msg.Offset = offset
		messages = append(messages, msg)
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

	_, err := l.runCompaction()
	return err
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

// FindOffsetByTimestamp finds the first offset whose message timestamp is >= the given timestamp.
// Uses binary search for efficiency when possible, falls back to linear scan.
// Returns (offset, actualTimestamp, error).
func (l *logImpl) FindOffsetByTimestamp(targetTimestamp int64) (Offset, int64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return 0, 0, ErrLogClosed
	}

	startOff := Offset(atomic.LoadInt64(&l.logStartOffset))
	endOff := Offset(atomic.LoadInt64(&l.highWaterMark))

	// Empty log
	if startOff >= endOff {
		return endOff, 0, nil
	}

	// Binary search for the target timestamp
	// We're looking for the first message with timestamp >= targetTimestamp
	low := startOff
	high := endOff - 1
	result := endOff // Default to end if not found
	var resultTimestamp int64

	for low <= high {
		mid := low + (high-low)/2

		// Read the message at mid offset
		msg, err := l.readMessageAt(mid)
		if err != nil {
			// If we can't read this offset, try linear scan from low
			break
		}

		msgTimestamp := msg.Timestamp.UnixNano()

		if msgTimestamp >= targetTimestamp {
			// This message is at or after target, but there might be an earlier one
			result = mid
			resultTimestamp = msgTimestamp
			high = mid - 1
		} else {
			// This message is before target, search later
			low = mid + 1
		}
	}

	// If binary search didn't find anything, fall back to linear scan
	if result == endOff && resultTimestamp == 0 {
		for offset := startOff; offset < endOff; offset++ {
			msg, err := l.readMessageAt(offset)
			if err != nil {
				continue
			}
			msgTimestamp := msg.Timestamp.UnixNano()
			if msgTimestamp >= targetTimestamp {
				return offset, msgTimestamp, nil
			}
		}
	}

	return result, resultTimestamp, nil
}

// readMessageAt reads a single message at the given offset (internal helper)
// Caller must hold at least read lock
func (l *logImpl) readMessageAt(offset Offset) (*Message, error) {
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

	if !found {
		// Try WAL
		value, err = l.wal.Read(offset)
		if err != nil {
			return nil, err
		}
	}

	msg := l.deserializeMessage(value)
	msg.Offset = offset
	return msg, nil
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

// Record formats used by serializeMessage / deserializeMessage.
//
// Three formats exist on disk and all three are readable:
//
//	v0: [KeyLen:4][Key][ValueLen:4][Value]
//	v1: [Timestamp:8][KeyLen:4][Key][ValueLen:4][Value]
//	v2: [Magic:4][Version:1][Timestamp:8][KeyLen:4][Key][ValueLen:4][Value]
//	    [HeaderCount:4]([NameLen:4][Name][ValueLen:4][Value])*
//
// v2 exists because v0 and v1 have nowhere to put Message.Headers, so headers
// written through them were silently discarded on read. A record is written in
// v2 only when it actually carries headers, which keeps every header-less
// record byte-identical to what earlier versions wrote.
//
// recordMagicV2 is chosen so it cannot be mistaken for either older format:
// read as a v0 key length it is far above the 1 MB sanity bound, and read as
// the high half of a v1 nanosecond timestamp it is a date hundreds of
// thousands of years beyond the int64 nanosecond range.
const (
	recordMagicV2   uint32 = 0xFFFFFFFF
	recordVersionV2 byte   = 2
	// maxSaneKeyLen is the v0/v1 key-length sanity bound used to tell the two
	// apart. A first word above it cannot be a real key length.
	maxSaneKeyLen uint32 = 1048576
)

// serializeMessage serializes a single message.
//
// See the record format constants above: a message with headers is written in
// v2, and one without in v1.
func (l *logImpl) serializeMessage(msg *Message) []byte {
	if len(msg.Headers) > 0 {
		return serializeMessageV2(msg)
	}

	size := 8 + 4 + len(msg.Key) + 4 + len(msg.Value) // Added 8 bytes for timestamp
	buf := make([]byte, size)
	offset := 0

	// Timestamp (Unix nanoseconds)
	binary.BigEndian.PutUint64(buf[offset:], uint64(msg.Timestamp.UnixNano()))
	offset += 8

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Key)))
	offset += 4
	copy(buf[offset:], msg.Key)
	offset += len(msg.Key)

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Value)))
	offset += 4
	copy(buf[offset:], msg.Value)

	return buf
}

// serializeMessageV2 writes a message in the header-carrying record format.
func serializeMessageV2(msg *Message) []byte {
	size := 4 + 1 + 8 + 4 + len(msg.Key) + 4 + len(msg.Value) + 4
	names := sortedHeaderNames(msg.Headers)
	for _, name := range names {
		size += 4 + len(name) + 4 + len(msg.Headers[name])
	}

	buf := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint32(buf[offset:], recordMagicV2)
	offset += 4
	buf[offset] = recordVersionV2
	offset++

	binary.BigEndian.PutUint64(buf[offset:], uint64(msg.Timestamp.UnixNano()))
	offset += 8

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Key)))
	offset += 4
	copy(buf[offset:], msg.Key)
	offset += len(msg.Key)

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(msg.Value)))
	offset += 4
	copy(buf[offset:], msg.Value)
	offset += len(msg.Value)

	// Headers are written in name order so the same message always produces
	// identical bytes, which keeps CRCs and compaction comparisons stable.
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(names)))
	offset += 4
	for _, name := range names {
		value := msg.Headers[name]
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(name)))
		offset += 4
		copy(buf[offset:], name)
		offset += len(name)
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(value)))
		offset += 4
		copy(buf[offset:], value)
		offset += len(value)
	}

	return buf
}

// sortedHeaderNames returns header names in sorted order.
func sortedHeaderNames(headers map[string][]byte) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// isRecordV2 reports whether data starts with the v2 record prefix.
func isRecordV2(data []byte) bool {
	return len(data) >= 5 &&
		binary.BigEndian.Uint32(data[0:4]) == recordMagicV2 &&
		data[4] == recordVersionV2
}

// deserializeMessageV2 parses a header-carrying record. A truncated record
// yields whatever parsed cleanly rather than panicking on a slice bound.
func deserializeMessageV2(data []byte) *Message {
	offset := 5 // magic + version

	if offset+8 > len(data) {
		return &Message{}
	}
	timestamp := time.Unix(0, int64(binary.BigEndian.Uint64(data[offset:])))
	offset += 8

	key, offset, ok := readLengthPrefixed(data, offset)
	if !ok {
		return &Message{Timestamp: timestamp}
	}
	value, offset, ok := readLengthPrefixed(data, offset)
	if !ok {
		return &Message{Key: key, Timestamp: timestamp}
	}

	msg := &Message{Key: key, Value: value, Timestamp: timestamp}

	if offset+4 > len(data) {
		return msg
	}
	count := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// Bound the count by the bytes left: every header needs at least its two
	// length prefixes, so a larger count means the record is corrupt.
	if count > uint32(len(data)-offset)/8 {
		return msg
	}

	headers := make(map[string][]byte, count)
	for i := uint32(0); i < count; i++ {
		var name, headerValue []byte
		name, offset, ok = readLengthPrefixed(data, offset)
		if !ok {
			break
		}
		headerValue, offset, ok = readLengthPrefixed(data, offset)
		if !ok {
			break
		}
		headers[string(name)] = headerValue
	}

	if len(headers) > 0 {
		msg.Headers = headers
	}

	return msg
}

// readLengthPrefixed reads a 4-byte length followed by that many bytes,
// returning the value, the new offset, and whether the read succeeded.
func readLengthPrefixed(data []byte, offset int) ([]byte, int, bool) {
	if offset+4 > len(data) {
		return nil, offset, false
	}
	length := binary.BigEndian.Uint32(data[offset:])
	offset += 4
	if offset+int(length) > len(data) {
		return nil, offset, false
	}
	out := make([]byte, length)
	copy(out, data[offset:offset+int(length)])
	return out, offset + int(length), true
}

// deserializeMessage deserializes a single message, reading any of the three
// record formats described above the format constants.
func (l *logImpl) deserializeMessage(data []byte) *Message {
	if isRecordV2(data) {
		return deserializeMessageV2(data)
	}

	offset := 0
	var timestamp time.Time

	// Distinguish v1 (timestamp first) from v0 (key length first) by whether
	// the first word could be a sane key length.
	if len(data) >= 8 {
		possibleKeyLen := binary.BigEndian.Uint32(data[0:4])
		if possibleKeyLen > maxSaneKeyLen || len(data) < 8+4 {
			// Looks like new format with timestamp
			timestamp = time.Unix(0, int64(binary.BigEndian.Uint64(data[offset:])))
			offset += 8
		}
		// else: old format, no timestamp
	}

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
		Key:       key,
		Value:     value,
		Timestamp: timestamp,
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
