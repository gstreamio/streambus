package replication

import (
	"errors"
	"testing"
)

func TestMetricsCollector_RegisterPartition(t *testing.T) {
	mc := NewMetricsCollector()

	pm := mc.RegisterPartition("test-topic", 0)
	if pm == nil {
		t.Fatal("RegisterPartition returned nil")
	}

	if pm.Topic != "test-topic" {
		t.Errorf("Topic = %s, want test-topic", pm.Topic)
	}
	if pm.PartitionID != 0 {
		t.Errorf("PartitionID = %d, want 0", pm.PartitionID)
	}

	// Verify global count incremented
	if mc.global.TotalPartitions.Load() != 1 {
		t.Errorf("TotalPartitions = %d, want 1", mc.global.TotalPartitions.Load())
	}
}

func TestMetricsCollector_UnregisterPartition(t *testing.T) {
	mc := NewMetricsCollector()

	mc.RegisterPartition("test-topic", 0)
	mc.RegisterPartition("test-topic", 1)

	if mc.global.TotalPartitions.Load() != 2 {
		t.Errorf("TotalPartitions = %d, want 2", mc.global.TotalPartitions.Load())
	}

	mc.UnregisterPartition("test-topic", 0)

	if mc.global.TotalPartitions.Load() != 1 {
		t.Errorf("TotalPartitions = %d, want 1", mc.global.TotalPartitions.Load())
	}

	// Verify partition removed
	pm := mc.GetPartitionMetrics("test-topic", 0)
	if pm != nil {
		t.Error("Partition should be unregistered")
	}
}

func TestPartitionMetrics_RecordFetch(t *testing.T) {
	pm := &PartitionMetrics{
		Topic:       "test-topic",
		PartitionID: 0,
	}

	// Record successful fetch
	pm.RecordFetch(1024, 10, nil)

	if pm.FetchRequestCount.Load() != 1 {
		t.Errorf("FetchRequestCount = %d, want 1", pm.FetchRequestCount.Load())
	}
	if pm.FetchBytesTotal.Load() != 1024 {
		t.Errorf("FetchBytesTotal = %d, want 1024", pm.FetchBytesTotal.Load())
	}
	if pm.LastFetchLatencyMs.Load() != 10 {
		t.Errorf("LastFetchLatencyMs = %d, want 10", pm.LastFetchLatencyMs.Load())
	}
	if pm.FetchErrorCount.Load() != 0 {
		t.Errorf("FetchErrorCount = %d, want 0", pm.FetchErrorCount.Load())
	}

	// Record failed fetch
	pm.RecordFetch(0, 20, errors.New("offset out of range"))

	if pm.FetchRequestCount.Load() != 2 {
		t.Errorf("FetchRequestCount = %d, want 2", pm.FetchRequestCount.Load())
	}
	if pm.FetchErrorCount.Load() != 1 {
		t.Errorf("FetchErrorCount = %d, want 1", pm.FetchErrorCount.Load())
	}
}

func TestPartitionMetrics_RecordISRChange(t *testing.T) {
	pm := &PartitionMetrics{
		Topic:       "test-topic",
		PartitionID: 0,
	}

	// Initial ISR size
	pm.RecordISRChange(3, false)
	if pm.ISRSize.Load() != 3 {
		t.Errorf("ISRSize = %d, want 3", pm.ISRSize.Load())
	}
	if pm.ISRExpandCount.Load() != 1 {
		t.Errorf("ISRExpandCount = %d, want 1", pm.ISRExpandCount.Load())
	}

	// ISR shrink
	pm.RecordISRChange(2, true)
	if pm.ISRSize.Load() != 2 {
		t.Errorf("ISRSize = %d, want 2", pm.ISRSize.Load())
	}
	if pm.ISRShrinkCount.Load() != 1 {
		t.Errorf("ISRShrinkCount = %d, want 1", pm.ISRShrinkCount.Load())
	}

	// ISR expand
	pm.RecordISRChange(3, false)
	if pm.ISRExpandCount.Load() != 2 {
		t.Errorf("ISRExpandCount = %d, want 2", pm.ISRExpandCount.Load())
	}
}

func TestPartitionMetrics_RecordFailover(t *testing.T) {
	pm := &PartitionMetrics{
		Topic:       "test-topic",
		PartitionID: 0,
	}

	pm.RecordFailover(2)

	if pm.FailoverCount.Load() != 1 {
		t.Errorf("FailoverCount = %d, want 1", pm.FailoverCount.Load())
	}
	if pm.LeaderEpoch.Load() != 2 {
		t.Errorf("LeaderEpoch = %d, want 2", pm.LeaderEpoch.Load())
	}
	if pm.LastFailoverTime.Load() == 0 {
		t.Error("LastFailoverTime should be set")
	}
}

func TestPartitionMetrics_UpdateOffsets(t *testing.T) {
	pm := &PartitionMetrics{
		Topic:       "test-topic",
		PartitionID: 0,
	}

	pm.UpdateOffsets(100, 90)

	if pm.LogEndOffset.Load() != 100 {
		t.Errorf("LogEndOffset = %d, want 100", pm.LogEndOffset.Load())
	}
	if pm.HighWaterMark.Load() != 90 {
		t.Errorf("HighWaterMark = %d, want 90", pm.HighWaterMark.Load())
	}
}

func TestPartitionMetrics_GetSnapshot(t *testing.T) {
	pm := &PartitionMetrics{
		Topic:       "test-topic",
		PartitionID: 0,
	}

	pm.UpdateOffsets(100, 90)
	pm.RecordFetch(1024, 10, nil)
	pm.RecordISRChange(3, false)
	pm.RecordFailover(2)

	snapshot := pm.GetSnapshot()

	if snapshot.Topic != "test-topic" {
		t.Errorf("Snapshot Topic = %s, want test-topic", snapshot.Topic)
	}
	if snapshot.LogEndOffset != 100 {
		t.Errorf("Snapshot LogEndOffset = %d, want 100", snapshot.LogEndOffset)
	}
	if snapshot.HighWaterMark != 90 {
		t.Errorf("Snapshot HighWaterMark = %d, want 90", snapshot.HighWaterMark)
	}
	if snapshot.FetchRequestCount != 1 {
		t.Errorf("Snapshot FetchRequestCount = %d, want 1", snapshot.FetchRequestCount)
	}
	if snapshot.ISRSize != 3 {
		t.Errorf("Snapshot ISRSize = %d, want 3", snapshot.ISRSize)
	}
	if snapshot.LeaderEpoch != 2 {
		t.Errorf("Snapshot LeaderEpoch = %d, want 2", snapshot.LeaderEpoch)
	}
}

func TestMetricsCollector_GetAllPartitionMetrics(t *testing.T) {
	mc := NewMetricsCollector()

	mc.RegisterPartition("topic1", 0)
	mc.RegisterPartition("topic1", 1)
	mc.RegisterPartition("topic2", 0)

	all := mc.GetAllPartitionMetrics()

	if len(all) != 3 {
		t.Errorf("GetAllPartitionMetrics returned %d partitions, want 3", len(all))
	}

	// Verify keys exist
	if all[partitionKey("topic1", 0)] == nil {
		t.Error("topic1:0 not found")
	}
	if all[partitionKey("topic1", 1)] == nil {
		t.Error("topic1:1 not found")
	}
	if all[partitionKey("topic2", 0)] == nil {
		t.Error("topic2:0 not found")
	}
}

func TestMetricsCollector_ComputeGlobalMetrics(t *testing.T) {
	mc := NewMetricsCollector()

	// Create leader partition (no lag)
	pm1 := mc.RegisterPartition("topic1", 0)
	pm1.UpdateOffsets(100, 100)
	pm1.RecordISRChange(3, false)

	// Create follower partition (with lag)
	pm2 := mc.RegisterPartition("topic1", 1)
	pm2.UpdateOffsets(95, 90)
	pm2.ReplicationLagMessages.Store(5)
	pm2.RecordFetch(1024, 10, nil)

	// Create another follower (with more lag)
	pm3 := mc.RegisterPartition("topic2", 0)
	pm3.UpdateOffsets(90, 85)
	pm3.ReplicationLagMessages.Store(10)
	pm3.RecordFetch(2048, 15, nil)

	// Compute global metrics
	mc.ComputeGlobalMetrics()

	// Verify counts
	if mc.global.TotalPartitions.Load() != 3 {
		t.Errorf("TotalPartitions = %d, want 3", mc.global.TotalPartitions.Load())
	}
	if mc.global.LeaderPartitions.Load() != 1 {
		t.Errorf("LeaderPartitions = %d, want 1", mc.global.LeaderPartitions.Load())
	}
	if mc.global.FollowerPartitions.Load() != 2 {
		t.Errorf("FollowerPartitions = %d, want 2", mc.global.FollowerPartitions.Load())
	}

	// Verify lag metrics
	if mc.global.MaxReplicationLagMessages.Load() != 10 {
		t.Errorf("MaxReplicationLagMessages = %d, want 10", mc.global.MaxReplicationLagMessages.Load())
	}

	avgLag := mc.global.AvgReplicationLagMessages.Load()
	if avgLag != 7 { // (5 + 10) / 2 = 7.5, truncated to 7
		t.Errorf("AvgReplicationLagMessages = %d, want 7", avgLag)
	}

	// Verify fetch metrics
	if mc.global.TotalFetchRequests.Load() != 2 {
		t.Errorf("TotalFetchRequests = %d, want 2", mc.global.TotalFetchRequests.Load())
	}
	if mc.global.TotalFetchBytes.Load() != 3072 {
		t.Errorf("TotalFetchBytes = %d, want 3072", mc.global.TotalFetchBytes.Load())
	}
}

func TestMetricsCollector_GetGlobalSnapshot(t *testing.T) {
	mc := NewMetricsCollector()

	pm1 := mc.RegisterPartition("topic1", 0)
	pm1.UpdateOffsets(100, 100)
	pm1.RecordISRChange(3, false)

	pm2 := mc.RegisterPartition("topic1", 1)
	pm2.UpdateOffsets(95, 90)
	pm2.ReplicationLagMessages.Store(5)

	snapshot := mc.GetGlobalSnapshot()

	if snapshot.TotalPartitions != 2 {
		t.Errorf("Snapshot TotalPartitions = %d, want 2", snapshot.TotalPartitions)
	}
	if snapshot.LeaderPartitions != 1 {
		t.Errorf("Snapshot LeaderPartitions = %d, want 1", snapshot.LeaderPartitions)
	}
	if snapshot.FollowerPartitions != 1 {
		t.Errorf("Snapshot FollowerPartitions = %d, want 1", snapshot.FollowerPartitions)
	}
	if snapshot.MaxReplicationLagMessages != 5 {
		t.Errorf("Snapshot MaxReplicationLagMessages = %d, want 5", snapshot.MaxReplicationLagMessages)
	}
}

func TestPartitionMetrics_ConcurrentUpdates(t *testing.T) {
	pm := &PartitionMetrics{
		Topic:       "test-topic",
		PartitionID: 0,
	}

	// Concurrent fetch recordings
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				pm.RecordFetch(100, 5, nil)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify correct count (atomic operations should be thread-safe)
	if pm.FetchRequestCount.Load() != 1000 {
		t.Errorf("FetchRequestCount = %d, want 1000", pm.FetchRequestCount.Load())
	}
	if pm.FetchBytesTotal.Load() != 100000 {
		t.Errorf("FetchBytesTotal = %d, want 100000", pm.FetchBytesTotal.Load())
	}
}
