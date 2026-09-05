package link

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
)

// ErrTLSNotImplemented is returned when a replication link's cluster
// security config requests EnableTLS but this package does not yet apply any
// TLS settings to the underlying client connection. Failing loudly here
// avoids silently replicating data across cluster boundaries in plaintext
// when an operator explicitly configured encryption.
var ErrTLSNotImplemented = errors.New("replication link: EnableTLS is set but TLS is not yet implemented for cross-cluster replication connections")

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

	// statsMu guards metrics and health.
	//
	// These are deliberately not covered by mu: Stop holds mu while waiting
	// for the worker, health-check and metrics goroutines to exit, so any of
	// those taking mu would deadlock against a concurrent Stop. They are also
	// the same objects the link manager exposes through GetMetrics/GetHealth,
	// which runs under the manager's own lock - so a single lock dedicated to
	// this state is what keeps the two sides from racing.
	//
	// Lock order is always mu then statsMu; never the reverse.
	statsMu sync.Mutex

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
	wg     sync.WaitGroup //nolint:unused // Used to track partition worker goroutines

	// Current offsets
	sourceOffset int64
	targetOffset int64

	// Metrics
	messagesReplicated int64 //nolint:unused // Reserved for future use in metrics collection
	bytesReplicated    int64 //nolint:unused // Reserved for future use in metrics collection
	errors             int64
	lastReplicatedAt   time.Time //nolint:unused // Reserved for tracking replication timestamp
}

// NewStreamHandler creates a new stream handler for a replication link
func NewStreamHandler(link *ReplicationLink, checkpointStore Storage) (*StreamHandler, error) {
	if err := link.Validate(); err != nil {
		return nil, fmt.Errorf("invalid link configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// The handler takes its own copies of metrics and health rather than
	// aliasing the link's. Sharing them made the link struct mutable from the
	// handler's worker goroutines, so anything the manager did with the link -
	// Clone, SaveLink, ListLinks - raced with replication. The manager reads
	// live values back through WithStats instead.
	handler := &StreamHandler{
		link:             link,
		partitionWorkers: make(map[string]*partitionWorker),
		metrics:          cloneMetrics(link.Metrics),
		health:           initialHealth(link.Health),
		checkpointStore:  checkpointStore,
		ctx:              ctx,
		cancel:           cancel,
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
		_ = h.sourceClient.Close() // failing to close a client we are abandoning changes nothing
		return fmt.Errorf("failed to connect to target cluster: %w", err)
	}
	h.targetClient = targetClient

	// Get topics to replicate
	topics, err := h.getTopicsToReplicate()
	if err != nil {
		_ = h.sourceClient.Close() // failing to close clients we are abandoning changes nothing
		_ = h.targetClient.Close()
		return fmt.Errorf("failed to get topics: %w", err)
	}

	// Start partition workers for each topic
	for _, topic := range topics {
		if err := h.startTopicReplication(topic); err != nil {
			_ = h.Stop()
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
	h.setHealthStatus("healthy")

	return nil
}

// Stop stops the replication stream
func (h *StreamHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return nil
	}

	// Cancel the handler context, which cascades to every worker, then cancel
	// each worker explicitly so a worker that never started still releases
	// its context.
	h.cancel()
	for _, worker := range h.partitionWorkers {
		if worker.cancel != nil {
			worker.cancel()
		}
	}

	// Wait for all workers to finish
	h.wg.Wait()

	// Close clients. A close error here has no recovery path - the stream is
	// stopping either way - so it is dropped deliberately rather than
	// masking the caller's own result.
	if h.sourceClient != nil {
		_ = h.sourceClient.Close()
	}
	if h.targetClient != nil {
		_ = h.targetClient.Close()
	}

	h.started = false
	h.setHealthStatus("stopped")

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

	// Start from the client defaults and override only what the cluster
	// config actually specifies. Building a bare client.Config here would
	// leave required fields such as MaxConnectionsPerBroker at zero, which
	// client.New rejects - making every StartLink fail before it connected.
	clientConfig := client.DefaultConfig()
	clientConfig.Brokers = brokers

	if config.ConnectionTimeout > 0 {
		clientConfig.ConnectTimeout = config.ConnectionTimeout
	}
	if config.RequestTimeout > 0 {
		clientConfig.RequestTimeout = config.RequestTimeout
	}
	if config.RetryBackoff > 0 {
		clientConfig.RetryBackoff = config.RetryBackoff
	}
	if config.MaxRetries > 0 {
		clientConfig.MaxRetries = config.MaxRetries
	}

	// Apply security configuration if present.
	// TODO: Configure TLS settings when Security.EnableTLS is true.
	// Until that's implemented, refuse to silently fall back to a plaintext
	// connection when TLS was explicitly requested.
	if config.Security != nil && config.Security.EnableTLS {
		return nil, ErrTLSNotImplemented
	}

	// Create and connect client
	c, err := client.New(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// client.New does not dial, so verify the cluster is actually reachable
	// before reporting the link as connected. Without this a link starts
	// "active" and healthy against brokers that do not exist, and silently
	// replicates nothing.
	if err := verifyClusterReachable(c, brokers, clientConfig.ConnectTimeout); err != nil {
		_ = c.Close() // the connection is being discarded; a close error is moot
		return nil, err
	}

	return c, nil
}

// verifyClusterReachable health-checks the cluster's brokers, succeeding as
// soon as one responds. A cluster is usable if any of its brokers answers;
// requiring all of them would make a single down broker fail the whole link.
func verifyClusterReachable(c *client.Client, brokers []string, timeout time.Duration) error {
	if len(brokers) == 0 {
		return fmt.Errorf("no brokers configured")
	}

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var lastErr error
	for _, broker := range brokers {
		if err := c.HealthCheck(ctx, broker); err != nil {
			lastErr = err
			continue
		}
		return nil
	}

	return fmt.Errorf("no reachable broker among %v: %w", brokers, lastErr)
}

// getTopicsToReplicate returns the list of topics to replicate
func (h *StreamHandler) getTopicsToReplicate() ([]string, error) {
	// If specific topics are configured, use those
	if len(h.link.Topics) > 0 {
		return h.link.Topics, nil
	}

	// Otherwise, list all topics from source cluster.
	// Start() has no caller-supplied context to thread through here, so use
	// a background context - consistent with Start() itself taking none.
	topics, err := h.sourceClient.ListTopics(context.Background())
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
	// Release the worker's context regardless of why the loop exits.
	// Without this, each worker's child context stays registered on the
	// handler's context for the handler's whole lifetime, which leaks for a
	// handler that starts replication for topics repeatedly.
	defer w.cancel()

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
				w.handler.recordReplicationFailure()

				// Backoff on error
				time.Sleep(time.Duration(w.handler.link.Config.FetchWaitMaxMs) * time.Millisecond)
				continue
			}

			w.handler.recordReplicationSuccess()
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
	h.statsMu.Lock()
	defer h.statsMu.Unlock()

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

	h.statsMu.Lock()
	lastMessages := h.metrics.TotalMessagesReplicated
	lastBytes := h.metrics.TotalBytesReplicated
	lastErrors := h.metrics.TotalErrors
	h.statsMu.Unlock()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(lastUpdate).Seconds()

			h.statsMu.Lock()

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

			h.statsMu.Unlock()
		}
	}
}

// initialHealth returns the handler's starting health. A link that has never
// run carries no health of its own, and a handler that has been constructed
// but not started is initializing rather than of unknown health.
func initialHealth(source *ReplicationHealth) *ReplicationHealth {
	if source == nil {
		return &ReplicationHealth{Status: "initializing"}
	}
	return cloneHealth(source)
}

// setHealthStatus records the stream's overall health status.
func (h *StreamHandler) setHealthStatus(status string) {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	h.health.Status = status
}

// recordReplicationFailure counts a failed replication batch and marks the
// stream unhealthy once failures pass the link's tolerance.
func (h *StreamHandler) recordReplicationFailure() {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()

	h.metrics.TotalErrors++
	h.metrics.ConsecutiveFailures++

	maxFailures := 0
	if h.link.FailoverConfig != nil {
		maxFailures = int(h.link.FailoverConfig.MaxConsecutiveFailures)
	}
	if maxFailures > 0 && h.metrics.ConsecutiveFailures > maxFailures {
		h.health.Status = "unhealthy"
		h.health.Issues = append(h.health.Issues,
			fmt.Sprintf("Too many consecutive failures: %d", h.metrics.ConsecutiveFailures))
	}
}

// recordReplicationSuccess clears the consecutive-failure counter.
func (h *StreamHandler) recordReplicationSuccess() {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	h.metrics.ConsecutiveFailures = 0
}

// ResetFailureStats clears failure counters and health issues, used when a
// link is (re)started.
func (h *StreamHandler) ResetFailureStats() {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()

	h.metrics.ConsecutiveFailures = 0
	h.health.Status = "healthy"
	h.health.Issues = nil
	h.health.Warnings = nil
}

// WithStats runs fn with exclusive access to the stream's metrics and health.
//
// The link manager holds the same pointers and must read them under this lock
// while the stream is running, otherwise a snapshot races with the worker and
// health-check goroutines updating them.
func (h *StreamHandler) WithStats(fn func(metrics *ReplicationMetrics, health *ReplicationHealth)) {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	fn(h.metrics, h.health)
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
//
//nolint:unused // Reserved for future use when message filtering is fully implemented
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
