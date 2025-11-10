package replication

import (
	"testing"
)

func TestPartitionMetrics_UpdateFromReplicationMetrics(t *testing.T) {
	collector := NewMetricsCollector()
	pm := collector.RegisterPartition("test-topic", 0)

	// Update with replication metrics
	rm := ReplicationMetrics{
		ReplicationLagMessages: 100,
		ReplicationLagMs:       500,
		LastFetchLatencyMs:     25,
	}

	pm.UpdateFromReplicationMetrics(rm)

	// Verify values were updated
	if pm.ReplicationLagMessages.Load() != 100 {
		t.Errorf("ReplicationLagMessages = %d, want 100", pm.ReplicationLagMessages.Load())
	}

	if pm.ReplicationLagMs.Load() != 500 {
		t.Errorf("ReplicationLagMs = %d, want 500", pm.ReplicationLagMs.Load())
	}

	if pm.LastFetchLatencyMs.Load() != 25 {
		t.Errorf("LastFetchLatencyMs = %d, want 25", pm.LastFetchLatencyMs.Load())
	}
}
