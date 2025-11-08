package replication

import (
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector collects and tracks replication metrics
type MetricsCollector struct {
	mu sync.RWMutex

	// Replication metrics per partition
	partitionMetrics map[string]*PartitionMetrics

	// Global metrics
	global GlobalReplicationMetrics
}

// PartitionMetrics tracks metrics for a single partition
type PartitionMetrics struct {
	// Partition identification
	Topic       string
	PartitionID int

	// Offset tracking
	LogEndOffset  atomic.Int64
	HighWaterMark atomic.Int64

	// Replication lag (for followers)
	ReplicationLagMessages atomic.Int64 // Messages behind leader
	ReplicationLagMs       atomic.Int64 // Time lag in milliseconds

	// Fetch metrics (for followers)
	FetchRequestCount  atomic.Int64
	FetchBytesTotal    atomic.Int64
	FetchErrorCount    atomic.Int64
	LastFetchLatencyMs atomic.Int64
	LastFetchTime      atomic.Int64 // Unix timestamp in ms

	// ISR metrics (for leaders)
	ISRSize         atomic.Int32
	ISRShrinkCount  atomic.Int64
	ISRExpandCount  atomic.Int64
	LastISRChange   atomic.Int64 // Unix timestamp in ms

	// Failover metrics
	FailoverCount      atomic.Int64
	LastFailoverTime   atomic.Int64 // Unix timestamp in ms
	LeaderEpoch        atomic.Int64
}

// GlobalReplicationMetrics tracks cluster-wide replication metrics
type GlobalReplicationMetrics struct {
	// Total partition count
	TotalPartitions atomic.Int32
	LeaderPartitions atomic.Int32
	FollowerPartitions atomic.Int32

	// Aggregate replication lag
	MaxReplicationLagMessages atomic.Int64
	AvgReplicationLagMessages atomic.Int64

	// Aggregate fetch metrics
	TotalFetchRequests atomic.Int64
	TotalFetchBytes    atomic.Int64
	TotalFetchErrors   atomic.Int64

	// ISR health
	TotalISRShrinks atomic.Int64
	TotalISRExpands atomic.Int64
	PartitionsUnderReplicated atomic.Int32 // Partitions with ISR < replication factor

	// Failover metrics
	TotalFailovers atomic.Int64
	FailedFailovers atomic.Int64
	AvgFailoverTimeMs atomic.Int64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		partitionMetrics: make(map[string]*PartitionMetrics),
	}
}

// RegisterPartition registers a partition for metrics tracking
func (mc *MetricsCollector) RegisterPartition(topic string, partitionID int) *PartitionMetrics {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := partitionKey(topic, partitionID)
	if metrics, exists := mc.partitionMetrics[key]; exists {
		return metrics
	}

	metrics := &PartitionMetrics{
		Topic:       topic,
		PartitionID: partitionID,
	}
	mc.partitionMetrics[key] = metrics

	mc.global.TotalPartitions.Add(1)

	return metrics
}

// UnregisterPartition removes a partition from metrics tracking
func (mc *MetricsCollector) UnregisterPartition(topic string, partitionID int) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := partitionKey(topic, partitionID)
	delete(mc.partitionMetrics, key)

	mc.global.TotalPartitions.Add(-1)
}

// GetPartitionMetrics returns metrics for a specific partition
func (mc *MetricsCollector) GetPartitionMetrics(topic string, partitionID int) *PartitionMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	key := partitionKey(topic, partitionID)
	return mc.partitionMetrics[key]
}

// GetAllPartitionMetrics returns metrics for all partitions
func (mc *MetricsCollector) GetAllPartitionMetrics() map[string]*PartitionMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Return a copy to prevent external modification
	metrics := make(map[string]*PartitionMetrics, len(mc.partitionMetrics))
	for key, pm := range mc.partitionMetrics {
		metrics[key] = pm
	}
	return metrics
}

// GetGlobalMetrics returns global replication metrics
func (mc *MetricsCollector) GetGlobalMetrics() GlobalReplicationMetrics {
	// Snapshot current values
	return mc.global
}

// UpdateFromReplicationMetrics updates metrics from ReplicationMetrics
func (pm *PartitionMetrics) UpdateFromReplicationMetrics(rm ReplicationMetrics) {
	pm.ReplicationLagMessages.Store(rm.ReplicationLagMessages)
	pm.ReplicationLagMs.Store(rm.ReplicationLagMs)
	pm.LastFetchLatencyMs.Store(rm.LastFetchLatencyMs)
}

// RecordFetch records a fetch operation
func (pm *PartitionMetrics) RecordFetch(bytesRead int, latencyMs int64, err error) {
	pm.FetchRequestCount.Add(1)
	pm.FetchBytesTotal.Add(int64(bytesRead))
	pm.LastFetchLatencyMs.Store(latencyMs)
	pm.LastFetchTime.Store(time.Now().UnixMilli())

	if err != nil {
		pm.FetchErrorCount.Add(1)
	}
}

// RecordISRChange records an ISR membership change
func (pm *PartitionMetrics) RecordISRChange(newSize int, isShrink bool) {
	pm.ISRSize.Store(int32(newSize))
	pm.LastISRChange.Store(time.Now().UnixMilli())

	if isShrink {
		pm.ISRShrinkCount.Add(1)
	} else {
		pm.ISRExpandCount.Add(1)
	}
}

// RecordFailover records a leader failover
func (pm *PartitionMetrics) RecordFailover(newEpoch int64) {
	pm.FailoverCount.Add(1)
	pm.LastFailoverTime.Store(time.Now().UnixMilli())
	pm.LeaderEpoch.Store(newEpoch)
}

// UpdateOffsets updates LEO and HW
func (pm *PartitionMetrics) UpdateOffsets(leo, hw Offset) {
	pm.LogEndOffset.Store(int64(leo))
	pm.HighWaterMark.Store(int64(hw))
}

// GetSnapshot returns a snapshot of partition metrics
func (pm *PartitionMetrics) GetSnapshot() PartitionMetricsSnapshot {
	return PartitionMetricsSnapshot{
		Topic:                  pm.Topic,
		PartitionID:            pm.PartitionID,
		LogEndOffset:           pm.LogEndOffset.Load(),
		HighWaterMark:          pm.HighWaterMark.Load(),
		ReplicationLagMessages: pm.ReplicationLagMessages.Load(),
		ReplicationLagMs:       pm.ReplicationLagMs.Load(),
		FetchRequestCount:      pm.FetchRequestCount.Load(),
		FetchBytesTotal:        pm.FetchBytesTotal.Load(),
		FetchErrorCount:        pm.FetchErrorCount.Load(),
		LastFetchLatencyMs:     pm.LastFetchLatencyMs.Load(),
		LastFetchTime:          time.UnixMilli(pm.LastFetchTime.Load()),
		ISRSize:                pm.ISRSize.Load(),
		ISRShrinkCount:         pm.ISRShrinkCount.Load(),
		ISRExpandCount:         pm.ISRExpandCount.Load(),
		LastISRChange:          time.UnixMilli(pm.LastISRChange.Load()),
		FailoverCount:          pm.FailoverCount.Load(),
		LastFailoverTime:       time.UnixMilli(pm.LastFailoverTime.Load()),
		LeaderEpoch:            pm.LeaderEpoch.Load(),
	}
}

// PartitionMetricsSnapshot is a point-in-time snapshot of partition metrics
type PartitionMetricsSnapshot struct {
	Topic       string
	PartitionID int

	LogEndOffset  int64
	HighWaterMark int64

	ReplicationLagMessages int64
	ReplicationLagMs       int64

	FetchRequestCount  int64
	FetchBytesTotal    int64
	FetchErrorCount    int64
	LastFetchLatencyMs int64
	LastFetchTime      time.Time

	ISRSize       int32
	ISRShrinkCount int64
	ISRExpandCount int64
	LastISRChange time.Time

	FailoverCount    int64
	LastFailoverTime time.Time
	LeaderEpoch      int64
}

// ComputeGlobalMetrics computes global metrics from partition metrics
func (mc *MetricsCollector) ComputeGlobalMetrics() {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	var (
		totalLag          int64
		maxLag            int64
		totalFetchReqs    int64
		totalFetchBytes   int64
		totalFetchErrors  int64
		totalISRShrinks   int64
		totalISRExpands   int64
		underReplicated   int32
		leaderCount       int32
		followerCount     int32
		partitionCount    int32
	)

	for _, pm := range mc.partitionMetrics {
		partitionCount++

		// Lag metrics
		lag := pm.ReplicationLagMessages.Load()
		if lag > 0 {
			totalLag += lag
			if lag > maxLag {
				maxLag = lag
			}
			followerCount++
		} else {
			leaderCount++
		}

		// Fetch metrics
		totalFetchReqs += pm.FetchRequestCount.Load()
		totalFetchBytes += pm.FetchBytesTotal.Load()
		totalFetchErrors += pm.FetchErrorCount.Load()

		// ISR metrics
		totalISRShrinks += pm.ISRShrinkCount.Load()
		totalISRExpands += pm.ISRExpandCount.Load()

		// Under-replicated check (ISR size < desired)
		if pm.ISRSize.Load() < 3 { // Assuming RF=3
			underReplicated++
		}
	}

	// Update global metrics
	mc.global.TotalPartitions.Store(partitionCount)
	mc.global.LeaderPartitions.Store(leaderCount)
	mc.global.FollowerPartitions.Store(followerCount)
	mc.global.MaxReplicationLagMessages.Store(maxLag)

	if followerCount > 0 {
		mc.global.AvgReplicationLagMessages.Store(totalLag / int64(followerCount))
	} else {
		mc.global.AvgReplicationLagMessages.Store(0)
	}

	mc.global.TotalFetchRequests.Store(totalFetchReqs)
	mc.global.TotalFetchBytes.Store(totalFetchBytes)
	mc.global.TotalFetchErrors.Store(totalFetchErrors)
	mc.global.TotalISRShrinks.Store(totalISRShrinks)
	mc.global.TotalISRExpands.Store(totalISRExpands)
	mc.global.PartitionsUnderReplicated.Store(underReplicated)
}

// GetGlobalSnapshot returns a snapshot of global metrics
func (mc *MetricsCollector) GetGlobalSnapshot() GlobalMetricsSnapshot {
	mc.ComputeGlobalMetrics()
	gm := mc.global

	return GlobalMetricsSnapshot{
		TotalPartitions:           gm.TotalPartitions.Load(),
		LeaderPartitions:          gm.LeaderPartitions.Load(),
		FollowerPartitions:        gm.FollowerPartitions.Load(),
		MaxReplicationLagMessages: gm.MaxReplicationLagMessages.Load(),
		AvgReplicationLagMessages: gm.AvgReplicationLagMessages.Load(),
		TotalFetchRequests:        gm.TotalFetchRequests.Load(),
		TotalFetchBytes:           gm.TotalFetchBytes.Load(),
		TotalFetchErrors:          gm.TotalFetchErrors.Load(),
		TotalISRShrinks:           gm.TotalISRShrinks.Load(),
		TotalISRExpands:           gm.TotalISRExpands.Load(),
		PartitionsUnderReplicated: gm.PartitionsUnderReplicated.Load(),
		TotalFailovers:            gm.TotalFailovers.Load(),
		FailedFailovers:           gm.FailedFailovers.Load(),
		AvgFailoverTimeMs:         gm.AvgFailoverTimeMs.Load(),
	}
}

// GlobalMetricsSnapshot is a point-in-time snapshot of global metrics
type GlobalMetricsSnapshot struct {
	TotalPartitions    int32
	LeaderPartitions   int32
	FollowerPartitions int32

	MaxReplicationLagMessages int64
	AvgReplicationLagMessages int64

	TotalFetchRequests int64
	TotalFetchBytes    int64
	TotalFetchErrors   int64

	TotalISRShrinks           int64
	TotalISRExpands           int64
	PartitionsUnderReplicated int32

	TotalFailovers    int64
	FailedFailovers   int64
	AvgFailoverTimeMs int64
}
