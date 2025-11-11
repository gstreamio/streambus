# StreamBus Python SDK

A high-level Python SDK for StreamBus that provides simplified interfaces for producing and consuming messages.

## Installation

Since this SDK is not yet published to PyPI, you can install it locally:

```bash
# From the streambus-tests directory
export PYTHONPATH="/Users/shawntherrien/Projects/streambus/sdk/python:$PYTHONPATH"
```

Or copy the `streambus_sdk` directory to your project.

## Quick Start

### Producer

```python
from streambus_sdk import connect

# Connect to StreamBus
client = connect("localhost", 9092)

# Create a producer
producer = client.new_producer()

# Send a simple message
producer.send("my-topic", b"key1", b"Hello, StreamBus!")

# Send JSON message
producer.send_json("orders", "order-123", {
    "id": "order-123",
    "amount": 99.99,
    "status": "pending"
})

# Clean up
producer.close()
client.close()
```

### Consumer

```python
from streambus_sdk import connect

# Connect to StreamBus
client = connect("localhost", 9092)

# Create a consumer
consumer = client.new_consumer("my-topic", partition=0, start_offset=0)

# Consume messages with a handler
def process_message(msg):
    print(f"Received: {msg.value_as_str()}")
    print(f"Offset: {msg.offset}")
    print(f"Timestamp: {msg.timestamp}")

consumer.consume(process_message)
```

### Consumer with JSON

```python
# Automatically parse JSON messages
def process_order(msg, data):
    print(f"Order {data['id']}: ${data['amount']}")

consumer.consume_json(process_order)
```

### Iterator-based consumption

```python
# Use as an iterator
for msg in consumer.consume_iter(max_messages=10):
    print(f"Message: {msg.value_as_str()}")
```

## Features

- **Simple API**: Easy-to-use interface for producing and consuming messages
- **JSON Support**: Built-in JSON serialization and deserialization
- **Context Managers**: Support for `with` statements for automatic cleanup
- **Type Hints**: Full type hints for better IDE support
- **Logging**: Comprehensive logging for debugging

## API Reference

### Client

- `connect(broker, port)` - Create a client connection
- `client.new_producer()` - Create a producer
- `client.new_consumer(topic, partition, start_offset)` - Create a consumer

### Producer

- `send(topic, key, value)` - Send a raw message
- `send_json(topic, key, value)` - Send a JSON message
- `close()` - Close the producer

### Consumer

- `fetch()` - Fetch a batch of messages
- `consume(handler)` - Consume messages with a handler function
- `consume_iter()` - Consume messages as an iterator
- `consume_json(handler)` - Consume and parse JSON messages
- `seek(offset)` - Seek to a specific offset
- `seek_to_beginning()` - Seek to the beginning
- `close()` - Close the consumer

## Examples

See the `streambus-tests/python/` directory for example usage.
