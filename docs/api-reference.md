# StreamBus API Reference

Complete API reference for StreamBus client library (v0.1.0 - Milestone 1.2).

## Table of Contents

- [Client API](#client-api)
- [Producer API](#producer-api)
- [Consumer API](#consumer-api)
- [Configuration](#configuration)
- [Data Types](#data-types)
- [Error Handling](#error-handling)

## Client API

The `Client` provides the core connection and topic management functionality.

### Creating a Client

```go
import "github.com/shawntherrien/streambus/pkg/client"

// With default configuration
config := client.DefaultConfig()
config.Brokers = []string{"localhost:9092"}

c, err := client.New(config)
if err != nil {
    log.Fatal(err)
}
defer c.Close()

// With custom configuration
config := &client.Config{
    Brokers:         []string{"broker1:9092", "broker2:9092"},
    ConnectTimeout:  10 * time.Second,
    RequestTimeout:  30 * time.Second,
    MaxRetries:      3,
    RetryBackoff:    100 * time.Millisecond,
}

c, err := client.New(config)
```

### Client Methods

#### HealthCheck

Check if a broker is healthy and responsive.

```go
func (c *Client) HealthCheck(broker string) error
```

**Parameters:**
- `broker`: Broker address (e.g., "localhost:9092")

**Returns:**
- `error`: Error if health check fails

**Example:**
```go
if err := c.HealthCheck("localhost:9092"); err != nil {
    log.Printf("Broker is unhealthy: %v", err)
}
```

#### CreateTopic

Create a new topic with specified partitions and replication factor.

```go
func (c *Client) CreateTopic(topic string, numPartitions uint32, replicationFactor uint16) error
```

**Parameters:**
- `topic`: Name of the topic to create
- `numPartitions`: Number of partitions (must be > 0)
- `replicationFactor`: Replication factor (currently ignored, reserved for future use)

**Returns:**
- `error`: Error if topic creation fails

**Example:**
```go
// Create topic with 3 partitions
err := c.CreateTopic("events", 3, 1)
if err != nil {
    log.Fatal(err)
}
```

#### DeleteTopic

Delete an existing topic and all its data.

```go
func (c *Client) DeleteTopic(topic string) error
```

**Parameters:**
- `topic`: Name of the topic to delete

**Returns:**
- `error`: Error if topic deletion fails

**Example:**
```go
err := c.DeleteTopic("events")
if err != nil {
    log.Fatal(err)
}
```

#### ListTopics

List all topics on the broker.

```go
func (c *Client) ListTopics() ([]string, error)
```

**Returns:**
- `[]string`: List of topic names
- `error`: Error if listing fails

**Example:**
```go
topics, err := c.ListTopics()
if err != nil {
    log.Fatal(err)
}

for _, topic := range topics {
    fmt.Println(topic)
}
```

#### Close

Close the client and release all resources.

```go
func (c *Client) Close() error
```

**Returns:**
- `error`: Error if close fails

**Example:**
```go
defer c.Close()
```

#### Stats

Get client statistics.

```go
func (c *Client) Stats() ClientStats
```

**Returns:**
- `ClientStats`: Statistics about client operations

**Example:**
```go
stats := c.Stats()
fmt.Printf("Requests sent: %d\n", stats.RequestsSent)
fmt.Printf("Requests failed: %d\n", stats.RequestsFailed)
fmt.Printf("Bytes written: %d\n", stats.BytesWritten)
fmt.Printf("Bytes read: %d\n", stats.BytesRead)
fmt.Printf("Uptime: %v\n", stats.Uptime)
```

---

## Producer API

The `Producer` handles sending messages to topics.

### Creating a Producer

```go
import "github.com/shawntherrien/streambus/pkg/client"

// Create client first
config := client.DefaultConfig()
c, err := client.New(config)
if err != nil {
    log.Fatal(err)
}

// Create producer
producer := client.NewProducer(c)
defer producer.Close()
```

### Producer Methods

#### Send

Send a single message to a topic (automatic partition selection).

```go
func (p *Producer) Send(topic string, key, value []byte) error
```

**Parameters:**
- `topic`: Topic name
- `key`: Message key (can be nil)
- `value`: Message value

**Returns:**
- `error`: Error if send fails

**Example:**
```go
err := producer.Send("events", []byte("key1"), []byte("Hello World"))
if err != nil {
    log.Fatal(err)
}
```

#### SendToPartition

Send a message to a specific partition.

```go
func (p *Producer) SendToPartition(topic string, partitionID uint32, key, value []byte) error
```

**Parameters:**
- `topic`: Topic name
- `partitionID`: Target partition ID
- `key`: Message key (can be nil)
- `value`: Message value

**Returns:**
- `error`: Error if send fails

**Example:**
```go
// Send to partition 0
err := producer.SendToPartition("events", 0, []byte("key1"), []byte("Hello"))
if err != nil {
    log.Fatal(err)
}
```

#### SendMessages

Send multiple messages to a topic (batch operation).

```go
func (p *Producer) SendMessages(topic string, messages []protocol.Message) error
```

**Parameters:**
- `topic`: Topic name
- `messages`: Slice of messages to send

**Returns:**
- `error`: Error if batch send fails

**Example:**
```go
messages := []protocol.Message{
    {Key: []byte("key1"), Value: []byte("value1")},
    {Key: []byte("key2"), Value: []byte("value2")},
}

err := producer.SendMessages("events", messages)
if err != nil {
    log.Fatal(err)
}
```

#### SendMessagesToPartition

Send multiple messages to a specific partition.

```go
func (p *Producer) SendMessagesToPartition(topic string, partitionID uint32, messages []protocol.Message) error
```

**Parameters:**
- `topic`: Topic name
- `partitionID`: Target partition ID
- `messages`: Slice of messages to send

**Returns:**
- `error`: Error if batch send fails

**Example:**
```go
messages := []protocol.Message{
    {Key: []byte("key1"), Value: []byte("value1")},
    {Key: []byte("key2"), Value: []byte("value2")},
}

err := producer.SendMessagesToPartition("events", 0, messages)
```

#### Flush

Flush pending batched messages for a specific topic.

```go
func (p *Producer) Flush(topic string) error
```

**Parameters:**
- `topic`: Topic name to flush

**Returns:**
- `error`: Error if flush fails

**Example:**
```go
err := producer.Flush("events")
if err != nil {
    log.Fatal(err)
}
```

#### FlushAll

Flush all pending batched messages for all topics.

```go
func (p *Producer) FlushAll() error
```

**Returns:**
- `error`: Error if flush fails

**Example:**
```go
defer producer.FlushAll()
```

#### Close

Close the producer and flush all pending messages.

```go
func (p *Producer) Close() error
```

**Returns:**
- `error`: Error if close fails

**Example:**
```go
defer producer.Close()
```

#### Stats

Get producer statistics.

```go
func (p *Producer) Stats() ProducerStats
```

**Returns:**
- `ProducerStats`: Statistics about producer operations

**Example:**
```go
stats := producer.Stats()
fmt.Printf("Messages sent: %d\n", stats.MessagesSent)
fmt.Printf("Messages failed: %d\n", stats.MessagesFailed)
fmt.Printf("Batches sent: %d\n", stats.BatchesSent)
fmt.Printf("Bytes written: %d\n", stats.BytesWritten)
```

---

## Consumer API

The `Consumer` handles reading messages from topics.

### Creating a Consumer

```go
import "github.com/shawntherrien/streambus/pkg/client"

// Create client first
config := client.DefaultConfig()
c, err := client.New(config)
if err != nil {
    log.Fatal(err)
}

// Create consumer for topic "events", partition 0
consumer := client.NewConsumer(c, "events", 0)
defer consumer.Close()
```

### Consumer Methods

#### Fetch

Fetch messages from the current offset (default batch size).

```go
func (c *Consumer) Fetch() ([]protocol.Message, error)
```

**Returns:**
- `[]protocol.Message`: Fetched messages
- `error`: Error if fetch fails

**Example:**
```go
messages, err := consumer.Fetch()
if err != nil {
    log.Fatal(err)
}

for _, msg := range messages {
    fmt.Printf("Offset: %d, Value: %s\n", msg.Offset, msg.Value)
}
```

#### FetchN

Fetch up to N messages from the current offset.

```go
func (c *Consumer) FetchN(maxMessages int) ([]protocol.Message, error)
```

**Parameters:**
- `maxMessages`: Maximum number of messages to fetch

**Returns:**
- `[]protocol.Message`: Fetched messages (may be fewer than maxMessages)
- `error`: Error if fetch fails

**Example:**
```go
messages, err := consumer.FetchN(10)
if err != nil {
    log.Fatal(err)
}
```

#### FetchOne

Fetch a single message from the current offset.

```go
func (c *Consumer) FetchOne() (*protocol.Message, error)
```

**Returns:**
- `*protocol.Message`: The fetched message (nil if no messages available)
- `error`: Error if fetch fails

**Example:**
```go
msg, err := consumer.FetchOne()
if err != nil {
    log.Fatal(err)
}
if msg != nil {
    fmt.Printf("Offset: %d, Value: %s\n", msg.Offset, msg.Value)
}
```

#### Seek

Seek to a specific offset.

```go
func (c *Consumer) Seek(offset int64) error
```

**Parameters:**
- `offset`: Target offset (must be >= 0)

**Returns:**
- `error`: Error if seek fails

**Example:**
```go
// Seek to offset 100
err := consumer.Seek(100)
if err != nil {
    log.Fatal(err)
}
```

#### SeekToBeginning

Seek to the beginning of the partition (offset 0).

```go
func (c *Consumer) SeekToBeginning() error
```

**Returns:**
- `error`: Error if seek fails

**Example:**
```go
err := consumer.SeekToBeginning()
if err != nil {
    log.Fatal(err)
}
```

#### SeekToEnd

Seek to the end of the partition (next message to be written).

```go
func (c *Consumer) SeekToEnd() error
```

**Returns:**
- `error`: Error if seek fails

**Example:**
```go
err := consumer.SeekToEnd()
if err != nil {
    log.Fatal(err)
}
```

#### GetEndOffset

Get the end offset (next offset to be written) for the partition.

```go
func (c *Consumer) GetEndOffset() (int64, error)
```

**Returns:**
- `int64`: End offset
- `error`: Error if request fails

**Example:**
```go
endOffset, err := consumer.GetEndOffset()
if err != nil {
    log.Fatal(err)
}
fmt.Printf("End offset: %d\n", endOffset)
```

#### CurrentOffset

Get the current consumer offset.

```go
func (c *Consumer) CurrentOffset() int64
```

**Returns:**
- `int64`: Current offset

**Example:**
```go
offset := consumer.CurrentOffset()
fmt.Printf("Current offset: %d\n", offset)
```

#### Topic

Get the topic name this consumer is reading from.

```go
func (c *Consumer) Topic() string
```

**Returns:**
- `string`: Topic name

#### Partition

Get the partition ID this consumer is reading from.

```go
func (c *Consumer) Partition() uint32
```

**Returns:**
- `uint32`: Partition ID

#### Poll

Poll for messages at regular intervals with a handler function.

```go
func (c *Consumer) Poll(interval time.Duration, handler func([]protocol.Message) error) error
```

**Parameters:**
- `interval`: Polling interval
- `handler`: Function to call with fetched messages

**Returns:**
- `error`: Error if polling fails

**Example:**
```go
err := consumer.Poll(100*time.Millisecond, func(messages []protocol.Message) error {
    for _, msg := range messages {
        fmt.Printf("Received: %s\n", msg.Value)
    }
    return nil
})
```

#### Close

Close the consumer.

```go
func (c *Consumer) Close() error
```

**Returns:**
- `error`: Error if close fails

**Example:**
```go
defer consumer.Close()
```

#### Stats

Get consumer statistics.

```go
func (c *Consumer) Stats() ConsumerStats
```

**Returns:**
- `ConsumerStats`: Statistics about consumer operations

**Example:**
```go
stats := consumer.Stats()
fmt.Printf("Messages read: %d\n", stats.MessagesRead)
fmt.Printf("Bytes read: %d\n", stats.BytesRead)
fmt.Printf("Fetch count: %d\n", stats.FetchCount)
```

---

## Configuration

### Config

Main client configuration structure.

```go
type Config struct {
    // Broker addresses
    Brokers []string

    // Connection timeout
    ConnectTimeout time.Duration

    // Read timeout for requests
    ReadTimeout time.Duration

    // Write timeout for requests
    WriteTimeout time.Duration

    // Maximum number of connections per broker
    MaxConnectionsPerBroker int

    // Retry configuration
    MaxRetries    int
    RetryBackoff  time.Duration
    RetryMaxDelay time.Duration

    // Request timeout
    RequestTimeout time.Duration

    // Keep-alive configuration
    KeepAlive       bool
    KeepAlivePeriod time.Duration

    // Producer configuration
    ProducerConfig ProducerConfig

    // Consumer configuration
    ConsumerConfig ConsumerConfig
}
```

### ProducerConfig

Producer-specific configuration.

```go
type ProducerConfig struct {
    // Whether to require acknowledgment from server
    RequireAck bool

    // Batch size for batching messages (0 = no batching)
    BatchSize int

    // Maximum time to wait before flushing batch
    BatchTimeout time.Duration

    // Compression type (none, gzip, snappy, lz4)
    Compression string

    // Maximum number of in-flight requests
    MaxInFlightRequests int
}
```

### ConsumerConfig

Consumer-specific configuration.

```go
type ConsumerConfig struct {
    // Consumer group ID
    GroupID string

    // Offset to start consuming from (earliest, latest, or specific offset)
    StartOffset int64

    // Maximum bytes to fetch per request
    MaxFetchBytes uint32

    // Minimum bytes before server responds
    MinFetchBytes uint32

    // Maximum wait time for server to accumulate min bytes
    MaxWaitTime time.Duration

    // Auto-commit offset interval
    AutoCommitInterval time.Duration
}
```

### Default Configuration

```go
config := client.DefaultConfig()
// Returns:
// {
//     Brokers: []string{"localhost:9092"},
//     ConnectTimeout: 10 * time.Second,
//     ReadTimeout: 30 * time.Second,
//     WriteTimeout: 30 * time.Second,
//     MaxConnectionsPerBroker: 5,
//     MaxRetries: 3,
//     RetryBackoff: 100 * time.Millisecond,
//     RetryMaxDelay: 30 * time.Second,
//     RequestTimeout: 30 * time.Second,
//     KeepAlive: true,
//     KeepAlivePeriod: 30 * time.Second,
//     ProducerConfig: {
//         RequireAck: true,
//         BatchSize: 100,
//         BatchTimeout: 10 * time.Millisecond,
//         Compression: "none",
//         MaxInFlightRequests: 5,
//     },
//     ConsumerConfig: {
//         GroupID: "",
//         StartOffset: -1, // Latest
//         MaxFetchBytes: 1024 * 1024, // 1MB
//         MinFetchBytes: 1,
//         MaxWaitTime: 500 * time.Millisecond,
//         AutoCommitInterval: 5 * time.Second,
//     },
// }
```

---

## Data Types

### Message

Represents a message in StreamBus.

```go
type Message struct {
    Offset    int64     // Message offset in partition
    Key       []byte    // Message key (optional)
    Value     []byte    // Message value
    Timestamp time.Time // Message timestamp
}
```

### ClientStats

Client operation statistics.

```go
type ClientStats struct {
    RequestsSent   int64         // Total requests sent
    RequestsFailed int64         // Total failed requests
    BytesWritten   int64         // Total bytes written
    BytesRead      int64         // Total bytes read
    Uptime         time.Duration // Client uptime
}
```

### ProducerStats

Producer operation statistics.

```go
type ProducerStats struct {
    MessagesSent   int64 // Total messages sent
    MessagesFailed int64 // Total messages failed
    BatchesSent    int64 // Total batches sent
    BytesWritten   int64 // Total bytes written
}
```

### ConsumerStats

Consumer operation statistics.

```go
type ConsumerStats struct {
    MessagesRead int64 // Total messages read
    BytesRead    int64 // Total bytes read
    FetchCount   int64 // Total fetch operations
}
```

---

## Error Handling

### Common Errors

```go
var (
    // Client errors
    ErrClientClosed         = errors.New("client is closed")
    ErrNoBrokers           = errors.New("no brokers specified")
    ErrInvalidTimeout      = errors.New("invalid timeout")
    ErrInvalidMaxConnections = errors.New("invalid max connections")
    ErrInvalidRetries      = errors.New("invalid retries")

    // Producer errors
    ErrProducerClosed      = errors.New("producer is closed")
    ErrInvalidTopic        = errors.New("invalid topic")
    ErrInvalidPartition    = errors.New("invalid partition")

    // Consumer errors
    ErrConsumerClosed      = errors.New("consumer is closed")
    ErrInvalidOffset       = errors.New("invalid offset")
    ErrEndOfPartition      = errors.New("end of partition")

    // Network errors
    ErrConnectionFailed    = errors.New("connection failed")
    ErrRequestTimeout      = errors.New("request timeout")
    ErrBrokerUnavailable   = errors.New("broker unavailable")
)
```

### Error Checking

```go
import "errors"

// Check for specific error
if errors.Is(err, client.ErrClientClosed) {
    log.Println("Client is closed")
}

// Check for network errors
if errors.Is(err, client.ErrConnectionFailed) {
    // Retry logic
}
```

---

## Best Practices

### Connection Management

```go
// Always use defer to ensure cleanup
config := client.DefaultConfig()
c, err := client.New(config)
if err != nil {
    log.Fatal(err)
}
defer c.Close()
```

### Producer Usage

```go
// Create producer
producer := client.NewProducer(c)
defer producer.Close()

// Always flush before closing
defer producer.FlushAll()

// Send messages
for i := 0; i < 100; i++ {
    err := producer.Send("events", nil, []byte(fmt.Sprintf("message-%d", i)))
    if err != nil {
        log.Printf("Failed to send: %v", err)
    }
}
```

### Consumer Usage

```go
// Create consumer
consumer := client.NewConsumer(c, "events", 0)
defer consumer.Close()

// Seek to beginning for new consumer
consumer.SeekToBeginning()

// Fetch in loop
for {
    messages, err := consumer.Fetch()
    if err != nil {
        log.Printf("Fetch error: %v", err)
        continue
    }

    for _, msg := range messages {
        // Process message
        fmt.Printf("Offset: %d, Value: %s\n", msg.Offset, msg.Value)
    }
}
```

### Error Handling

```go
// Use retries for transient errors
maxRetries := 3
for i := 0; i < maxRetries; i++ {
    err := producer.Send("events", nil, []byte("data"))
    if err == nil {
        break
    }

    if errors.Is(err, client.ErrRequestTimeout) {
        // Retry on timeout
        time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
        continue
    }

    // Don't retry on other errors
    log.Fatal(err)
}
```

---

## Complete Example

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/shawntherrien/streambus/pkg/client"
)

func main() {
    // Create client
    config := client.DefaultConfig()
    config.Brokers = []string{"localhost:9092"}

    c, err := client.New(config)
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // Create topic
    err = c.CreateTopic("example", 3, 1)
    if err != nil {
        log.Printf("Topic may already exist: %v", err)
    }

    // Producer example
    producer := client.NewProducer(c)
    defer producer.Close()
    defer producer.FlushAll()

    for i := 0; i < 10; i++ {
        key := []byte(fmt.Sprintf("key-%d", i))
        value := []byte(fmt.Sprintf("Hello from StreamBus #%d", i))

        err := producer.Send("example", key, value)
        if err != nil {
            log.Printf("Failed to send: %v", err)
        }
    }

    // Consumer example
    consumer := client.NewConsumer(c, "example", 0)
    defer consumer.Close()

    consumer.SeekToBeginning()

    messages, err := consumer.Fetch()
    if err != nil {
        log.Fatal(err)
    }

    for _, msg := range messages {
        fmt.Printf("Offset: %d, Key: %s, Value: %s\n",
            msg.Offset, msg.Key, msg.Value)
    }

    // Print statistics
    stats := c.Stats()
    fmt.Printf("\nClient Stats:\n")
    fmt.Printf("  Requests: %d (failed: %d)\n",
        stats.RequestsSent, stats.RequestsFailed)
    fmt.Printf("  Bytes: %d written, %d read\n",
        stats.BytesWritten, stats.BytesRead)
}
```

---

## See Also

- [Getting Started Guide](GETTING_STARTED.md)
- [Architecture Documentation](ARCHITECTURE.md)
- [Examples](../examples/README.md)
- [Benchmarks](BENCHMARKS.md)
