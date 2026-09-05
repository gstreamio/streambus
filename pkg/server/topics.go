package server

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gstreamio/streambus/pkg/logger"
	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/storage"
	"go.uber.org/zap"
)

// TopicManager manages topics and their partitions
type TopicManager struct {
	mu         sync.RWMutex
	topics     map[string]*Topic
	dataDir    string
	storageDir string
}

// Topic represents a topic with multiple partitions
type Topic struct {
	name       string
	partitions map[uint32]*Partition
	mu         sync.RWMutex
}

// Partition represents a single partition with its log
type Partition struct {
	id  uint32
	log storage.Log

	// txnMu protects openTxns and abortedTxns, which back LastStableOffset
	// and IsAborted respectively. It is its own lock rather than piggybacking
	// on the topic/topic-manager locks because it is written on every
	// transactional produce and read on every read-committed fetch - both far
	// hotter paths than topic management.
	txnMu    sync.Mutex
	openTxns map[producerKey]int64

	// abortedTxns holds the offset ranges of aborted transactions still
	// within the partition's retained log, so a read-committed fetch keeps
	// hiding their records even after LastStableOffset stops treating them as
	// a barrier (AbortTransaction, like EndTransaction, clears the openTxns
	// entry once the marker is durable). See IsAborted and pruneAbortedLocked
	// for how this stays bounded rather than growing for the partition's
	// entire lifetime.
	abortedTxns []abortedRange
}

// producerKey identifies one producer epoch for open-transaction tracking on
// a partition. The epoch is part of the key, not just the ID, so a fenced
// producer's stale entry (should it ever fail to reach EndTxn) does not get
// confused with - or torn down by - the next producer instance's transaction.
type producerKey struct {
	producerID    int64
	producerEpoch int16
}

// abortedRange is the offset span one aborted transaction occupied on a
// partition: [start, end). start is the transaction's first record (the
// value BeginTransaction recorded); end is its abort marker's own offset,
// which is always past every record the transaction wrote, since the marker
// is only written once the coordinator has heard the transaction is done.
//
// Producer identity is part of the range, not just the offsets, because
// other producers can interleave records into the same span - only records
// this producer epoch actually wrote should be hidden.
type abortedRange struct {
	producerID    int64
	producerEpoch int16
	start         int64
	end           int64
}

// NewTopicManager creates a new topic manager
func NewTopicManager(dataDir string) *TopicManager {
	storageDir := filepath.Join(dataDir, "topics")
	_ = os.MkdirAll(storageDir, 0750)

	tm := &TopicManager{
		topics:     make(map[string]*Topic),
		dataDir:    dataDir,
		storageDir: storageDir,
	}

	// Load existing topics from disk
	_ = tm.loadExistingTopics()

	return tm
}

// loadExistingTopics scans the storage directory for existing topics
func (tm *TopicManager) loadExistingTopics() error {
	logger.Debug("scanning storage directory", zap.String("dir", tm.storageDir))
	entries, err := os.ReadDir(tm.storageDir)
	if err != nil {
		// Directory might not exist on first run
		logger.Debug("storage directory doesn't exist or can't be read", zap.Error(err))
		return nil
	}

	logger.Debug("found entries in storage directory", zap.Int("count", len(entries)))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		topicName := entry.Name()
		topicDir := filepath.Join(tm.storageDir, topicName)
		logger.Debug("loading topic", zap.String("topic", topicName), zap.String("dir", topicDir))

		// Count partitions by looking for partition directories
		partitionEntries, err := os.ReadDir(topicDir)
		if err != nil {
			continue
		}

		topic := &Topic{
			name:       topicName,
			partitions: make(map[uint32]*Partition),
		}

		for _, partEntry := range partitionEntries {
			if !partEntry.IsDir() || !strings.HasPrefix(partEntry.Name(), "partition-") {
				continue
			}

			// Parse partition ID from directory name
			var partitionID uint32
			if _, err := fmt.Sscanf(partEntry.Name(), "partition-%d", &partitionID); err != nil {
				continue
			}

			// Load the partition with storage recovery
			partitionDir := filepath.Join(topicDir, partEntry.Name())
			logger.Debug("creating log for partition",
				zap.Uint32("partition", partitionID),
				zap.String("dir", partitionDir))
			config := storage.DefaultConfig()
			log, err := storage.NewLog(partitionDir, *config)
			if err != nil {
				logger.Warn("failed to create log for partition",
					zap.Uint32("partition", partitionID),
					zap.Error(err))
				continue
			}

			logger.Debug("successfully loaded partition", zap.Uint32("partition", partitionID))
			topic.partitions[partitionID] = newRecoveredPartition(topicName, partitionID, log)
		}

		if len(topic.partitions) > 0 {
			tm.topics[topicName] = topic
			logger.Info("registered topic",
				zap.String("topic", topicName),
				zap.Int("partitions", len(topic.partitions)))
		} else {
			logger.Debug("skipping topic - no partitions loaded", zap.String("topic", topicName))
		}
	}

	logger.Info("topic loading complete", zap.Int("totalTopics", len(tm.topics)))
	return nil
}

// CreateTopic creates a new topic with the specified number of partitions
func (tm *TopicManager) CreateTopic(name string, numPartitions uint32) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if existing, exists := tm.topics[name]; exists {
		// Check if partition count matches
		existing.mu.RLock()
		partitionCount := len(existing.partitions)
		existing.mu.RUnlock()

		// Safe conversion with bounds check
		if partitionCount < 0 || partitionCount > int(^uint32(0)) {
			return fmt.Errorf("invalid partition count: %d", partitionCount)
		}
		existingPartitions := uint32(partitionCount)

		if existingPartitions != numPartitions {
			return fmt.Errorf("topic %s already exists with %d partitions, cannot recreate with %d partitions",
				name, existingPartitions, numPartitions)
		}
		// Topic exists with same partition count - idempotent operation
		return nil
	}

	topic := &Topic{
		name:       name,
		partitions: make(map[uint32]*Partition),
	}

	// Create partitions
	for i := uint32(0); i < numPartitions; i++ {
		partition, err := tm.createPartition(name, i)
		if err != nil {
			// Clean up any created partitions
			for j := uint32(0); j < i; j++ {
				if p := topic.partitions[j]; p != nil {
					p.log.Close()
				}
			}
			return fmt.Errorf("failed to create partition %d: %w", i, err)
		}
		topic.partitions[i] = partition
	}

	tm.topics[name] = topic
	return nil
}

// DeleteTopic deletes a topic and all its partitions
func (tm *TopicManager) DeleteTopic(name string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	topic, exists := tm.topics[name]
	if !exists {
		return fmt.Errorf("topic %s does not exist", name)
	}

	// Close all partition logs
	topic.mu.Lock()
	for _, partition := range topic.partitions {
		if err := partition.log.Close(); err != nil {
			topic.mu.Unlock()
			return fmt.Errorf("failed to close partition %d: %w", partition.id, err)
		}
	}
	topic.mu.Unlock()

	// Remove topic directory
	topicDir := filepath.Join(tm.storageDir, name)
	if err := os.RemoveAll(topicDir); err != nil {
		return fmt.Errorf("failed to remove topic directory: %w", err)
	}

	delete(tm.topics, name)
	return nil
}

// GetPartition gets a partition for a topic
func (tm *TopicManager) GetPartition(topic string, partitionID uint32) (*Partition, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	t, exists := tm.topics[topic]
	if !exists {
		return nil, fmt.Errorf("topic %s does not exist", topic)
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	partition, exists := t.partitions[partitionID]
	if !exists {
		return nil, fmt.Errorf("partition %d does not exist in topic %s", partitionID, topic)
	}

	return partition, nil
}

// ListTopics returns a list of all topics
func (tm *TopicManager) ListTopics() []TopicInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	topics := make([]TopicInfo, 0, len(tm.topics))
	for name, topic := range tm.topics {
		topic.mu.RLock()
		partitionCount := len(topic.partitions)
		// Safe conversion - partition count should always be valid
		numPartitions := uint32(partitionCount)
		if partitionCount > int(^uint32(0)) {
			numPartitions = ^uint32(0) // Cap at max uint32 if overflow
		}
		topics = append(topics, TopicInfo{
			Name:          name,
			NumPartitions: numPartitions,
		})
		topic.mu.RUnlock()
	}

	return topics
}

// TopicExists checks if a topic exists
func (tm *TopicManager) TopicExists(name string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	_, exists := tm.topics[name]

	// Only log at debug level to avoid flooding
	logger.Debug("topic existence check",
		zap.String("topic", name),
		zap.Bool("exists", exists),
		zap.Int("totalTopics", len(tm.topics)))

	return exists
}

// createPartition creates a single partition
func (tm *TopicManager) createPartition(topic string, partitionID uint32) (*Partition, error) {
	// Create partition directory
	partitionDir := filepath.Join(tm.storageDir, topic, fmt.Sprintf("partition-%d", partitionID))
	if err := os.MkdirAll(partitionDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create partition directory: %w", err)
	}

	// Create log config
	config := storage.DefaultConfig()

	// Create new log for partition
	log, err := storage.NewLog(partitionDir, *config)
	if err != nil {
		return nil, fmt.Errorf("failed to create log: %w", err)
	}

	return newRecoveredPartition(topic, partitionID, log), nil
}

// newRecoveredPartition builds a Partition over an already-recovered log and
// replays its retained records to rebuild the in-memory transaction state
// that recovery itself does not carry: which transactions were still open,
// and which aborted ones still have records in range that read-committed
// fetches must keep hiding (see rebuildTransactionState).
//
// A replay failure does not stop the partition from loading - it falls back
// to empty transaction state, the same as before this rebuild existed. That
// is a deliberate fail-open: refusing to serve an otherwise-readable
// partition because its transaction history didn't replay cleanly would be
// a worse outcome than what this rebuild is trying to fix in the first
// place.
func newRecoveredPartition(topic string, partitionID uint32, log storage.Log) *Partition {
	partition := &Partition{id: partitionID, log: log}

	if err := partition.rebuildTransactionState(); err != nil {
		logger.Warn("failed to rebuild transaction state for partition",
			zap.String("topic", topic), zap.Uint32("partition", partitionID), zap.Error(err))
	}

	return partition
}

// Close closes all topic logs
func (tm *TopicManager) Close() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var lastErr error
	for _, topic := range tm.topics {
		topic.mu.Lock()
		for _, partition := range topic.partitions {
			if err := partition.log.Close(); err != nil {
				lastErr = err
			}
		}
		topic.mu.Unlock()
	}

	return lastErr
}

// TopicInfo holds information about a topic
type TopicInfo struct {
	Name          string
	NumPartitions uint32
}

// ID returns the partition ID.
func (p *Partition) ID() uint32 {
	return p.id
}

// Log returns the partition's underlying log.
func (p *Partition) Log() storage.Log {
	return p.log
}

// BeginTransaction records that a transactional produce landed on this
// partition, if this producer epoch does not already have one tracked.
//
// Only the first call for a given (producerID, producerEpoch) has any
// effect: it is the offset of the *first* record of an open transaction that
// matters for LastStableOffset, so later records from the same transaction
// must not move the barrier forward. producerID 0 is the sentinel for a
// non-transactional batch and is silently ignored, so ordinary produce
// traffic never touches this bookkeeping.
func (p *Partition) BeginTransaction(producerID int64, producerEpoch int16, firstOffset int64) {
	if producerID == 0 {
		return
	}

	key := producerKey{producerID: producerID, producerEpoch: producerEpoch}

	p.txnMu.Lock()
	defer p.txnMu.Unlock()
	if p.openTxns == nil {
		p.openTxns = make(map[producerKey]int64)
	}
	if _, tracked := p.openTxns[key]; !tracked {
		p.openTxns[key] = firstOffset
	}
}

// EndTransaction clears a producer epoch's open-transaction entry once its
// marker has been written, whether the transaction committed or aborted:
// either way it is resolved, and LastStableOffset must stop treating its
// start offset as a barrier. Clearing an epoch with nothing tracked is a
// no-op, not an error - that is the normal case for a non-transactional
// producer's marker-less path, and for any partition a transaction never
// actually produced to.
func (p *Partition) EndTransaction(producerID int64, producerEpoch int16) {
	key := producerKey{producerID: producerID, producerEpoch: producerEpoch}

	p.txnMu.Lock()
	defer p.txnMu.Unlock()
	delete(p.openTxns, key)
}

// AbortTransaction is EndTransaction for the abort case: it clears the
// producer epoch's open-transaction entry exactly as EndTransaction does, and
// additionally remembers the transaction's offset range so a read-committed
// fetch keeps hiding its records once this clears the LastStableOffset
// barrier (see IsAborted). markerOffset is the abort marker's own offset,
// which becomes the range's exclusive end.
//
// Aborting a producer epoch with nothing tracked is a no-op beyond pruning,
// matching EndTransaction - there is no start offset to remember a range
// from, and nothing on the log needs hiding for a transaction this partition
// never saw open.
func (p *Partition) AbortTransaction(producerID int64, producerEpoch int16, markerOffset int64) {
	key := producerKey{producerID: producerID, producerEpoch: producerEpoch}

	p.txnMu.Lock()
	defer p.txnMu.Unlock()

	if start, tracked := p.openTxns[key]; tracked {
		p.abortedTxns = append(p.abortedTxns, abortedRange{
			producerID:    producerID,
			producerEpoch: producerEpoch,
			start:         start,
			end:           markerOffset,
		})
	}
	delete(p.openTxns, key)
	p.pruneAbortedLocked()
}

// IsAborted reports whether the record at offset, written by the given
// producer, belongs to a transaction this partition knows aborted. Only a
// read-committed fetch needs this: an aborted marker already resolved the
// transaction and lifted the LastStableOffset barrier, so without this check
// its records would become visible under read-committed too, which is
// exactly the gap this method closes.
//
// producerID 0 (the non-transactional sentinel) always returns false without
// taking the lock, since no aborted range can ever be keyed by it - this
// keeps the common, non-transactional fetch path off txnMu entirely.
//
// This checks live state under the lock, which is correct for a single
// lookup but the wrong tool for checking every record in a fetch response:
// see abortedRanges for the snapshot-once alternative visibleMessages uses.
func (p *Partition) IsAborted(producerID int64, producerEpoch int16, offset int64) bool {
	if producerID == 0 {
		return false
	}

	p.txnMu.Lock()
	defer p.txnMu.Unlock()

	return isAbortedInRanges(p.abortedTxns, producerID, producerEpoch, offset)
}

// abortedRanges returns a snapshot of this partition's currently known
// aborted ranges (pruning first, exactly as LastStableOffset does), for a
// caller that needs to check many records against it. A read-committed
// fetch calls this once per fetch and checks every record in the response
// against the snapshot via isAbortedInRanges, rather than calling IsAborted
// - and re-acquiring txnMu, and re-scanning abortedTxns - once per record; a
// fetch returning a thousand records would otherwise pay a lock acquisition
// and linear scan a thousand times over for what is, from the fetch's
// perspective, one fact checked repeatedly.
//
// The snapshot can go stale if another abort resolves while this fetch is
// still being assembled, but that is no different from every other value a
// fetch reads once at the start and returns consistently throughout
// (HighWaterMark, LastStableOffset itself): a fetch response is already a
// snapshot of a single point in time, not a live view, and an abort landing
// mid-fetch is racy regardless of how IsAborted is checked.
func (p *Partition) abortedRanges() []abortedRange {
	p.txnMu.Lock()
	defer p.txnMu.Unlock()

	p.pruneAbortedLocked()

	ranges := make([]abortedRange, len(p.abortedTxns))
	copy(ranges, p.abortedTxns)
	return ranges
}

// isAbortedInRanges reports whether offset, written by (producerID,
// producerEpoch), falls in any of ranges. Shared by IsAborted (checking live
// state under txnMu) and visibleMessages (checking a snapshot from
// abortedRanges taken once per fetch, no locking per record).
func isAbortedInRanges(ranges []abortedRange, producerID int64, producerEpoch int16, offset int64) bool {
	if producerID == 0 {
		return false
	}
	for _, r := range ranges {
		if r.producerID == producerID && r.producerEpoch == producerEpoch &&
			offset >= r.start && offset < r.end {
			return true
		}
	}
	return false
}

// pruneAbortedLocked drops aborted ranges the log has already forgotten via
// retention. Once the log's start offset has advanced past a range's end,
// retention has deleted every record that range could ever have hidden, so
// the entry has nothing left to do - keeping it anyway would grow
// abortedTxns for the entire life of the partition, one entry per abort,
// which is exactly the unbounded memory growth this bound exists to avoid.
// Caller must hold txnMu.
func (p *Partition) pruneAbortedLocked() {
	logStart := int64(p.log.StartOffset())

	kept := p.abortedTxns[:0]
	for _, r := range p.abortedTxns {
		if r.end > logStart {
			kept = append(kept, r)
		}
	}
	p.abortedTxns = kept
}

// rebuildReplayWindowBytes bounds how much of a partition's retained log
// rebuildTransactionState reads into memory at once. Replaying a record only
// needs its offset, headers and producer identity, but Read - like
// ReadRange - hands back fully decoded messages, keys and values included.
// Without a bound, rebuilding a heavily-retained partition's state at
// startup would need memory proportional to everything that partition has
// ever retained rather than a small constant; a broker with several such
// partitions could exhaust memory before it ever finished coming up, on
// exactly the data it needs to read to come up at all. 1MB matches the
// default single-fetch size used elsewhere in this codebase (see
// client.Config.MaxFetchBytes) - there is nothing special about replay that
// calls for a different window.
const rebuildReplayWindowBytes = 1024 * 1024

// rebuildTransactionState replays this partition's retained log to
// reconstruct openTxns and abortedTxns from scratch. Both live only in
// memory, so without this a restart would silently lose them: an in-flight
// transaction would stop blocking read-committed fetches it should still
// block, and - the specific regression this rebuild exists to prevent - an
// aborted transaction's records would become visible under read-committed
// again, because abortedTxns forgot they were ever aborted.
//
// This is a single forward pass over [StartOffset, HighWaterMark), read in
// bounded windows via Read rather than materialized all at once via
// ReadRange, so peak memory is a function of the window rather than of how
// much the partition retains. There is no shortcut here trading history for
// memory: an abort near the start of a long, heavily-retained log must
// still be found and hidden, so the fix for the memory cost is bounding how
// much is held at once, never bounding how far back replay looks.
//
// For each record, a transactional record (nonzero ProducerID) not yet seen
// for its producer epoch opens that epoch's tracking entry; a marker record
// closes it, and additionally records an aborted range if the marker says so
// and an entry was open (see replayRecord/replayMarker). Anything still open
// once the pass ends is genuinely still open as of recovery and becomes
// openTxns unchanged - not a leftover to clean up, since a real coordinator
// that had actually resolved it would have written a marker for the log to
// replay.
func (p *Partition) rebuildTransactionState() error {
	start := p.log.StartOffset()
	end := p.log.HighWaterMark()
	if start >= end {
		return nil
	}

	open := make(map[producerKey]int64)
	var aborted []abortedRange

	for offset := start; offset < end; {
		messages, err := p.log.Read(offset, rebuildReplayWindowBytes)
		if err != nil {
			return fmt.Errorf("replaying transaction state: %w", err)
		}
		if len(messages) == 0 {
			// Read only returns nothing once offset has caught up to the
			// log's live high water mark; offset < end <= that mark makes
			// this unreachable today. Breaking here rather than looping
			// forever means a future change to Read's contract turns into a
			// truncated replay, not a hang.
			break
		}

		for _, msg := range messages {
			aborted = replayRecord(msg, open, aborted)
			offset = msg.Offset + 1
		}
	}

	p.txnMu.Lock()
	p.openTxns = open
	p.abortedTxns = aborted
	p.txnMu.Unlock()

	return nil
}

// replayRecord applies one log record to the in-progress rebuild in
// rebuildTransactionState: a control record resolves (and, if it is an
// abort, ranges) its producer epoch via replayMarker; an ordinary
// transactional record not yet seen for its producer epoch opens that
// epoch's tracking entry, mirroring BeginTransaction's "first call only"
// rule. A non-transactional record (ProducerID 0) is not tracked, the same
// sentinel BeginTransaction itself ignores.
func replayRecord(msg *storage.Message, open map[producerKey]int64, aborted []abortedRange) []abortedRange {
	if protocol.IsControlRecord(msg.Headers) {
		return replayMarker(msg, open, aborted)
	}

	if msg.ProducerID == 0 {
		return aborted
	}
	key := producerKey{producerID: msg.ProducerID, producerEpoch: msg.ProducerEpoch}
	if _, tracked := open[key]; !tracked {
		open[key] = int64(msg.Offset)
	}
	return aborted
}

// replayMarker applies one transaction-marker record to the in-progress
// rebuild in rebuildTransactionState: it resolves the marker's producer
// epoch in open (mirroring EndTransaction/AbortTransaction), and appends an
// abortedRange for it when the marker is an abort and an entry was actually
// open to resolve. A marker this broker itself could not have written -
// unparseable headers, or resolving an epoch nothing tracked - is treated as
// a no-op rather than an error, the same tolerance EndTransaction/
// AbortTransaction already extend to a marker with no matching open entry.
func replayMarker(msg *storage.Message, open map[producerKey]int64, aborted []abortedRange) []abortedRange {
	producerID, producerEpoch, commit, ok := protocol.ParseTransactionMarker(msg.Headers)
	if !ok {
		return aborted
	}

	key := producerKey{producerID: producerID, producerEpoch: producerEpoch}
	start, tracked := open[key]
	delete(open, key)

	if tracked && !commit {
		aborted = append(aborted, abortedRange{
			producerID:    producerID,
			producerEpoch: producerEpoch,
			start:         start,
			end:           int64(msg.Offset),
		})
	}

	return aborted
}

// LastStableOffset returns the offset a read-committed fetch must not read
// past: the earliest start offset among this partition's still-open
// transactions, or the high water mark if none are open. It is always
// <= HighWaterMark, since a transaction cannot start beyond it.
//
// Every read-committed fetch calls this before consulting IsAborted, which
// makes it a convenient place to also prune abortedTxns: retention advances
// the log's start offset independently of any abort ever happening again on
// this partition, so pruning only from AbortTransaction would let ranges
// retention has already made irrelevant sit in memory until the next unlucky
// abort, rather than being reclaimed as soon as retention passes them.
func (p *Partition) LastStableOffset() int64 {
	p.txnMu.Lock()
	defer p.txnMu.Unlock()

	p.pruneAbortedLocked()

	lso := int64(p.log.HighWaterMark())
	for _, firstOffset := range p.openTxns {
		if firstOffset < lso {
			lso = firstOffset
		}
	}
	return lso
}

// PartitionOffsets returns the start offset, end offset and high water mark
// for a partition.
func (tm *TopicManager) PartitionOffsets(topic string, partitionID uint32) (start, end, highWaterMark int64, err error) {
	partition, err := tm.GetPartition(topic, partitionID)
	if err != nil {
		return 0, 0, 0, err
	}

	log := partition.Log()
	return int64(log.StartOffset()), int64(log.EndOffset()), int64(log.HighWaterMark()), nil
}

// ReadMessages reads up to limit messages from a partition starting at the
// given offset.
//
// An offset below the partition's start offset is clamped to the start offset
// so that callers browsing from 0 see the oldest retained messages rather than
// an error. Reading at or past the end offset returns an empty slice.
func (tm *TopicManager) ReadMessages(topic string, partitionID uint32, offset int64, limit int) ([]*storage.Message, error) {
	if limit <= 0 {
		return []*storage.Message{}, nil
	}

	partition, err := tm.GetPartition(topic, partitionID)
	if err != nil {
		return nil, err
	}

	log := partition.Log()
	startOffset := int64(log.StartOffset())
	endOffset := int64(log.EndOffset())

	if offset < startOffset {
		offset = startOffset
	}
	if offset >= endOffset {
		return []*storage.Message{}, nil
	}

	// ReadRange's end is exclusive; never read past the end of the log.
	rangeEnd := offset + int64(limit)
	if rangeEnd > endOffset {
		rangeEnd = endOffset
	}

	messages, err := log.ReadRange(storage.Offset(offset), storage.Offset(rangeEnd))
	if err != nil {
		return nil, fmt.Errorf("failed to read messages from %s:%d: %w", topic, partitionID, err)
	}

	if len(messages) > limit {
		messages = messages[:limit]
	}

	return messages, nil
}

// TopicDiskUsage returns the number of bytes the topic occupies on disk,
// summed across all of its partition directories.
func (tm *TopicManager) TopicDiskUsage(topic string) (int64, error) {
	if !tm.TopicExists(topic) {
		return 0, fmt.Errorf("topic %s does not exist", topic)
	}
	return dirSize(filepath.Join(tm.storageDir, topic))
}

// DiskUsageByTopic returns on-disk byte usage for every known topic. Topics
// whose directory cannot be read are reported as zero rather than failing the
// whole call, so a single unreadable topic does not hide usage for the rest.
func (tm *TopicManager) DiskUsageByTopic() map[string]int64 {
	tm.mu.RLock()
	names := make([]string, 0, len(tm.topics))
	for name := range tm.topics {
		names = append(names, name)
	}
	tm.mu.RUnlock()

	usage := make(map[string]int64, len(names))
	for _, name := range names {
		size, err := dirSize(filepath.Join(tm.storageDir, name))
		if err != nil {
			logger.Warn("failed to measure topic disk usage",
				zap.String("topic", name), zap.Error(err))
			size = 0
		}
		usage[name] = size
	}

	return usage
}

// dirSize sums the size of every regular file under root. A missing directory
// counts as zero bytes rather than an error: a topic that exists in memory but
// has not flushed anything to disk yet legitimately uses no disk.
func dirSize(root string) (int64, error) {
	var total int64

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			// The file was removed between listing and stat (e.g. compaction);
			// skip it rather than failing the whole measurement.
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return total, err
	}

	return total, nil
}
