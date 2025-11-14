package tenancy

import (
	"testing"
	"time"
)

func TestQuotaTracker_CheckThroughput(t *testing.T) {
	quotas := &Quotas{
		MaxBytesPerSecond:    1000,
		MaxMessagesPerSecond: 100,
	}

	tracker := NewQuotaTracker("tenant1", quotas)

	// Should succeed under quota
	err := tracker.CheckThroughput(500, 50)
	if err != nil {
		t.Errorf("Expected throughput check to succeed, got error: %v", err)
	}

	// Record usage
	tracker.RecordThroughput(500, 50)

	// Should succeed with remaining quota
	err = tracker.CheckThroughput(400, 40)
	if err != nil {
		t.Errorf("Expected throughput check to succeed, got error: %v", err)
	}

	// Record more usage
	tracker.RecordThroughput(400, 40)

	// Should fail when exceeding bytes quota
	err = tracker.CheckThroughput(200, 5)
	if err == nil {
		t.Error("Expected throughput check to fail for bytes quota")
	} else {
		// Error is expected, check if it's a QuotaError
		if quotaErr, ok := err.(*QuotaError); ok {
			if quotaErr.QuotaType != "bytes_per_second" {
				t.Errorf("Expected quota type 'bytes_per_second', got '%s'", quotaErr.QuotaType)
			}
		} else {
			t.Errorf("Expected QuotaError type, got %T", err)
		}
	}
}

func TestQuotaTracker_CheckThroughputMessages(t *testing.T) {
	quotas := &Quotas{
		MaxBytesPerSecond:    10000,
		MaxMessagesPerSecond: 100,
	}

	tracker := NewQuotaTracker("tenant1", quotas)

	// Record high message usage
	tracker.RecordThroughput(1000, 90)

	// Should fail when exceeding messages quota
	err := tracker.CheckThroughput(100, 20)
	if err == nil {
		t.Error("Expected throughput check to fail for messages quota")
	}

	quotaErr, ok := err.(*QuotaError)
	if !ok {
		t.Error("Expected QuotaError type")
	}

	if quotaErr.QuotaType != "messages_per_second" {
		t.Errorf("Expected quota type 'messages_per_second', got '%s'", quotaErr.QuotaType)
	}
}

func TestQuotaTracker_CheckConnection(t *testing.T) {
	quotas := &Quotas{
		MaxConnections: 10,
	}

	tracker := NewQuotaTracker("tenant1", quotas)

	// Add connections up to limit
	for i := 0; i < 10; i++ {
		err := tracker.AddConnection()
		if err != nil {
			t.Errorf("Failed to add connection %d: %v", i, err)
		}
	}

	// Should fail when exceeding connection quota
	err := tracker.AddConnection()
	if err == nil {
		t.Error("Expected connection check to fail")
	}

	// Remove a connection
	tracker.RemoveConnection()

	// Should succeed now
	err = tracker.AddConnection()
	if err != nil {
		t.Errorf("Expected connection check to succeed after removing connection: %v", err)
	}
}

func TestQuotaTracker_CheckProducer(t *testing.T) {
	quotas := &Quotas{
		MaxProducers: 5,
	}

	tracker := NewQuotaTracker("tenant1", quotas)

	// Add producers up to limit
	for i := 0; i < 5; i++ {
		err := tracker.AddProducer()
		if err != nil {
			t.Errorf("Failed to add producer %d: %v", i, err)
		}
	}

	// Should fail when exceeding producer quota
	err := tracker.AddProducer()
	if err == nil {
		t.Error("Expected producer check to fail")
	}

	// Remove a producer
	tracker.RemoveProducer()

	// Should succeed now
	err = tracker.AddProducer()
	if err != nil {
		t.Errorf("Expected producer check to succeed after removing producer: %v", err)
	}
}

func TestQuotaTracker_CheckStorage(t *testing.T) {
	quotas := &Quotas{
		MaxStorageBytes: 1000,
	}

	tracker := NewQuotaTracker("tenant1", quotas)

	// Set initial storage
	tracker.UpdateStorage(500)

	// Should succeed under quota
	err := tracker.CheckStorage(400)
	if err != nil {
		t.Errorf("Expected storage check to succeed, got error: %v", err)
	}

	// Should fail when exceeding storage quota
	err = tracker.CheckStorage(600)
	if err == nil {
		t.Error("Expected storage check to fail")
	}

	// Update storage
	tracker.UpdateStorage(900)

	// Should fail with new storage
	err = tracker.CheckStorage(200)
	if err == nil {
		t.Error("Expected storage check to fail with updated storage")
	}
}

func TestQuotaTracker_CheckTopic(t *testing.T) {
	quotas := &Quotas{
		MaxTopics:     10,
		MaxPartitions: 100,
	}

	tracker := NewQuotaTracker("tenant1", quotas)

	// Add topics up to limit
	for i := 0; i < 10; i++ {
		err := tracker.AddTopic(5)
		if err != nil {
			t.Errorf("Failed to add topic %d: %v", i, err)
		}
	}

	// Should fail when exceeding topic quota
	err := tracker.AddTopic(1)
	if err == nil {
		t.Error("Expected topic check to fail")
	}

	// Remove a topic
	tracker.RemoveTopic(5)

	// Should succeed now
	err = tracker.AddTopic(5)
	if err != nil {
		t.Errorf("Expected topic check to succeed after removing topic: %v", err)
	}
}

func TestQuotaTracker_CheckPartitions(t *testing.T) {
	quotas := &Quotas{
		MaxTopics:     100,
		MaxPartitions: 50,
	}

	tracker := NewQuotaTracker("tenant1", quotas)

	// Add topics with partitions up to limit
	err := tracker.AddTopic(30)
	if err != nil {
		t.Errorf("Failed to add topic: %v", err)
	}

	// Should succeed under partition quota
	err = tracker.AddTopic(15)
	if err != nil {
		t.Errorf("Expected topic check to succeed, got error: %v", err)
	}

	// Should fail when exceeding partition quota
	err = tracker.AddTopic(10)
	if err == nil {
		t.Error("Expected topic check to fail due to partition quota")
	}
}

func TestQuotaTracker_GetUsage(t *testing.T) {
	quotas := DefaultQuotas()
	tracker := NewQuotaTracker("tenant1", quotas)

	// Add some resources
	_ = tracker.AddConnection()
	_ = tracker.AddConnection()
	_ = tracker.AddProducer()
	_ = tracker.AddTopic(5)
	tracker.UpdateStorage(1000)
	tracker.RecordThroughput(100, 10)

	usage := tracker.GetUsage()

	if usage.TenantID != "tenant1" {
		t.Errorf("Expected tenant ID 'tenant1', got '%s'", usage.TenantID)
	}

	if usage.Connections != 2 {
		t.Errorf("Expected 2 connections, got %d", usage.Connections)
	}

	if usage.Producers != 1 {
		t.Errorf("Expected 1 producer, got %d", usage.Producers)
	}

	if usage.Topics != 1 {
		t.Errorf("Expected 1 topic, got %d", usage.Topics)
	}

	if usage.Partitions != 5 {
		t.Errorf("Expected 5 partitions, got %d", usage.Partitions)
	}

	if usage.StorageBytes != 1000 {
		t.Errorf("Expected 1000 storage bytes, got %d", usage.StorageBytes)
	}
}

func TestQuotaTracker_UtilizationPercent(t *testing.T) {
	quotas := &Quotas{
		MaxConnections:  100,
		MaxStorageBytes: 1000,
		MaxTopics:       10,
		MaxPartitions:    100,  // Need to set this for AddTopic to work
	}

	tracker := NewQuotaTracker("tenant1", quotas)

	// Add resources
	for i := 0; i < 50; i++ {
		_ = tracker.AddConnection()
	}
	tracker.UpdateStorage(500)

	// Add 3 topics
	for i := 0; i < 3; i++ {
		err := tracker.AddTopic(1)
		if err != nil {
			t.Fatalf("Failed to add topic %d: %v", i, err)
		}
	}

	// Check usage before testing utilization
	usage := tracker.GetUsage()
	t.Logf("Connections: %d, Storage: %d, Topics: %d", usage.Connections, usage.StorageBytes, usage.Topics)

	// Check utilization
	connUtil := tracker.UtilizationPercent("connections")
	if connUtil != 50.0 {
		t.Errorf("Expected 50%% connection utilization, got %.2f%%", connUtil)
	}

	storageUtil := tracker.UtilizationPercent("storage")
	if storageUtil != 50.0 {
		t.Errorf("Expected 50%% storage utilization, got %.2f%%", storageUtil)
	}

	topicUtil := tracker.UtilizationPercent("topics")
	if topicUtil != 30.0 {
		t.Errorf("Expected 30%% topic utilization, got %.2f%% (Topics: %d, Max: %d)",
			topicUtil, tracker.currentTopics, tracker.quotas.MaxTopics)
	}
}

func TestQuotaTracker_UnlimitedQuotas(t *testing.T) {
	quotas := UnlimitedQuotas()
	tracker := NewQuotaTracker("tenant1", quotas)

	// Should never fail with unlimited quotas
	err := tracker.CheckThroughput(1000000, 10000)
	if err != nil {
		t.Errorf("Expected unlimited throughput, got error: %v", err)
	}

	for i := 0; i < 1000; i++ {
		err := tracker.AddConnection()
		if err != nil {
			t.Errorf("Expected unlimited connections, got error: %v", err)
		}
	}

	err = tracker.CheckStorage(1000000000)
	if err != nil {
		t.Errorf("Expected unlimited storage, got error: %v", err)
	}
}

func TestRateWindow_Basic(t *testing.T) {
	window := NewRateWindow(1*time.Second, 100*time.Millisecond)

	// Add values
	window.Add(100)
	window.Add(200)
	window.Add(300)

	// Rate should be total / window duration
	rate := window.Rate()

	// Rate should be approximately 600 (total added)
	if rate < 500 || rate > 700 {
		t.Errorf("Expected rate around 600, got %.2f", rate)
	}
}

func TestRateWindow_Expiration(t *testing.T) {
	window := NewRateWindow(100*time.Millisecond, 20*time.Millisecond)

	// Add values
	window.Add(100)

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Rate should be approximately 0
	rate := window.Rate()
	if rate > 10 {
		t.Errorf("Expected rate near 0 after expiration, got %.2f", rate)
	}
}

func TestQuotaError_Error(t *testing.T) {
	err := &QuotaError{
		TenantID:  "tenant1",
		QuotaType: "connections",
		Current:   100,
		Limit:     100,
		Message:   "max connections exceeded",
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("Expected non-empty error message")
	}

	// Error message should contain tenant ID
	if len(errMsg) > 0 && errMsg[:len("quota exceeded")] != "quota exceeded" {
		t.Error("Error message should start with 'quota exceeded'")
	}
}
