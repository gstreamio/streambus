# StreamBus Quick Start Tutorial

Get up and running with StreamBus in under 10 minutes! This tutorial will guide you through setting up a broker, producing messages, and consuming them.

## What You'll Learn

- ✅ Start a StreamBus broker
- ✅ Create a topic
- ✅ Produce messages
- ✅ Consume messages
- ✅ View metrics

**Estimated Time**: 10 minutes

---

## Prerequisites

Choose one of the following:

**Option 1: Docker (Recommended)**
- Docker installed
- No build required

**Option 2: From Source**
- Go 1.21+ installed
- Git installed

---

## Step 1: Start StreamBus Broker

### Using Docker (Recommended)

```bash
# Pull and run StreamBus
docker run -d \
  --name streambus \
  -p 9092:9092 \
  -p 8080:8080 \
  streambus/broker:1.0.0

# Verify it's running
docker logs streambus
```

### Using Docker Compose (Multi-Broker Cluster)

```bash
# Clone repository
git clone https://github.com/shawntherrien/streambus.git
cd streambus

# Start 3-broker cluster
docker-compose -f deploy/docker-compose-cluster.yml up -d

# Verify cluster is running
docker-compose -f deploy/docker-compose-cluster.yml ps
```

### From Source

```bash
# Clone and build
git clone https://github.com/shawntherrien/streambus.git
cd streambus
make build

# Start broker
./bin/streambus-broker --config config/broker.yaml
```

---

## Step 2: Verify Broker is Running

```bash
# Check health endpoint
curl http://localhost:8080/health/live

# Expected output:
# {"status":"ok","timestamp":"2025-11-10T12:00:00Z"}

# Check metrics
curl http://localhost:8080/metrics | head -20
```

**Success!** Your StreamBus broker is now running.

---

## Step 3: Create a Topic

### Using CLI

```bash
# If using Docker, install CLI
docker exec streambus streambus-cli topic create my-first-topic

# Or if built from source
./bin/streambus-cli topic create my-first-topic --brokers localhost:9092
```

### Using Admin API

```bash
curl -X POST http://localhost:8080/api/v1/topics \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-first-topic",
    "partitions": 3,
    "replication_factor": 1
  }'
```

### Verify Topic Creation

```bash
# List topics
curl http://localhost:8080/api/v1/topics

# Expected output:
# {"topics":["my-first-topic"]}
```

---

## Step 4: Produce Messages

### Interactive Producer (CLI)

```bash
# Start interactive producer
streambus-cli produce my-first-topic --brokers localhost:9092

# Type messages and press Enter:
# > Hello StreamBus!
# > This is my first message
# > Press Ctrl+C to exit
```

### Programmatic Producer (Go)

Create a file `producer_example.go`:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/shawntherrien/streambus/pkg/client"
)

func main() {
    // Create client
    c, err := client.New(&client.Config{
        Brokers:        []string{"localhost:9092"},
        ConnectTimeout: 10 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // Create producer
    p, err := c.Producer(&client.ProducerConfig{
        Topic: "my-first-topic",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer p.Close()

    // Send 10 messages
    ctx := context.Background()
    for i := 0; i < 10; i++ {
        msg := &client.Message{
            Key:   []byte(fmt.Sprintf("key-%d", i)),
            Value: []byte(fmt.Sprintf("Hello StreamBus! Message %d", i)),
        }

        offset, err := p.Send(ctx, msg)
        if err != nil {
            log.Printf("Error sending message: %v", err)
            continue
        }

        fmt.Printf("Message sent successfully! Offset: %d\n", offset)
    }

    fmt.Println("All messages sent!")
}
```

Run it:

```bash
go mod init producer-example
go mod edit -replace github.com/shawntherrien/streambus=../streambus
go run producer_example.go
```

### Using HTTP API

```bash
# Send a single message
curl -X POST http://localhost:9092/produce \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "my-first-topic",
    "messages": [
      {
        "key": "user-123",
        "value": "Hello from REST API!"
      }
    ]
  }'
```

---

## Step 5: Consume Messages

### Interactive Consumer (CLI)

```bash
# Start consuming from beginning
streambus-cli consume my-first-topic --brokers localhost:9092 --from-beginning

# Expected output:
# Offset: 0 | Key: key-0 | Value: Hello StreamBus! Message 0
# Offset: 1 | Key: key-1 | Value: Hello StreamBus! Message 1
# ...
```

### Programmatic Consumer (Go)

Create a file `consumer_example.go`:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/shawntherrien/streambus/pkg/client"
)

func main() {
    // Create client
    c, err := client.New(&client.Config{
        Brokers:        []string{"localhost:9092"},
        ConnectTimeout: 10 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // Create consumer
    consumer, err := c.Consumer(&client.ConsumerConfig{
        Topic:     "my-first-topic",
        Partition: 0,
        Offset:    0, // Start from beginning
    })
    if err != nil {
        log.Fatal(err)
    }
    defer consumer.Close()

    // Consume messages
    ctx := context.Background()
    for i := 0; i < 10; i++ {
        msg, err := consumer.Consume(ctx)
        if err != nil {
            log.Printf("Error consuming: %v", err)
            break
        }

        fmt.Printf("Consumed message:\n")
        fmt.Printf("  Offset: %d\n", msg.Offset)
        fmt.Printf("  Key: %s\n", string(msg.Key))
        fmt.Printf("  Value: %s\n", string(msg.Value))
        fmt.Printf("  Timestamp: %v\n\n", msg.Timestamp)
    }

    fmt.Println("Consumption complete!")
}
```

Run it:

```bash
go mod init consumer-example
go mod edit -replace github.com/shawntherrien/streambus=../streambus
go run consumer_example.go
```

---

## Step 6: Consumer Groups

Consumer groups allow multiple consumers to work together to consume messages from a topic.

### Create Consumer Group

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/shawntherrien/streambus/pkg/client"
)

func main() {
    // Create client
    c, err := client.New(&client.Config{
        Brokers:        []string{"localhost:9092"},
        ConnectTimeout: 10 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // Join consumer group
    group, err := c.ConsumerGroup(&client.ConsumerGroupConfig{
        GroupID: "my-consumer-group",
        Topics:  []string{"my-first-topic"},
    })
    if err != nil {
        log.Fatal(err)
    }
    defer group.Close()

    // Consume messages
    ctx := context.Background()
    for {
        msg, err := group.Consume(ctx)
        if err != nil {
            log.Printf("Error: %v", err)
            break
        }

        fmt.Printf("Message: %s\n", string(msg.Value))

        // Commit offset
        if err := group.Commit(ctx, msg.Offset); err != nil {
            log.Printf("Commit error: %v", err)
        }
    }
}
```

---

## Step 7: View Metrics and Health

### Check Health

```bash
# Liveness check
curl http://localhost:8080/health/live

# Readiness check
curl http://localhost:8080/health/ready

# Detailed status
curl http://localhost:8080/health/status
```

### View Metrics

```bash
# All metrics
curl http://localhost:8080/metrics

# Filter specific metrics
curl http://localhost:8080/metrics | grep streambus_messages_produced_total
curl http://localhost:8080/metrics | grep streambus_messages_consumed_total
curl http://localhost:8080/metrics | grep streambus_broker_uptime_seconds
```

### View Topics

```bash
# List all topics
curl http://localhost:8080/api/v1/topics

# Get topic details
curl http://localhost:8080/api/v1/topics/my-first-topic
```

---

## Step 8: Advanced Features

### Transactions

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/shawntherrien/streambus/pkg/client"
)

func main() {
    c, _ := client.New(&client.Config{
        Brokers: []string{"localhost:9092"},
    })
    defer c.Close()

    // Create transactional producer
    p, _ := c.TransactionalProducer(&client.ProducerConfig{
        Topic:           "my-first-topic",
        TransactionalID: "my-tx-id",
    })
    defer p.Close()

    ctx := context.Background()

    // Begin transaction
    if err := p.BeginTransaction(ctx); err != nil {
        log.Fatal(err)
    }

    // Send messages within transaction
    for i := 0; i < 5; i++ {
        msg := &client.Message{
            Value: []byte(fmt.Sprintf("Transactional message %d", i)),
        }
        if _, err := p.Send(ctx, msg); err != nil {
            p.AbortTransaction(ctx)
            log.Fatal(err)
        }
    }

    // Commit transaction
    if err := p.CommitTransaction(ctx); err != nil {
        log.Fatal(err)
    }

    fmt.Println("Transaction committed successfully!")
}
```

### Schema Registry

```bash
# Register a schema
curl -X POST http://localhost:8080/api/v1/schemas \
  -H "Content-Type: application/json" \
  -d '{
    "subject": "user-events",
    "schema": {
      "type": "object",
      "properties": {
        "userId": {"type": "string"},
        "action": {"type": "string"},
        "timestamp": {"type": "integer"}
      },
      "required": ["userId", "action"]
    }
  }'

# Validate a message against schema
curl -X POST http://localhost:8080/api/v1/schemas/user-events/validate \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user-123",
    "action": "login",
    "timestamp": 1699632000
  }'
```

---

## Step 9: Cluster Management

### View Cluster Status

```bash
# Get cluster info
curl http://localhost:8080/api/v1/cluster/status

# List brokers
curl http://localhost:8080/api/v1/brokers

# View leader information
curl http://localhost:8080/api/v1/cluster/leader
```

### Admin Operations

```bash
# Create topic with replication
curl -X POST http://localhost:8080/api/v1/topics \
  -H "Content-Type: application/json" \
  -d '{
    "name": "replicated-topic",
    "partitions": 6,
    "replication_factor": 3
  }'

# Delete topic
curl -X DELETE http://localhost:8080/api/v1/topics/my-first-topic

# List consumer groups
curl http://localhost:8080/api/v1/consumer-groups
```

---

## Step 10: Cleanup

### Stop and Remove Containers

```bash
# Single broker (Docker)
docker stop streambus
docker rm streambus

# Cluster (Docker Compose)
docker-compose -f deploy/docker-compose-cluster.yml down -v
```

### Remove Data (Optional)

```bash
# Remove data volumes
docker volume rm streambus_broker-1-data
docker volume rm streambus_broker-2-data
docker volume rm streambus_broker-3-data
```

---

## Complete Example Application

Here's a complete example that combines producer and consumer:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "sync"
    "time"

    "github.com/shawntherrien/streambus/pkg/client"
)

func main() {
    // Create client
    c, err := client.New(&client.Config{
        Brokers:        []string{"localhost:9092"},
        ConnectTimeout: 10 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    topic := "demo-topic"
    ctx := context.Background()
    var wg sync.WaitGroup

    // Start producer
    wg.Add(1)
    go func() {
        defer wg.Done()
        producer, _ := c.Producer(&client.ProducerConfig{Topic: topic})
        defer producer.Close()

        for i := 0; i < 20; i++ {
            msg := &client.Message{
                Key:   []byte(fmt.Sprintf("key-%d", i)),
                Value: []byte(fmt.Sprintf("Message %d", i)),
            }
            offset, err := producer.Send(ctx, msg)
            if err != nil {
                log.Printf("Send error: %v", err)
                continue
            }
            fmt.Printf("✓ Produced: Message %d (offset %d)\n", i, offset)
            time.Sleep(100 * time.Millisecond)
        }
    }()

    // Start consumer
    wg.Add(1)
    go func() {
        defer wg.Done()
        time.Sleep(1 * time.Second) // Let producer start first

        consumer, _ := c.Consumer(&client.ConsumerConfig{
            Topic:     topic,
            Partition: 0,
            Offset:    0,
        })
        defer consumer.Close()

        consumed := 0
        for consumed < 20 {
            msg, err := consumer.Consume(ctx)
            if err != nil {
                log.Printf("Consume error: %v", err)
                break
            }
            fmt.Printf("✓ Consumed: %s (offset %d)\n", string(msg.Value), msg.Offset)
            consumed++
        }
    }()

    wg.Wait()
    fmt.Println("\n🎉 Demo complete!")
}
```

---

## Common Commands Cheat Sheet

```bash
# Broker
streambus-broker --config broker.yaml
curl http://localhost:8080/health/live

# Topics
streambus-cli topic create <name> --partitions 3
streambus-cli topic list
streambus-cli topic describe <name>
streambus-cli topic delete <name>

# Producer
streambus-cli produce <topic>
streambus-cli produce <topic> --key mykey --value myvalue

# Consumer
streambus-cli consume <topic> --from-beginning
streambus-cli consume <topic> --group my-group

# Admin
streambus-admin broker list
streambus-admin topic list
streambus-admin group list

# Monitoring
curl http://localhost:8080/metrics
curl http://localhost:8080/health/status
```

---

## Next Steps

Now that you've completed the quick start, explore these topics:

- **[Deployment Guide](DEPLOYMENT.md)** - Production deployment options
- **[Security Guide](SECURITY.md)** - Enable TLS, SASL, and ACLs
- **[Monitoring Guide](MONITORING.md)** - Set up Prometheus and Grafana
- **[Performance Tuning](PERFORMANCE.md)** - Optimize for your workload
- **[Architecture](ARCHITECTURE.md)** - Understand how StreamBus works
- **[API Reference](api-reference.md)** - Complete API documentation

---

## Troubleshooting

### Broker won't start

```bash
# Check logs
docker logs streambus

# Common issues:
# - Port already in use
# - Data directory permissions
# - Invalid configuration
```

### Can't connect to broker

```bash
# Verify broker is listening
netstat -an | grep 9092

# Test connection
telnet localhost 9092

# Check firewall rules
```

### Messages not being consumed

```bash
# Check topic exists
curl http://localhost:8080/api/v1/topics

# Check consumer offset
curl http://localhost:8080/api/v1/consumer-groups/<group>/offsets

# Check partition assignment
curl http://localhost:8080/api/v1/topics/<topic>
```

---

## Getting Help

- **Documentation**: Browse `docs/` directory
- **Examples**: Check `examples/` directory
- **Issues**: https://github.com/shawntherrien/streambus/issues
- **Discussions**: https://github.com/shawntherrien/streambus/discussions

---

**Congratulations!** 🎉 You've successfully set up StreamBus, produced messages, and consumed them. You're now ready to build scalable message streaming applications!
