package storage

import (
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"github.com/gstreamio/streambus/pkg/logger"
	"go.uber.org/zap"
)

// CompactionResult holds the outcome of a compaction run.
type CompactionResult struct {
	// MergedEntries is the total number of key-value entries in the output.
	MergedEntries int
	// InputTables is the number of input tables that were compacted.
	InputTables int
	// OutputSize is the total byte size of the compacted output.
	OutputSize int64
}

// compactor is the internal interface for compaction strategies.
type compactor interface {
	// compact performs compaction on the given immutable MemTables and returns
	// the compacted result tables along with metrics.
	compact(tables []MemTable, config CompactionConfig) ([]MemTable, *CompactionResult, error)
}

// newCompactor creates a compactor for the given strategy.
func newCompactor(strategy CompactionStrategy) compactor {
	switch strategy {
	case CompactionSizeTiered:
		return &sizeTieredCompactor{}
	case CompactionTimeWindow:
		return &timeWindowCompactor{}
	default:
		return &leveledCompactor{}
	}
}

// Compact triggers compaction using the configured strategy.
// Caller must hold the write lock (called from logImpl.Compact).
func (l *logImpl) runCompaction() (*CompactionResult, error) {
	if len(l.immutableMemTables) < 2 {
		return &CompactionResult{}, nil
	}

	c := newCompactor(l.config.Compaction.Strategy)

	compacted, result, err := c.compact(l.immutableMemTables, l.config.Compaction)
	if err != nil {
		return nil, fmt.Errorf("compaction failed: %w", err)
	}

	logger.Info("compaction complete",
		zap.Int("inputTables", result.InputTables),
		zap.Int("mergedEntries", result.MergedEntries),
		zap.Int64("outputSize", result.OutputSize),
		zap.Int("outputTables", len(compacted)))

	l.immutableMemTables = compacted
	return result, nil
}

// --- Leveled Compaction ---

// leveledCompactor organizes MemTables into levels (L0, L1, L2, ...).
// When a level exceeds its size threshold, overlapping tables are merged
// into the next level. The size ratio between levels is configurable.
type leveledCompactor struct{}

// compact merges immutable MemTables using a leveled strategy.
// Tables are assigned to levels based on their size. When a level
// accumulates enough data, its tables are merged into a single table
// at the next level.
func (lc *leveledCompactor) compact(tables []MemTable, config CompactionConfig) ([]MemTable, *CompactionResult, error) {
	if len(tables) < 2 {
		return tables, &CompactionResult{InputTables: len(tables)}, nil
	}

	sizeRatio := config.SizeRatio
	if sizeRatio <= 0 {
		sizeRatio = 10
	}

	levels := lc.assignLevels(tables, int64(sizeRatio))
	result := &CompactionResult{InputTables: len(tables)}

	output := make([]MemTable, 0, len(tables))

	for _, levelTables := range levels {
		merged := lc.mergeLevelTables(levelTables, result)
		output = append(output, merged...)
	}

	return output, result, nil
}

// assignLevels groups tables into levels based on size thresholds.
// L0 holds tables up to baseThreshold, L1 up to baseThreshold*sizeRatio, etc.
func (lc *leveledCompactor) assignLevels(tables []MemTable, sizeRatio int64) map[int][]MemTable {
	levels := make(map[int][]MemTable)

	// Sort by size ascending for level assignment
	sorted := make([]MemTable, len(tables))
	copy(sorted, tables)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Size() < sorted[j].Size()
	})

	// Find the median size to use as base threshold
	baseThreshold := sorted[0].Size()
	if baseThreshold <= 0 {
		baseThreshold = 1024
	}

	for _, t := range sorted {
		level := lc.computeLevel(t.Size(), baseThreshold, sizeRatio)
		levels[level] = append(levels[level], t)
	}

	return levels
}

// computeLevel determines which level a table belongs to based on its size.
func (lc *leveledCompactor) computeLevel(size, baseThreshold, sizeRatio int64) int {
	if size <= 0 || baseThreshold <= 0 {
		return 0
	}
	level := 0
	threshold := baseThreshold
	for size > threshold && level < 10 {
		threshold *= sizeRatio
		level++
	}
	return level
}

// mergeLevelTables merges tables within a level if there are enough to compact.
func (lc *leveledCompactor) mergeLevelTables(tables []MemTable, result *CompactionResult) []MemTable {
	if len(tables) < 2 {
		return tables
	}

	merged := mergeMemTables(tables)
	result.MergedEntries += countEntries(merged)
	result.OutputSize += merged.Size()

	return []MemTable{merged}
}

// --- Size-Tiered Compaction ---

// sizeTieredCompactor groups MemTables of similar size and merges them
// when enough accumulate (default: 4). This is optimized for write-heavy
// workloads and is simpler than leveled compaction.
type sizeTieredCompactor struct{}

const defaultMinSimilarTables = 4

// compact merges immutable MemTables using a size-tiered strategy.
func (sc *sizeTieredCompactor) compact(tables []MemTable, config CompactionConfig) ([]MemTable, *CompactionResult, error) {
	if len(tables) < 2 {
		return tables, &CompactionResult{InputTables: len(tables)}, nil
	}

	result := &CompactionResult{InputTables: len(tables)}
	buckets := sc.groupBySize(tables)

	output := make([]MemTable, 0, len(tables))

	for _, bucket := range buckets {
		merged := sc.mergeBucket(bucket, result)
		output = append(output, merged...)
	}

	return output, result, nil
}

// groupBySize groups tables into buckets where each table's size is within
// 2x of the bucket's minimum size.
func (sc *sizeTieredCompactor) groupBySize(tables []MemTable) [][]MemTable {
	// Sort by size
	sorted := make([]MemTable, len(tables))
	copy(sorted, tables)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Size() < sorted[j].Size()
	})

	var buckets [][]MemTable
	var currentBucket []MemTable
	var bucketMinSize int64

	for _, t := range sorted {
		size := t.Size()
		if size <= 0 {
			size = 1
		}

		if len(currentBucket) == 0 {
			bucketMinSize = size
			currentBucket = append(currentBucket, t)
			continue
		}

		// Tables are "similar" if the larger is at most 2x the smaller
		if size <= bucketMinSize*2 {
			currentBucket = append(currentBucket, t)
		} else {
			buckets = append(buckets, currentBucket)
			currentBucket = []MemTable{t}
			bucketMinSize = size
		}
	}

	if len(currentBucket) > 0 {
		buckets = append(buckets, currentBucket)
	}

	return buckets
}

// mergeBucket merges tables in a bucket if there are enough similar-sized tables.
func (sc *sizeTieredCompactor) mergeBucket(bucket []MemTable, result *CompactionResult) []MemTable {
	if len(bucket) < defaultMinSimilarTables {
		return bucket
	}

	merged := mergeMemTables(bucket)
	result.MergedEntries += countEntries(merged)
	result.OutputSize += merged.Size()

	return []MemTable{merged}
}

// --- Time-Window Compaction ---

// timeWindowCompactor groups MemTables by time windows and only compacts
// tables within the same window. This is ideal for time-series data
// and TTL-based retention. It never merges across time boundaries.
type timeWindowCompactor struct{}

const defaultTimeWindow = time.Hour

// compact merges immutable MemTables using a time-window strategy.
func (tc *timeWindowCompactor) compact(tables []MemTable, config CompactionConfig) ([]MemTable, *CompactionResult, error) {
	if len(tables) < 2 {
		return tables, &CompactionResult{InputTables: len(tables)}, nil
	}

	result := &CompactionResult{InputTables: len(tables)}
	windows := tc.groupByTimeWindow(tables, defaultTimeWindow)

	output := make([]MemTable, 0, len(tables))

	for _, windowTables := range windows {
		merged := tc.mergeWindow(windowTables, result)
		output = append(output, merged...)
	}

	return output, result, nil
}

// groupByTimeWindow groups tables by their time window based on the
// earliest timestamp found in each table.
func (tc *timeWindowCompactor) groupByTimeWindow(tables []MemTable, window time.Duration) map[int64][]MemTable {
	windows := make(map[int64][]MemTable)

	for _, t := range tables {
		windowKey := tc.getWindowKey(t, window)
		windows[windowKey] = append(windows[windowKey], t)
	}

	return windows
}

// getWindowKey determines which time window a table belongs to by examining
// its first entry's timestamp.
func (tc *timeWindowCompactor) getWindowKey(t MemTable, window time.Duration) int64 {
	iter := t.Iterator()
	defer iter.Close()

	if !iter.Next() {
		return 0
	}

	ts := extractTimestamp(iter.Value())
	if ts.IsZero() {
		return 0
	}

	windowNanos := window.Nanoseconds()
	if windowNanos <= 0 {
		windowNanos = defaultTimeWindow.Nanoseconds()
	}

	return ts.UnixNano() / windowNanos
}

// mergeWindow merges tables within a time window if there are at least 2.
func (tc *timeWindowCompactor) mergeWindow(tables []MemTable, result *CompactionResult) []MemTable {
	if len(tables) < 2 {
		return tables
	}

	merged := mergeMemTables(tables)
	result.MergedEntries += countEntries(merged)
	result.OutputSize += merged.Size()

	return []MemTable{merged}
}

// --- Shared helpers ---

// mergeMemTables merges multiple MemTables into a single new MemTable.
// For duplicate keys, the value from the last table (newest) wins.
func mergeMemTables(tables []MemTable) MemTable {
	output := NewMemTable()

	for _, t := range tables {
		iter := t.Iterator()
		for iter.Next() {
			key := make([]byte, len(iter.Key()))
			copy(key, iter.Key())
			value := make([]byte, len(iter.Value()))
			copy(value, iter.Value())

			_ = output.Put(key, value)
		}
		iter.Close()
	}

	return output
}

// countEntries counts the number of key-value entries in a MemTable.
func countEntries(t MemTable) int {
	count := 0
	iter := t.Iterator()
	for iter.Next() {
		count++
	}
	iter.Close()
	return count
}

// extractTimestamp extracts a timestamp from serialized message data,
// understanding the same record formats as logImpl.deserializeMessage.
//
// A v0 record carries no timestamp and yields the zero time; retention then
// treats it as ineligible for age-based cleanup rather than deleting it as
// infinitely old.
func extractTimestamp(data []byte) time.Time {
	// v2 and v3 both put the timestamp right after the magic and version
	// bytes - v3's producer identity is appended after the body, so it
	// doesn't move anything this function reads.
	switch newFormatVersion(data) {
	case recordVersionV2, recordVersionV3:
		if len(data) < 5+8 {
			return time.Time{}
		}
		return time.Unix(0, int64(binary.BigEndian.Uint64(data[5:13])))
	}

	if len(data) < 12 {
		return time.Time{}
	}

	// v1 vs v0: if the first word could not be a sane key length, it is the
	// high half of a timestamp.
	possibleKeyLen := binary.BigEndian.Uint32(data[0:4])
	if possibleKeyLen > maxSaneKeyLen {
		nanos := int64(binary.BigEndian.Uint64(data[0:8]))
		return time.Unix(0, nanos)
	}

	return time.Time{}
}
