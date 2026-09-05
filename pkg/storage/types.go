package storage

import (
	"errors"
	"fmt"
	"strings"
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

	// ProducerID and ProducerEpoch identify the producer that wrote this
	// record, stamped from the batch it arrived in (see logImpl.Append).
	// ProducerID 0 is the sentinel for a non-transactional record, matching
	// the convention already used by Partition's open-transaction tracking.
	// Persisted as part of the v3 record format so a reader - in particular
	// a read-committed fetch - can tell which transaction a record belonged
	// to without needing anything beyond the record itself.
	ProducerID    int64
	ProducerEpoch int16
}

// MessageBatch represents a batch of messages
type MessageBatch struct {
	Messages      []Message
	BaseOffset    Offset
	Compression   CompressionType
	Timestamp     time.Time
	ProducerID    int64
	ProducerEpoch int16
	// LeaderEpoch is the epoch of the partition leader when this batch was written.
	// Used for leader fencing and offset validation during replication.
	LeaderEpoch int64
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

	// MessageFormatVersion selects the on-disk record format newly appended
	// records are written in. The zero value, MessageFormatUnset, means "use
	// the default" so existing callers that never set this field keep
	// writing the current default format with no change in behaviour.
	//
	// This is StreamBus's equivalent of Kafka's
	// inter.broker.protocol.version / log.message.format.version: it exists
	// so a rolling upgrade can run the new broker binary while it still
	// writes the old record format, and only flip to the new format once the
	// whole fleet is upgraded and rollback past that point is no longer
	// needed. See the record format documentation in log.go for what each
	// version carries on disk; reading is unaffected by this setting - every
	// version this broker knows how to read (v0-v3) is always read
	// correctly, regardless of what it currently writes.
	//
	// Setting this to MessageFormatV2 costs transactional isolation: v2 has
	// nowhere to put a record's producer identity, so a transactional record
	// (nonzero ProducerID) cannot be downgraded to v2 without losing the
	// information a read-committed fetch needs to keep hiding that
	// transaction's records if it is later aborted. Append refuses to write
	// such a record under v2 rather than silently dropping its producer
	// identity - see ErrTransactionalRecordNeedsV3. An operator who selects
	// v2 is choosing to lose read_committed isolation for the duration; if
	// transactional records must keep working, stay on v3 or don't run
	// producers transactionally while pinned to v2.
	MessageFormatVersion MessageFormatVersion
}

// MessageFormatVersion selects the record format Append serializes new
// messages with. See Config.MessageFormatVersion for the operational
// rationale.
type MessageFormatVersion int

const (
	// MessageFormatUnset is Config's zero value: it means "use the current
	// default format" (v3 as of this writing). Kept distinct from
	// MessageFormatV3 so DefaultConfig and zero-value Configs are visibly
	// "no opinion set" rather than accidentally pinning a version that a
	// future default change would then need to migrate away from.
	MessageFormatUnset MessageFormatVersion = 0
	// MessageFormatV2 writes the header-carrying record format without
	// producer identity. Readable by any broker that understands v2 or
	// later, but see Config.MessageFormatVersion's doc comment for the
	// transactional-isolation cost.
	MessageFormatV2 MessageFormatVersion = 2
	// MessageFormatV3 writes the current record format, including producer
	// identity.
	MessageFormatV3 MessageFormatVersion = 3
)

// String renders a MessageFormatVersion the way it is spelled in config
// files and error messages ("v2", "v3"), so operators and log output see the
// same spelling ParseMessageFormatVersion accepts back.
func (v MessageFormatVersion) String() string {
	switch v {
	case MessageFormatV2:
		return "v2"
	case MessageFormatV3:
		return "v3"
	case MessageFormatUnset:
		return "unset"
	default:
		return fmt.Sprintf("invalid(%d)", int(v))
	}
}

// ParseMessageFormatVersion parses the storage.message_format_version
// configuration value. An empty string is MessageFormatUnset (use the
// default). Any value other than "v2" or "v3" (case-insensitive) is
// rejected, so a typo'd version cannot silently fall back to the default
// instead of the version the operator asked for.
func ParseMessageFormatVersion(s string) (MessageFormatVersion, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return MessageFormatUnset, nil
	case "v2":
		return MessageFormatV2, nil
	case "v3":
		return MessageFormatV3, nil
	default:
		return MessageFormatUnset, fmt.Errorf("%w: %q", ErrInvalidMessageFormatVersion, s)
	}
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

	// ErrInvalidMessageFormatVersion is returned when a configured
	// MessageFormatVersion (or its unparsed string form) is not one of the
	// versions Append can write ("v2", "v3").
	ErrInvalidMessageFormatVersion = errors.New("invalid message format version")

	// ErrTransactionalRecordNeedsV3 is returned by Append when the log is
	// configured to write MessageFormatV2 and asked to append a
	// transactional record (nonzero ProducerID). v2 has nowhere to persist
	// producer identity, so writing the record anyway would silently drop
	// the information read_committed needs to keep hiding that
	// transaction's records if it is later aborted - Append refuses instead
	// of doing that quietly. See Config.MessageFormatVersion.
	ErrTransactionalRecordNeedsV3 = errors.New("transactional record requires message format v3")
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
