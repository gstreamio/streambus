# StreamBus Examples

This directory contains example applications demonstrating how to use StreamBus.

## Prerequisites

1. **Start a StreamBus server:**

```bash
cd /Users/shawntherrien/Projects/streambus
go run cmd/server/main.go
```

The server will start on `localhost:9092` by default.

## Producer Example

The producer example shows how to:
- Connect to a StreamBus server
- Create a topic with multiple partitions
- Send messages to a topic
- Handle errors and check statistics

### Run the producer:

```bash
cd examples/producer
go run main.go
```

**Expected output:**
```
StreamBus Producer Example
==========================

Creating topic 'events'...

Producing messages...
✓ Sent message 0
✓ Sent message 1
...
✓ Sent message 9

Flushing producer...

Producer Statistics:
  Messages Sent:   10
  Messages Failed: 0
  Batches Sent:    10

✓ Producer example completed successfully!
```

## Consumer Example

The consumer example shows how to:
- Connect to a StreamBus server
- Create a consumer for a specific partition
- Seek to the beginning of a partition
- Fetch and process messages
- Check consumer statistics

### Run the consumer:

```bash
cd examples/consumer
go run main.go
```

**Expected output:**
```
StreamBus Consumer Example
==========================

Seeking to beginning of partition 0...
Consuming messages (Press Ctrl+C to stop)...

Message #1:
  Offset:    0
  Key:       key-0
  Value:     message-0: Hello from StreamBus at 2025-01-07T...
  Timestamp: 2025-01-07T...
...

Consumer Statistics:
  Messages Read: 10
  Bytes Read:    850
  Fetch Count:   5

✓ Consumer example completed successfully!
```

## Full Workflow

1. Start the StreamBus server:
   ```bash
   go run cmd/server/main.go
   ```

2. In a new terminal, run the producer:
   ```bash
   cd examples/producer
   go run main.go
   ```

3. In another terminal, run the consumer:
   ```bash
   cd examples/consumer
   go run main.go
   ```

The consumer will read all messages produced by the producer!

## Performance Benchmarks

To see StreamBus performance metrics:

```bash
# Client benchmarks
cd pkg/client
go test -bench=. -benchmem

# Protocol benchmarks
cd pkg/protocol
go test -bench=. -benchmem

# Storage benchmarks
cd pkg/storage
go test -bench=. -benchmem
```

## Configuration

Both examples use default configuration. You can customize:

- **Brokers**: Change `config.Brokers` to connect to different servers
- **Batch Size**: Adjust `config.ProducerConfig.BatchSize` for batching
- **Start Offset**: Set `config.ConsumerConfig.StartOffset` to start from different positions
- **Timeouts**: Configure `config.RequestTimeout`, `config.ReadTimeout`, etc.

See `pkg/client/config.go` for all available options.
