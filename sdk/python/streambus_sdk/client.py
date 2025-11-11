"""StreamBus Client implementation."""

import logging
from typing import Optional

from .producer import Producer
from .consumer import Consumer

logger = logging.getLogger(__name__)


class StreamBusClient:
    """
    High-level StreamBus client.

    Provides a simplified interface for creating producers and consumers.
    """

    def __init__(self, broker: str = "localhost", port: int = 9092):
        """
        Initialize StreamBus client.

        Args:
            broker: StreamBus broker address
            port: StreamBus broker port
        """
        self.broker = broker
        self.port = port
        self._closed = False

        logger.info(f"StreamBus client initialized: {broker}:{port}")

    def new_producer(self) -> Producer:
        """
        Create a new producer.

        Returns:
            Producer: A new producer instance
        """
        return Producer(self.broker, self.port)

    def new_consumer(self, topic: str, partition: int = 0, start_offset: int = 0) -> Consumer:
        """
        Create a new consumer.

        Args:
            topic: Topic to consume from
            partition: Partition ID (default: 0)
            start_offset: Starting offset (default: 0)

        Returns:
            Consumer: A new consumer instance
        """
        return Consumer(
            broker=self.broker,
            port=self.port,
            topic=topic,
            partition=partition,
            start_offset=start_offset
        )

    def close(self):
        """Close the client."""
        if not self._closed:
            self._closed = True
            logger.info("StreamBus client closed")

    def __enter__(self):
        """Context manager entry."""
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit."""
        self.close()


def connect(broker: str = "localhost", port: int = 9092) -> StreamBusClient:
    """
    Connect to StreamBus broker with default configuration.

    Args:
        broker: StreamBus broker address (default: "localhost")
        port: StreamBus broker port (default: 9092)

    Returns:
        StreamBusClient: Connected client instance

    Example:
        >>> client = connect("localhost", 9092)
        >>> producer = client.new_producer()
        >>> producer.send("my-topic", b"key", b"value")
    """
    return StreamBusClient(broker, port)
