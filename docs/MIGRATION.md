# Migration Guide

Guide for migrating from other messaging systems to StreamBus.

## Table of Contents

- [Migration from Apache Kafka](#migration-from-apache-kafka)
- [Migration from RabbitMQ](#migration-from-rabbitmq)
- [Migration from NATS](#migration-from-nats)
- [Migration from Apache Pulsar](#migration-from-apache-pulsar)
- [Migration Strategies](#migration-strategies)
- [Migration Tools](#migration-tools)

## Migration from Apache Kafka

### Conceptual Mapping

| Kafka | StreamBus | Notes |
|-------|-----------|-------|
| Broker | Broker | Similar concept |
| Topic | Topic | Direct equivalent |
| Partition | Partition | Same purpose |
| Consumer Group | Consumer Group | Identical |
| Producer | Producer | Similar API |
| Consumer | Consumer | Similar API |
| ZooKeeper | Raft | Built-in consensus |
| Offset | Offset | Same concept |

### Architecture Differences

**Kafka:**
```
┌──────────┐     ┌──────────┐
│  Broker  │────▶│ ZooKeeper│
└──────────┘     └──────────┘
     │                │
     └────────────────┘
     Requires ZooKeeper
```

**StreamBus:**
```
┌──────────┐
│  Broker  │ (Raft consensus built-in)
└──────────┘
No external dependencies
```

### Code Migration

**Producer (Kafka):**
```java
// Kafka
Properties props = new Properties();
props.put("bootstrap.servers", "localhost:9092");
props.put("key.serializer", "org.apache.kafka.common.serialization.StringSerializer");
props.put("value.serializer", "org.apache.kafka.common.serialization.StringSerializer");

KafkaProducer<String, String> producer = new KafkaProducer<>(props);
producer.send(new ProducerRecord<>("my-topic", "key", "value"));
producer.close();
```

**Producer (StreamBus):**
```go
// StreamBus
producer, _ := client.NewProducer(&client.ProducerConfig{
    Brokers: []string{"localhost:9092"},
})

producer.Send(context.Background(), &client.Message{
    Topic: "my-topic",
    Key:   []byte("key"),
    Value: []byte("value"),
})
producer.Close()
```

**Consumer (Kafka):**
```java
// Kafka
Properties props = new Properties();
props.put("bootstrap.servers", "localhost:9092");
props.put("group.id", "my-group");
props.put("key.deserializer", "org.apache.kafka.common.serialization.StringDeserializer");
props.put("value.deserializer", "org.apache.kafka.common.serialization.StringDeserializer");

KafkaConsumer<String, String> consumer = new KafkaConsumer<>(props);
consumer.subscribe(Arrays.asList("my-topic"));

while (true) {
    ConsumerRecords<String, String> records = consumer.poll(Duration.ofMillis(100));
    for (ConsumerRecord<String, String> record : records) {
        System.out.printf("offset = %d, key = %s, value = %s%n",
            record.offset(), record.key(), record.value());
    }
}
```

**Consumer (StreamBus):**
```go
// StreamBus
consumer, _ := client.NewConsumer(&client.ConsumerConfig{
    Brokers: []string{"localhost:9092"},
    GroupID: "my-group",
    Topics:  []string{"my-topic"},
})

for {
    msg, err := consumer.Receive(context.Background())
    if err != nil {
        break
    }
    fmt.Printf("offset = %d, key = %s, value = %s\n",
        msg.Offset, msg.Key, msg.Value)
    consumer.Commit(context.Background(), msg)
}
```

### Configuration Migration

**Kafka server.properties → StreamBus broker.yaml:**

| Kafka | StreamBus |
|-------|-----------|
| `broker.id` | `server.brokerId` |
| `listeners` | `server.port` |
| `log.dirs` | `storage.dataDir` |
| `num.partitions` | Topic-specific |
| `replication.factor` | `cluster.replicationFactor` |
| `log.retention.hours` | `storage.retentionDays` |

### Migration Steps

1. **Run Both Systems in Parallel**

   ```bash
   # Keep Kafka running
   # Start StreamBus cluster
   helm install streambus-operator streambus/streambus-operator
   ```

2. **Mirror Topics**

   ```bash
   # Create matching topics in StreamBus
   for topic in $(kafka-topics --list); do
       streambus-cli topic create $topic \
           --partitions $(kafka-topics --describe --topic $topic | grep PartitionCount | awk '{print $2}')
   done
   ```

3. **Dual-Write Pattern**

   ```go
   // Write to both systems
   func publish(msg Message) error {
       var wg sync.WaitGroup
       var kafkaErr, streambusErr error

       wg.Add(2)

       // Write to Kafka
       go func() {
           defer wg.Done()
           kafkaErr = kafkaProducer.Send(msg)
       }()

       // Write to StreamBus
       go func() {
           defer wg.Done()
           streambusErr = streambusProducer.Send(msg)
       }()

       wg.Wait()

       if kafkaErr != nil {
           return kafkaErr
       }
       return streambusErr
   }
   ```

4. **Migrate Consumers Gradually**

   ```bash
   # Start new consumers reading from StreamBus
   # Gradually shut down Kafka consumers
   # Monitor for any issues
   ```

5. **Stop Dual-Writing**

   ```go
   // Once all consumers migrated, write only to StreamBus
   func publish(msg Message) error {
       return streambusProducer.Send(msg)
   }
   ```

6. **Decommission Kafka**

   ```bash
   # Verify all consumers migrated
   # Stop Kafka cluster
   # Archive Kafka data if needed
   ```

### Compatibility Notes

**Fully Compatible:**
- Topic/partition model
- Consumer groups
- Offset management
- Message ordering guarantees

**Partially Compatible:**
- Different serialization (protobuf vs Kafka's format)
- Different admin APIs

**Not Compatible:**
- Kafka Streams (use StreamBus transactions instead)
- Kafka Connect (build custom connectors)
- Schema Registry (coming soon)

## Migration from RabbitMQ

### Conceptual Mapping

| RabbitMQ | StreamBus | Notes |
|----------|-----------|-------|
| Exchange | Topic | Similar routing |
| Queue | Partition | Storage unit |
| Binding | Subscription | Message routing |
| Routing Key | Message Key | Message routing |
| Consumer | Consumer | Similar concept |
| Publisher | Producer | Similar concept |

### Architecture Differences

**RabbitMQ** (Queue-based):
```
Producer → Exchange → Queue → Consumer
           (routing)   (storage)
```

**StreamBus** (Log-based):
```
Producer → Topic → Partition → Consumer
           (append)   (log)      (offset)
```

### Code Migration

**Producer (RabbitMQ):**
```go
// RabbitMQ
conn, _ := amqp.Dial("amqp://localhost")
ch, _ := conn.Channel()

ch.Publish(
    "my-exchange",  // exchange
    "routing.key",  // routing key
    false,          // mandatory
    false,          // immediate
    amqp.Publishing{
        ContentType: "text/plain",
        Body:        []byte("message"),
    },
)
```

**Producer (StreamBus):**
```go
// StreamBus
producer, _ := client.NewProducer(&client.ProducerConfig{
    Brokers: []string{"localhost:9092"},
})

producer.Send(context.Background(), &client.Message{
    Topic: "my-exchange",
    Key:   []byte("routing.key"),
    Value: []byte("message"),
})
```

**Consumer (RabbitMQ):**
```go
// RabbitMQ
msgs, _ := ch.Consume(
    "my-queue",
    "",     // consumer
    true,   // auto-ack
    false,  // exclusive
    false,  // no-local
    false,  // no-wait
    nil,    // args
)

for msg := range msgs {
    fmt.Printf("Received: %s\n", msg.Body)
}
```

**Consumer (StreamBus):**
```go
// StreamBus
consumer, _ := client.NewConsumer(&client.ConsumerConfig{
    Brokers: []string{"localhost:9092"},
    Topics:  []string{"my-exchange"},
})

for {
    msg, _ := consumer.Receive(context.Background())
    fmt.Printf("Received: %s\n", msg.Value)
    consumer.Commit(context.Background(), msg)
}
```

### Migration Strategy

**1. Fan-Out Pattern (RabbitMQ) → Partitions (StreamBus):**

```go
// RabbitMQ: Multiple queues bound to exchange
// Exchange → Queue1, Queue2, Queue3

// StreamBus: Multiple consumers in different groups
// Topic → Consumer Group 1, Consumer Group 2, Consumer Group 3
```

**2. Work Queue Pattern:**

```go
// RabbitMQ: Multiple consumers on same queue (round-robin)
// Queue → Consumer1, Consumer2, Consumer3

// StreamBus: Multiple consumers in same group (partition-based)
// Topic (partitions) → Consumer Group → Consumer1, Consumer2, Consumer3
```

## Migration from NATS

### Conceptual Mapping

| NATS | StreamBus | Notes |
|------|-----------|-------|
| Subject | Topic | Message routing |
| Stream | Topic | Persistent storage (JetStream) |
| Consumer | Consumer | Message consumption |
| Publisher | Producer | Message production |

### Key Differences

**NATS Core** (Fire and forget):
- No persistence
- At-most-once delivery

**NATS JetStream** (Persistent):
- Stream storage
- At-least-once delivery

**StreamBus** (Always persistent):
- All messages persisted
- Configurable guarantees

### Code Migration

**NATS:**
```go
nc, _ := nats.Connect("nats://localhost:4222")
nc.Publish("subject", []byte("message"))
```

**StreamBus:**
```go
producer, _ := client.NewProducer(&client.ProducerConfig{
    Brokers: []string{"localhost:9092"},
})
producer.Send(context.Background(), &client.Message{
    Topic: "subject",
    Value: []byte("message"),
})
```

## Migration from Apache Pulsar

### Conceptual Mapping

| Pulsar | StreamBus | Notes |
|--------|-----------|-------|
| Topic | Topic | Same |
| Partition | Partition | Same |
| Subscription | Consumer Group | Similar |
| Producer | Producer | Similar |
| Consumer | Consumer | Similar |
| BookKeeper | Raft | Different storage |

### Architecture Comparison

**Pulsar** (Layered):
```
Broker (stateless)
     ↓
BookKeeper (storage)
     ↓
ZooKeeper (metadata)
```

**StreamBus** (Integrated):
```
Broker (storage + consensus)
     ↓
Raft (integrated)
```

### Migration Benefits

Moving to StreamBus from Pulsar:
- Simpler architecture (fewer components)
- Easier operations (no BookKeeper/ZooKeeper)
- Lower operational overhead
- Comparable performance

## Migration Strategies

### 1. Big Bang Migration

**Pros:**
- Clean cut-over
- No dual-writing complexity

**Cons:**
- Higher risk
- Requires downtime

**Process:**
1. Prepare StreamBus cluster
2. Schedule maintenance window
3. Stop producers
4. Backup existing data
5. Migrate data (if needed)
6. Switch to StreamBus
7. Start producers/consumers

### 2. Gradual Migration

**Pros:**
- Lower risk
- No downtime
- Can be reversed

**Cons:**
- More complex
- Longer migration period

**Process:**
1. Deploy StreamBus alongside existing system
2. Dual-write to both systems
3. Migrate consumers one group at a time
4. Verify each migration step
5. Stop dual-writing
6. Decommission old system

### 3. Blue-Green Deployment

**Pros:**
- Zero downtime
- Easy rollback

**Cons:**
- Requires 2x resources temporarily
- More complex setup

**Process:**
1. Set up StreamBus (green) alongside Kafka (blue)
2. Configure load balancer to route to blue
3. Sync data to green
4. Switch load balancer to green
5. Verify green is working
6. Keep blue as backup
7. Decommission blue after verification period

## Migration Tools

### Data Migration Tool

```bash
# Install migration tool
go install github.com/shawntherrien/streambus/tools/migrate

# Migrate from Kafka
streambus-migrate kafka \
    --source-brokers kafka1:9092,kafka2:9092 \
    --dest-brokers streambus1:9092,streambus2:9092 \
    --topics topic1,topic2 \
    --start-offset earliest

# Migrate from RabbitMQ
streambus-migrate rabbitmq \
    --source-url amqp://localhost \
    --dest-brokers streambus1:9092 \
    --queue-mapping queue1=topic1,queue2=topic2
```

### Validation Tool

```bash
# Verify migration completeness
streambus-validate \
    --source kafka://localhost:9092 \
    --dest streambus://localhost:9092 \
    --topic my-topic
```

## Migration Checklist

Pre-migration:
- [ ] StreamBus cluster deployed and tested
- [ ] Topics created with appropriate partitions
- [ ] Security configured (TLS, auth)
- [ ] Monitoring setup
- [ ] Backup of existing data
- [ ] Migration plan documented
- [ ] Rollback plan ready

During migration:
- [ ] Dual-write implemented (if gradual)
- [ ] Consumers migrated (one group at a time)
- [ ] Data validation running
- [ ] Monitoring both systems
- [ ] Performance compared

Post-migration:
- [ ] All consumers migrated
- [ ] Dual-write stopped
- [ ] Old system decommissioned
- [ ] Documentation updated
- [ ] Team trained on StreamBus

## Getting Help

- Documentation: https://docs.streambus.io
- GitHub Issues: https://github.com/shawntherrien/streambus/issues
- Community Slack: https://streambus.slack.com
- Professional Services: support@streambus.io

## See Also

- [Production Deployment](PRODUCTION.md)
- [Performance Tuning](PERFORMANCE_TUNING.md)
- [Client Libraries](../pkg/client/README.md)
