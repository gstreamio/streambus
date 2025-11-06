package storage

import (
	"errors"
	"time"
)

// Offset represents a message offset in a partition
type Offset int64

// Message represents a single message in the log
type Message struct {
	Offset    Offset            // Message offset
	Key       []byte            // Message key (optional)
	Value     []byte            // Message value
	Headers   map[string][]byte // Message headers
	Timestamp time.Time         // Message timestamp
	CRC       uint32            // CRC32C checksum
}

// MessageBatch represents a batch of messages
type MessageBatch struct {
	Messages     []Message
	BaseOffset   Offset
	Compression  CompressionType
	Timestamp    time.Time
	ProducerID   int64
	ProducerEpoch int16
}

// CompressionType represents the compression algorithm
type CompressionType int8

const (
	CompressionNone CompressionType = iota
	CompressionGzip
	CompressionSnappy
	CompressionLZ4
	CompressionZstd
)

// Config holds storage engine configuration
type Config struct {
	// Directory for data files
	DataDir string

	// WAL configuration
	WAL WALConfig

	// MemTable configuration
	MemTable MemTableConfig

	// SSTable configuration
	SSTable SSTableConfig

	// Compaction configuration
	Compaction CompactionConfig
}

// WALConfig holds Write-Ahead Log configuration
type WALConfig struct {
	// Segment size in bytes (default: 1GB)
	SegmentSize int64

	// Fsync policy
	FsyncPolicy FsyncPolicy

	// Fsync interval for FsyncInterval policy
	FsyncInterval time.Duration
}

// FsyncPolicy determines when to fsync writes
type FsyncPolicy int

const (
	FsyncAlways FsyncPolicy = iota
	FsyncInterval
	FsyncNever
)

// MemTableConfig holds MemTable configuration
type MemTableConfig struct {
	// Max size before flush (default: 64MB)
	MaxSize int64

	// Number of immutable memtables to keep
	NumImmutable int
}

// SSTableConfig holds SSTable configuration
type SSTableConfig struct {
	// Block size (default: 4KB)
	BlockSize int

	// Enable bloom filters
	BloomFilterEnabled bool

	// Bloom filter false positive rate
	BloomFilterFPRate float64

	// Compression for data blocks
	Compression CompressionType
}

// CompactionConfig holds compaction configuration
type CompactionConfig struct {
	// Strategy
	Strategy CompactionStrategy

	// Max concurrent compactions
	MaxConcurrent int

	// Size ratio for leveled compaction
	SizeRatio int
}

// CompactionStrategy represents the compaction strategy
type CompactionStrategy int

const (
	CompactionLeveled CompactionStrategy = iota
	CompactionSizeTiered
	CompactionTimeWindow
)

// Common errors
var (
	ErrOffsetOutOfRange = errors.New("offset out of range")
	ErrLogClosed        = errors.New("log is closed")
	ErrLogCorrupted     = errors.New("log is corrupted")
	ErrInvalidOffset    = errors.New("invalid offset")
	ErrSegmentNotFound  = errors.New("segment not found")
	ErrChecksumMismatch = errors.New("checksum mismatch")
)

// DefaultConfig returns default storage configuration
func DefaultConfig() *Config {
	return &Config{
		DataDir: "/var/lib/streambus/data",
		WAL: WALConfig{
			SegmentSize:   1024 * 1024 * 1024, // 1GB
			FsyncPolicy:   FsyncInterval,
			FsyncInterval: 1 * time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      64 * 1024 * 1024, // 64MB
			NumImmutable: 3,
		},
		SSTable: SSTableConfig{
			BlockSize:          4096, // 4KB
			BloomFilterEnabled: true,
			BloomFilterFPRate:  0.01,
			Compression:        CompressionLZ4,
		},
		Compaction: CompactionConfig{
			Strategy:      CompactionLeveled,
			MaxConcurrent: 2,
			SizeRatio:     10,
		},
	}
}
