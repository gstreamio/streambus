package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/logger"
	"go.uber.org/zap"
)

// RetentionConfig holds configuration for retention enforcement.
type RetentionConfig struct {
	// CheckInterval is how often the retention manager checks for expired data.
	// Defaults to 5 minutes.
	CheckInterval time.Duration

	// RetentionMs is the maximum age of messages in milliseconds.
	// Messages older than this are eligible for deletion.
	// A value <= 0 means no time-based retention.
	RetentionMs int64

	// RetentionBytes is the maximum size per partition in bytes.
	// When exceeded, the oldest messages are deleted first (FIFO).
	// A value <= 0 means no size-based retention.
	RetentionBytes int64
}

// DefaultRetentionConfig returns a default retention configuration with no retention limits.
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		CheckInterval:  5 * time.Minute,
		RetentionMs:    -1, // unlimited
		RetentionBytes: -1, // unlimited
	}
}

// RetentionStats tracks metrics for a single enforcement cycle.
type RetentionStats struct {
	// SegmentsDeleted is the number of logical segments (offset ranges) deleted.
	SegmentsDeleted int64

	// BytesReclaimed is the estimated bytes reclaimed.
	BytesReclaimed int64

	// PartitionsChecked is the number of partitions that were evaluated.
	PartitionsChecked int64
}

// RetentionManager enforces retention policies on partition logs.
// It runs a background goroutine that periodically checks all registered
// logs and deletes data that exceeds the configured retention limits.
type RetentionManager struct {
	config RetentionConfig

	mu   sync.RWMutex
	logs map[string]Log // partitionID -> Log

	// Cumulative stats
	totalSegmentsDeleted int64
	totalBytesReclaimed  int64

	cancel context.CancelFunc
	done   chan struct{}
}

// NewRetentionManager creates a new RetentionManager with the given configuration.
// The manager does not start automatically; call Start to begin enforcement.
func NewRetentionManager(config RetentionConfig) *RetentionManager {
	if config.CheckInterval <= 0 {
		config.CheckInterval = 5 * time.Minute
	}
	return &RetentionManager{
		config: config,
		logs:   make(map[string]Log),
		done:   make(chan struct{}),
	}
}

// RegisterLog adds a partition log to be managed by this retention manager.
func (rm *RetentionManager) RegisterLog(partitionID string, log Log) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.logs[partitionID] = log
}

// UnregisterLog removes a partition log from the retention manager.
func (rm *RetentionManager) UnregisterLog(partitionID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.logs, partitionID)
}

// Start begins the background retention enforcement loop.
// It blocks until the context is cancelled or Stop is called.
func (rm *RetentionManager) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	rm.mu.Lock()
	rm.cancel = cancel
	rm.mu.Unlock()
	defer close(rm.done)

	ticker := time.NewTicker(rm.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rm.enforce()
		}
	}
}

// Stop signals the background goroutine to stop and waits for it to finish.
func (rm *RetentionManager) Stop() {
	rm.mu.RLock()
	cancel := rm.cancel
	rm.mu.RUnlock()

	if cancel != nil {
		cancel()
		<-rm.done
	}
}

// EnforceNow runs retention enforcement immediately (useful for testing).
// Returns stats for this enforcement cycle.
func (rm *RetentionManager) EnforceNow() RetentionStats {
	return rm.enforce()
}

// Stats returns cumulative retention stats.
func (rm *RetentionManager) Stats() RetentionStats {
	return RetentionStats{
		SegmentsDeleted: atomic.LoadInt64(&rm.totalSegmentsDeleted),
		BytesReclaimed:  atomic.LoadInt64(&rm.totalBytesReclaimed),
	}
}

// enforce runs one cycle of retention enforcement across all registered logs.
func (rm *RetentionManager) enforce() RetentionStats {
	rm.mu.RLock()
	snapshots := make(map[string]Log, len(rm.logs))
	for id, log := range rm.logs {
		snapshots[id] = log
	}
	rm.mu.RUnlock()

	var stats RetentionStats
	for id, log := range snapshots {
		s := rm.enforceLog(id, log)
		stats.SegmentsDeleted += s.SegmentsDeleted
		stats.BytesReclaimed += s.BytesReclaimed
		stats.PartitionsChecked++
	}

	atomic.AddInt64(&rm.totalSegmentsDeleted, stats.SegmentsDeleted)
	atomic.AddInt64(&rm.totalBytesReclaimed, stats.BytesReclaimed)

	return stats
}

// enforceLog applies retention to a single partition log.
func (rm *RetentionManager) enforceLog(partitionID string, log Log) RetentionStats {
	var stats RetentionStats

	timeStats := rm.enforceTimeBased(partitionID, log)
	stats.SegmentsDeleted += timeStats.SegmentsDeleted
	stats.BytesReclaimed += timeStats.BytesReclaimed

	sizeStats := rm.enforceSizeBased(partitionID, log)
	stats.SegmentsDeleted += sizeStats.SegmentsDeleted
	stats.BytesReclaimed += sizeStats.BytesReclaimed

	return stats
}

// enforceTimeBased deletes messages older than RetentionMs.
func (rm *RetentionManager) enforceTimeBased(partitionID string, log Log) RetentionStats {
	var stats RetentionStats

	if rm.config.RetentionMs <= 0 {
		return stats
	}

	cutoffTime := time.Now().Add(-time.Duration(rm.config.RetentionMs) * time.Millisecond)
	cutoffNanos := cutoffTime.UnixNano()

	startOffset := log.StartOffset()
	endOffset := log.EndOffset()

	if startOffset >= endOffset {
		return stats
	}

	// Find the first offset at or after the cutoff time.
	newStartOffset, _, err := log.FindOffsetByTimestamp(cutoffNanos)
	if err != nil {
		logger.Warn("retention: failed to find offset by timestamp",
			zap.String("partition", partitionID),
			zap.Error(err))
		return stats
	}

	if newStartOffset <= startOffset {
		return stats
	}

	deletedCount := int64(newStartOffset - startOffset)
	stats.SegmentsDeleted = deletedCount
	stats.BytesReclaimed = estimateBytes(startOffset, newStartOffset)

	if err := log.Delete(newStartOffset); err != nil {
		logger.Warn("retention: failed to delete before offset",
			zap.String("partition", partitionID),
			zap.Int64("beforeOffset", int64(newStartOffset)),
			zap.Error(err))
		return RetentionStats{}
	}

	logger.Info("retention: time-based cleanup",
		zap.String("partition", partitionID),
		zap.Int64("deletedMessages", deletedCount),
		zap.Int64("newStartOffset", int64(newStartOffset)))

	return stats
}

// enforceSizeBased deletes oldest messages when partition size exceeds RetentionBytes.
func (rm *RetentionManager) enforceSizeBased(partitionID string, log Log) RetentionStats {
	var stats RetentionStats

	if rm.config.RetentionBytes <= 0 {
		return stats
	}

	startOffset := log.StartOffset()
	endOffset := log.EndOffset()

	if startOffset >= endOffset {
		return stats
	}

	totalSize := rm.estimatePartitionSize(log, startOffset, endOffset)
	if totalSize <= rm.config.RetentionBytes {
		return stats
	}

	excessBytes := totalSize - rm.config.RetentionBytes
	newStartOffset := rm.findOffsetForSizeReduction(log, startOffset, endOffset, excessBytes)

	if newStartOffset <= startOffset {
		return stats
	}

	deletedCount := int64(newStartOffset - startOffset)
	stats.SegmentsDeleted = deletedCount
	stats.BytesReclaimed = estimateBytes(startOffset, newStartOffset)

	if err := log.Delete(newStartOffset); err != nil {
		logger.Warn("retention: failed to delete for size-based retention",
			zap.String("partition", partitionID),
			zap.Int64("beforeOffset", int64(newStartOffset)),
			zap.Error(err))
		return RetentionStats{}
	}

	logger.Info("retention: size-based cleanup",
		zap.String("partition", partitionID),
		zap.Int64("deletedMessages", deletedCount),
		zap.Int64("newStartOffset", int64(newStartOffset)),
		zap.Int64("estimatedBytesReclaimed", stats.BytesReclaimed))

	return stats
}

// estimatePartitionSize estimates the total byte size of a partition by sampling.
func (rm *RetentionManager) estimatePartitionSize(log Log, start, end Offset) int64 {
	messageCount := int64(end - start)
	if messageCount <= 0 {
		return 0
	}

	// Sample up to 10 messages to estimate average size.
	sampleSize := int64(10)
	if messageCount < sampleSize {
		sampleSize = messageCount
	}

	totalSampleBytes := int64(0)
	samplesRead := int64(0)
	step := messageCount / sampleSize

	if step < 1 {
		step = 1
	}

	for i := int64(0); i < sampleSize; i++ {
		offset := start + Offset(i*step)
		msgs, err := log.Read(offset, 1)
		if err != nil || len(msgs) == 0 {
			continue
		}
		totalSampleBytes += int64(len(msgs[0].Key) + len(msgs[0].Value))
		samplesRead++
	}

	if samplesRead == 0 {
		return 0
	}

	avgSize := totalSampleBytes / samplesRead
	return avgSize * messageCount
}

// findOffsetForSizeReduction scans from the start to find the offset to delete up to
// in order to reclaim at least excessBytes.
func (rm *RetentionManager) findOffsetForSizeReduction(
	log Log, start, end Offset, excessBytes int64,
) Offset {
	var accumulated int64
	for offset := start; offset < end; offset++ {
		msgs, err := log.Read(offset, 1)
		if err != nil || len(msgs) == 0 {
			// Estimate a small size for unreadable messages.
			accumulated += 64
			if accumulated >= excessBytes {
				return offset + 1
			}
			continue
		}
		accumulated += int64(len(msgs[0].Key) + len(msgs[0].Value))
		if accumulated >= excessBytes {
			return offset + 1
		}
	}
	return end
}

// estimateBytes provides a rough byte estimate for a range of offsets.
// Uses a conservative average of 256 bytes per message.
func estimateBytes(from, to Offset) int64 {
	count := int64(to - from)
	if count <= 0 {
		return 0
	}
	return count * 256
}
