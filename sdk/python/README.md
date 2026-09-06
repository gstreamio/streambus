# StreamBus Python SDK

A high-level Python SDK for StreamBus that provides simplified interfaces for producing and consuming messages.

Speaks the StreamBus binary protocol directly over a socket and depends on
nothing outside the standard library.

## Requirements

- Python 3.9 or newer
- A running StreamBus broker

## Installation

Not yet published to PyPI. Install from a checkout:

```bash
pip install ./sdk/python
```

Or, for development against the working tree:

```bash
pip install -e ./sdk/python
```

## Quick Start

### Producer

```python
from streambus_sdk import connect

client = connect("localhost", 9092)

producer = client.new_producer()

# Send a message; returns the offset it was written at
offset = producer.send("my-topic", b"key1", b"Hello, StreamBus!")
print(f"wrote at offset {offset}")

# Attach headers
producer.send("my-topic", b"key2", b"payload", headers={"tenant_id": b"acme"})

# Send JSON
producer.send_json("orders", "order-123", {
    "id": "order-123",
    "amount": 99.99,
    "status": "pending",
})

# Write a batch in a single request
result = producer.send_batch("my-topic", [
    {"key": b"a", "value": b"one"},
    {"key": b"b", "value": b"two", "headers": {"source": b"import"}},
])
print(f"base offset {result.base_offset}, {result.num_messages} written")

producer.close()
client.close()
```

### Consumer

```python
from streambus_sdk import connect

client = connect("localhost", 9092)

consumer = client.new_consumer("my-topic", partition=0, start_offset=0)

def process_message(msg):
    print(f"Received: {msg.value_as_str()}")
    print(f"Offset: {msg.offset}")
    print(f"Timestamp: {msg.timestamp}")
    print(f"Tenant: {msg.header_as_str('tenant_id')}")

consumer.consume(process_message)
```

### Reading only committed transactions

```python
from streambus_sdk import connect, ISOLATION_READ_COMMITTED

client = connect("localhost", 9092)
consumer = client.new_consumer(
    "my-topic",
    partition=0,
    start_offset=0,
    isolation_level=ISOLATION_READ_COMMITTED,
)
```

Under `ISOLATION_READ_COMMITTED` the broker withholds records belonging to
transactions that have not yet committed, and filters out those that aborted.

### Consumer with JSON

```python
def process_order(msg, data):
    print(f"Order {data['id']}: ${data['amount']}")

consumer.consume_json(process_order)
```

### Iterator-based consumption

```python
for msg in consumer.consume_iter(max_messages=10):
    print(f"Message: {msg.value_as_str()}")
```

## Features

- **Simple API**: Easy-to-use interface for producing and consuming messages
- **Headers**: Message headers round-trip, including `tenant_id`
- **Batching**: `send_batch` writes many messages in one request
- **Isolation levels**: read_uncommitted (default) and read_committed
- **JSON Support**: Built-in JSON serialization and deserialization
- **Context Managers**: Support for `with` statements for automatic cleanup
- **Type Hints**: Full type hints for better IDE support

## API Reference

### Client

- `connect(broker, port)` - Create a client connection
- `client.new_producer()` - Create a producer
- `client.new_consumer(topic, partition, start_offset, isolation_level)` - Create a consumer

### Producer

- `send(topic, key, value, partition=0, headers=None) -> int` - Send one message, returns its offset
- `send_batch(topic, messages, partition=0) -> ProduceResult` - Send several in one request
- `send_json(topic, key, value, partition=0) -> int` - Send a JSON message
- `close()` - Close the producer

### Consumer

- `fetch(max_bytes=1MB) -> list[Message]` - Fetch a batch of messages
- `consume(handler, max_messages=None)` - Consume messages with a handler function
- `consume_iter(max_messages=None)` - Consume messages as an iterator
- `consume_json(handler, max_messages=None)` - Consume and parse JSON messages
- `seek(offset)` / `seek_to_beginning()` - Reposition the consumer
- `close()` - Close the consumer

After a fetch, `consumer.high_water_mark` and `consumer.last_stable_offset`
report where the partition stands.

### Message

- `value_as_str()` / `key_as_str()` - Decode payload or key as text
- `header_as_str(name)` - Decode one header value, or `None` if absent
- `headers` - Raw `dict[str, bytes]`, since header values are bytes on the wire

### Errors

- `BrokerError` - The broker answered with a non-OK status; carries `status` and `error_code`
- `ProtocolError` - A frame was malformed or failed its CRC check

## Testing

```bash
pip install ./sdk/python[test]
pytest sdk/python/tests
```

The wire tests are pinned to byte-for-byte fixtures taken from the Go codec in
`pkg/protocol/codec.go`, so a divergence between this SDK and the broker fails
the suite rather than showing up as corrupt messages at runtime.
