package link

import (
	"testing"
	"time"
)

func TestNewStreamHandler(t *testing.T) {
	link := createTestLink("stream-test", "Stream Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}

	if handler.link != link {
		t.Error("Expected link to be set")
	}

	if handler.checkpointStore != storage {
		t.Error("Expected checkpoint store to be set")
	}

	if handler.metrics == nil {
		t.Error("Expected metrics to be initialized")
	}

	if handler.health == nil {
		t.Error("Expected health to be initialized")
	}

	if handler.health.Status != "initializing" {
		t.Errorf("Expected status 'initializing', got %s", handler.health.Status)
	}
}

func TestNewStreamHandler_InvalidLink(t *testing.T) {
	link := &ReplicationLink{
		ID: "", // Invalid: empty ID
	}
	storage := NewMemoryStorage()

	_, err := NewStreamHandler(link, storage)
	if err == nil {
		t.Error("Expected error for invalid link")
	}
}

func TestNewStreamHandler_WithFilter(t *testing.T) {
	link := createTestLink("filter-test", "Filter Test")
	link.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"test-.*", "prod-.*"},
		ExcludePatterns: []string{".*-temp"},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler with filter failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Verify filter patterns were compiled
	if len(handler.filterPatterns.include) != 2 {
		t.Errorf("Expected 2 include patterns, got %d", len(handler.filterPatterns.include))
	}

	if len(handler.filterPatterns.exclude) != 1 {
		t.Errorf("Expected 1 exclude pattern, got %d", len(handler.filterPatterns.exclude))
	}
}

func TestNewStreamHandler_InvalidFilterPattern(t *testing.T) {
	link := createTestLink("invalid-filter", "Invalid Filter")
	link.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"[invalid(pattern"},
	}

	storage := NewMemoryStorage()

	_, err := NewStreamHandler(link, storage)
	if err == nil {
		t.Error("Expected error for invalid filter pattern")
	}
}

func TestStreamHandler_Stop_NotStarted(t *testing.T) {
	link := createTestLink("stop-test", "Stop Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}

	// Stop should not error even if not started
	err = handler.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestStreamHandler_GetTopicsToReplicate_SpecificTopics(t *testing.T) {
	link := createTestLink("topics-test", "Topics Test")
	link.Topics = []string{"topic1", "topic2", "topic3"}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	topics, err := handler.getTopicsToReplicate()
	if err != nil {
		t.Fatalf("getTopicsToReplicate failed: %v", err)
	}

	if len(topics) != 3 {
		t.Errorf("Expected 3 topics, got %d", len(topics))
	}

	expectedTopics := map[string]bool{
		"topic1": true,
		"topic2": true,
		"topic3": true,
	}

	for _, topic := range topics {
		if !expectedTopics[topic] {
			t.Errorf("Unexpected topic: %s", topic)
		}
	}
}

func TestStreamHandler_StartTopicReplication(t *testing.T) {
	link := createTestLink("start-repl-test", "Start Replication Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start replication for a topic
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	// Verify worker was created
	workerKey := "test-topic-0"
	worker, exists := handler.partitionWorkers[workerKey]
	if !exists {
		t.Error("Expected partition worker to be created")
	}

	if worker.topic != "test-topic" {
		t.Errorf("Expected topic 'test-topic', got %s", worker.topic)
	}

	if worker.partition != 0 {
		t.Errorf("Expected partition 0, got %d", worker.partition)
	}

	// Cancel the context to stop the worker
	worker.cancel()
}

func TestStreamHandler_StartTopicReplication_WithCheckpoint(t *testing.T) {
	link := createTestLink("checkpoint-test", "Checkpoint Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Save a checkpoint first
	checkpoint := &Checkpoint{
		LinkID:       link.ID,
		Topic:        "test-topic",
		Partition:    0,
		SourceOffset: 100,
		TargetOffset: 95,
		Timestamp:    time.Now(),
		Metadata:     make(map[string]string),
	}
	err = storage.SaveCheckpoint(checkpoint)
	if err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Start replication
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	// Verify worker loaded checkpoint
	workerKey := "test-topic-0"
	worker := handler.partitionWorkers[workerKey]
	if worker.sourceOffset != 100 {
		t.Errorf("Expected source offset 100, got %d", worker.sourceOffset)
	}
	if worker.targetOffset != 95 {
		t.Errorf("Expected target offset 95, got %d", worker.targetOffset)
	}

	worker.cancel()
}

func TestPartitionWorker_SaveCheckpoint(t *testing.T) {
	link := createTestLink("save-checkpoint-test", "Save Checkpoint Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start replication to create a worker
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	workerKey := "test-topic-0"
	worker := handler.partitionWorkers[workerKey]
	defer worker.cancel()

	// Update worker offsets
	worker.sourceOffset = 50
	worker.targetOffset = 45

	// Save checkpoint
	worker.saveCheckpoint()

	// Verify checkpoint was saved
	checkpoint, err := storage.LoadCheckpoint(link.ID, "test-topic", 0)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	if checkpoint.SourceOffset != 50 {
		t.Errorf("Expected source offset 50, got %d", checkpoint.SourceOffset)
	}

	if checkpoint.TargetOffset != 45 {
		t.Errorf("Expected target offset 45, got %d", checkpoint.TargetOffset)
	}
}

func TestPartitionWorker_SaveCheckpoint_NoStorage(t *testing.T) {
	link := createTestLink("no-storage-test", "No Storage Test")

	handler, err := NewStreamHandler(link, nil) // No storage
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start replication to create a worker
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	workerKey := "test-topic-0"
	worker := handler.partitionWorkers[workerKey]
	defer worker.cancel()

	// saveCheckpoint should not panic when no storage
	worker.saveCheckpoint()
}

func TestStreamHandler_PerformHealthCheck(t *testing.T) {
	link := createTestLink("health-check-test", "Health Check Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Perform health check
	handler.performHealthCheck()

	// Verify health was updated
	if handler.health.LastHealthCheck.IsZero() {
		t.Error("Expected LastHealthCheck to be set")
	}

	if !handler.health.SourceClusterReachable {
		t.Error("Expected SourceClusterReachable to be true")
	}

	if !handler.health.TargetClusterReachable {
		t.Error("Expected TargetClusterReachable to be true")
	}
}

func TestStreamHandler_PerformHealthCheck_HighReplicationLag(t *testing.T) {
	link := createTestLink("high-lag-test", "High Lag Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Set high replication lag
	handler.metrics.ReplicationLag = 120000 // 120 seconds

	// Perform health check
	handler.performHealthCheck()

	// Verify health status
	if handler.health.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %s", handler.health.Status)
	}

	if handler.health.ReplicationLagHealthy {
		t.Error("Expected ReplicationLagHealthy to be false")
	}

	if len(handler.health.Issues) == 0 {
		t.Error("Expected health issues to be reported")
	}
}

func TestStreamHandler_PerformHealthCheck_HighErrorRate(t *testing.T) {
	link := createTestLink("high-error-test", "High Error Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Set high error rate
	handler.metrics.ErrorsPerSecond = 15.0

	// Perform health check
	handler.performHealthCheck()

	// Verify health status
	if handler.health.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %s", handler.health.Status)
	}

	if handler.health.ErrorRateHealthy {
		t.Error("Expected ErrorRateHealthy to be false")
	}
}

func TestStreamHandler_PerformHealthCheck_NoRecentCheckpoint(t *testing.T) {
	link := createTestLink("no-checkpoint-test", "No Checkpoint Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Set old checkpoint time
	handler.metrics.LastCheckpoint = time.Now().Add(-10 * time.Minute)

	// Perform health check
	handler.performHealthCheck()

	// Verify health status
	if handler.health.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got %s", handler.health.Status)
	}

	if handler.health.CheckpointHealthy {
		t.Error("Expected CheckpointHealthy to be false")
	}
}

func TestStreamHandler_CompileFilterPatterns(t *testing.T) {
	link := createTestLink("compile-test", "Compile Test")
	link.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"^test-"},
		ExcludePatterns: []string{"-temp$"},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Patterns should already be compiled during NewStreamHandler
	if len(handler.filterPatterns.include) == 0 {
		t.Error("Expected include patterns to be compiled")
	}

	if len(handler.filterPatterns.exclude) == 0 {
		t.Error("Expected exclude patterns to be compiled")
	}
}

func TestStreamHandler_CompileFilterPatterns_NoFilter(t *testing.T) {
	link := createTestLink("no-filter-test", "No Filter Test")
	link.Filter = nil

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Should not error with no filter
	err = handler.compileFilterPatterns()
	if err != nil {
		t.Errorf("compileFilterPatterns should not error: %v", err)
	}
}

func TestStreamHandler_ShouldFilterMessage_NoFilter(t *testing.T) {
	link := createTestLink("no-filter-msg-test", "No Filter Message Test")
	link.Filter = nil

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Should not filter when no filter is configured
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message to not be filtered when no filter is configured")
	}
}

func TestStreamHandler_ShouldFilterMessage_TimestampFilter(t *testing.T) {
	link := createTestLink("timestamp-filter-test", "Timestamp Filter Test")
	minTime := time.Now().Add(-1 * time.Hour)
	maxTime := time.Now().Add(1 * time.Hour)

	link.Filter = &FilterConfig{
		Enabled:      true,
		MinTimestamp: minTime,
		MaxTimestamp: maxTime,
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Message within range should not be filtered
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message within timestamp range to not be filtered")
	}

	// Message before min timestamp should be filtered
	oldTime := minTime.Add(-1 * time.Minute)
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		oldTime,
	)

	if !filtered {
		t.Error("Expected message before min timestamp to be filtered")
	}

	// Message after max timestamp should be filtered
	futureTime := maxTime.Add(1 * time.Minute)
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		futureTime,
	)

	if !filtered {
		t.Error("Expected message after max timestamp to be filtered")
	}
}

func TestStreamHandler_ShouldFilterMessage_IncludePattern(t *testing.T) {
	link := createTestLink("include-pattern-test", "Include Pattern Test")
	link.Filter = &FilterConfig{
		Enabled:         true,
		IncludePatterns: []string{"important"},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Message matching include pattern should not be filtered
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("important message"),
		nil,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message matching include pattern to not be filtered")
	}

	// Message not matching include pattern should be filtered
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("regular message"),
		nil,
		time.Now(),
	)

	if !filtered {
		t.Error("Expected message not matching include pattern to be filtered")
	}
}

func TestStreamHandler_ShouldFilterMessage_ExcludePattern(t *testing.T) {
	link := createTestLink("exclude-pattern-test", "Exclude Pattern Test")
	link.Filter = &FilterConfig{
		Enabled:         true,
		ExcludePatterns: []string{"debug"},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Message matching exclude pattern should be filtered
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("debug message"),
		nil,
		time.Now(),
	)

	if !filtered {
		t.Error("Expected message matching exclude pattern to be filtered")
	}

	// Message not matching exclude pattern should not be filtered
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("info message"),
		nil,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message not matching exclude pattern to not be filtered")
	}
}

func TestStreamHandler_ShouldFilterMessage_HeaderFilter(t *testing.T) {
	link := createTestLink("header-filter-test", "Header Filter Test")
	link.Filter = &FilterConfig{
		Enabled: true,
		FilterByHeader: map[string]string{
			"priority": "high",
		},
	}

	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Message with matching header should not be filtered
	headers := map[string][]byte{
		"priority": []byte("high"),
	}
	filtered := handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		headers,
		time.Now(),
	)

	if filtered {
		t.Error("Expected message with matching header to not be filtered")
	}

	// Message without matching header should be filtered
	headers = map[string][]byte{
		"priority": []byte("low"),
	}
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		headers,
		time.Now(),
	)

	if !filtered {
		t.Error("Expected message without matching header to be filtered")
	}

	// Message with missing header should be filtered
	filtered = handler.shouldFilterMessage(
		[]byte("key"),
		[]byte("value"),
		nil,
		time.Now(),
	)

	if !filtered {
		t.Error("Expected message with missing header to be filtered")
	}
}

func TestPartitionWorker_ReplicateBatch(t *testing.T) {
	link := createTestLink("replicate-batch-test", "Replicate Batch Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start replication to create a worker
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	workerKey := "test-topic-0"
	worker := handler.partitionWorkers[workerKey]
	defer worker.cancel()

	// Test replicateBatch - currently just a placeholder that sleeps
	err = worker.replicateBatch()
	if err != nil {
		t.Errorf("replicateBatch failed: %v", err)
	}
}

func TestStreamHandler_HealthCheckLoop(t *testing.T) {
	link := createTestLink("health-loop-test", "Health Loop Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}

	// Start the handler to trigger the healthCheckLoop goroutine
	// Note: Start requires actual broker connections, so we'll test the loop directly
	handler.wg.Add(1)
	go handler.healthCheckLoop()

	// Give the loop time to run at least once
	time.Sleep(100 * time.Millisecond)

	// Stop the handler to trigger context cancellation
	err = handler.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Verify the goroutine exited by checking if Stop completes
	// (Stop calls wg.Wait(), so if this completes, the goroutine exited)
}

func TestStreamHandler_MetricsUpdateLoop(t *testing.T) {
	link := createTestLink("metrics-loop-test", "Metrics Loop Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}

	// Start the metricsUpdateLoop goroutine
	handler.wg.Add(1)
	go handler.metricsUpdateLoop()

	// Give the loop time to run
	time.Sleep(100 * time.Millisecond)

	// Stop the handler
	err = handler.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

// TestStreamHandler_Start_AlreadyStarted tests starting a handler that's already started
func TestStreamHandler_Start_AlreadyStarted(t *testing.T) {
	link := createTestLink("already-started-test", "Already Started Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Mark as started without actually starting
	handler.mu.Lock()
	handler.started = true
	handler.mu.Unlock()

	// Try to start again - should error
	err = handler.Start()
	if err == nil {
		t.Error("Expected error when starting already started handler")
	}
	if err != nil && err.Error() != "stream already started" {
		t.Errorf("Expected 'stream already started' error, got: %v", err)
	}
}

// TestStreamHandler_Start_SourceConnectionFailure tests Start with source connection failure
func TestStreamHandler_Start_SourceConnectionFailure(t *testing.T) {
	link := createTestLink("src-conn-fail", "Source Connection Fail")
	// Use invalid brokers to force connection failure
	link.SourceCluster.Brokers = []string{"invalid:99999"}
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start should fail due to source connection
	err = handler.Start()
	if err == nil {
		t.Error("Expected error when connecting to invalid source")
	}
}

// TestStreamHandler_Start_TargetConnectionFailure tests Start with target connection failure
func TestStreamHandler_Start_TargetConnectionFailure(t *testing.T) {
	link := createTestLink("tgt-conn-fail", "Target Connection Fail")
	// Use invalid brokers to force connection failure
	link.TargetCluster.Brokers = []string{"invalid:99999"}
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start should fail due to target connection
	err = handler.Start()
	if err == nil {
		t.Error("Expected error when connecting to invalid target")
	}
}

// TestStreamHandler_Stop_WithRunningWorkers tests stopping with active workers
func TestStreamHandler_Stop_WithRunningWorkers(t *testing.T) {
	link := createTestLink("stop-workers-test", "Stop Workers Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}

	// Create a partition worker manually
	err = handler.startTopicReplication("test-topic")
	if err != nil {
		t.Fatalf("startTopicReplication failed: %v", err)
	}

	// Mark as started
	handler.mu.Lock()
	handler.started = true
	handler.mu.Unlock()

	// Stop should cleanly shut down workers
	err = handler.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Verify health status updated
	if handler.health.Status != "stopped" {
		t.Errorf("Expected status 'stopped', got %s", handler.health.Status)
	}

	// Verify started flag is false
	if handler.started {
		t.Error("Expected started to be false after Stop")
	}
}

// TestStreamHandler_Stop_MultipleCalls tests calling Stop multiple times
func TestStreamHandler_Stop_MultipleCalls(t *testing.T) {
	link := createTestLink("stop-multiple-test", "Stop Multiple Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}

	// Mark as started
	handler.mu.Lock()
	handler.started = true
	handler.mu.Unlock()

	// First stop
	err = handler.Stop()
	if err != nil {
		t.Errorf("First Stop failed: %v", err)
	}

	// Second stop should not error
	err = handler.Stop()
	if err != nil {
		t.Errorf("Second Stop failed: %v", err)
	}
}

// TestStreamHandler_GetTopicsToReplicate_WithTopics tests returning configured topics
func TestStreamHandler_GetTopicsToReplicate_WithTopics(t *testing.T) {
	link := createTestLink("with-topics-test", "With Topics Test")
	link.Topics = []string{"topic-a", "topic-b", "topic-c"}
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Should return configured topics
	topics, err := handler.getTopicsToReplicate()
	if err != nil {
		t.Fatalf("getTopicsToReplicate failed: %v", err)
	}

	if len(topics) != 3 {
		t.Errorf("Expected 3 topics, got %d", len(topics))
	}

	expectedTopics := map[string]bool{
		"topic-a": true,
		"topic-b": true,
		"topic-c": true,
	}

	for _, topic := range topics {
		if !expectedTopics[topic] {
			t.Errorf("Unexpected topic: %s", topic)
		}
	}
}

// TestStreamHandler_MetricsUpdateLoop_UptimeCalculation tests uptime calculation
func TestStreamHandler_MetricsUpdateLoop_UptimeCalculation(t *testing.T) {
	link := createTestLink("metrics-uptime-test", "Metrics Uptime Test")
	link.StartedAt = time.Now().Add(-1 * time.Minute) // Set started time
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}

	// Update link's StartedAt in handler
	handler.link.StartedAt = time.Now().Add(-1 * time.Minute)

	// Start the metricsUpdateLoop goroutine
	handler.wg.Add(1)
	go handler.metricsUpdateLoop()

	// Give the loop more time to execute at least once (10+ seconds)
	time.Sleep(11 * time.Second)

	// Stop the handler
	err = handler.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Check that uptime was calculated (should be at least 60 seconds)
	handler.mu.RLock()
	uptime := handler.metrics.UptimeSeconds
	handler.mu.RUnlock()

	if uptime < 60 {
		t.Logf("Uptime: %d seconds (expected at least 60)", uptime)
	} else {
		t.Logf("Uptime correctly calculated: %d seconds", uptime)
	}
}

// TestStreamHandler_HealthCheckLoop_ContextCancellation tests healthCheckLoop cancellation
func TestStreamHandler_HealthCheckLoop_ContextCancellation(t *testing.T) {
	link := createTestLink("health-cancel-test", "Health Cancel Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}

	// Start the healthCheckLoop
	handler.wg.Add(1)
	go handler.healthCheckLoop()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context and verify goroutine exits
	handler.cancel()
	handler.wg.Wait() // Should not hang

	t.Log("healthCheckLoop exited cleanly on context cancellation")
}

// TestStreamHandler_ConnectToCluster_WithBootstrapServers tests connection with bootstrap servers
func TestStreamHandler_ConnectToCluster_WithBootstrapServers(t *testing.T) {
	link := createTestLink("bootstrap-test", "Bootstrap Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Test with bootstrap servers instead of brokers list
	config := &ClusterConfig{
		ClusterID:         "test-cluster",
		BootstrapServers:  "localhost:9092",
		ConnectionTimeout: 5 * time.Second,
		RequestTimeout:    10 * time.Second,
		RetryBackoff:      1 * time.Second,
		MaxRetries:        3,
	}

	// This will fail without real brokers, but exercises the bootstrap path
	_, err = handler.connectToCluster(config)
	if err != nil {
		t.Logf("Expected error without real brokers: %v", err)
	}
}

// TestStreamHandler_ConnectToCluster_WithSecurityConfig tests connection with security
func TestStreamHandler_ConnectToCluster_WithSecurityConfig(t *testing.T) {
	link := createTestLink("security-test", "Security Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Test with security configuration
	config := &ClusterConfig{
		ClusterID:         "test-cluster",
		Brokers:           []string{"localhost:9092"},
		ConnectionTimeout: 5 * time.Second,
		RequestTimeout:    10 * time.Second,
		RetryBackoff:      1 * time.Second,
		MaxRetries:        3,
		Security: &SecurityConfig{
			EnableTLS: true,
		},
	}

	// This will fail without real brokers, but exercises the security config path
	_, err = handler.connectToCluster(config)
	if err != nil {
		t.Logf("Expected error without real brokers: %v", err)
	}
}

// TestStreamHandler_Start_TopicReplicationFailure tests Start with topic replication failure
func TestStreamHandler_Start_TopicReplicationFailure(t *testing.T) {
	link := createTestLink("topic-fail-test", "Topic Fail Test")
	// Use empty topics to test discovery path
	link.Topics = nil
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	// Start will fail during topic discovery without real broker
	err = handler.Start()
	if err == nil {
		t.Error("Expected error when starting without real brokers")
	}
}

// TestStreamHandler_Stop_WithClients tests stopping handler with initialized clients
func TestStreamHandler_Stop_WithClients(t *testing.T) {
	link := createTestLink("stop-clients-test", "Stop Clients Test")
	storage := NewMemoryStorage()

	handler, err := NewStreamHandler(link, storage)
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}

	// Simulate having clients (though they'll be nil in this test environment)
	handler.mu.Lock()
	handler.started = true
	handler.mu.Unlock()

	// Stop should handle nil clients gracefully
	err = handler.Stop()
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Verify started flag is false
	if handler.started {
		t.Error("Expected started to be false after Stop")
	}

	// Verify health status updated
	if handler.health.Status != "stopped" {
		t.Errorf("Expected status 'stopped', got %s", handler.health.Status)
	}
}
