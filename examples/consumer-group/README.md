# StreamBus Consumer Group Example

This example demonstrates how to use StreamBus consumer groups for coordinated, scalable message consumption.

## Overview

Consumer groups allow multiple consumers to coordinate and share the work of consuming messages from topics. Each consumer in a group is assigned a subset of partitions, ensuring that each message is processed by exactly one consumer in the group.

## Features Demonstrated

1. **Group Membership**: Joining a consumer group and coordinating with other consumers
2. **Rebalancing**: Automatic partition assignment and rebalancing when consumers join/leave
3. **Rebalance Listeners**: Custom callbacks for partition assignment changes
4. **Message Polling**: Consuming messages from assigned partitions
5. **Offset Management**: Manual offset commits for exactly-once semantics
6. **Heartbeats**: Automatic heartbeat sending to maintain group membership
7. **Graceful Shutdown**: Clean shutdown with final offset commits

## Prerequisites

1. A running StreamBus broker:
   ```bash
   # Start a broker
   make run-broker
   ```

2. Create some test data (using the producer example):
   ```bash
   cd examples/producer
   go run main.go
   ```

## Running the Example

### Single Consumer

Run one consumer:

```bash
cd examples/consumer-group
go run main.go consumer-1
```

The consumer will:
- Join the "example-consumer-group" group
- Subscribe to the "events" topic
- Be assigned all available partitions
- Start consuming messages
- Print statistics every 5 seconds
- Commit offsets every 10 seconds

### Multiple Consumers (Load Balancing)

To see consumer group coordination in action, run multiple consumers:

**Terminal 1:**
```bash
cd examples/consumer-group
go run main.go consumer-1
```

**Terminal 2:**
```bash
cd examples/consumer-group
go run main.go consumer-2
```

**Terminal 3:**
```bash
cd examples/consumer-group
go run main.go consumer-3
```

You'll observe:
1. First consumer gets all partitions
2. When second consumer joins, rebalance occurs and partitions are redistributed
3. When third consumer joins, another rebalance occurs
4. Each consumer processes messages from its assigned partitions only
5. When a consumer leaves, its partitions are reassigned to remaining consumers

## Configuration Options

The example uses the following consumer group configuration:

```go
groupConfig := client.DefaultGroupConsumerConfig()
groupConfig.GroupID = "example-consumer-group"       // Consumer group ID
groupConfig.Topics = []string{"events"}              // Topics to subscribe to
groupConfig.ClientID = consumerID                    // Unique consumer ID
groupConfig.SessionTimeoutMs = 30000                 // Session timeout (30s)
groupConfig.HeartbeatIntervalMs = 3000               // Heartbeat interval (3s)
groupConfig.RebalanceTimeoutMs = 60000               // Rebalance timeout (60s)
groupConfig.AssignmentStrategy = "range"             // Assignment strategy
groupConfig.AutoCommit = false                       // Manual commit
```

### Assignment Strategies

- **range**: Assigns contiguous partitions to each consumer (default)
- **roundrobin**: Distributes partitions evenly in round-robin fashion
- **sticky**: Minimizes partition movement during rebalances

### Offset Commit Modes

**Auto Commit** (groupConfig.AutoCommit = true):
- Offsets are automatically committed at regular intervals
- Simpler but may result in message reprocessing or loss on failure

**Manual Commit** (groupConfig.AutoCommit = false):
- You control when offsets are committed
- Enables exactly-once processing semantics
- More complex but provides better guarantees

## Rebalance Listeners

The example implements a custom rebalance listener:

```go
type CustomRebalanceListener struct {
    consumerID string
}

func (l *CustomRebalanceListener) OnPartitionsRevoked(partitions map[string][]int32) {
    // Called before partitions are taken away
    // Good time to commit offsets or flush buffers
}

func (l *CustomRebalanceListener) OnPartitionsAssigned(partitions map[string][]int32) {
    // Called after new partitions are assigned
    // Good time to initialize state or seek to offsets
}
```

## Graceful Shutdown

The example demonstrates graceful shutdown:

1. Press `Ctrl+C` to trigger shutdown
2. Consumer commits any pending offsets
3. Consumer leaves the group (triggers rebalance for others)
4. Partitions are reassigned to remaining consumers

## Output Example

```
StreamBus Consumer Group Example
=================================
Consumer ID: consumer-1

Group Configuration:
  Group ID: example-consumer-group
  Topics: [events]
  Assignment Strategy: range
  Auto Commit: false

📡 Subscribing to consumer group...

[consumer-1] ✓ Partitions Assigned:
  Topic: events, Partitions: [0 1 2 3]

✓ Successfully joined consumer group

Starting to consume messages (Press Ctrl+C to stop)...

📨 Message #1:
  Topic:     events
  Partition: 0
  Offset:    0
  Key:       key-0
  Value:     Hello StreamBus!
  Timestamp: 2025-11-09T10:30:00Z

📊 Status Update - Messages processed: 10, Assigned partitions: map[events:[0 1 2 3]]

💾 Committing offsets...
✓ Committed offsets for 1 topics
```

## Advanced Usage

### Custom Processing Logic

You can modify the message processing loop to implement your own business logic:

```go
for topic, partitions := range messages {
    for partition, msgs := range partitions {
        for _, msg := range msgs {
            // Your custom processing logic here
            if err := processMessage(msg); err != nil {
                log.Printf("Failed to process message: %v", err)
                continue
            }

            // Track offset for commit only after successful processing
            offsetsToCommit[topic][partition] = msg.Offset + 1
        }
    }
}
```

### Error Handling

Implement retry logic and error handling:

```go
const maxRetries = 3
for i := 0; i < maxRetries; i++ {
    if err := processMessage(msg); err == nil {
        break
    }
    if i == maxRetries-1 {
        log.Printf("Failed to process after %d retries: %v", maxRetries, err)
        // Send to dead letter queue or skip
    }
    time.Sleep(time.Second * time.Duration(i+1))
}
```

## Testing Fault Tolerance

1. Start multiple consumers
2. Kill one consumer (Ctrl+C or kill process)
3. Observe remaining consumers pick up the partitions
4. Restart the killed consumer
5. Observe rebalance as it rejoins the group

## Common Patterns

### Batch Processing

Process messages in batches for better throughput:

```go
batchSize := 100
batch := make([]protocol.Message, 0, batchSize)

for _, msg := range msgs {
    batch = append(batch, msg)
    if len(batch) >= batchSize {
        processBatch(batch)
        batch = batch[:0]
    }
}
```

### State Management

Track state per partition:

```go
type PartitionState struct {
    lastOffset   int64
    messageCount int64
}

partitionStates := make(map[string]map[int32]*PartitionState)
```

## Troubleshooting

**Consumer keeps rebalancing:**
- Check heartbeat and session timeout settings
- Ensure message processing doesn't exceed session timeout
- Look for network connectivity issues

**Messages not being consumed:**
- Verify broker is running
- Check topic exists and has messages
- Verify consumer is assigned partitions

**Duplicate message processing:**
- Ensure offsets are committed after processing
- Consider using manual commit mode
- Implement idempotent processing logic

## See Also

- [Simple Consumer Example](../consumer) - Single partition consumer
- [Producer Example](../producer) - Message production
- [StreamBus Documentation](../../docs) - Full documentation
