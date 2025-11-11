# StreamBus SDK

A simple, intuitive Go SDK for StreamBus that makes working with distributed messaging easy.

## Features

- **Simple API** - Minimal boilerplate, maximum productivity
- **Type-Safe** - Full Go type safety with generics support
- **Fluent Interface** - Chain methods for cleaner code
- **Error Handling** - Clear, actionable error messages
- **Auto-Retry** - Built-in retry logic with exponential backoff
- **JSON Support** - First-class JSON serialization/deserialization
- **Consumer Groups** - Coordinated consumption across multiple instances
- **Admin Operations** - Full topic and cluster management
- **Observability** - Built-in metrics and logging hooks

## Installation

```bash
go get github.com/shawntherrien/streambus-sdk
```

## Quick Start

### Connect to StreamBus

```go
import "github.com/shawntherrien/streambus-sdk/streambus"

// Connect with defaults (localhost:9092)
client, err := streambus.Connect()

// Or specify brokers
client, err := streambus.Connect("broker1:9092", "broker2:9092")

// Don't forget to clean up
defer client.Close()
```

### Producing Messages

#### Simple Producer

```go
// Create a producer
producer := client.NewProducer()
defer producer.Close()

// Send a simple message
err := producer.Send("my-topic", "Hello, World!")

// Send with a key for ordering
err = producer.SendWithKey("orders", "order-123", "Order created")
```

#### JSON Producer

```go
type Event struct {
    ID   string `json:"id"`
    Type string `json:"type"`
    Data map[string]interface{} `json:"data"`
}

event := Event{
    ID:   "evt-001",
    Type: "user.signup",
    Data: map[string]interface{}{
        "user_id": "usr-123",
        "email":   "user@example.com",
    },
}

// Send as JSON
err := producer.SendJSON("events", event.ID, event)
```

#### Batch Producer

```go
messages := []streambus.Message{
    {Key: []byte("key1"), Value: []byte("value1")},
    {Key: []byte("key2"), Value: []byte("value2")},
}

err := producer.SendBatch("batch-topic", messages)
```

### Consuming Messages

#### Simple Consumer

```go
// Create a consumer
consumer := client.NewConsumer("my-topic")
defer consumer.Close()

// Consume messages
ctx := context.Background()
err := consumer.Consume(ctx, func(msg *streambus.ReceivedMessage) error {
    fmt.Printf("Received: %s\n", string(msg.Value))
    return nil
})
```

#### Consumer with Auto-Commit

```go
consumer := client.NewConsumerBuilder("orders").
    WithAutoCommit().
    WithRetries(3).
    Build()

err := consumer.Consume(ctx, func(msg *streambus.ReceivedMessage) error {
    // Process the message
    processOrder(msg.Value)

    // Offset is automatically committed on success
    return nil
})
```

#### JSON Consumer

```go
type Order struct {
    ID     string `json:"id"`
    Amount float64 `json:"amount"`
}

var order Order
err := consumer.ConsumeJSON(ctx, func(msg *streambus.ReceivedMessage, data interface{}) error {
    ord := data.(*Order)
    fmt.Printf("Order %s: $%.2f\n", ord.ID, ord.Amount)
    return nil
}, &order)
```

### Consumer Groups

```go
// Create a consumer group
group := client.NewConsumerGroupBuilder("my-group").
    WithTopics("topic1", "topic2").
    WithAutoCommit(5 * time.Second).
    Build()

// Start consuming
err := group.Start(ctx, func(msg *streambus.ReceivedMessage) error {
    fmt.Printf("[%s] Message: %s\n", msg.Topic, string(msg.Value))
    return nil
})
```

### Admin Operations

#### Topic Management

```go
admin := client.Admin()

// Create a simple topic
err := admin.QuickCreateTopic("my-topic", 10, 3) // 10 partitions, 3 replicas

// Create with full configuration
config := streambus.TopicConfig{
    Name:              "events",
    Partitions:        20,
    ReplicationFactor: 3,
    RetentionTime:     7 * 24 * time.Hour,
    CompressionType:   "snappy",
}
err = admin.CreateTopic(config)

// List topics
topics, err := admin.ListTopics()

// Describe topic
metadata, err := admin.DescribeTopic("my-topic")
```

#### Cluster Information

```go
// Get cluster metadata
metadata, err := admin.GetClusterMetadata(ctx)
fmt.Printf("Cluster ID: %s\n", metadata.ClusterID)
fmt.Printf("Topics: %d\n", metadata.TotalTopics)

for _, broker := range metadata.Brokers {
    fmt.Printf("Broker %d: %s:%d\n", broker.ID, broker.Host, broker.Port)
}
```

## Advanced Features

### Custom Logger

```go
type MyLogger struct{}

func (l *MyLogger) Debug(msg string, keysAndValues ...interface{}) {
    // Your logging implementation
}
// ... implement Info, Warn, Error

client.WithLogger(&MyLogger{})
```

### Error Handling

The SDK provides clear error types for different scenarios:

```go
err := producer.Send("topic", "message")
switch {
case errors.Is(err, streambus.ErrConnectionFailed):
    // Handle connection failure
case errors.Is(err, streambus.ErrMessageTooLarge):
    // Handle message size issue
case errors.Is(err, streambus.ErrTopicNotFound):
    // Handle missing topic
}
```

### Message Ordering

Messages with the same key are guaranteed to be ordered:

```go
// All messages for user-123 will be in order
producer.SendWithKey("user-events", "user-123", "login")
producer.SendWithKey("user-events", "user-123", "view_profile")
producer.SendWithKey("user-events", "user-123", "logout")
```

### Offset Management

```go
consumer := client.NewConsumer("my-topic")

// Start from beginning
consumer.SeekToBeginning()

// Start from end
consumer.SeekToEnd()

// Start from specific offset
consumer.SeekToOffset(12345)

// Manual commit
consumer.Commit()
```

## Builder Pattern

The SDK supports a fluent builder pattern for complex configurations:

```go
// Producer with defaults
producer := client.NewProducerBuilder().
    WithTopic("default-topic").
    WithPartition(0).
    Build()

// Consumer with options
consumer := client.NewConsumerBuilder("events").
    WithAutoCommit().
    WithRetries(5).
    Build()

// Consumer group with configuration
group := client.NewConsumerGroupBuilder("processing-group").
    WithTopics("orders", "payments", "shipments").
    WithAutoCommit(10 * time.Second).
    WithSessionTimeout(30 * time.Second).
    Build()
```

## Examples

See the [examples](examples/) directory for complete working examples:

- [Simple Producer](examples/simple_producer.go) - Basic message production
- [Simple Consumer](examples/simple_consumer.go) - Basic message consumption
- [Admin Operations](examples/admin_operations.go) - Topic and cluster management

## Performance Tips

1. **Batching** - Use batch operations when sending multiple messages
2. **Connection Pooling** - The SDK automatically pools connections
3. **Async Operations** - Use async methods for non-blocking sends
4. **Compression** - Enable compression for large messages
5. **Consumer Groups** - Use consumer groups for parallel processing

## Error Handling Best Practices

```go
// Retry with exponential backoff
err := consumer.Consume(ctx, func(msg *streambus.ReceivedMessage) error {
    for retry := 0; retry < 3; retry++ {
        if err := processMessage(msg); err != nil {
            time.Sleep(time.Duration(1<<uint(retry)) * time.Second)
            continue
        }
        return nil
    }

    // Send to dead letter queue after retries
    return producer.Send("dead-letter-queue", string(msg.Value))
})
```

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](../CONTRIBUTING.md) for details.

## License

StreamBus SDK is licensed under the MIT License. See [LICENSE](../LICENSE) for details.