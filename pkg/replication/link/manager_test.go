package link

import (
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

	manager.CreateLink(link1)
	manager.CreateLink(link2)
	manager.CreateLink(link3)

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
	manager.CreateLink(link)

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
	manager.CreateLink(link)

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
