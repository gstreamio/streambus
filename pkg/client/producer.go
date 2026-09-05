package client

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
)

// Producer sends messages to StreamBus topics
type Producer struct {
	client *Client
	config ProducerConfig

	// txnProducerID and txnProducerEpoch tag every ProduceRequest this
	// producer sends as belonging to an open transaction, so
	// Partition.BeginTransaction on the broker registers the write and a
	// read-committed fetch gates on its resolving marker instead of treating
	// it as an ordinary, immediately-visible write (see
	// protocol.ProduceRequest's doc on these fields).
	//
	// Zero - the default for every Producer created through NewProducer or
	// NewProducerWithConfig - means "not transactional", which is
	// load-bearing on the wire: it is how the broker tells a plain write
	// from a transactional one apart, and it is also what an idempotent-only
	// producer is supposed to send. Only newTransactionalInternalProducer
	// sets these away from zero; nothing outside this package may.
	txnProducerID    int64
	txnProducerEpoch int16

	// Batching
	mu          sync.Mutex
	batches     map[string]*messageBatch // topic+partition -> batch
	batchTicker *time.Ticker
	batchCtx    context.Context
	batchCancel context.CancelFunc
	batchWg     sync.WaitGroup

	// State
	closed int32

	// Metrics
	messagesSent   int64
	messagesFailed int64
	batchesSent    int64
	bytesSent      int64
}

// messageBatch holds messages for batching
type messageBatch struct {
	topic       string
	partitionID uint32
	messages    []protocol.Message
	mu          sync.Mutex
}

// NewProducer creates a new producer
func NewProducer(client *Client) *Producer {
	return NewProducerWithConfig(client, client.config.ProducerConfig)
}

// NewProducerWithConfig creates a new producer with custom config
func NewProducerWithConfig(client *Client, config ProducerConfig) *Producer {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Producer{
		client:      client,
		config:      config,
		batches:     make(map[string]*messageBatch),
		batchCtx:    ctx,
		batchCancel: cancel,
	}

	// Start batch flusher if batching is enabled
	if config.BatchSize > 0 && config.BatchTimeout > 0 {
		p.batchTicker = time.NewTicker(config.BatchTimeout)
		p.batchWg.Add(1)
		go p.batchFlusher()
	}

	return p
}

// newTransactionalInternalProducer creates a plain Producer whose writes are
// tagged with a transaction's producer id/epoch (see the Producer.txnProducerID
// field comment), so the broker recognizes them as belonging to that open
// transaction instead of as ordinary writes visible under every isolation
// level the instant they land. This is TransactionalProducer.flushMessages'
// own internal producer, not something any other caller should construct.
func newTransactionalInternalProducer(client *Client, producerID int64, producerEpoch int16) *Producer {
	p := NewProducer(client)
	p.txnProducerID = producerID
	p.txnProducerEpoch = producerEpoch
	return p
}

// Send sends a single message
func (p *Producer) Send(ctx context.Context, topic string, key, value []byte) error {
	return p.SendToPartition(ctx, topic, 0, key, value)
}

// SendToPartition sends a message to a specific partition
func (p *Producer) SendToPartition(ctx context.Context, topic string, partitionID uint32, key, value []byte) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return ErrProducerClosed
	}

	if topic == "" {
		return ErrInvalidTopic
	}

	msg := protocol.Message{
		Key:       key,
		Value:     value,
		Timestamp: time.Now().UnixNano(),
		Headers:   nil,
	}

	// If batching is enabled, add to batch
	if p.config.BatchSize > 0 {
		return p.addToBatch(ctx, topic, partitionID, msg)
	}

	// Otherwise, send immediately
	return p.sendMessage(ctx, topic, partitionID, []protocol.Message{msg})
}

// SendMessages sends multiple messages
func (p *Producer) SendMessages(ctx context.Context, topic string, messages []protocol.Message) error {
	return p.SendMessagesToPartition(ctx, topic, 0, messages)
}

// SendMessagesToPartition sends multiple messages to a specific partition
func (p *Producer) SendMessagesToPartition(ctx context.Context, topic string, partitionID uint32, messages []protocol.Message) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return ErrProducerClosed
	}

	if topic == "" {
		return ErrInvalidTopic
	}

	if len(messages) == 0 {
		return nil
	}

	// Set timestamps if not set
	now := time.Now().UnixNano()
	for i := range messages {
		if messages[i].Timestamp == 0 {
			messages[i].Timestamp = now
		}
	}

	// If batching is enabled and batch size not exceeded, add to batch
	if p.config.BatchSize > 0 && len(messages) <= p.config.BatchSize {
		for _, msg := range messages {
			if err := p.addToBatch(ctx, topic, partitionID, msg); err != nil {
				return err
			}
		}
		return nil
	}

	// Otherwise, send immediately
	return p.sendMessage(ctx, topic, partitionID, messages)
}

// addToBatch adds a message to the batch
func (p *Producer) addToBatch(ctx context.Context, topic string, partitionID uint32, msg protocol.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := fmt.Sprintf("%s:%d", topic, partitionID)
	batch := p.batches[key]

	if batch == nil {
		batch = &messageBatch{
			topic:       topic,
			partitionID: partitionID,
			messages:    make([]protocol.Message, 0, p.config.BatchSize),
		}
		p.batches[key] = batch
	}

	batch.mu.Lock()
	batch.messages = append(batch.messages, msg)
	full := len(batch.messages) >= p.config.BatchSize
	batch.mu.Unlock()

	// If batch is full, flush it
	if full {
		return p.flushBatch(ctx, key, batch)
	}

	return nil
}

// flushBatch flushes a batch
func (p *Producer) flushBatch(ctx context.Context, key string, batch *messageBatch) error {
	batch.mu.Lock()
	if len(batch.messages) == 0 {
		batch.mu.Unlock()
		return nil
	}

	messages := batch.messages
	batch.messages = make([]protocol.Message, 0, p.config.BatchSize)
	batch.mu.Unlock()

	return p.sendMessage(ctx, batch.topic, batch.partitionID, messages)
}

// sendMessage sends messages to the server
func (p *Producer) sendMessage(ctx context.Context, topic string, partitionID uint32, messages []protocol.Message) error {
	flags := protocol.FlagNone
	if p.config.RequireAck {
		flags = protocol.FlagRequireAck
	}

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			Type:    protocol.RequestTypeProduce,
			Version: protocol.ProtocolVersion,
			Flags:   flags,
		},
		Payload: &protocol.ProduceRequest{
			Topic:         topic,
			PartitionID:   partitionID,
			Messages:      messages,
			ProducerID:    p.txnProducerID,
			ProducerEpoch: p.txnProducerEpoch,
		},
	}

	// Send to first broker (later will have partition routing)
	broker := p.client.config.Brokers[0]
	_, err := p.client.sendRequestWithRetry(ctx, broker, req)

	if err != nil {
		atomic.AddInt64(&p.messagesFailed, int64(len(messages)))
		return err
	}

	atomic.AddInt64(&p.messagesSent, int64(len(messages)))
	atomic.AddInt64(&p.batchesSent, 1)

	// Calculate bytes sent
	var bytes int64
	for _, msg := range messages {
		bytes += int64(len(msg.Key) + len(msg.Value))
	}
	atomic.AddInt64(&p.bytesSent, bytes)

	return nil
}

// batchFlusher periodically flushes batches
func (p *Producer) batchFlusher() {
	defer p.batchWg.Done()

	for {
		select {
		case <-p.batchCtx.Done():
			// Flush all remaining batches before exiting. Use a fresh
			// context, not the already-cancelled p.batchCtx - otherwise the
			// flush would abort immediately instead of actually draining
			// pending messages on shutdown.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = p.FlushAll(shutdownCtx)
			cancel()
			return
		case <-p.batchTicker.C:
			_ = p.FlushAll(p.batchCtx)
		}
	}
}

// Flush flushes all pending batches for a topic
func (p *Producer) Flush(ctx context.Context, topic string) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return ErrProducerClosed
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for key, batch := range p.batches {
		if batch.topic == topic {
			if err := p.flushBatch(ctx, key, batch); err != nil {
				lastErr = err
			}
		}
	}

	return lastErr
}

// FlushAll flushes all pending batches
func (p *Producer) FlushAll(ctx context.Context) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return ErrProducerClosed
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for key, batch := range p.batches {
		if err := p.flushBatch(ctx, key, batch); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Close closes the producer
func (p *Producer) Close() error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return ErrProducerClosed
	}

	// Flush all pending batches before closing. Surface the error instead of
	// discarding it - a caller relying on batched Send() + Close() to
	// eventually deliver messages otherwise has no way to learn the final
	// flush failed and messages were dropped.
	flushErr := p.FlushAll(p.batchCtx)

	// Now mark as closed
	atomic.StoreInt32(&p.closed, 1)

	// Stop batch flusher
	if p.batchTicker != nil {
		p.batchCancel()
		p.batchTicker.Stop()
		p.batchWg.Wait()
	}

	return flushErr
}

// Stats returns producer statistics
func (p *Producer) Stats() ProducerStats {
	return ProducerStats{
		MessagesSent:   atomic.LoadInt64(&p.messagesSent),
		MessagesFailed: atomic.LoadInt64(&p.messagesFailed),
		BatchesSent:    atomic.LoadInt64(&p.batchesSent),
		BytesSent:      atomic.LoadInt64(&p.bytesSent),
	}
}

// ProducerStats holds producer statistics
type ProducerStats struct {
	MessagesSent   int64
	MessagesFailed int64
	BatchesSent    int64
	BytesSent      int64
}
