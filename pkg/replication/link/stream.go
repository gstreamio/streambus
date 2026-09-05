package link

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sync"
	"time"

	"github.com/gstreamio/streambus/pkg/client"
	"github.com/gstreamio/streambus/pkg/protocol"
)

// ErrTransformExpressionsNotImplemented is returned when a link's Transform
// config sets KeyTransform or ValueTransform. Neither this package nor
// anywhere else in this codebase implements an expression language for
// these fields; silently ignoring them would replicate messages the
// operator explicitly asked to have their key or value rewritten,
// unrewritten. This fails loudly instead.
var ErrTransformExpressionsNotImplemented = errors.New("replication link: Transform.KeyTransform and Transform.ValueTransform are not implemented; only header transforms (HeaderTransforms, AddHeaders, RemoveHeaders) are supported")

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

	// startupIssues holds problems discovered once, at Start (such as a
	// target topic with fewer partitions than the source). performHealthCheck
	// re-seeds h.health.Issues with these on every run, since it otherwise
	// resets Issues to nil each cycle and a startup-time problem does not go
	// away on its own. Written only during Start, before healthCheckLoop
	// starts reading it, so it needs no lock of its own.
	startupIssues []string

	// dataPlaneConfirmed is set once a real fetch against the source has
	// succeeded - proof this link can actually move data, not just that it
	// connected. Guarded by statsMu; performHealthCheck will not report
	// "healthy" until this is true, however clean Issues/Warnings look.
	dataPlaneConfirmed bool
}

// partitionWorker handles replication for a single partition. Its mutable
// fields (sourceOffset, targetOffset, errors, pendingMappings) are owned by
// the single goroutine running run() - the only synchronization they need
// is ctx/cancel to stop that goroutine.
type partitionWorker struct {
	topic     string
	partition int32
	handler   *StreamHandler

	// targetTopic is the topic this partition replicates into - the source
	// topic name with TopicPrefix/TopicConfig applied. Resolved once, at
	// creation, rather than recomputed on every batch.
	targetTopic string

	ctx    context.Context
	cancel context.CancelFunc

	// groupID identifies this worker to the target cluster's consumer-group
	// offset store (see replicationGroupID): the (group, topic, partition)
	// key under which commitBatch records this worker's source progress,
	// atomically with the records that progress corresponds to.
	//
	// producer is this worker's own transactional producer against the
	// target cluster, holding its own transactional id (see
	// replicationTransactionID) - never shared with another worker, so no
	// two worker goroutines ever contend over the same transaction.
	groupID  string
	producer *client.TransactionalProducer

	// Current offsets. sourceOffset is authoritative recovery state: it is
	// exactly what was last committed to the target cluster inside the same
	// transaction as the records produced for it (see commitBatch and
	// recoverSourceOffset) - not merely a local field that happens to track
	// progress. targetOffset, by contrast, is an inferred, best-effort
	// estimate kept only for metrics and the local checkpoint
	// (recordBatchReplicated, saveCheckpoint): SendMessagesToPartition never
	// reports the offsets it actually assigned on the target, so this
	// number must never be treated as ground truth or used to make a
	// resume decision - only sourceOffset is used for that.
	sourceOffset int64
	targetOffset int64

	errors int64

	// pendingMappings accumulates source->target offset translations for
	// messages produced since the last flush. It is written out alongside
	// the periodic/final checkpoint in saveCheckpoint rather than on every
	// batch, to keep offset-mapping persistence off the hot path. Like
	// targetOffset above, this is observability only, kept for the
	// failover/failback event data manager.go already builds from it - it is
	// not part of how this link resumes after a restart.
	pendingMappings map[int64]int64
}

// startOffsetForNewLink is where a partition worker begins replicating when
// the target cluster has no offset committed for it yet (see
// recoverSourceOffset): a brand-new link, or a groupID that has genuinely
// never had a transaction commit under it. This mirrors the topic from its
// beginning - a deliberate choice, not the int64 zero value silently making
// it for us. A link that instead wants to start from the current tail on
// first run would need a different, explicit policy; this package does not
// offer one.
const startOffsetForNewLink int64 = 0

// replicationGroupID returns the identifier a partition worker uses to
// record its consumed source position in the target cluster's consumer-group
// offsets, via SendOffsetsToTransaction and FetchOffsets (see commitBatch and
// recoverSourceOffset).
//
// Nothing ever joins this "group" in the ordinary consumer sense - it exists
// purely as the (group, topic, partition) key the target's group coordinator
// uses to store one committed number on this link's behalf - but it still
// must be unique per link and partition, so two links, or two partitions of
// the same link, can never overwrite each other's recorded position.
func replicationGroupID(linkID, topic string, partition int32) string {
	return fmt.Sprintf("__replication__%s__%s__%d", linkID, topic, partition)
}

// replicationTransactionID returns the transactional id a partition worker
// claims from the target cluster's transaction coordinator, derived the same
// way as replicationGroupID and deliberately stable across restarts:
// reclaiming it after a crash bumps the producer epoch (see
// client.TransactionalProducer / InitProducerID), fencing off any earlier
// instance of this same worker that might still be alive somewhere, rather
// than letting two producers write under overlapping identities.
func replicationTransactionID(linkID, topic string, partition int32) string {
	return fmt.Sprintf("replication-%s-%s-%d", linkID, topic, partition)
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

	if link.Transform != nil && link.Transform.Enabled &&
		(link.Transform.KeyTransform != "" || link.Transform.ValueTransform != "") {
		cancel()
		return nil, ErrTransformExpressionsNotImplemented
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

	// There is no handler-wide producer to configure here: each partition
	// worker owns its own client.TransactionalProducer (created in
	// startPartitionWorker), and CommitTransaction's flushMessages already
	// calls FlushAll before EndTxn - so "the broker actually has it, not
	// just enqueued" is guaranteed by the transactional path itself, not by
	// a batching setting this handler has to get right.

	// Get topics to replicate
	topics, err := h.getTopicsToReplicate()
	if err != nil {
		h.stopLocked()
		return fmt.Errorf("failed to get topics: %w", err)
	}

	// Start partition workers for each topic
	for _, topic := range topics {
		if err := h.startTopicReplication(topic); err != nil {
			h.stopLocked()
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

	// Not "healthy": Start has only proven the clusters are reachable, not
	// that this link can actually move data. performHealthCheck promotes
	// this to "healthy" once dataPlaneConfirmed is set by a real fetch
	// cycle - see recordDataPlaneConfirmed.
	h.setHealthStatus("unverified")

	return nil
}

// Stop stops the replication stream
func (h *StreamHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return nil
	}

	h.stopLocked()
	return nil
}

// stopLocked performs the actual shutdown; callers must already hold h.mu.
//
// Start's own failure paths call this directly rather than Stop(): Stop()
// re-acquires h.mu, which Start already holds, so calling it from inside
// Start would deadlock; and Stop()'s "not started" guard would skip cleanup
// entirely for a Start that failed partway through, leaking whatever clients
// and workers it had already brought up.
func (h *StreamHandler) stopLocked() {
	// Cancel the handler context, which cascades to every worker, then cancel
	// each worker explicitly so a worker that never started still releases
	// its context.
	h.cancel()
	for _, worker := range h.partitionWorkers {
		if worker.cancel != nil {
			worker.cancel()
		}
	}

	// Wait for all workers to finish. Each worker closes its own
	// TransactionalProducer as part of its own shutdown (see run's defer)
	// before this returns - aborting, through a fresh background context,
	// any transaction a failure left open - so no transaction is left
	// dangling against the target cluster by the time clients close below.
	h.wg.Wait()

	// Close clients last, after everything that produces or fetches through
	// them has already stopped. A close error here has no recovery path -
	// the stream is stopping either way - so it is dropped deliberately
	// rather than masking the caller's own result.
	if h.sourceClient != nil {
		_ = h.sourceClient.Close()
	}
	if h.targetClient != nil {
		_ = h.targetClient.Close()
	}

	h.started = false
	h.setHealthStatus("stopped")
}

// connectToCluster creates a client connection to a cluster
func (h *StreamHandler) connectToCluster(config *ClusterConfig) (*client.Client, error) {
	brokers := resolveBrokers(config)

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

	// Apply security configuration if present. buildClientSecurityConfig
	// returns nil when TLS is not enabled, which leaves clientConfig.Security
	// nil and the connection plaintext - the same behavior as before TLS
	// support existed for links that never asked for it.
	clientConfig.Security = buildClientSecurityConfig(config.Security)

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

// resolveBrokers returns the broker addresses configured for a cluster,
// falling back to BootstrapServers when Brokers is empty.
func resolveBrokers(config *ClusterConfig) []string {
	if len(config.Brokers) > 0 {
		return config.Brokers
	}
	if config.BootstrapServers != "" {
		return []string{config.BootstrapServers}
	}
	return nil
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

// startTopicReplication starts a worker for each partition of topic that
// this link can actually replicate: the source's real partition count, not
// the previous placeholder that assumed exactly one, clamped to whatever
// partitions the target topic actually has once ensureTargetTopic tries to
// create it with a matching count.
func (h *StreamHandler) startTopicReplication(topic string) error {
	if cfg, ok := h.link.TopicConfig[topic]; ok && !cfg.Enabled {
		return nil
	}

	targetTopic := h.targetTopicName(topic)

	sourcePartitions, err := h.sourcePartitionCount(topic)
	if err != nil {
		return fmt.Errorf("get source partition count for %s: %w", topic, err)
	}

	workerPartitions := h.ensureTargetTopic(topic, targetTopic, sourcePartitions)

	for partition := int32(0); partition < workerPartitions; partition++ {
		if err := h.startPartitionWorker(topic, targetTopic, partition); err != nil {
			return fmt.Errorf("start worker for %s/%d: %w", topic, partition, err)
		}
	}

	return nil
}

// targetTopicName resolves the topic a source topic replicates into: an
// explicit per-topic TargetTopic wins (Validate defaults it to the source
// topic name whenever a TopicConfig entry exists but leaves it blank),
// otherwise TopicPrefix is applied.
func (h *StreamHandler) targetTopicName(topic string) string {
	if cfg, ok := h.link.TopicConfig[topic]; ok && cfg.TargetTopic != "" {
		return cfg.TargetTopic
	}
	return h.link.TopicPrefix + topic
}

// sourcePartitionCount returns how many partitions topic has on the source
// cluster. A topic that does not exist there yet - a replication link is
// often created before the topic it will mirror - is not an error: it falls
// back to a single partition, the same assumption the broker itself makes
// when auto-creating a topic on first produce/fetch.
func (h *StreamHandler) sourcePartitionCount(topic string) (int32, error) {
	counts, err := h.sourceClient.TopicPartitionCounts(context.Background(), []string{topic})
	if err != nil {
		return 0, err
	}

	n := counts[topic]
	if n == 0 {
		return 1, nil
	}
	if n > math.MaxInt32 {
		n = math.MaxInt32
	}
	return int32(n), nil //nolint:gosec // bounded against MaxInt32 above
}

// ensureTargetTopic creates the target topic with the same partition count
// as the source, and returns how many of those partitions this link can
// actually replicate.
//
// Replication assumes a straight 1:1 partition mapping (source partition N
// -> target partition N), so if the target topic already exists with fewer
// partitions than the source - CreateTopic is idempotent when the counts
// already match, and errors otherwise - only the partitions present on both
// sides are replicated. The remainder is recorded as a standing health issue
// rather than failing the whole link over one topic.
func (h *StreamHandler) ensureTargetTopic(sourceTopic, targetTopic string, sourcePartitions int32) int32 {
	ctx := context.Background()

	if sourcePartitions < 0 {
		h.startupIssues = append(h.startupIssues,
			fmt.Sprintf("topic %s: invalid negative source partition count %d", sourceTopic, sourcePartitions))
		return 0
	}

	createErr := h.targetClient.CreateTopic(ctx, targetTopic, uint32(sourcePartitions), 1)
	if createErr == nil {
		return sourcePartitions
	}

	counts, countErr := h.targetClient.TopicPartitionCounts(ctx, []string{targetTopic})
	rawCount := counts[targetTopic]
	if rawCount > math.MaxInt32 {
		rawCount = math.MaxInt32
	}
	targetPartitions := int32(rawCount) //nolint:gosec // bounded against MaxInt32 above
	if countErr != nil || targetPartitions == 0 {
		// Could not create the topic, and cannot find out what it already
		// has: nothing can be replicated into it, but that must not take
		// every other topic on this link down too.
		h.startupIssues = append(h.startupIssues,
			fmt.Sprintf("target topic %s could not be created or inspected: %v", targetTopic, createErr))
		return 0
	}

	if targetPartitions < sourcePartitions {
		h.startupIssues = append(h.startupIssues, fmt.Sprintf(
			"topic %s: target %s has %d partitions vs source %d; only replicating the first %d",
			sourceTopic, targetTopic, targetPartitions, sourcePartitions, targetPartitions))
		return targetPartitions
	}

	return sourcePartitions
}

// startPartitionWorker creates and runs the worker for one partition,
// resuming from the source offset already committed for it on the target
// cluster (see recoverSourceOffset) - never from local checkpoint storage,
// which this design deliberately no longer trusts for resume (see
// partitionWorker's sourceOffset/targetOffset field comment).
func (h *StreamHandler) startPartitionWorker(topic, targetTopic string, partition int32) error {
	workerKey := fmt.Sprintf("%s-%d", topic, partition)
	groupID := replicationGroupID(h.link.ID, topic, partition)

	producer, err := client.NewTransactionalProducer(h.targetClient, client.TransactionalProducerConfig{
		TransactionID: replicationTransactionID(h.link.ID, topic, partition),
	})
	if err != nil {
		return fmt.Errorf("create transactional producer: %w", err)
	}

	worker := &partitionWorker{
		topic:       topic,
		targetTopic: targetTopic,
		partition:   partition,
		handler:     h,
		groupID:     groupID,
		producer:    producer,
	}
	worker.ctx, worker.cancel = context.WithCancel(h.ctx)

	sourceOffset, err := h.recoverSourceOffset(worker.ctx, topic, partition, groupID)
	if err != nil {
		worker.cancel()
		_ = producer.Close()
		return fmt.Errorf("recover resume offset from target: %w", err)
	}
	worker.sourceOffset = sourceOffset

	// The local checkpoint's TargetOffset, if one exists, seeds only the
	// best-effort target-offset estimate used for metrics and future
	// checkpoints (see the field comment) - it plays no part in deciding
	// where replication resumes; recoverSourceOffset alone does that.
	if h.checkpointStore != nil {
		if checkpoint, err := h.checkpointStore.LoadCheckpoint(h.link.ID, topic, partition); err == nil {
			worker.targetOffset = checkpoint.TargetOffset
		}
	}

	h.partitionWorkers[workerKey] = worker

	h.wg.Add(1)
	go worker.run()
	return nil
}

// recoverSourceOffset resumes a partition worker's position from the offset
// committed to groupID on the target cluster - the same offset commitBatch
// records, inside the same transaction as the records that offset
// corresponds to (via SendOffsetsToTransaction). That shared transaction is
// what makes this exactly-once: the produced records and the consumed
// position either both took effect or neither did, so recovering from
// whatever the target reports is always consistent with what the target
// actually has. Recovering from a local checkpoint instead could be ahead of
// that (if a periodic checkpoint save raced a crash) or behind it - either
// one reopens the gap this design exists to close - which is why this
// package no longer does that for sourceOffset.
//
// An OffsetNoCommittedValue result means no transaction has ever committed
// under groupID - a fresh link, or a fresh partition - and resumes from
// startOffsetForNewLink instead.
func (h *StreamHandler) recoverSourceOffset(ctx context.Context, topic string, partition int32, groupID string) (int64, error) {
	resp, err := h.targetClient.FetchOffsets(ctx, &protocol.OffsetFetchRequest{
		GroupID: groupID,
		Topics:  []protocol.OffsetFetchTopic{{Topic: topic, Partitions: []int32{partition}}},
	})
	if err != nil {
		return 0, fmt.Errorf("fetch committed offset from target: %w", err)
	}

	for _, t := range resp.Topics {
		if t.Topic != topic {
			continue
		}
		for _, p := range t.Partitions {
			if p.Partition != partition {
				continue
			}
			if p.ErrorCode != protocol.ErrNone {
				return 0, fmt.Errorf("target reported error fetching committed offset: %s", p.ErrorCode)
			}
			if p.Offset == protocol.OffsetNoCommittedValue {
				return startOffsetForNewLink, nil
			}
			return p.Offset, nil
		}
	}

	// The target answered but said nothing about this exact topic/partition.
	// FetchOffsets responses are only obligated to cover what was asked for,
	// so treat that the same as "no committed offset" rather than as an
	// error.
	return startOffsetForNewLink, nil
}

// run is the main loop for a partition worker
func (w *partitionWorker) run() {
	defer w.handler.wg.Done()
	// Release the worker's context regardless of why the loop exits.
	// Without this, each worker's child context stays registered on the
	// handler's context for the handler's whole lifetime, which leaks for a
	// handler that starts replication for topics repeatedly.
	defer w.cancel()
	// Close this worker's transactional producer before wg.Done() lets Stop()
	// return. If a failure left a transaction open against the target (most
	// likely because the abort attempt in commitBatch itself raced this same
	// shutdown), Close aborts it through a fresh background context - see
	// client.TransactionalProducer.Close - so Stop() never returns with a
	// transaction still hanging open on the target cluster.
	defer w.closeProducer()

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

				// Backoff on error, but wake up immediately on shutdown
				// rather than blocking Stop() for the rest of the backoff.
				w.waitForNextPoll()
				continue
			}

			w.handler.recordReplicationSuccess()
		}
	}
}

// replicateBatch fetches one round of messages from the source partition and
// produces the surviving ones (after filtering and transformation) to the
// target. It returns nil for an empty fetch - catching up to an idle source
// partition is the normal steady state, not a failure.
//
// When there is anything to produce, commitBatch writes it to the target and
// records this worker's new source position with it, both inside one
// transaction (see commitBatch) - so this link's own recovery decision (see
// recoverSourceOffset) never observes the position without the records
// having committed, or vice versa: a crash before EndTxn resolves leaves no
// committed offset to resume past, so the batch is retried in full, not
// half-replayed. That is what makes recovery exactly-once rather than
// at-least-once. Consumers reading the target MUST use read_committed
// isolation for that to reach them: a read_uncommitted consumer can observe
// a batch that later aborts, or one whose transaction has not resolved yet
// at all. See commitBatch's doc comment for the one narrower case (a marker
// write failing partway through EndTxn) this package still cannot recover
// from on its own.
//
// A batch left with nothing to produce (every message filtered out) still
// needs no transaction: nothing lands on the target for it, so replaying it
// after a restart is harmless (the same messages get filtered again), and
// sourceOffset simply advances in memory - saveCheckpoint's local checkpoint
// of that advance is an observability aid only (see its own comment), not
// something recovery depends on.
func (w *partitionWorker) replicateBatch() error {
	fetched, err := w.fetchFromSource()
	if err != nil {
		return fmt.Errorf("fetch from source %s/%d: %w", w.topic, w.partition, err)
	}

	// A real fetch just completed, whether or not it returned anything - the
	// pipeline works, even if there is nothing to move right now.
	w.handler.recordDataPlaneConfirmed()

	if len(fetched.Messages) == 0 {
		w.waitForNextPoll()
		return nil
	}

	toProduce, bytes := w.prepareMessages(fetched.Messages)
	if len(toProduce) > 0 {
		if err := w.commitBatch(toProduce, fetched.NextOffset); err != nil {
			return fmt.Errorf("commit batch to target %s/%d: %w", w.targetTopic, w.partition, err)
		}

		startTarget := w.targetOffset
		w.recordOffsetMappings(toProduce, startTarget)
		w.targetOffset = startTarget + int64(len(toProduce))
		w.handler.recordBatchReplicated(w.topic, w.partition, len(toProduce), bytes,
			w.sourceOffset, w.targetOffset, toProduce[len(toProduce)-1].Timestamp)
	}

	// Advance past this fetch on the source side regardless of how many
	// messages survived filtering - a filtered-out message still consumed
	// its offset and must not be re-fetched next time.
	w.sourceOffset = fetched.NextOffset
	return nil
}

// fetchFromSource requests the next batch starting at this worker's current
// source offset, bounded by the link's configured MaxBytes.
func (w *partitionWorker) fetchFromSource() (*client.FetchResponse, error) {
	maxBytes := w.handler.link.Config.MaxBytes
	if maxBytes > math.MaxInt32 {
		maxBytes = math.MaxInt32
	}

	return w.handler.sourceClient.Fetch(w.ctx, &client.FetchRequest{
		Topic:     w.topic,
		Partition: w.partition,
		Offset:    w.sourceOffset,
		MaxBytes:  int32(maxBytes), //nolint:gosec // bounded against MaxInt32 above
	})
}

// prepareMessages applies the link's filter and header-transform rules to a
// fetched batch, returning only the messages that should reach the target
// and their total key+value size.
func (w *partitionWorker) prepareMessages(fetched []protocol.Message) ([]protocol.Message, int64) {
	out := make([]protocol.Message, 0, len(fetched))
	var bytes int64

	for _, msg := range fetched {
		if w.handler.shouldFilterMessage(msg.Key, msg.Value, msg.Headers, time.Unix(0, msg.Timestamp)) {
			continue
		}
		transformed := w.handler.applyTransform(msg)
		out = append(out, transformed)
		bytes += int64(len(transformed.Key) + len(transformed.Value))
	}

	return out, bytes
}

// commitBatch produces messages to the target partition (at the same
// partition index as the source - see ensureTargetTopic: no hash-based
// repartitioning, a straight 1:1 mapping) and records nextSourceOffset as
// this worker's consumed position, both inside one transaction against the
// target cluster.
//
// Either both take effect - the records land on the target and the position
// advances - or neither does, because CommitTransaction resolves them
// together. A failure at any point aborts whatever was staged so far (see
// abortAndReturn), and recoverSourceOffset is what a restart reads back to
// decide where this worker resumes - never nextSourceOffset having been
// applied locally, since it might not have committed at all.
//
// CommitTransaction writes the batch to the target log and then calls EndTxn
// to write the resolving marker, as two separate round trips inside one call
// (client.TransactionalProducer.CommitTransaction / flushMessages). A crash
// in the gap between them leaves those exact records durably on the target
// log with no marker ever written for them yet. That gap is invisible to a
// read_committed fetch: flushMessages tags its writes with this
// transaction's real producer id/epoch (client.newTransactionalInternalProducer),
// so Partition.BeginTransaction on the broker registers them as an open
// transaction, and a read_committed fetch gates on the marker exactly as
// commitBatch's callers depend on. (This did not always hold - flushMessages
// used to write through a plain, untagged client.Producer, which put
// everything under producer id 0, the broker's "not transactional" sentinel,
// making every such write visible under every isolation level the instant
// it landed. Fixed alongside this package; see
// pkg/client/transactional_producer.go and pkg/client/producer.go.)
//
// One narrower caveat remains, outside this package: if EndTxn's own marker
// write fails partway - the target partition briefly unreachable during
// commit, say, not merely a slow or dropped response - the coordinator
// leaves the transaction in an intermediate prepare state that neither a
// client retry, the coordinator's own expiry sweep, nor reclaiming a new
// producer epoch on restart currently resolves (see
// pkg/transaction/coordinator.go's EndTxn/checkExpiredTransactions/
// InitProducerID). This worker's replicationTransactionID is deterministic
// and never changes, so that specific failure wedges this partition's
// commitBatch calls indefinitely rather than self-healing on retry or
// restart. An ordinary crash-and-restart, at any other point, recovers
// cleanly - recoverSourceOffset simply resumes the retried batch from the
// last offset the target actually has.
func (w *partitionWorker) commitBatch(toProduce []protocol.Message, nextSourceOffset int64) error {
	ctx := w.ctx

	if err := w.producer.BeginTransaction(ctx); err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := w.sendToTarget(ctx, toProduce); err != nil {
		return w.abortAndReturn(ctx, fmt.Errorf("produce to target %s/%d: %w", w.targetTopic, w.partition, err))
	}

	offsets := map[string]map[int32]int64{w.topic: {w.partition: nextSourceOffset}}
	if err := w.producer.SendOffsetsToTransaction(ctx, w.groupID, offsets); err != nil {
		return w.abortAndReturn(ctx, fmt.Errorf("record source offset: %w", err))
	}

	if err := w.producer.CommitTransaction(ctx); err != nil {
		// CommitTransaction documents this outcome as genuinely unknown when
		// EndTxn itself fails - the records may already be flushed. Aborting
		// on top of an uncertain commit could report failure for a
		// transaction that actually landed, so this is surfaced as-is rather
		// than compounded with an abort attempt; BeginTransaction on the next
		// batch fails loudly if a transaction is somehow still open.
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// sendToTarget stages messages in the current transaction, one at a time -
// TransactionalProducer.Send has no batch form - addressed to this worker's
// target partition.
func (w *partitionWorker) sendToTarget(ctx context.Context, messages []protocol.Message) error {
	if w.partition < 0 {
		return fmt.Errorf("invalid negative partition %d for topic %s", w.partition, w.targetTopic)
	}
	for _, msg := range messages {
		if err := w.producer.Send(ctx, w.targetTopic, w.partition, msg); err != nil {
			return err
		}
	}
	return nil
}

// abortAndReturn aborts the worker's current transaction and returns cause -
// what actually failed the batch - folding in the abort's own error only
// when it adds information, so a caller never loses the original failure
// behind a secondary one.
func (w *partitionWorker) abortAndReturn(ctx context.Context, cause error) error {
	if err := w.producer.AbortTransaction(ctx); err != nil {
		return fmt.Errorf("%w (and abort failed: %v)", cause, err) //nolint:errorlint // cause is already wrapped; err is reported for its message, not to be matched on
	}
	return cause
}

// closeProducer releases this worker's transactional producer. See run's
// defer for why every worker calls this on the way out, regardless of how
// the loop exited.
func (w *partitionWorker) closeProducer() {
	if w.producer == nil {
		return
	}
	if err := w.producer.Close(); err != nil {
		w.errors++
	}
}

// waitForNextPoll backs off before the next fetch of an idle partition,
// waking up immediately if the worker is stopped instead of blocking Stop()
// for the rest of the wait.
func (w *partitionWorker) waitForNextPoll() {
	select {
	case <-w.ctx.Done():
	case <-time.After(w.handler.fetchBackoff()):
	}
}

// fetchBackoff returns how long a worker should wait before polling an idle
// partition again. FetchWaitMaxMs is optional cluster tuning - Validate does
// not require it - so a link that leaves it unset gets a conservative
// default rather than a zero-duration hot loop.
func (h *StreamHandler) fetchBackoff() time.Duration {
	ms := h.link.Config.FetchWaitMaxMs
	if ms <= 0 {
		ms = 500
	}
	return time.Duration(ms) * time.Millisecond
}

// recordOffsetMappings queues source->target offset translations for a
// produced batch; flushOffsetMappings persists them. This is observability
// only - the failover/failback event data manager.go builds from
// OffsetMapping - and is not part of how this link recovers after a restart
// (see recoverSourceOffset).
//
// Target offsets are inferred, not read back from the broker:
// SendMessagesToPartition does not return the offsets it assigned, and
// committing a transaction does not change that (see
// TransactionalProducer.flushMessages). This assumes the worker is the
// exclusive writer to the target partition - the normal replication
// topology, since a mirrored topic is meant to be read-only at the target -
// so offsets are assigned contiguously starting at startTarget. A producer
// other than this link writing to the target partition concurrently would
// make this mapping wrong; it would not, however, make replication itself
// any less exactly-once, since nothing here depends on it being right.
func (w *partitionWorker) recordOffsetMappings(produced []protocol.Message, startTarget int64) {
	if w.pendingMappings == nil {
		w.pendingMappings = make(map[int64]int64, len(produced))
	}
	target := startTarget
	for _, msg := range produced {
		w.pendingMappings[msg.Offset] = target
		target++
	}
}

// flushOffsetMappings persists pendingMappings, merging them into whatever
// mapping is already stored rather than overwriting it.
func (w *partitionWorker) flushOffsetMappings() {
	if len(w.pendingMappings) == 0 {
		return
	}

	store := w.handler.checkpointStore
	mapping, err := store.LoadOffsetMapping(w.handler.link.ID, w.topic, w.partition)
	if err != nil || mapping == nil {
		mapping = &OffsetMapping{
			LinkID:    w.handler.link.ID,
			Topic:     w.topic,
			Partition: w.partition,
			Mappings:  make(map[int64]int64, len(w.pendingMappings)),
		}
	}
	for src, tgt := range w.pendingMappings {
		mapping.Mappings[src] = tgt
	}
	mapping.LastUpdated = time.Now()

	if err := store.SaveOffsetMapping(mapping); err != nil {
		w.errors++
		return
	}
	w.pendingMappings = nil
}

// saveCheckpoint saves the current offset checkpoint and flushes any
// pending offset mappings alongside it. This is an observability aid -
// something to inspect from outside the link, and what manager.go's
// GetCheckpoint/failover event data reads - not this link's recovery
// mechanism: a restart resumes from recoverSourceOffset's read of the target
// cluster's committed offset, not from what is saved here.
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
	} else {
		w.handler.recordCheckpointSaved(checkpoint.Timestamp)
	}

	w.flushOffsetMappings()
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
	// Issues resets every cycle, but a problem found once at Start (see
	// ensureTargetTopic) does not go away on its own - re-seed it here
	// instead of only reporting it for the one cycle right after startup.
	h.health.Issues = append([]string(nil), h.startupIssues...)
	h.health.Warnings = nil

	h.health.SourceClusterReachable = h.checkReachable(h.sourceClient, &h.link.SourceCluster, "source")
	h.health.TargetClusterReachable = h.checkReachable(h.targetClient, &h.link.TargetCluster, "target")

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

	// Determine overall health status. dataPlaneConfirmed takes priority over
	// Warnings but not Issues: a real problem is a real problem regardless of
	// whether replication has ever run, but absent one, "no data has moved
	// yet" must not be reported as merely a warning-level "degraded" -
	// that's still one step short of the honest "healthy" claim.
	switch {
	case len(h.health.Issues) > 0:
		h.health.Status = "unhealthy"
	case !h.dataPlaneConfirmed:
		h.health.Status = "unverified"
	case len(h.health.Warnings) > 0:
		h.health.Status = "degraded"
	default:
		h.health.Status = "healthy"
	}
}

// checkReachable pings a cluster and records an Issue when it cannot be
// reached, replacing what used to be an unconditional true regardless of
// whether the cluster was actually reachable. Callers must hold statsMu (it
// appends to h.health.Issues). A nil client means Start has not run yet -
// not itself something to report as an issue.
func (h *StreamHandler) checkReachable(c *client.Client, cfg *ClusterConfig, label string) bool {
	if c == nil {
		return false
	}
	if err := verifyClusterReachable(c, resolveBrokers(cfg), cfg.ConnectionTimeout); err != nil {
		h.health.Issues = append(h.health.Issues, fmt.Sprintf("%s cluster unreachable: %v", label, err))
		return false
	}
	return true
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

// recordCheckpointSaved records that a checkpoint was just written, so
// performHealthCheck's "no checkpoint in 5 minutes" check reflects reality.
// Before this, h.metrics.LastCheckpoint never left its zero value even
// though saveCheckpoint was writing checkpoints on schedule, which tripped
// that check permanently regardless of the truth.
func (h *StreamHandler) recordCheckpointSaved(at time.Time) {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	h.metrics.LastCheckpoint = at
}

// recordDataPlaneConfirmed marks that a real fetch against the source has
// succeeded at least once. See the dataPlaneConfirmed field comment.
func (h *StreamHandler) recordDataPlaneConfirmed() {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()
	h.dataPlaneConfirmed = true
}

// recordBatchReplicated updates aggregate and per-partition metrics after a
// batch is durably produced to the target.
func (h *StreamHandler) recordBatchReplicated(
	topic string, partition int32, count int, bytes int64,
	sourceOffset, targetOffset, lastMessageTimestampNanos int64,
) {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()

	now := time.Now()
	h.metrics.TotalMessagesReplicated += int64(count)
	h.metrics.TotalBytesReplicated += bytes
	h.metrics.LastSuccessfulReplication = now

	// End-to-end lag: how long ago the last message actually replicated was
	// originally produced at the source, not an offset-count difference -
	// source and target offsets are different, unrelated sequences.
	lagMs := now.UnixNano()/int64(time.Millisecond) - lastMessageTimestampNanos/int64(time.Millisecond)
	if lagMs < 0 {
		lagMs = 0
	}
	h.metrics.ReplicationLag = lagMs
	if lagMs > h.metrics.MaxReplicationLag {
		h.metrics.MaxReplicationLag = lagMs
	}

	h.updatePartitionMetrics(topic, partition, count, bytes, sourceOffset, targetOffset, lagMs, now)
}

// updatePartitionMetrics updates the per-partition metrics map. Called with
// statsMu already held by recordBatchReplicated.
func (h *StreamHandler) updatePartitionMetrics(
	topic string, partition int32, count int, bytes int64,
	sourceOffset, targetOffset, lagMs int64, at time.Time,
) {
	if h.metrics.PartitionMetrics == nil {
		h.metrics.PartitionMetrics = make(map[string]*PartitionReplicationMetrics)
	}
	key := fmt.Sprintf("%s-%d", topic, partition)
	pm := h.metrics.PartitionMetrics[key]
	if pm == nil {
		pm = &PartitionReplicationMetrics{Topic: topic, Partition: partition}
		h.metrics.PartitionMetrics[key] = pm
	}
	pm.SourceOffset = sourceOffset
	pm.TargetOffset = targetOffset
	pm.MessagesReplicated += int64(count)
	pm.BytesReplicated += bytes
	pm.LastReplicatedAt = at
	pm.Lag = lagMs
}

// ResetFailureStats clears failure counters and health issues, used when a
// link is (re)started.
func (h *StreamHandler) ResetFailureStats() {
	h.statsMu.Lock()
	defer h.statsMu.Unlock()

	h.metrics.ConsecutiveFailures = 0
	// Not "healthy": (re)starting a link does not itself prove it can move
	// data. performHealthCheck promotes this once the data plane confirms
	// that, the same as a fresh Start does.
	h.health.Status = "unverified"
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

// applyTransform applies the link's configured header transformations to a
// message. KeyTransform/ValueTransform have no expression engine anywhere in
// this codebase; NewStreamHandler refuses to construct a handler for a link
// that sets them, so this only ever has header rules to apply.
func (h *StreamHandler) applyTransform(msg protocol.Message) protocol.Message {
	if h.link.Transform == nil || !h.link.Transform.Enabled {
		return msg
	}

	t := h.link.Transform
	if len(msg.Headers) == 0 && len(t.AddHeaders) == 0 {
		return msg
	}

	headers := make(map[string][]byte, len(msg.Headers)+len(t.AddHeaders))
	for k, v := range msg.Headers {
		newKey := k
		if renamed, ok := t.HeaderTransforms[k]; ok {
			newKey = renamed
		}
		headers[newKey] = v
	}
	for _, k := range t.RemoveHeaders {
		delete(headers, k)
	}
	for k, v := range t.AddHeaders {
		headers[k] = []byte(v)
	}
	msg.Headers = headers

	return msg
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
