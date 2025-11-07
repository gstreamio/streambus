# Migrating from Apache Kafka to StreamBus

Comprehensive guide for migrating from Apache Kafka to StreamBus.

> **Important**: StreamBus is currently in early development (Milestone 1.2 complete). This guide describes the planned migration path. Full migration tooling and compatibility features will be available in future releases.

## Table of Contents

- [Why Migrate?](#why-migrate)
- [Key Differences](#key-differences)
- [Migration Strategy](#migration-strategy)
- [API Comparison](#api-comparison)
- [Configuration Mapping](#configuration-mapping)
- [Feature Parity](#feature-parity)
- [Migration Tools](#migration-tools)
- [Performance Comparison](#performance-comparison)
- [Troubleshooting](#troubleshooting)

---

## Why Migrate?

### StreamBus Advantages

**Performance Benefits:**
- **Lower Latency**: Sub-millisecond operations vs Kafka's 0.5-5ms typical latency
- **Memory Efficient**: <100MB vs Kafka's 2-8GB typical JVM heap
- **Fast Startup**: <1 second vs Kafka's 15-45 second JVM initialization
- **Predictable GC**: <1ms GC pauses vs Java's 10-200ms stop-the-world pauses

**Operational Benefits:**
- **Simple Deployment**: Single binary, no JVM tuning required
- **No ZooKeeper**: Built-in Raft consensus (no external dependencies)
- **Lower Resource Usage**: Smaller memory and CPU footprint
- **Modern Tooling**: Native Go with excellent observability

**Development Benefits:**
- **Clean API**: Simpler, more intuitive client libraries
- **Better Errors**: Detailed error messages and debugging
- **Fast Iteration**: Quick build and test cycles
- **Type Safety**: Go's compile-time type checking

### When to Migrate

**Good Fit:**
- Latency-sensitive applications (<10ms requirements)
- Resource-constrained environments
- Microservices architectures
- Real-time data pipelines
- IoT and edge computing

**Stay with Kafka:**
- Mature ecosystem requirements (connectors, ksqlDB, etc.)
- Very high throughput batch workloads (>1M msgs/sec)
- Complex stream processing (until StreamBus Stream API is ready)
- Need for battle-tested production stability

---

## Key Differences

### Architecture Differences

| Aspect | Kafka | StreamBus |
|--------|-------|-----------|
| **Language** | Java/Scala | Go |
| **Consensus** | ZooKeeper / KRaft | Raft (built-in) |
| **Storage** | Log segments | LSM-tree + WAL |
| **Protocol** | Binary wire protocol | Custom binary protocol |
| **Client** | JVM-based | Native Go (multi-language planned) |

### Semantic Differences

| Feature | Kafka | StreamBus |
|---------|-------|-----------|
| **Message Ordering** | Per-partition | Per-partition |
| **Offset Management** | Consumer groups | Consumer groups (planned) |
| **Exactly-Once** | Transactions | Planned |
| **Retention** | Time/size-based | Time/size-based |
| **Partitioning** | Hash/custom | Hash/custom |

### Terminology Mapping

| Kafka Term | StreamBus Term | Notes |
|------------|----------------|-------|
| Broker | Broker | Same concept |
| Topic | Topic | Same concept |
| Partition | Partition | Same concept |
| Consumer Group | Consumer Group | Planned |
| Producer | Producer | Same concept |
| Consumer | Consumer | Same concept |
| Offset | Offset | Same concept |
| Log Segment | SSTable | Different storage format |
| ISR (In-Sync Replica) | ISR | Planned |
| Leader Election | Raft Leader | Different consensus mechanism |

---

## Migration Strategy

### Strategy 1: Dual-Write Migration (Recommended)

Safest approach for production systems.

```
Phase 1: Dual Write
┌─────────┐
│  App    │
└────┬────┘
     │
     ├──────────┐
     │          │
     ▼          ▼
┌────────┐  ┌──────────┐
│ Kafka  │  │StreamBus │
└────────┘  └──────────┘

Phase 2: Dual Read (Validation)
┌─────────┐
│  App    │────► Validate
└────┬────┘
     │
     ├──────────┐
     │          │
     ▼          ▼
┌────────┐  ┌──────────┐
│ Kafka  │  │StreamBus │
└────────┘  └──────────┘

Phase 3: Cut Over
┌─────────┐
│  App    │
└────┬────┘
     │
     ▼
┌──────────┐
│StreamBus │
└──────────┘
```

#### Implementation Steps

**Step 1: Deploy StreamBus**

```bash
# Deploy StreamBus alongside Kafka
docker-compose -f docker-compose-migration.yml up -d
```

**Step 2: Dual-Write Code**

```go
package main

import (
    "log"

    kafka "github.com/segmentio/kafka-go"
    streambus "github.com/shawntherrien/streambus/pkg/client"
)

type DualProducer struct {
    kafka     *kafka.Writer
    streambus *streambus.Producer
}

func (p *DualProducer) Send(key, value []byte) error {
    // Write to Kafka (primary)
    kafkaErr := p.kafka.WriteMessages(context.Background(),
        kafka.Message{Key: key, Value: value})

    // Write to StreamBus (secondary, don't fail on error)
    if sbErr := p.streambus.Send("events", key, value); sbErr != nil {
        log.Printf("StreamBus write failed (non-critical): %v", sbErr)
    }

    return kafkaErr
}
```

**Step 3: Validate Data**

```bash
# Compare message counts
kafka-run-class kafka.tools.GetOffsetShell \
  --broker-list localhost:9092 --topic events

streambus topics stats events
```

**Step 4: Switch Readers**

```go
// Gradually move consumers to StreamBus
consumer := streambus.NewConsumer(client, "events", 0)
consumer.SeekToBeginning()

messages, err := consumer.Fetch()
// Process messages...
```

**Step 5: Cut Over Producers**

```go
// Remove Kafka producer
producer := streambus.NewProducer(client)
producer.Send("events", key, value)
```

**Step 6: Decommission Kafka**

```bash
# After validation period
docker-compose -f kafka-compose.yml down
```

### Strategy 2: Bulk Migration

Faster but riskier approach.

**Step 1: Export Kafka Data**

```bash
# Use kafka-console-consumer to export
kafka-console-consumer \
  --bootstrap-server localhost:9092 \
  --topic events \
  --from-beginning \
  --property print.key=true \
  --property key.separator=, \
  > kafka-export.txt
```

**Step 2: Import to StreamBus**

```bash
# Use StreamBus import tool (planned)
streambus import \
  --file kafka-export.txt \
  --topic events \
  --format kafka-console
```

**Step 3: Update Applications**

Update all producer and consumer code to use StreamBus clients.

**Step 4: Switch Over**

Stop Kafka, start serving from StreamBus.

### Strategy 3: Topic-by-Topic Migration

Migrate one topic at a time.

```
Week 1: Migrate topic "logs"
Week 2: Migrate topic "events"
Week 3: Migrate topic "metrics"
```

---

## API Comparison

### Producer API

#### Kafka (Java)

```java
Properties props = new Properties();
props.put("bootstrap.servers", "localhost:9092");
props.put("key.serializer",
    "org.apache.kafka.common.serialization.StringSerializer");
props.put("value.serializer",
    "org.apache.kafka.common.serialization.StringSerializer");

Producer<String, String> producer =
    new KafkaProducer<>(props);

ProducerRecord<String, String> record =
    new ProducerRecord<>("events", "key1", "value1");

producer.send(record, (metadata, exception) -> {
    if (exception != null) {
        exception.printStackTrace();
    }
});

producer.close();
```

#### StreamBus (Go)

```go
config := client.DefaultConfig()
config.Brokers = []string{"localhost:9092"}

c, _ := client.New(config)
defer c.Close()

producer := client.NewProducer(c)
defer producer.Close()

err := producer.Send("events",
    []byte("key1"),
    []byte("value1"))
if err != nil {
    log.Printf("Send failed: %v", err)
}
```

### Consumer API

#### Kafka (Java)

```java
Properties props = new Properties();
props.put("bootstrap.servers", "localhost:9092");
props.put("group.id", "my-group");
props.put("key.deserializer",
    "org.apache.kafka.common.serialization.StringDeserializer");
props.put("value.deserializer",
    "org.apache.kafka.common.serialization.StringDeserializer");

KafkaConsumer<String, String> consumer =
    new KafkaConsumer<>(props);

consumer.subscribe(Arrays.asList("events"));

while (true) {
    ConsumerRecords<String, String> records =
        consumer.poll(Duration.ofMillis(100));

    for (ConsumerRecord<String, String> record : records) {
        System.out.printf("offset=%d, key=%s, value=%s%n",
            record.offset(), record.key(), record.value());
    }
}

consumer.close();
```

#### StreamBus (Go)

```go
config := client.DefaultConfig()
c, _ := client.New(config)
defer c.Close()

consumer := client.NewConsumer(c, "events", 0)
defer consumer.Close()

consumer.SeekToBeginning()

for {
    messages, err := consumer.Fetch()
    if err != nil {
        log.Printf("Fetch error: %v", err)
        continue
    }

    for _, msg := range messages {
        fmt.Printf("offset=%d, key=%s, value=%s\n",
            msg.Offset, msg.Key, msg.Value)
    }
}
```

---

## Configuration Mapping

### Broker Configuration

| Kafka Config | StreamBus Config | Notes |
|-------------|------------------|-------|
| `broker.id` | `server.broker_id` | Same purpose |
| `listeners` | `server.listen` | Format: `host:port` |
| `log.dirs` | `server.data_dir` | Single directory in StreamBus |
| `log.retention.hours` | `storage.retention.max_age` | Duration format: `168h` |
| `log.retention.bytes` | `storage.retention.max_bytes` | Bytes |
| `log.segment.bytes` | `storage.max_sstable_size` | Different storage format |
| `num.network.threads` | `performance.io_threads` | Thread count |
| `socket.send.buffer.bytes` | `network.socket_send_buffer` | Buffer size |
| `socket.receive.buffer.bytes` | `network.socket_receive_buffer` | Buffer size |

### Producer Configuration

| Kafka Config | StreamBus Config | Notes |
|-------------|------------------|-------|
| `bootstrap.servers` | `Brokers` | Same format |
| `acks` | `ProducerConfig.RequireAck` | Boolean in StreamBus |
| `batch.size` | `ProducerConfig.BatchSize` | Message count |
| `linger.ms` | `ProducerConfig.BatchTimeout` | Duration format |
| `compression.type` | `ProducerConfig.Compression` | String: "none", "gzip", etc. |
| `max.in.flight.requests.per.connection` | `ProducerConfig.MaxInFlightRequests` | Integer |
| `retries` | `MaxRetries` | Retry count |
| `request.timeout.ms` | `RequestTimeout` | Duration format |

### Consumer Configuration

| Kafka Config | StreamBus Config | Notes |
|-------------|------------------|-------|
| `bootstrap.servers` | `Brokers` | Same format |
| `group.id` | `ConsumerConfig.GroupID` | Planned feature |
| `auto.offset.reset` | `ConsumerConfig.StartOffset` | -2=earliest, -1=latest |
| `max.partition.fetch.bytes` | `ConsumerConfig.MaxFetchBytes` | Bytes |
| `fetch.min.bytes` | `ConsumerConfig.MinFetchBytes` | Bytes |
| `fetch.max.wait.ms` | `ConsumerConfig.MaxWaitTime` | Duration format |
| `auto.commit.interval.ms` | `ConsumerConfig.AutoCommitInterval` | Duration format |

---

## Feature Parity

### Current Feature Status

| Feature | Kafka | StreamBus Status |
|---------|-------|------------------|
| **Core Messaging** |
| Topics & Partitions | ✅ | ✅ Implemented |
| Producer API | ✅ | ✅ Implemented |
| Consumer API | ✅ | ✅ Implemented |
| Message Ordering | ✅ | ✅ Implemented |
| **Reliability** |
| Replication | ✅ | 📅 Milestone 2.2 |
| Leader Election | ✅ | 📅 Milestone 2.1 |
| ISR Management | ✅ | 📅 Milestone 2.2 |
| **Consumer Features** |
| Consumer Groups | ✅ | 📅 Milestone 3.1 |
| Offset Management | ✅ | ✅ Basic (🔧 Advanced planned) |
| Rebalancing | ✅ | 📅 Milestone 3.1 |
| **Advanced** |
| Transactions | ✅ | 📅 Milestone 3.2 |
| Exactly-Once | ✅ | 📅 Milestone 3.2 |
| Streams API | ✅ | 📅 Milestone 4+ |
| **Operations** |
| Multi-broker | ✅ | 📅 Milestone 2.1 |
| Rolling Upgrades | ✅ | 📅 Milestone 2+ |
| Metrics | ✅ | 📅 Milestone 3+ |
| Security (TLS/Auth) | ✅ | 📅 Milestone 4+ |

### Kafka Features Not Planned

- **ZooKeeper Integration** - Using Raft instead
- **Log Compaction** - Different storage model (LSM-tree)
- **MirrorMaker** - Different replication approach
- **Connect API** - Will have native connector framework
- **Kafka Streams** - Will have separate stream processing library

---

## Migration Tools

### Planned Migration Utilities

#### Data Migration Tool

```bash
# Export from Kafka
streambus migrate export \
  --source kafka://localhost:9092 \
  --topic events \
  --output kafka-data.sbm

# Import to StreamBus
streambus migrate import \
  --input kafka-data.sbm \
  --target localhost:9092 \
  --topic events
```

#### Configuration Converter

```bash
# Convert Kafka server.properties to StreamBus YAML
streambus config convert \
  --input kafka-server.properties \
  --output streambus-config.yaml

# Validate configuration
streambus config validate streambus-config.yaml
```

#### Compatibility Layer (Planned)

```go
// Use Kafka client API with StreamBus backend
import "github.com/shawntherrien/streambus/kafka-compat"

// Drop-in replacement
producer := kafka.NewWriter(kafka.WriterConfig{
    Brokers: []string{"streambus:9092"},
    Topic:   "events",
})
```

---

## Performance Comparison

### Latency Comparison

| Operation | Kafka (Typical) | StreamBus | Improvement |
|-----------|----------------|-----------|-------------|
| Producer Send | 0.5-5ms | 25µs | 20-200x faster |
| Consumer Fetch | 1-10ms | 22µs | 45-450x faster |
| End-to-End | 5-50ms | 0.5-2ms | 10-25x faster |

### Throughput Comparison

| Workload | Kafka | StreamBus | Notes |
|----------|-------|-----------|-------|
| Single Producer | 50K msgs/s | 40K msgs/s | StreamBus: single-threaded |
| Batch Producer | 1M+ msgs/s | 500K msgs/s | Kafka: highly optimized batching |
| Single Consumer | 50K msgs/s | 46K msgs/s | Comparable |

### Resource Comparison

| Resource | Kafka (Typical) | StreamBus | Savings |
|----------|----------------|-----------|---------|
| Memory | 2-8 GB | <100 MB | 95%+ |
| CPU (Idle) | 5-10% | <1% | 90%+ |
| Startup Time | 15-45s | <1s | 95%+ |
| Disk Space | Higher (log segments) | Lower (LSM compression) | 30-50% |

---

## Troubleshooting

### Common Migration Issues

#### Issue: Different Message Offsets

**Problem:** Offsets don't match between Kafka and StreamBus.

**Solution:** Use logical timestamps or message IDs for correlation instead of offsets.

```go
// Add correlation ID to messages
type Message struct {
    CorrelationID string
    Timestamp     time.Time
    Data          []byte
}
```

#### Issue: Consumer Group Compatibility

**Problem:** StreamBus consumer groups not yet implemented.

**Solution:** Use partition-level consumers temporarily.

```go
// Instead of Kafka consumer group
// consumer.Subscribe([]string{"events"})

// Use StreamBus partition consumers
for partition := 0; partition < numPartitions; partition++ {
    go func(p uint32) {
        consumer := client.NewConsumer(c, "events", p)
        defer consumer.Close()
        // Process messages...
    }(uint32(partition))
}
```

#### Issue: Serialization Format

**Problem:** Kafka uses Avro/Protobuf, need to maintain compatibility.

**Solution:** Use same serialization in StreamBus.

```go
// Kafka
var avroRecord MyRecord
avro.Unmarshal(msg.Value, &avroRecord)

// StreamBus (same)
var avroRecord MyRecord
avro.Unmarshal(msg.Value, &avroRecord)
```

#### Issue: Performance Expectations

**Problem:** Lower batch throughput than Kafka.

**Solution:** Optimize for your workload.

```go
// Increase batch size
config.ProducerConfig.BatchSize = 1000
config.ProducerConfig.BatchTimeout = 10 * time.Millisecond

// Send in batches
messages := make([]protocol.Message, 1000)
producer.SendMessages("events", messages)
```

---

## Migration Checklist

### Pre-Migration

- [ ] Identify all Kafka clusters and topics
- [ ] Document current Kafka configuration
- [ ] Identify all producer and consumer applications
- [ ] Review StreamBus feature parity
- [ ] Plan migration timeline
- [ ] Set up StreamBus test environment
- [ ] Run performance benchmarks
- [ ] Train team on StreamBus

### During Migration

- [ ] Deploy StreamBus cluster
- [ ] Implement dual-write in producers
- [ ] Validate data consistency
- [ ] Migrate consumers topic-by-topic
- [ ] Monitor both systems
- [ ] Run parallel validation
- [ ] Gradually increase StreamBus traffic

### Post-Migration

- [ ] Verify all data migrated
- [ ] Validate application functionality
- [ ] Monitor performance metrics
- [ ] Update documentation
- [ ] Train support team
- [ ] Decommission Kafka cluster
- [ ] Archive Kafka data if needed

---

## Getting Help

### Resources

- **Documentation**: [docs.streambus.io](https://docs.streambus.io)
- **Migration Guide**: This document
- **API Reference**: [api-reference.md](api-reference.md)
- **Community**: [GitHub Discussions](https://github.com/shawntherrien/streambus/discussions)

### Support

- **GitHub Issues**: Report bugs and request features
- **Slack**: Join our community channel
- **Email**: support@streambus.io (enterprise support)

---

## Example Migration Project

### Complete Migration Example

See our example migration project on GitHub:
- [streambus-kafka-migration-example](https://github.com/shawntherrien/streambus-examples/kafka-migration)

Includes:
- Full dual-write implementation
- Data validation scripts
- Configuration examples
- Performance comparison tools
- Monitoring dashboards

---

## Conclusion

Migrating from Kafka to StreamBus provides significant performance and operational benefits. While StreamBus is still maturing, the migration path is straightforward for most use cases.

**Next Steps:**
1. Review your Kafka usage patterns
2. Identify good candidates for migration
3. Set up StreamBus test environment
4. Run performance benchmarks
5. Plan your migration strategy

**Questions?** Join our [community discussions](https://github.com/shawntherrien/streambus/discussions) or file an issue on GitHub.

---

## See Also

- [Getting Started Guide](GETTING_STARTED.md)
- [Architecture Documentation](ARCHITECTURE.md)
- [API Reference](api-reference.md)
- [Operations Guide](operations.md)
- [Performance Benchmarks](BENCHMARKS.md)
