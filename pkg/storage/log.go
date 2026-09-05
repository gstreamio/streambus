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

		// Stamp each message with the batch's producer identity before it is
		// serialized. Without this, ProducerID/ProducerEpoch never reach the
		// record format at all: the batch carries them, but nothing before
		// this point copies them onto the individual messages that actually
		// get written.
		batch.Messages[i].ProducerID = batch.ProducerID
		batch.Messages[i].ProducerEpoch = batch.ProducerEpoch

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
// Four formats exist on disk and all four are readable:
//
//	v0: [KeyLen:4][Key][ValueLen:4][Value]
//	v1: [Timestamp:8][KeyLen:4][Key][ValueLen:4][Value]
//	v2: [Magic:4][Version:1][Timestamp:8][KeyLen:4][Key][ValueLen:4][Value]
//	    [HeaderCount:4]([NameLen:4][Name][ValueLen:4][Value])*
//	v3: v2, followed by [ProducerID:8][ProducerEpoch:2]
//
// v2 exists for two reasons. v0 and v1 have nowhere to put Message.Headers,
// so headers written through them were silently discarded on read. And
// telling v0 from v1 requires a heuristic - is the first word a key length or
// the high half of a timestamp? - which is genuinely ambiguous: a record
// timestamped at or near the Unix epoch has a first word of zero, reads as a
// v0 record with a zero-length key, and comes back with its key and value
// both silently empty. v2's magic prefix removes that guesswork; the
// heuristic survives only to read records written before the format existed.
//
// v3 exists because a read-committed fetch that must keep hiding an aborted
// transaction's records needs to tell, from the record alone, which producer
// wrote it - v2 has nowhere to put that either. The producer fields are
// appended after the header section, rather than woven into v2's body, so a
// v3 record's key/value/header layout is byte-identical to v2's and the two
// share one parser (parseRecordBody) for everything but the trailing fields.
//
// Every new record is written in v3; v0, v1 and v2 survive only to read
// records written before v3 existed.
//
// recordMagicV2 is chosen so it cannot be mistaken for either older format:
// read as a v0 key length it is far above the 1 MB sanity bound, and read as
// the high half of a v1 nanosecond timestamp it is a date hundreds of
// thousands of years beyond the int64 nanosecond range. v3 reuses it rather
// than minting a new one, since the version byte that already follows it is
// what tells v2 and v3 apart.
const (
	recordMagicV2   uint32 = 0xFFFFFFFF
	recordVersionV2 byte   = 2
	recordVersionV3 byte   = 3
	// maxSaneKeyLen is the v0/v1 key-length sanity bound used to tell the two
	// apart. A first word above it cannot be a real key length.
	maxSaneKeyLen uint32 = 1048576
	// maxRecordFieldLen bounds any single length-prefixed field in a v2/v3
	// record, matching the codec's own message ceiling.
	maxRecordFieldLen = 1024 * 1024 * 10
)

// serializeMessage serializes a single message in the current record format.
//
// Everything is written as v3. See the record format constants above for why
// a magic-prefixed format is needed at all, and why v3 exists on top of v2.
func (l *logImpl) serializeMessage(msg *Message) []byte {
	return serializeMessageV3(msg)
}

// serializeMessageV2 writes a message in the header-carrying record format,
// without producer identity. Still used for reading pre-v3 records back in
// tests; production writes go through serializeMessageV3.
func serializeMessageV2(msg *Message) []byte {
	names := sortedHeaderNames(msg.Headers)
	buf := make([]byte, 5+recordBodySize(msg, names))

	binary.BigEndian.PutUint32(buf, recordMagicV2)
	buf[4] = recordVersionV2

	writeRecordBody(buf, 5, msg, names)
	return buf
}

// serializeMessageV3 writes a message in the current record format: v2's
// body, plus the producer identity a read-committed fetch needs to keep
// hiding an aborted transaction's records after the fact (see the format
// comment above).
func serializeMessageV3(msg *Message) []byte {
	names := sortedHeaderNames(msg.Headers)
	buf := make([]byte, 5+recordBodySize(msg, names)+8+2)

	binary.BigEndian.PutUint32(buf, recordMagicV2)
	buf[4] = recordVersionV3

	offset := writeRecordBody(buf, 5, msg, names)
	// #nosec G115 -- same-width reinterpretation of a signed producer ID
	binary.BigEndian.PutUint64(buf[offset:], uint64(msg.ProducerID))
	offset += 8
	// #nosec G115 -- same-width reinterpretation of a signed producer epoch
	binary.BigEndian.PutUint16(buf[offset:], uint16(msg.ProducerEpoch))

	return buf
}

// recordBodySize returns the encoded size of the v2/v3 body (timestamp, key,
// value, headers) for msg, given its sorted header names.
func recordBodySize(msg *Message, names []string) int {
	size := 8 + 4 + len(msg.Key) + 4 + len(msg.Value) + 4
	for _, name := range names {
		size += 4 + len(name) + 4 + len(msg.Headers[name])
	}
	return size
}

// writeRecordBody writes the v2/v3 body - timestamp, key, value, headers -
// into buf starting at offset, and returns the offset immediately after it,
// so a v3 writer can append its producer fields at that point. Headers are
// written in name order so the same message always produces identical bytes,
// which keeps CRCs and compaction comparisons stable.
func writeRecordBody(buf []byte, offset int, msg *Message, names []string) int {
	binary.BigEndian.PutUint64(buf[offset:], uint64(msg.Timestamp.UnixNano()))
	offset += 8

	putRecordLen(buf[offset:], len(msg.Key))
	offset += 4
	copy(buf[offset:], msg.Key)
	offset += len(msg.Key)

	putRecordLen(buf[offset:], len(msg.Value))
	offset += 4
	copy(buf[offset:], msg.Value)
	offset += len(msg.Value)

	putRecordLen(buf[offset:], len(names))
	offset += 4
	for _, name := range names {
		value := msg.Headers[name]
		putRecordLen(buf[offset:], len(name))
		offset += 4
		copy(buf[offset:], name)
		offset += len(name)
		putRecordLen(buf[offset:], len(value))
		offset += 4
		copy(buf[offset:], value)
		offset += len(value)
	}

	return offset
}

// putRecordLen writes a length prefix.
//
// A length beyond maxRecordFieldLen cannot come from a message this process
// built - serializeMessageV2 sizes its buffer from the same lengths, so an
// out-of-range value would already have failed the allocation. The bound is
// what makes that argument explicit rather than an unchecked narrowing at
// each of the five call sites.
func putRecordLen(buf []byte, n int) {
	if n < 0 || n > maxRecordFieldLen {
		binary.BigEndian.PutUint32(buf, 0)
		return
	}
	binary.BigEndian.PutUint32(buf, uint32(n))
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

// newFormatVersion returns the version byte of a magic-prefixed record (v2,
// v3, ...), or 0 if data does not start with the shared magic prefix at all.
// 0 is never a real version, so callers can compare its result directly
// against the recordVersionVN constants without a separate "present" flag.
func newFormatVersion(data []byte) byte {
	if len(data) >= 5 && binary.BigEndian.Uint32(data[0:4]) == recordMagicV2 {
		return data[4]
	}
	return 0
}

// deserializeMessageV2 parses a header-carrying record, without producer
// identity. A truncated record yields whatever parsed cleanly rather than
// panicking on a slice bound.
func deserializeMessageV2(data []byte) *Message {
	msg, _ := parseRecordBody(data, 5)
	return msg
}

// deserializeMessageV3 parses a header-carrying record that also carries the
// identity of the producer that wrote it. Producer identity is appended
// after the header section rather than woven into it, so parseRecordBody is
// shared verbatim with v2; a truncated producer-identity tail is handled the
// same way as every other truncation in this format - the field is simply
// left at its zero value rather than erroring.
func deserializeMessageV3(data []byte) *Message {
	msg, offset := parseRecordBody(data, 5)

	if offset+8 > len(data) {
		return msg
	}
	// #nosec G115 -- same-width reinterpretation of the stored producer ID
	msg.ProducerID = int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	if offset+2 > len(data) {
		return msg
	}
	// #nosec G115 -- same-width reinterpretation of the stored epoch
	msg.ProducerEpoch = int16(binary.BigEndian.Uint16(data[offset:]))

	return msg
}

// parseRecordBody parses the v2/v3 body - timestamp, key, value, headers -
// starting at offset, returning the message parsed so far and the offset
// immediately past the last field it could read. A truncated record yields a
// partial message rather than panicking on a slice bound; the returned
// offset lets a v3 caller continue parsing its trailing producer fields from
// wherever the body actually ended.
func parseRecordBody(data []byte, offset int) (*Message, int) {
	if offset+8 > len(data) {
		return &Message{}, offset
	}
	// #nosec G115 -- same-width reinterpretation of the stored nanoseconds
	timestamp := time.Unix(0, int64(binary.BigEndian.Uint64(data[offset:])))
	offset += 8

	key, offset, ok := readLengthPrefixed(data, offset)
	if !ok {
		return &Message{Timestamp: timestamp}, offset
	}
	value, offset, ok := readLengthPrefixed(data, offset)
	if !ok {
		return &Message{Key: key, Timestamp: timestamp}, offset
	}

	msg := &Message{Key: key, Value: value, Timestamp: timestamp}

	if offset+4 > len(data) {
		return msg, offset
	}
	count := binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// Bound the count by the bytes left: every header needs at least its two
	// length prefixes, so a larger count means the record is corrupt.
	remaining := len(data) - offset
	if remaining < 0 || count > uint32(remaining)/8 { // #nosec G115 -- remaining is non-negative, checked here
		return msg, offset
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

	return msg, offset
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

// deserializeMessage deserializes a single message, reading any of the four
// record formats described above the format constants.
func (l *logImpl) deserializeMessage(data []byte) *Message {
	switch newFormatVersion(data) {
	case recordVersionV3:
		return deserializeMessageV3(data)
	case recordVersionV2:
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
