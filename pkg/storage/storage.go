package storage

import (
	"context"
	"io"
)

// Storage is the main interface for the storage engine
type Storage interface {
	// Append appends a batch of messages to a partition and returns their offsets
	Append(ctx context.Context, partition int, batch *MessageBatch) ([]Offset, error)

	// Read reads messages from a partition starting at the given offset
	Read(ctx context.Context, partition int, offset Offset, maxBytes int) ([]*Message, error)

	// ReadRange reads messages in the specified offset range
	ReadRange(ctx context.Context, partition int, startOffset, endOffset Offset) ([]*Message, error)

	// GetHighWaterMark returns the high water mark (last committed offset + 1)
	GetHighWaterMark(partition int) (Offset, error)

	// GetLogStartOffset returns the first available offset
	GetLogStartOffset(partition int) (Offset, error)

	// GetLogEndOffset returns the last offset + 1
	GetLogEndOffset(partition int) (Offset, error)

	// Flush forces all pending writes to disk
	Flush() error

	// Compact triggers compaction for a partition
	Compact(partition int) error

	// Delete deletes messages before the given offset
	Delete(partition int, beforeOffset Offset) error

	// Close closes the storage engine
	Close() error
}

// Log represents a partition log
type Log interface {
	// Append appends a batch of messages
	Append(batch *MessageBatch) ([]Offset, error)

	// Read reads messages starting at offset
	Read(offset Offset, maxBytes int) ([]*Message, error)

	// ReadRange reads messages in range
	ReadRange(startOffset, endOffset Offset) ([]*Message, error)

	// HighWaterMark returns the high water mark
	HighWaterMark() Offset

	// StartOffset returns the first available offset
	StartOffset() Offset

	// EndOffset returns the last offset + 1
	EndOffset() Offset

	// Flush flushes pending writes
	Flush() error

	// Compact triggers compaction
	Compact() error

	// Delete deletes messages before offset
	Delete(beforeOffset Offset) error

	// Close closes the log
	Close() error
}

// WAL represents the Write-Ahead Log
type WAL interface {
	// Append appends a record to the WAL
	Append(data []byte) (Offset, error)

	// Read reads a record at the given offset
	Read(offset Offset) ([]byte, error)

	// Sync forces all pending writes to disk
	Sync() error

	// Truncate truncates the WAL before the given offset
	Truncate(beforeOffset Offset) error

	// Close closes the WAL
	Close() error
}

// MemTable represents an in-memory table
type MemTable interface {
	// Put adds a key-value pair
	Put(key []byte, value []byte) error

	// Get retrieves a value by key
	Get(key []byte) ([]byte, bool, error)

	// Delete marks a key as deleted
	Delete(key []byte) error

	// Size returns the current size in bytes
	Size() int64

	// Iterator returns an iterator over all entries
	Iterator() Iterator

	// Clear clears all entries
	Clear()
}

// SSTable represents a Sorted String Table
type SSTable interface {
	// Get retrieves a value by key
	Get(key []byte) ([]byte, bool, error)

	// Iterator returns an iterator over all entries
	Iterator() Iterator

	// Close closes the SSTable
	Close() error
}

// Iterator provides ordered iteration over key-value pairs
type Iterator interface {
	// Next advances to the next entry
	Next() bool

	// Key returns the current key
	Key() []byte

	// Value returns the current value
	Value() []byte

	// Err returns any error encountered
	Err() error

	// Close closes the iterator
	Close() error
}

// Index provides offset to file position mapping
type Index interface {
	// Lookup finds the file position for an offset
	Lookup(offset Offset) (position int64, err error)

	// Add adds an offset mapping
	Add(offset Offset, position int64) error

	// Truncate truncates the index
	Truncate(beforeOffset Offset) error

	// Sync syncs the index to disk
	Sync() error

	// Close closes the index
	Close() error
}

// Segment represents a log segment
type Segment interface {
	// BaseOffset returns the base offset of this segment
	BaseOffset() Offset

	// NextOffset returns the next offset to be written
	NextOffset() Offset

	// Size returns the size in bytes
	Size() int64

	// Append appends a batch
	Append(batch *MessageBatch) error

	// Read reads messages starting at offset
	Read(offset Offset, maxBytes int) ([]*Message, error)

	// Sync syncs to disk
	Sync() error

	// Close closes the segment
	Close() error

	// IsFull checks if segment is full
	IsFull() bool

	io.Closer
}
