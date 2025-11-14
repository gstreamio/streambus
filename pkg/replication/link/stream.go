package link

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/shawntherrien/streambus/pkg/client"
)

// StreamHandler handles the replication stream for a single link
type StreamHandler struct {
	link *ReplicationLink

	// sourceClient is the client for the source cluster
	sourceClient *client.Client

	// targetClient is the client for the target cluster
	targetClient *client.Client

	// partitionWorkers tracks running partition workers
	partitionWorkers map[string]*partitionWorker

	// metrics tracks replication metrics
	metrics *ReplicationMetrics

	// health tracks health status
	health *ReplicationHealth

	// checkpointStore stores checkpoints
	checkpointStore Storage

	// ctx is the stream context
	ctx context.Context

	// cancel cancels the stream
	cancel context.CancelFunc

	// wg tracks worker goroutines
	wg sync.WaitGroup

	// mu protects mutable state
	mu sync.RWMutex

	// started indicates if the stream has been started
	started bool

	// filterPatterns are compiled regex patterns for filtering
	filterPatterns struct {
		include []*regexp.Regexp
		exclude []*regexp.Regexp
	}
}

// partitionWorker handles replication for a single partition
type partitionWorker struct {
	topic     string
	partition int32
	handler   *StreamHandler

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Current offsets
	sourceOffset int64
	targetOffset int64

	// Metrics
	messagesReplicated int64
	bytesReplicated    int64
	errors             int64
	lastReplicatedAt   time.Time
}

// NewStreamHandler creates a new stream handler for a replication link
func NewStreamHandler(link *ReplicationLink, checkpointStore Storage) (*StreamHandler, error) {
	if err := link.Validate(); err != nil {
		return nil, fmt.Errorf("invalid link configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	handler := &StreamHandler{
		link:             link,
		partitionWorkers: make(map[string]*partitionWorker),
		metrics:          link.Metrics,
		health:           link.Health,
		checkpointStore:  checkpointStore,
		ctx:              ctx,
		cancel:           cancel,
	}

	// Initialize metrics if not present
	if handler.metrics == nil {
		handler.metrics = &ReplicationMetrics{
			PartitionMetrics: make(map[string]*PartitionReplicationMetrics),
		}
	}

	// Initialize health if not present
	if handler.health == nil {
		handler.health = &ReplicationHealth{
			Status: "initializing",
		}
	}

	// Compile filter patterns if filtering is enabled
	if link.Filter != nil && link.Filter.Enabled {
		if err := handler.compileFilterPatterns(); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to compile filter patterns: %w", err)
		}
	}

	return handler, nil
}

// Start starts the replication stream
func (h *StreamHandler) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return fmt.Errorf("stream already started")
	}

	// Connect to source cluster
	sourceClient, err := h.connectToCluster(&h.link.SourceCluster)
	if err != nil {
		return fmt.Errorf("failed to connect to source cluster: %w", err)
	}
	h.sourceClient = sourceClient

	// Connect to target cluster
	targetClient, err := h.connectToCluster(&h.link.TargetCluster)
	if err != nil {
		h.sourceClient.Close()
		return fmt.Errorf("failed to connect to target cluster: %w", err)
	}
	h.targetClient = targetClient

	// Get topics to replicate
	topics, err := h.getTopicsToReplicate()
	if err != nil {
		h.sourceClient.Close()
		h.targetClient.Close()
		return fmt.Errorf("failed to get topics: %w", err)
	}

	// Start partition workers for each topic
	for _, topic := range topics {
		if err := h.startTopicReplication(topic); err != nil {
			h.Stop()
			return fmt.Errorf("failed to start replication for topic %s: %w", topic, err)
		}
	}

	// Start health check goroutine
	h.wg.Add(1)
	go h.healthCheckLoop()

	// Start metrics update goroutine
	h.wg.Add(1)
	go h.metricsUpdateLoop()

	h.started = true
	h.health.Status = "healthy"

	return nil
}

// Stop stops the replication stream
func (h *StreamHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return nil
	}

	// Cancel context to stop all workers
	h.cancel()

	// Wait for all workers to finish
	h.wg.Wait()

	// Close clients
	if h.sourceClient != nil {
		h.sourceClient.Close()
	}
	if h.targetClient != nil {
		h.targetClient.Close()
	}

	h.started = false
	h.health.Status = "stopped"

	return nil
}

// connectToCluster creates a client connection to a cluster
func (h *StreamHandler) connectToCluster(config *ClusterConfig) (*client.Client, error) {
	// Build broker addresses
	brokers := config.Brokers
	if len(brokers) == 0 && config.BootstrapServers != "" {
		// Parse bootstrap servers
		brokers = []string{config.BootstrapServers}
	}

	// Create client configuration
	clientConfig := &client.Config{
		Brokers:        brokers,
		ConnectTimeout: config.ConnectionTimeout,
		RequestTimeout: config.RequestTimeout,
		RetryBackoff:   config.RetryBackoff,
		MaxRetries:     config.MaxRetries,
	}

	// Apply security configuration if present
	if config.Security != nil && config.Security.EnableTLS {
		// TODO: Configure TLS settings
	}

	// Create and connect client
	c, err := client.New(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c, nil
}

// getTopicsToReplicate returns the list of topics to replicate
func (h *StreamHandler) getTopicsToReplicate() ([]string, error) {
	// If specific topics are configured, use those
	if len(h.link.Topics) > 0 {
		return h.link.Topics, nil
	}

	// Otherwise, list all topics from source cluster
	topics, err := h.sourceClient.ListTopics()
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %w", err)
	}

	return topics, nil
}

// startTopicReplication starts replication for a topic
func (h *StreamHandler) startTopicReplication(topic string) error {
	// Get topic metadata from source
	// TODO: Implement when client has GetTopicMetadata method
	// For now, assume 1 partition for simplicity
	numPartitions := 1

	// Start a worker for each partition
	for partition := 0; partition < numPartitions; partition++ {
		workerKey := fmt.Sprintf("%s-%d", topic, partition)

		worker := &partitionWorker{
			topic:     topic,
			partition: int32(partition),
			handler:   h,
		}

		worker.ctx, worker.cancel = context.WithCancel(h.ctx)

		// Load checkpoint if available
		if h.checkpointStore != nil {
			checkpoint, err := h.checkpointStore.LoadCheckpoint(h.link.ID, topic, int32(partition))
			if err == nil {
				worker.sourceOffset = checkpoint.SourceOffset
				worker.targetOffset = checkpoint.TargetOffset
			}
		}

		h.partitionWorkers[workerKey] = worker

		// Start worker goroutine
		h.wg.Add(1)
		go worker.run()
	}

	return nil
}

// run is the main loop for a partition worker
func (w *partitionWorker) run() {
	defer w.handler.wg.Done()

	ticker := time.NewTicker(time.Duration(w.handler.link.Config.CheckpointIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			// Save final checkpoint before exiting
			w.saveCheckpoint()
			return

		case <-ticker.C:
			// Periodic checkpoint
			w.saveCheckpoint()

		default:
			// Fetch and replicate messages
			if err := w.replicateBatch(); err != nil {
				w.errors++
				w.handler.metrics.TotalErrors++
				w.handler.metrics.ConsecutiveFailures++

				// Check if we should mark as failed
				if w.handler.metrics.ConsecutiveFailures > int(w.handler.link.FailoverConfig.MaxConsecutiveFailures) {
					w.handler.health.Status = "unhealthy"
					w.handler.health.Issues = append(w.handler.health.Issues,
						fmt.Sprintf("Too many consecutive failures: %d", w.handler.metrics.ConsecutiveFailures))
				}

				// Backoff on error
				time.Sleep(time.Duration(w.handler.link.Config.FetchWaitMaxMs) * time.Millisecond)
				continue
			}

			// Reset consecutive failures on success
			w.handler.metrics.ConsecutiveFailures = 0
		}
	}
}

// replicateBatch fetches and replicates a batch of messages
func (w *partitionWorker) replicateBatch() error {
	// TODO: Implement actual fetch and produce logic
	// This is a placeholder that will be implemented when client supports
	// the necessary methods for cross-cluster replication

	// For now, just simulate some work
	time.Sleep(100 * time.Millisecond)

	return nil
}

// saveCheckpoint saves the current offset checkpoint
func (w *partitionWorker) saveCheckpoint() {
	if w.handler.checkpointStore == nil {
		return
	}

	checkpoint := &Checkpoint{
		LinkID:       w.handler.link.ID,
		Topic:        w.topic,
		Partition:    w.partition,
		SourceOffset: w.sourceOffset,
		TargetOffset: w.targetOffset,
		Timestamp:    time.Now(),
		Metadata:     make(map[string]string),
	}

	if err := w.handler.checkpointStore.SaveCheckpoint(checkpoint); err != nil {
		// Log error but don't fail
		w.errors++
	}
}

// healthCheckLoop periodically checks health status
func (h *StreamHandler) healthCheckLoop() {
	defer h.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.performHealthCheck()
		}
	}
}

// performHealthCheck checks the health of the replication stream
func (h *StreamHandler) performHealthCheck() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.health.LastHealthCheck = time.Now()
	h.health.Issues = nil
	h.health.Warnings = nil

	// Check source cluster connectivity
	// TODO: Implement ping when client supports it
	h.health.SourceClusterReachable = true

	// Check target cluster connectivity
	h.health.TargetClusterReachable = true

	// Check replication lag
	if h.metrics.ReplicationLag > 60000 { // 60 seconds
		h.health.ReplicationLagHealthy = false
		h.health.Issues = append(h.health.Issues, "Replication lag exceeds 60 seconds")
	} else {
		h.health.ReplicationLagHealthy = true
	}

	// Check error rate
	if h.metrics.ErrorsPerSecond > 10 {
		h.health.ErrorRateHealthy = false
		h.health.Issues = append(h.health.Issues, "Error rate exceeds threshold")
	} else {
		h.health.ErrorRateHealthy = true
	}

	// Check checkpoint status
	timeSinceCheckpoint := time.Since(h.metrics.LastCheckpoint)
	if timeSinceCheckpoint > 5*time.Minute {
		h.health.CheckpointHealthy = false
		h.health.Issues = append(h.health.Issues, "No checkpoint in last 5 minutes")
	} else {
		h.health.CheckpointHealthy = true
	}

	// Determine overall health status
	if len(h.health.Issues) > 0 {
		h.health.Status = "unhealthy"
	} else if len(h.health.Warnings) > 0 {
		h.health.Status = "degraded"
	} else {
		h.health.Status = "healthy"
	}
}

// metricsUpdateLoop periodically updates metrics
func (h *StreamHandler) metricsUpdateLoop() {
	defer h.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	lastUpdate := time.Now()
	lastMessages := h.metrics.TotalMessagesReplicated
	lastBytes := h.metrics.TotalBytesReplicated
	lastErrors := h.metrics.TotalErrors

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(lastUpdate).Seconds()

			h.mu.Lock()

			// Calculate rates
			messagesDelta := h.metrics.TotalMessagesReplicated - lastMessages
			bytesDelta := h.metrics.TotalBytesReplicated - lastBytes
			errorsDelta := h.metrics.TotalErrors - lastErrors

			h.metrics.MessagesPerSecond = float64(messagesDelta) / elapsed
			h.metrics.BytesPerSecond = float64(bytesDelta) / elapsed
			h.metrics.ErrorsPerSecond = float64(errorsDelta) / elapsed

			// Update last values
			lastUpdate = now
			lastMessages = h.metrics.TotalMessagesReplicated
			lastBytes = h.metrics.TotalBytesReplicated
			lastErrors = h.metrics.TotalErrors

			// Calculate uptime
			if !h.link.StartedAt.IsZero() {
				h.metrics.UptimeSeconds = int64(time.Since(h.link.StartedAt).Seconds())
			}

			h.mu.Unlock()
		}
	}
}

// compileFilterPatterns compiles regex patterns for message filtering
func (h *StreamHandler) compileFilterPatterns() error {
	if h.link.Filter == nil || !h.link.Filter.Enabled {
		return nil
	}

	// Compile include patterns
	for _, pattern := range h.link.Filter.IncludePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid include pattern %s: %w", pattern, err)
		}
		h.filterPatterns.include = append(h.filterPatterns.include, re)
	}

	// Compile exclude patterns
	for _, pattern := range h.link.Filter.ExcludePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid exclude pattern %s: %w", pattern, err)
		}
		h.filterPatterns.exclude = append(h.filterPatterns.exclude, re)
	}

	return nil
}

// shouldFilterMessage determines if a message should be filtered out
func (h *StreamHandler) shouldFilterMessage(key, value []byte, headers map[string][]byte, timestamp time.Time) bool {
	if h.link.Filter == nil || !h.link.Filter.Enabled {
		return false
	}

	// Check timestamp filters
	if !h.link.Filter.MinTimestamp.IsZero() && timestamp.Before(h.link.Filter.MinTimestamp) {
		return true
	}
	if !h.link.Filter.MaxTimestamp.IsZero() && timestamp.After(h.link.Filter.MaxTimestamp) {
		return true
	}

	// Check include patterns (if any)
	if len(h.filterPatterns.include) > 0 {
		matched := false
		valueStr := string(value)
		for _, re := range h.filterPatterns.include {
			if re.MatchString(valueStr) {
				matched = true
				break
			}
		}
		if !matched {
			return true
		}
	}

	// Check exclude patterns
	if len(h.filterPatterns.exclude) > 0 {
		valueStr := string(value)
		for _, re := range h.filterPatterns.exclude {
			if re.MatchString(valueStr) {
				return true
			}
		}
	}

	// Check header filters
	if len(h.link.Filter.FilterByHeader) > 0 {
		for key, expectedValue := range h.link.Filter.FilterByHeader {
			headerValue, exists := headers[key]
			if !exists || string(headerValue) != expectedValue {
				return true
			}
		}
	}

	return false
}
