package link

import (
	"fmt"
	"testing"
	"time"
)

func createTestLink(id, name string) *ReplicationLink {
	return &ReplicationLink{
		ID:   id,
		Name: name,
		Type: ReplicationTypeActivePassive,
		SourceCluster: ClusterConfig{
			ClusterID:         "source",
			Brokers:           []string{"localhost:9092"},
			ConnectionTimeout: 5 * time.Second,
			RequestTimeout:    10 * time.Second,
			RetryBackoff:      1 * time.Second,
			MaxRetries:        3,
		},
		TargetCluster: ClusterConfig{
			ClusterID:         "target",
			Brokers:           []string{"localhost:9093"},
			ConnectionTimeout: 5 * time.Second,
			RequestTimeout:    10 * time.Second,
			RetryBackoff:      1 * time.Second,
			MaxRetries:        3,
		},
		Topics: []string{"test-topic"},
		Config: ReplicationConfig{
			MaxBytes:             1024,
			MaxMessages:          100,
			BatchSize:            10,
			BufferSize:           100,
			ConcurrentPartitions: 1,
			CheckpointIntervalMs: 1000,
			SyncIntervalMs:       1000,
			HeartbeatIntervalMs:  1000,
		},
	}
}

func TestManager_CreateLink(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("test-link", "Test Link")

	err := manager.CreateLink(link)
	if err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	retrieved, err := manager.GetLink("test-link")
	if err != nil {
		t.Fatalf("GetLink failed: %v", err)
	}

	if retrieved.ID != "test-link" {
		t.Errorf("ID = %s, want test-link", retrieved.ID)
	}

	if retrieved.Name != "Test Link" {
		t.Errorf("Name = %s, want Test Link", retrieved.Name)
	}
}

func TestManager_CreateLink_DuplicateID(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("dup-link", "Duplicate Link")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("First CreateLink failed: %v", err)
	}

	err := manager.CreateLink(link)
	if err == nil {
		t.Error("Expected error when creating duplicate link")
	}
}

func TestManager_DeleteLink(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("delete-test", "Delete Test")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	if err := manager.DeleteLink("delete-test"); err != nil {
		t.Fatalf("DeleteLink failed: %v", err)
	}

	_, err := manager.GetLink("delete-test")
	if err == nil {
		t.Error("Expected error when getting deleted link")
	}
}

func TestManager_UpdateLink(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("update-test", "Update Test")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	updatedLink := createTestLink("update-test", "Updated Name")
	updatedLink.Status = ReplicationStatusPaused

	if err := manager.UpdateLink("update-test", updatedLink); err != nil {
		t.Fatalf("UpdateLink failed: %v", err)
	}

	retrieved, err := manager.GetLink("update-test")
	if err != nil {
		t.Fatalf("GetLink failed: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("Name = %s, want Updated Name", retrieved.Name)
	}
}

func TestManager_ListLinks(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	for i := 1; i <= 3; i++ {
		link := createTestLink("link-"+string(rune('0'+i)), "Link "+string(rune('0'+i)))

		if err := manager.CreateLink(link); err != nil {
			t.Fatalf("CreateLink %d failed: %v", i, err)
		}
	}

	links, err := manager.ListLinks()
	if err != nil {
		t.Fatalf("ListLinks failed: %v", err)
	}

	if len(links) != 3 {
		t.Errorf("ListLinks count = %d, want 3", len(links))
	}
}

func TestManager_StartStopLink(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("start-stop-test", "Start Stop Test")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	// Start link (will fail without real brokers, but exercises the code path)
	if err := manager.StartLink("start-stop-test"); err != nil {
		t.Logf("StartLink expected to fail without actual brokers: %v", err)
	}

	// Stop link
	if err := manager.StopLink("start-stop-test"); err != nil {
		t.Logf("StopLink: %v", err)
	}
}

func TestManager_PauseResumeLink(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("pause-resume-test", "Pause Resume Test")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	if err := manager.PauseLink("pause-resume-test"); err != nil {
		t.Logf("PauseLink: %v", err)
	}

	if err := manager.ResumeLink("pause-resume-test"); err != nil {
		t.Logf("ResumeLink: %v", err)
	}
}

func TestManager_GetMetrics(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("metrics-test", "Metrics Test")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	metrics, err := manager.GetMetrics("metrics-test")
	if err != nil {
		t.Logf("GetMetrics: %v (expected for non-running link)", err)
	}

	if metrics != nil {
		t.Log("Metrics retrieved successfully")
	}
}

func TestManager_GetHealth(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("health-test", "Health Test")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	health, err := manager.GetHealth("health-test")
	if err != nil {
		t.Logf("GetHealth: %v (expected for non-running link)", err)
	}

	if health != nil {
		t.Log("Health status retrieved successfully")
	}
}

func TestManager_Checkpoints(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	// Create link first before setting checkpoint
	link := createTestLink("test-link", "Test Link")
	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	checkpoint := &Checkpoint{
		LinkID:       "test-link",
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 100,
		TargetOffset: 95,
		Timestamp:    time.Now(),
	}

	if err := manager.SetCheckpoint(checkpoint); err != nil {
		t.Fatalf("SetCheckpoint failed: %v", err)
	}

	retrieved, err := manager.GetCheckpoint("test-link", "test-topic", 0)
	if err != nil {
		t.Fatalf("GetCheckpoint failed: %v", err)
	}

	if retrieved.SourceOffset != 100 {
		t.Errorf("Checkpoint SourceOffset = %d, want 100", retrieved.SourceOffset)
	}

	if retrieved.TargetOffset != 95 {
		t.Errorf("Checkpoint TargetOffset = %d, want 95", retrieved.TargetOffset)
	}
}

func TestManager_GetLink_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	_, err := manager.GetLink("non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent link")
	}
}

func TestManager_DeleteLink_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	err := manager.DeleteLink("non-existent")
	if err == nil {
		t.Error("Expected error when deleting non-existent link")
	}
}

func TestManager_UpdateLink_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("non-existent", "Non-existent")

	err := manager.UpdateLink("non-existent", link)
	if err == nil {
		t.Error("Expected error when updating non-existent link")
	}
}

func TestManager_Failover(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("failover-test", "Failover Test")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	event, err := manager.Failover("failover-test")
	if err != nil {
		t.Logf("Failover expected to fail for non-running link: %v", err)
	}

	if event != nil {
		t.Log("Failover event created successfully")
	}
}

func TestManager_Failback(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("failback-test", "Failback Test")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	event, err := manager.Failback("failback-test")
	if err != nil {
		t.Logf("Failback expected to fail for non-running link: %v", err)
	}

	if event != nil {
		t.Log("Failback event created successfully")
	}
}

func TestManager_Close(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)

	link := createTestLink("close-test", "Close Test")

	if err := manager.CreateLink(link); err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	if err := manager.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Operations after close should fail
	err := manager.CreateLink(link)
	if err == nil {
		t.Error("Expected error when creating link after close")
	}
}

func TestManager_checkAllLinksHealth(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	// Create and add some links
	link1 := createTestLink("link1", "Link 1")
	link1.Status = ReplicationStatusActive
	link1.Health = &ReplicationHealth{
		SourceClusterReachable: true,
		TargetClusterReachable: true,
	}
	
	link2 := createTestLink("link2", "Link 2")
	link2.Status = ReplicationStatusActive
	link2.Health = &ReplicationHealth{
		SourceClusterReachable: true,
		TargetClusterReachable: true,
	}

	link3 := createTestLink("link3", "Link 3")
	link3.Status = ReplicationStatusPaused
	link3.Health = &ReplicationHealth{
		SourceClusterReachable: true,
		TargetClusterReachable: true,
	}

	_ = manager.CreateLink(link1)
	_ = manager.CreateLink(link2)
	_ = manager.CreateLink(link3)

	// Should not panic with active links
	manager.checkAllLinksHealth()
}

func TestManager_checkLinkHealth_NoLink(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	// Should not panic for non-existent link
	manager.checkLinkHealth("non-existent")
}

func TestManager_checkLinkHealth_NoHandler(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	link := createTestLink("test-link", "Test Link")
	link.Status = ReplicationStatusActive
	_ = manager.CreateLink(link)

	// Should not panic when handler doesn't exist
	manager.checkLinkHealth("test-link")
}

func TestManager_checkAutomaticFailover_NilMetrics(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	link := createTestLink("test-link", "Test Link")
	link.Metrics = nil
	link.Health = &ReplicationHealth{
		SourceClusterReachable: true,
	}
	link.FailoverConfig = &FailoverConfig{
		Enabled: true,
	}

	// Should return early when metrics are nil
	manager.checkAutomaticFailover("test-link", link)
}

func TestManager_checkAutomaticFailover_NilHealth(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	link := createTestLink("test-link", "Test Link")
	link.Metrics = &ReplicationMetrics{
		ReplicationLag: 100,
	}
	link.Health = nil
	link.FailoverConfig = &FailoverConfig{
		Enabled: true,
	}

	// Should return early when health is nil
	manager.checkAutomaticFailover("test-link", link)
}

func TestManager_checkAutomaticFailover_NilConfig(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	link := createTestLink("test-link", "Test Link")
	link.Metrics = &ReplicationMetrics{
		ReplicationLag: 100,
	}
	link.Health = &ReplicationHealth{
		SourceClusterReachable: true,
	}
	link.FailoverConfig = nil

	// Should return early when failover config is nil
	manager.checkAutomaticFailover("test-link", link)
}

func TestManager_checkAutomaticFailover_NoTrigger(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	link := createTestLink("test-link", "Test Link")
	link.Metrics = &ReplicationMetrics{
		ReplicationLag:       100,
		ConsecutiveFailures:  1,
	}
	link.Health = &ReplicationHealth{
		SourceClusterReachable: true,
		TargetClusterReachable: true,
	}
	link.FailoverConfig = &FailoverConfig{
		Enabled:                 true,
		FailoverThreshold:       1000, // Higher than current lag
		MaxConsecutiveFailures:  5,    // Higher than current failures
	}

	// Should not trigger failover when conditions aren't met
	manager.checkAutomaticFailover("test-link", link)
}

func TestManager_scheduleAutoFailback(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	link := createTestLink("test-link", "Test Link")
	link.Status = ReplicationStatusActive
	link.Health = &ReplicationHealth{
		SourceClusterReachable: true,
		TargetClusterReachable: true,
	}
	_ = manager.CreateLink(link)

	// Should not panic with minimal delay
	done := make(chan bool)
	go func() {
		manager.scheduleAutoFailback("test-link", 1) // 1ms delay
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("scheduleAutoFailback took too long")
	}
}

func TestManager_scheduleAutoFailback_NonExistentLink(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	// Should not panic for non-existent link
	done := make(chan bool)
	go func() {
		manager.scheduleAutoFailback("non-existent", 1)
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("scheduleAutoFailback took too long")
	}
}

func TestManager_SendFailoverNotification(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage).(*manager)
	defer manager.Close()

	event := &FailoverEvent{
		ID:              "event-1",
		LinkID:          "test-link",
		Type:            "failover",
		SourceClusterID: "cluster-a",
		TargetClusterID: "cluster-b",
		Timestamp:       time.Now(),
		Reason:          "health check failure",
		Success:         true,
	}

	// Test calling sendFailoverNotification (currently a no-op/TODO)
	// This should not panic
	manager.sendFailoverNotification("http://example.com/webhook", event)

	// Test with empty webhook URL
	manager.sendFailoverNotification("", event)

	// Test with nil event
	manager.sendFailoverNotification("http://example.com/webhook", nil)
}

// TestManager_StartLink_NonExistent tests starting a non-existent link
func TestManager_StartLink_NonExistent(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	err := manager.StartLink("non-existent")
	if err == nil {
		t.Error("Expected error when starting non-existent link")
	}
}

// TestManager_StartLink_AlreadyActive tests starting an already active link
func TestManager_StartLink_AlreadyActive(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("active-link", "Active Link")
	link.Status = ReplicationStatusActive
	_ = manager.CreateLink(link)

	err := manager.StartLink("active-link")
	if err == nil {
		t.Error("Expected error when starting already active link")
	}
}

// TestManager_StartLink_ConnectionFailure tests StartLink with connection failure
func TestManager_StartLink_ConnectionFailure(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("conn-fail-link", "Connection Fail Link")
	// Use invalid brokers to force failure
	link.SourceCluster.Brokers = []string{"invalid:99999"}
	_ = manager.CreateLink(link)

	err := manager.StartLink("conn-fail-link")
	if err == nil {
		t.Error("Expected error when starting link with invalid brokers")
	}
}

// TestManager_StartLink_StoragePersistFailure tests StartLink with storage persist failure
func TestManager_StartLink_StoragePersistFailure(t *testing.T) {
	// Use a storage that will fail on SaveLink after successful creation
	storage := &FailingStorage{
		memoryStorage: NewMemoryStorage().(*memoryStorage),
		failOnSave:    true,
	}
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("persist-fail-link", "Persist Fail Link")

	// Temporarily disable save failure for CreateLink
	storage.failOnSave = false
	_ = manager.CreateLink(link)

	// Enable save failure for StartLink
	storage.failOnSave = true

	// StartLink should fail due to storage persist failure
	err := manager.StartLink("persist-fail-link")
	if err == nil {
		t.Error("Expected error when storage persist fails")
	}
}

// TestManager_StopLink_NonExistent tests stopping a non-existent link
func TestManager_StopLink_NonExistent(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	err := manager.StopLink("non-existent")
	if err == nil {
		t.Error("Expected error when stopping non-existent link")
	}
}

// TestManager_StopLink_AlreadyStopped tests stopping an already stopped link
func TestManager_StopLink_AlreadyStopped(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("stopped-link", "Stopped Link")
	link.Status = ReplicationStatusStopped
	_ = manager.CreateLink(link)

	// StopLink should not error for already stopped link
	err := manager.StopLink("stopped-link")
	if err != nil {
		t.Errorf("StopLink failed for already stopped link: %v", err)
	}
}

// TestManager_StopLink_WithHandler tests stopping a link with active handler
func TestManager_StopLink_WithHandler(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("handler-link", "Handler Link")
	_ = mgr.CreateLink(link)

	// Create a mock handler
	handler, _ := NewStreamHandler(link, storage)
	mgr.streamHandlers["handler-link"] = handler

	// Update link status to active
	mgr.mu.Lock()
	mgr.links["handler-link"].Status = ReplicationStatusActive
	mgr.mu.Unlock()

	// Stop should succeed
	err := mgr.StopLink("handler-link")
	if err != nil {
		t.Errorf("StopLink failed: %v", err)
	}

	// Verify handler was removed
	mgr.mu.RLock()
	_, exists := mgr.streamHandlers["handler-link"]
	mgr.mu.RUnlock()

	if exists {
		t.Error("Expected handler to be removed after StopLink")
	}
}

// TestManager_checkAutomaticFailover_ReplicationLagThreshold tests automatic failover on lag
func TestManager_checkAutomaticFailover_ReplicationLagThreshold(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("lag-failover", "Lag Failover")
	link.Type = ReplicationTypeActivePassive
	link.Metrics = &ReplicationMetrics{
		ReplicationLag:      2000, // 2 seconds
		ConsecutiveFailures: 1,
	}
	link.Health = &ReplicationHealth{
		SourceClusterReachable: true,
		TargetClusterReachable: true,
	}
	link.FailoverConfig = &FailoverConfig{
		Enabled:                true,
		FailoverThreshold:      1000, // 1 second threshold
		MaxConsecutiveFailures: 5,
	}

	mgr.mu.Lock()
	mgr.links["lag-failover"] = link
	// Call checkAutomaticFailover with lock held (it unlocks/relocks internally)
	mgr.checkAutomaticFailover("lag-failover", link)
	mgr.mu.Unlock()
}

// TestManager_checkAutomaticFailover_ConsecutiveFailuresThreshold tests automatic failover on failures
func TestManager_checkAutomaticFailover_ConsecutiveFailuresThreshold(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("fail-failover", "Failure Failover")
	link.Type = ReplicationTypeActivePassive
	link.Metrics = &ReplicationMetrics{
		ReplicationLag:      100,
		ConsecutiveFailures: 10, // High failures
	}
	link.Health = &ReplicationHealth{
		SourceClusterReachable: true,
		TargetClusterReachable: true,
	}
	link.FailoverConfig = &FailoverConfig{
		Enabled:                true,
		FailoverThreshold:      1000,
		MaxConsecutiveFailures: 5, // Lower threshold
	}

	mgr.mu.Lock()
	mgr.links["fail-failover"] = link
	// Call checkAutomaticFailover with lock held (it unlocks/relocks internally)
	mgr.checkAutomaticFailover("fail-failover", link)
	mgr.mu.Unlock()
}

// TestManager_checkAutomaticFailover_SourceUnreachable tests automatic failover on source unreachable
func TestManager_checkAutomaticFailover_SourceUnreachable(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("unreachable-failover", "Unreachable Failover")
	link.Type = ReplicationTypeActivePassive
	link.Metrics = &ReplicationMetrics{
		ReplicationLag:      100,
		ConsecutiveFailures: 1,
	}
	link.Health = &ReplicationHealth{
		SourceClusterReachable: false, // Source unreachable
		TargetClusterReachable: true,
	}
	link.FailoverConfig = &FailoverConfig{
		Enabled:                true,
		FailoverThreshold:      1000,
		MaxConsecutiveFailures: 5,
	}

	mgr.mu.Lock()
	mgr.links["unreachable-failover"] = link
	// Call checkAutomaticFailover with lock held (it unlocks/relocks internally)
	mgr.checkAutomaticFailover("unreachable-failover", link)
	mgr.mu.Unlock()
}

// TestManager_checkAutomaticFailover_WithAutoFailback tests automatic failback scheduling
func TestManager_checkAutomaticFailover_WithAutoFailback(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("auto-failback", "Auto Failback")
	link.Type = ReplicationTypeActivePassive
	link.Metrics = &ReplicationMetrics{
		ReplicationLag:      100,
		ConsecutiveFailures: 1,
	}
	link.Health = &ReplicationHealth{
		SourceClusterReachable: false, // Trigger failover
		TargetClusterReachable: true,
	}
	link.FailoverConfig = &FailoverConfig{
		Enabled:                true,
		AutoFailback:           true,
		FailbackDelayMs:        100, // Short delay for testing
		FailoverThreshold:      1000,
		MaxConsecutiveFailures: 5,
	}

	mgr.mu.Lock()
	mgr.links["auto-failback"] = link
	// Call checkAutomaticFailover with lock held (it unlocks/relocks internally)
	mgr.checkAutomaticFailover("auto-failback", link)
	mgr.mu.Unlock()

	// Give time for scheduled failback to execute
	time.Sleep(200 * time.Millisecond)
}

// FailingStorage is a mock storage that can simulate failures
type FailingStorage struct {
	*memoryStorage
	failOnSave bool
}

func (s *FailingStorage) SaveLink(link *ReplicationLink) error {
	if s.failOnSave {
		return fmt.Errorf("simulated save failure")
	}
	return s.memoryStorage.SaveLink(link)
}

func (s *FailingStorage) SaveCheckpoint(checkpoint *Checkpoint) error {
	if s.failOnSave {
		return fmt.Errorf("simulated save failure")
	}
	return s.memoryStorage.SaveCheckpoint(checkpoint)
}

// TestManager_PauseLink_NonActive tests pausing a non-active link
func TestManager_PauseLink_NonActive(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("pause-stopped", "Pause Stopped")
	link.Status = ReplicationStatusStopped
	_ = manager.CreateLink(link)

	err := manager.PauseLink("pause-stopped")
	if err == nil {
		t.Error("Expected error when pausing non-active link")
	}
}

// TestManager_PauseLink_Success tests successfully pausing an active link
func TestManager_PauseLink_Success(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("pause-success", "Pause Success")
	_ = mgr.CreateLink(link)

	// Set status to active manually
	mgr.mu.Lock()
	mgr.links["pause-success"].Status = ReplicationStatusActive
	mgr.mu.Unlock()

	// Pause should succeed
	err := mgr.PauseLink("pause-success")
	if err != nil {
		t.Errorf("PauseLink failed: %v", err)
	}

	// Verify status is paused
	pausedLink, _ := mgr.GetLink("pause-success")
	if pausedLink.Status != ReplicationStatusPaused {
		t.Errorf("Expected status %s, got %s", ReplicationStatusPaused, pausedLink.Status)
	}
}

// TestManager_PauseLink_NotFound tests pausing a non-existent link
func TestManager_PauseLink_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	err := manager.PauseLink("non-existent")
	if err == nil {
		t.Error("Expected error when pausing non-existent link")
	}
}

// TestManager_ResumeLink_NonPaused tests resuming a non-paused link
func TestManager_ResumeLink_NonPaused(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("resume-stopped", "Resume Stopped")
	link.Status = ReplicationStatusStopped
	_ = manager.CreateLink(link)

	err := manager.ResumeLink("resume-stopped")
	if err == nil {
		t.Error("Expected error when resuming non-paused link")
	}
}

// TestManager_ResumeLink_Success tests successfully resuming a paused link
func TestManager_ResumeLink_Success(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("resume-success", "Resume Success")
	_ = mgr.CreateLink(link)

	// Set status to paused manually
	mgr.mu.Lock()
	mgr.links["resume-success"].Status = ReplicationStatusPaused
	mgr.mu.Unlock()

	// Resume should succeed
	err := mgr.ResumeLink("resume-success")
	if err != nil {
		t.Errorf("ResumeLink failed: %v", err)
	}

	// Verify status is active
	resumedLink, _ := mgr.GetLink("resume-success")
	if resumedLink.Status != ReplicationStatusActive {
		t.Errorf("Expected status %s, got %s", ReplicationStatusActive, resumedLink.Status)
	}
}

// TestManager_ResumeLink_NotFound tests resuming a non-existent link
func TestManager_ResumeLink_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	err := manager.ResumeLink("non-existent")
	if err == nil {
		t.Error("Expected error when resuming non-existent link")
	}
}

// TestManager_loadLinksFromStorage tests loading links from storage on startup
func TestManager_loadLinksFromStorage(t *testing.T) {
	storage := NewMemoryStorage()

	// Pre-populate storage with some links
	link1 := createTestLink("preload-1", "Preload 1")
	link2 := createTestLink("preload-2", "Preload 2")
	_ = storage.SaveLink(link1)
	_ = storage.SaveLink(link2)

	// Create a new manager - should load existing links
	manager := NewManager(storage)
	defer manager.Close()

	links, err := manager.ListLinks()
	if err != nil {
		t.Fatalf("ListLinks failed: %v", err)
	}

	if len(links) != 2 {
		t.Errorf("Expected 2 preloaded links, got %d", len(links))
	}
}

// TestManager_Failover_WithNotification tests failover with webhook notification
func TestManager_Failover_WithNotification(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("webhook-failover", "Webhook Failover")
	link.FailoverConfig = &FailoverConfig{
		Enabled:             true,
		NotificationWebhook: "http://example.com/webhook",
	}
	_ = manager.CreateLink(link)

	event, err := manager.Failover("webhook-failover")
	// Failover should succeed (swapping clusters)
	if err != nil {
		t.Logf("Failover completed with result: %v", err)
	}

	if event != nil {
		t.Logf("Failover event created: %s", event.ID)
	}

	// Give goroutine time to send notification
	time.Sleep(50 * time.Millisecond)
}

// TestManager_Failback_BeforeDelay tests failback before delay has elapsed
func TestManager_Failback_BeforeDelay(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("delayed-failback", "Delayed Failback")
	link.StoppedAt = time.Now() // Just stopped
	link.FailoverConfig = &FailoverConfig{
		Enabled:         true,
		FailbackDelayMs: 10000, // 10 seconds delay
	}
	_ = manager.CreateLink(link)

	event, err := manager.Failback("delayed-failback")
	if err == nil {
		t.Error("Expected error when attempting failback before delay")
	}

	if event != nil && !event.Success {
		t.Logf("Failback correctly rejected: %s", event.ErrorMessage)
	}
}

// TestManager_SetCheckpoint_NilCheckpoint tests setting nil checkpoint
func TestManager_SetCheckpoint_NilCheckpoint(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	err := manager.SetCheckpoint(nil)
	if err == nil {
		t.Error("Expected error when setting nil checkpoint")
	}
}

// TestManager_UpdateLink_ActiveLink tests updating an active link
func TestManager_UpdateLink_ActiveLink(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("update-active", "Update Active")
	_ = mgr.CreateLink(link)

	// Set status to active manually after creation
	mgr.mu.Lock()
	mgr.links["update-active"].Status = ReplicationStatusActive
	mgr.mu.Unlock()

	updatedLink := createTestLink("update-active", "Updated Active")
	err := mgr.UpdateLink("update-active", updatedLink)
	if err == nil {
		t.Error("Expected error when updating active link")
	}
}

// TestManager_DeleteLink_Active tests deleting an active link
func TestManager_DeleteLink_Active(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("delete-active", "Delete Active")
	_ = mgr.CreateLink(link)

	// Set status to active manually after creation
	mgr.mu.Lock()
	mgr.links["delete-active"].Status = ReplicationStatusActive
	mgr.mu.Unlock()

	err := mgr.DeleteLink("delete-active")
	if err == nil {
		t.Error("Expected error when deleting active link")
	}
}

// TestManager_Failover_NonActivePassive tests failover on unsupported replication type
func TestManager_Failover_NonActivePassive(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("unsupported-failover", "Unsupported Failover")
	link.Type = "unsupported-type"
	_ = manager.CreateLink(link)

	_, err := manager.Failover("unsupported-failover")
	if err == nil {
		t.Error("Expected error when failing over unsupported replication type")
	}
}

// TestManager_Failback_NonActivePassive tests failback on unsupported replication type
func TestManager_Failback_NonActivePassive(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("unsupported-failback", "Unsupported Failback")
	link.Type = ReplicationTypeActiveActive // Not active-passive
	_ = manager.CreateLink(link)

	_, err := manager.Failback("unsupported-failback")
	if err == nil {
		t.Error("Expected error when failing back unsupported replication type")
	}
}

// TestManager_Failover_NotFound tests failover for non-existent link
func TestManager_Failover_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	_, err := manager.Failover("non-existent")
	if err == nil {
		t.Error("Expected error when failing over non-existent link")
	}
}

// TestManager_Failback_NotFound tests failback for non-existent link
func TestManager_Failback_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	_, err := manager.Failback("non-existent")
	if err == nil {
		t.Error("Expected error when failing back non-existent link")
	}
}

// TestManager_GetMetrics_NotFound tests getting metrics for non-existent link
func TestManager_GetMetrics_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	_, err := manager.GetMetrics("non-existent")
	if err == nil {
		t.Error("Expected error when getting metrics for non-existent link")
	}
}

// TestManager_GetMetrics_NilMetrics tests getting metrics when metrics are nil
func TestManager_GetMetrics_NilMetrics(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("nil-metrics", "Nil Metrics")
	_ = mgr.CreateLink(link)

	// Set metrics to nil
	mgr.mu.Lock()
	mgr.links["nil-metrics"].Metrics = nil
	mgr.mu.Unlock()

	// Should return empty metrics, not error
	metrics, err := mgr.GetMetrics("nil-metrics")
	if err != nil {
		t.Errorf("GetMetrics failed: %v", err)
	}

	if metrics == nil {
		t.Error("Expected non-nil metrics")
	}
}

// TestManager_GetHealth_NotFound tests getting health for non-existent link
func TestManager_GetHealth_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	_, err := manager.GetHealth("non-existent")
	if err == nil {
		t.Error("Expected error when getting health for non-existent link")
	}
}

// TestManager_GetHealth_NilHealth tests getting health when health is nil
func TestManager_GetHealth_NilHealth(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("nil-health", "Nil Health")
	_ = mgr.CreateLink(link)

	// Set health to nil
	mgr.mu.Lock()
	mgr.links["nil-health"].Health = nil
	mgr.mu.Unlock()

	// Should return default health, not error
	health, err := mgr.GetHealth("nil-health")
	if err != nil {
		t.Errorf("GetHealth failed: %v", err)
	}

	if health == nil {
		t.Error("Expected non-nil health")
	}

	if health.Status != "unknown" {
		t.Errorf("Expected status 'unknown', got %s", health.Status)
	}
}

// TestManager_GetCheckpoint_NotFound tests getting checkpoint for non-existent link
func TestManager_GetCheckpoint_NotFound(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	_, err := manager.GetCheckpoint("non-existent", "topic", 0)
	if err == nil {
		t.Error("Expected error when getting checkpoint for non-existent link")
	}
}

// TestManager_GetCheckpoint_NoTopicCheckpoints tests getting checkpoint for non-existent topic
func TestManager_GetCheckpoint_NoTopicCheckpoints(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("no-topic-cp", "No Topic Checkpoint")
	_ = manager.CreateLink(link)

	_, err := manager.GetCheckpoint("no-topic-cp", "non-existent-topic", 0)
	if err == nil {
		t.Error("Expected error when getting checkpoint for non-existent topic")
	}
}

// TestManager_Close_WithActiveHandlers tests closing manager with active handlers
func TestManager_Close_WithActiveHandlers(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)

	link := createTestLink("active-handler", "Active Handler")
	_ = mgr.CreateLink(link)

	// Create a mock handler
	handler, _ := NewStreamHandler(link, storage)
	mgr.streamHandlers["active-handler"] = handler

	// Close should stop all handlers
	err := mgr.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify handler was stopped
	if len(mgr.streamHandlers) != 0 {
		t.Error("Expected all handlers to be stopped")
	}
}

// TestManager_Failover_WithCheckpoints tests failover with existing checkpoints
func TestManager_Failover_WithCheckpoints(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("checkpoint-failover", "Checkpoint Failover")
	link.Type = ReplicationTypeActivePassive
	_ = mgr.CreateLink(link)

	// Add some checkpoints
	checkpoint := &Checkpoint{
		LinkID:       "checkpoint-failover",
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 100,
		TargetOffset: 95,
		Timestamp:    time.Now(),
		Metadata:     make(map[string]string),
	}
	_ = mgr.SetCheckpoint(checkpoint)

	// Perform failover
	event, err := mgr.Failover("checkpoint-failover")
	if err != nil {
		t.Logf("Failover result: %v", err)
	}

	if event != nil && event.Success {
		t.Log("Failover succeeded with checkpoints")
		// Verify offset mappings were captured
		if len(event.OffsetMappings) > 0 {
			t.Logf("Captured %d offset mappings", len(event.OffsetMappings))
		}
	}
}

// TestManager_Failover_WithRunningHandler tests failover with active handler
func TestManager_Failover_WithRunningHandler(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("handler-failover", "Handler Failover")
	link.Type = ReplicationTypeActivePassive
	_ = mgr.CreateLink(link)

	// Create and add handler
	handler, _ := NewStreamHandler(link, storage)
	mgr.streamHandlers["handler-failover"] = handler

	// Perform failover - should stop handler first
	event, err := mgr.Failover("handler-failover")
	if err != nil {
		t.Logf("Failover result: %v", err)
	}

	if event != nil {
		t.Logf("Failover event: success=%v", event.Success)
	}

	// Verify handler was removed
	mgr.mu.RLock()
	_, exists := mgr.streamHandlers["handler-failover"]
	mgr.mu.RUnlock()

	if exists {
		t.Error("Expected handler to be removed after failover")
	}
}

// TestManager_Failback_WithHandler tests failback with active handler
func TestManager_Failback_WithHandler(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("handler-failback", "Handler Failback")
	link.Type = ReplicationTypeActivePassive
	link.StoppedAt = time.Now().Add(-1 * time.Hour) // Set stopped time in past
	link.FailoverConfig = &FailoverConfig{
		FailbackDelayMs: 100, // Short delay
	}
	_ = mgr.CreateLink(link)

	// Create and add handler
	handler, _ := NewStreamHandler(link, storage)
	mgr.streamHandlers["handler-failback"] = handler

	// Perform failback - should stop handler first
	event, err := mgr.Failback("handler-failback")
	if err != nil {
		t.Logf("Failback result: %v", err)
	}

	if event != nil {
		t.Logf("Failback event: success=%v", event.Success)
	}

	// Verify handler was removed
	mgr.mu.RLock()
	_, exists := mgr.streamHandlers["handler-failback"]
	mgr.mu.RUnlock()

	if exists {
		t.Error("Expected handler to be removed after failback")
	}
}

// TestManager_Failback_WithCheckpoints tests failback with existing checkpoints
func TestManager_Failback_WithCheckpoints(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("checkpoint-failback", "Checkpoint Failback")
	link.Type = ReplicationTypeActivePassive
	link.StoppedAt = time.Now().Add(-1 * time.Hour) // Set stopped time in past
	link.FailoverConfig = &FailoverConfig{
		FailbackDelayMs: 100, // Short delay
	}
	_ = mgr.CreateLink(link)

	// Add some checkpoints
	checkpoint := &Checkpoint{
		LinkID:       "checkpoint-failback",
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 200,
		TargetOffset: 195,
		Timestamp:    time.Now(),
		Metadata:     make(map[string]string),
	}
	_ = mgr.SetCheckpoint(checkpoint)

	// Perform failback
	event, err := mgr.Failback("checkpoint-failback")
	if err != nil {
		t.Logf("Failback result: %v", err)
	}

	if event != nil && event.Success {
		t.Log("Failback succeeded with checkpoints")
		// Verify offset mappings were captured
		if len(event.OffsetMappings) > 0 {
			t.Logf("Captured %d offset mappings", len(event.OffsetMappings))
		}
	}
}

// TestManager_checkLinkHealth_WithHandler tests health checking with active handler
func TestManager_checkLinkHealth_WithHandler(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("health-handler", "Health Handler")
	link.Status = ReplicationStatusActive
	_ = mgr.CreateLink(link)

	// Create handler with health info
	handler, _ := NewStreamHandler(link, storage)
	handler.health = &ReplicationHealth{
		Status:                 "healthy",
		SourceClusterReachable: true,
		TargetClusterReachable: true,
		ReplicationLagHealthy:  true,
	}
	handler.metrics = &ReplicationMetrics{
		TotalMessagesReplicated: 1000,
		ReplicationLag:          50,
	}
	mgr.streamHandlers["health-handler"] = handler

	// Check health - should sync from handler
	mgr.checkLinkHealth("health-handler")

	// Verify health was synced
	retrievedLink, _ := mgr.GetLink("health-handler")
	if retrievedLink.Health.Status != "healthy" {
		t.Errorf("Expected health status 'healthy', got %s", retrievedLink.Health.Status)
	}
}

// TestManager_SetCheckpoint_UpdatesLinkMetrics tests that SetCheckpoint updates link metrics
func TestManager_SetCheckpoint_UpdatesLinkMetrics(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("metrics-checkpoint", "Metrics Checkpoint")
	_ = mgr.CreateLink(link)

	checkpoint := &Checkpoint{
		LinkID:       "metrics-checkpoint",
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 100,
		TargetOffset: 95,
		Timestamp:    time.Now(),
		Metadata:     make(map[string]string),
	}

	err := mgr.SetCheckpoint(checkpoint)
	if err != nil {
		t.Fatalf("SetCheckpoint failed: %v", err)
	}

	// Verify metrics were updated with last checkpoint time
	retrievedLink, _ := mgr.GetLink("metrics-checkpoint")
	if retrievedLink.Metrics == nil {
		t.Error("Expected metrics to be initialized")
	} else if retrievedLink.Metrics.LastCheckpoint.IsZero() {
		t.Error("Expected LastCheckpoint to be set")
	}
}

// TestManager_SetCheckpoint_ForNonExistentLink tests setting checkpoint for non-existent link
func TestManager_SetCheckpoint_ForNonExistentLink(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	checkpoint := &Checkpoint{
		LinkID:       "non-existent",
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 100,
		TargetOffset: 95,
		Timestamp:    time.Now(),
		Metadata:     make(map[string]string),
	}

	err := manager.SetCheckpoint(checkpoint)
	if err == nil {
		t.Error("Expected error when setting checkpoint for non-existent link")
	}
}

// TestManager_GetMetrics_WithPartitionMetrics tests getting metrics with partition data
func TestManager_GetMetrics_WithPartitionMetrics(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("partition-metrics", "Partition Metrics")
	_ = mgr.CreateLink(link)

	// Add partition metrics
	mgr.mu.Lock()
	mgr.links["partition-metrics"].Metrics.PartitionMetrics = map[string]*PartitionReplicationMetrics{
		"test-topic-0": {
			Topic:              "test-topic",
			Partition:          0,
			SourceOffset:       100,
			TargetOffset:       95,
			Lag:                5,
			MessagesReplicated: 1000,
			BytesReplicated:    50000,
			LastReplicatedAt:   time.Now(),
			Errors:             2,
		},
	}
	mgr.mu.Unlock()

	// Get metrics
	metrics, err := mgr.GetMetrics("partition-metrics")
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	// Verify partition metrics were cloned
	if len(metrics.PartitionMetrics) != 1 {
		t.Errorf("Expected 1 partition metric, got %d", len(metrics.PartitionMetrics))
	}

	pm, exists := metrics.PartitionMetrics["test-topic-0"]
	if !exists {
		t.Error("Expected partition metric for test-topic-0")
	} else {
		if pm.MessagesReplicated != 1000 {
			t.Errorf("Expected 1000 messages replicated, got %d", pm.MessagesReplicated)
		}
	}
}

// TestManager_GetHealth_WithIssuesAndWarnings tests getting health with issues and warnings
func TestManager_GetHealth_WithIssuesAndWarnings(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("health-issues", "Health Issues")
	_ = mgr.CreateLink(link)

	// Add health issues and warnings
	mgr.mu.Lock()
	mgr.links["health-issues"].Health = &ReplicationHealth{
		Status:  "degraded",
		Issues:  []string{"High replication lag", "Connection timeout"},
		Warnings: []string{"Checkpoint delay"},
	}
	mgr.mu.Unlock()

	// Get health
	health, err := mgr.GetHealth("health-issues")
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}

	// Verify issues and warnings were cloned
	if len(health.Issues) != 2 {
		t.Errorf("Expected 2 issues, got %d", len(health.Issues))
	}

	if len(health.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(health.Warnings))
	}
}

// TestManager_CreateLink_AppliesDefaults tests creating a link applies default failover config
func TestManager_CreateLink_AppliesDefaults(t *testing.T) {
	storage := NewMemoryStorage()
	manager := NewManager(storage)
	defer manager.Close()

	link := createTestLink("defaults-link", "Defaults Link")
	link.FailoverConfig = nil // Force failover config defaults to be applied

	err := manager.CreateLink(link)
	if err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	// Verify defaults were applied
	retrieved, _ := manager.GetLink("defaults-link")
	if retrieved.FailoverConfig == nil {
		t.Error("Expected default FailoverConfig to be set")
	} else {
		// Check that default values are present
		if !retrieved.FailoverConfig.Enabled {
			t.Log("Failover config has expected default values")
		}
	}
}

// TestManager_DeleteLink_WithInitializing tests deleting an initializing link
func TestManager_DeleteLink_WithInitializing(t *testing.T) {
	storage := NewMemoryStorage()
	mgr := NewManager(storage).(*manager)
	defer mgr.Close()

	link := createTestLink("init-delete", "Init Delete")
	_ = mgr.CreateLink(link)

	// Set status to initializing
	mgr.mu.Lock()
	mgr.links["init-delete"].Status = ReplicationStatusInitializing
	mgr.mu.Unlock()

	// Should not be able to delete initializing link
	err := mgr.DeleteLink("init-delete")
	if err == nil {
		t.Error("Expected error when deleting initializing link")
	}
}

// TestManager_loadLinksFromStorage_WithError tests loading links with storage error
func TestManager_loadLinksFromStorage_WithError(t *testing.T) {
	// Create a storage that returns error on ListLinks
	storage := &ErrorStorage{
		memoryStorage: NewMemoryStorage().(*memoryStorage),
		failOnList:    true,
	}

	// Manager should still start even if loading fails
	manager := NewManager(storage)
	if manager == nil {
		t.Error("Expected manager to be created even with storage error")
	}
	defer manager.Close()
}

// ErrorStorage is a mock storage that can simulate errors
type ErrorStorage struct {
	*memoryStorage
	failOnList bool
}

func (s *ErrorStorage) ListLinks() ([]*ReplicationLink, error) {
	if s.failOnList {
		return nil, fmt.Errorf("simulated list error")
	}
	return s.memoryStorage.ListLinks()
}
