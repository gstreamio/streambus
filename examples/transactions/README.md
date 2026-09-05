# StreamBus Transactions Example

> **Note on read isolation**: committing writes a durable transaction marker
> to every participating partition, and an aborted transaction writes no user
> records at all (messages are buffered until commit). Set
> `ConsumerConfig.IsolationLevel` to `protocol.IsolationReadCommitted` so a
> consumer never observes part of a transaction still in flight; marker
> records are hidden from consumers at any isolation level.

This example demonstrates **exactly-once semantics** using StreamBus transactions. It shows how to implement a transactional message processor that consumes from one topic, processes messages, and produces to another topic - all within atomic transactions.

## Overview

Transactions in StreamBus provide:
- **Exactly-Once Semantics**: Each message is processed exactly once, even in the presence of failures
- **Atomic Writes**: Multiple messages written atomically across partitions
- **Producer Fencing**: Prevents duplicate writes from zombie producers
- **Coordinated Offsets**: Consumer offsets committed atomically with produced messages

## Use Case: Transactional Stream Processor

This example implements a common pattern: read-process-write within a transaction.

```
[Input Topic] → [Process] → [Output Topic]
      ↓                            ↑
   Offsets ←─── Transaction ────→ Messages
```

All operations succeed or fail together, ensuring exactly-once processing.

## How It Works

### 1. Initialize Transactional Producer

```go
producerConfig := client.DefaultTransactionalProducerConfig()
producerConfig.TransactionID = "example-processor-txn"

producer, err := client.NewTransactionalProducer(client, producerConfig)
```

The producer gets a unique Producer ID and Epoch from the coordinator. The epoch is used to fence zombie producers.

### 2. Begin Transaction

```go
err := producer.BeginTransaction(ctx)
```

Starts a new transaction. Only one transaction can be active at a time per producer.

### 3. Send Messages

```go
msg := protocol.Message{
    Key:   []byte("key-1"),
    Value: []byte("processed-data"),
}

err := producer.Send(ctx, "output-topic", partition, msg)
```

Messages are buffered locally until the transaction is committed.

### 4. Add Consumer Offsets (Optional)

```go
offsets := map[string]map[int32]int64{
    "input-topic": {
        0: 100, // Consumed up to offset 100
    },
}

err := producer.SendOffsetsToTransaction(ctx, "consumer-group", offsets)
```

This ensures consumer offsets are committed atomically with the produced messages.

### 5. Commit or Abort

```go
// Success
err := producer.CommitTransaction(ctx)

// Failure
err := producer.AbortTransaction(ctx)
```

On commit, all messages become visible to consumers atomically. On abort, all messages are discarded.

## Running the Example

### Prerequisites

1. Start a StreamBus broker:
   ```bash
   make run-broker
   ```

2. Create topics (if needed):
   ```bash
   # Create input and output topics
   ```

### Run the Example

```bash
cd examples/transactions
go run main.go
```

You should see output like:

```
StreamBus Transactions Example
===============================

This example demonstrates exactly-once semantics using transactions.
Messages are consumed, processed, and produced atomically within transactions.

✓ Initialized transactional producer (ID: 1001, Epoch: 0)

Starting transactional message processing...
(Press Ctrl+C to stop)

  📨 [Txn 1] Processed message 1: input-message-1 -> PROCESSED: INPUT-MESSAGE-1
  📨 [Txn 1] Processed message 2: input-message-2 -> PROCESSED: INPUT-MESSAGE-2
  📨 [Txn 1] Processed message 3: input-message-3 -> PROCESSED: INPUT-MESSAGE-3
  ✅ [Txn 1] Committed successfully (offset: 3)

  📨 [Txn 2] Processed message 4: input-message-4 -> PROCESSED: INPUT-MESSAGE-4
  ...

═════════════════════════════════════
       FINAL STATISTICS
═════════════════════════════════════
Producer ID:              1001
Producer Epoch:           0
Producer State:           Ready
─────────────────────────────────────
Total Messages Processed: 30
Total Transactions:       10
Transactions Committed:   10
Transactions Aborted:     0
Success Rate:             100.0%
═════════════════════════════════════

✓ Transactional processing completed successfully!
```

## Transaction Lifecycle

```
┌─────────────────────────────────────────────────────────┐
│                  Transaction Lifecycle                   │
└─────────────────────────────────────────────────────────┘

1. InitProducerID()
   └─> Coordinator assigns Producer ID + Epoch

2. BeginTransaction()
   └─> Local state: Ongoing

3. Send(msg1, msg2, ...)
   └─> Messages buffered locally
   └─> Partitions added to transaction

4. SendOffsetsToTransaction(offsets)
   └─> Consumer offsets added to transaction

5. CommitTransaction() or AbortTransaction()
   │
   ├─> COMMIT PATH:
   │   ├─> Prepare Phase: Write transaction markers
   │   ├─> Complete Phase: Finalize commit
   │   └─> Messages become visible atomically
   │
   └─> ABORT PATH:
       ├─> Prepare Phase: Mark for abort
       ├─> Complete Phase: Finalize abort
       └─> Messages are discarded
```

## Error Handling

The example demonstrates proper error handling:

```go
if err := producer.BeginTransaction(ctx); err != nil {
    // Handle error - transaction not started
    return err
}

if err := producer.Send(ctx, topic, partition, msg); err != nil {
    // On error, abort transaction
    producer.AbortTransaction(ctx)
    return err
}

if err := producer.CommitTransaction(ctx); err != nil {
    // Commit failed - transaction was aborted
    return err
}
```

## Producer Fencing Example

Producer fencing prevents zombie producers from corrupting data:

```go
// Producer 1 (zombie) - Epoch 0
producer1, _ := NewTransactionalProducer(client, config)
// Producer1 hangs or crashes...

// Producer 2 (new) - Epoch 1 (fences Producer 1)
producer2, _ := NewTransactionalProducer(client, config)

// Now if Producer 1 wakes up and tries to write:
producer1.Send(...)  // ❌ REJECTED - Producer fenced (epoch mismatch)

// Only Producer 2 can write:
producer2.Send(...)  // ✅ ACCEPTED
```

## Advanced Patterns

### Pattern 1: Consume-Transform-Produce

```go
for {
    // Begin transaction
    producer.BeginTransaction(ctx)

    // Consume batch of messages
    messages := consumer.Fetch()

    // Process each message
    for _, msg := range messages {
        processed := transform(msg)
        producer.Send(ctx, "output", 0, processed)
    }

    // Commit consumer offsets with transaction
    producer.SendOffsetsToTransaction(ctx, groupID, offsets)

    // Commit transaction
    producer.CommitTransaction(ctx)
}
```

### Pattern 2: Multi-Topic Atomic Writes

```go
producer.BeginTransaction(ctx)

// Write to multiple topics atomically
producer.Send(ctx, "orders", 0, order)
producer.Send(ctx, "inventory", 0, inventoryUpdate)
producer.Send(ctx, "notifications", 0, notification)

// All messages commit together or none do
producer.CommitTransaction(ctx)
```

### Pattern 3: Idempotent Processing with Deduplication

```go
producer.BeginTransaction(ctx)

// Track processed message IDs in external store
if !isProcessed(messageID) {
    processed := process(message)
    producer.Send(ctx, "output", 0, processed)
    markAsProcessed(messageID)
}

producer.CommitTransaction(ctx)
```

## Transaction Configuration

```go
config := client.DefaultTransactionalProducerConfig()

// Unique transaction ID (required)
config.TransactionID = "my-processor"

// Transaction timeout (default: 60s)
config.TransactionTimeout = 60 * time.Second

// Request timeout for coordinator operations (default: 30s)
config.RequestTimeout = 30 * time.Second
```

## Performance Considerations

1. **Batch Size**: Larger batches improve throughput but increase latency
2. **Transaction Timeout**: Set based on expected processing time
3. **Commit Frequency**: Balance durability vs. throughput
4. **Partition Count**: More partitions = more parallelism

## Guarantees

✅ **Exactly-Once Semantics**: Each message processed exactly once
✅ **Atomic Multi-Partition Writes**: All or nothing across partitions
✅ **Producer Fencing**: Zombie producers cannot corrupt data
✅ **Coordinated Offsets**: Consumer progress tracked with transactions
✅ **Idempotent Writes**: Retries don't create duplicates

## Common Issues

**Transaction Timeout**
```
Error: transaction timeout exceeded
```
Solution: Increase `TransactionTimeout` or process messages faster

**Producer Fenced**
```
Error: producer has been fenced
```
Solution: Don't run multiple producers with same TransactionID

**No Transaction in Progress**
```
Error: no transaction in progress
```
Solution: Call `BeginTransaction()` before `Send()`

## See Also

- [Consumer Group Example](../consumer-group) - Coordinated consumption
- [Producer Example](../producer) - Basic message production
- [StreamBus Documentation](../../docs) - Full documentation

## Further Reading

- [Kafka Transactions Design](https://cwiki.apache.org/confluence/display/KAFKA/KIP-98+-+Exactly+Once+Delivery+and+Transactional+Messaging)
- [Exactly-Once Semantics](https://www.confluent.io/blog/exactly-once-semantics-are-possible-heres-how-apache-kafka-does-it/)
