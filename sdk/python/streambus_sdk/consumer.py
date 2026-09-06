"""StreamBus Consumer implementation."""

import json
import logging
import socket
import time
from datetime import datetime, timezone
from typing import Any, Callable, Iterator, List, Optional

from . import _wire
from ._wire import BrokerError, ProtocolError
from .message import Message

logger = logging.getLogger(__name__)


class Consumer:
    """
    High-level StreamBus consumer.

    Provides simplified methods for consuming messages from StreamBus topics.
    """

    def __init__(
        self,
        broker: str,
        port: int,
        topic: str,
        partition: int = 0,
        start_offset: int = 0,
        fetch_timeout: float = 5.0,
        isolation_level: int = _wire.ISOLATION_READ_UNCOMMITTED,
    ):
        """
        Initialize consumer.

        Args:
            broker: StreamBus broker address
            port: StreamBus broker port
            topic: Topic to consume from
            partition: Partition ID (default: 0)
            start_offset: Starting offset (default: 0)
            fetch_timeout: Timeout for fetch requests in seconds
            isolation_level: ISOLATION_READ_UNCOMMITTED (default) or
                ISOLATION_READ_COMMITTED to hide uncommitted transactions
        """
        self.broker = broker
        self.port = port
        self.topic = topic
        self.partition = partition
        self.offset = start_offset
        self.fetch_timeout = fetch_timeout
        self.isolation_level = isolation_level

        self.high_water_mark = 0
        self.last_stable_offset = 0

        self._socket: Optional[socket.socket] = None
        self._connected = False
        self._request_id = 1

        logger.info(
            f"Consumer initialized: {broker}:{port}, topic={topic}, "
            f"partition={partition}, offset={start_offset}"
        )

    def connect(self):
        """Connect to StreamBus broker."""
        if self._connected:
            return

        try:
            self._socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            self._socket.settimeout(self.fetch_timeout)
            self._socket.connect((self.broker, self.port))
            self._connected = True
            logger.info(f"Consumer connected to {self.broker}:{self.port}")
        except socket.error as e:
            logger.error(f"Failed to connect: {e}")
            raise ConnectionError(f"Failed to connect to {self.broker}:{self.port}: {e}")

    def _next_request_id(self) -> int:
        request_id = self._request_id
        self._request_id += 1
        return request_id

    def fetch(self, max_bytes: int = 1024 * 1024) -> List[Message]:
        """
        Fetch messages from the current offset.

        Args:
            max_bytes: Maximum bytes to fetch per request

        Returns:
            List of Message objects
        """
        if not self._connected:
            self.connect()

        request = _wire.encode_fetch_request(
            self._next_request_id(),
            self.topic,
            self.partition,
            self.offset,
            max_bytes,
            self.isolation_level,
        )

        try:
            self._socket.sendall(request)
            _, payload = _wire.read_response(self._recv_exactly)
        except socket.timeout:
            # The request went out but no reply arrived. A late response
            # would be read as the *next* request's reply, so the connection
            # cannot be reused.
            logger.debug("Fetch timed out; dropping the connection to stay in sync")
            self.close()
            return []
        except BrokerError:
            raise
        except (socket.error, ConnectionError, ProtocolError):
            self.close()
            raise

        result = _wire.decode_fetch_response(payload)

        self.high_water_mark = result.high_water_mark
        self.last_stable_offset = result.last_stable_offset

        # Advance using NextOffset where the broker supplies it. Control
        # records (transaction markers) are filtered out server-side, so a
        # window containing only a marker returns no messages but must still
        # move the consumer forward - deriving the next offset from the last
        # message would re-read that window forever.
        if result.next_offset != _wire.NEXT_OFFSET_UNSET:
            self.offset = result.next_offset
        elif result.messages:
            self.offset = result.messages[-1].offset + 1

        return [self._to_message(msg) for msg in result.messages]

    def _to_message(self, wire_msg: _wire.WireMessage) -> Message:
        """Convert a wire message into the public Message dataclass."""
        return Message(
            topic=self.topic,
            partition=self.partition,
            offset=wire_msg.offset,
            key=wire_msg.key,
            value=wire_msg.value,
            timestamp=datetime.fromtimestamp(wire_msg.timestamp / 1_000_000_000, tz=timezone.utc),
            headers=wire_msg.headers or None,
        )

    def consume(self, handler: Callable[[Message], None], max_messages: Optional[int] = None):
        """
        Consume messages continuously with a handler function.

        Args:
            handler: Function to call for each message
            max_messages: Maximum number of messages to consume (None for infinite)

        Example:
            >>> def process_message(msg):
            ...     print(f"Received: {msg.value_as_str()}")
            >>> consumer.consume(process_message)
        """
        try:
            for msg in self.consume_iter(max_messages):
                handler(msg)
        except KeyboardInterrupt:
            logger.info("Consumer interrupted by user")

    def consume_iter(self, max_messages: Optional[int] = None) -> Iterator[Message]:
        """
        Consume messages as an iterator.

        Args:
            max_messages: Maximum number of messages to consume (None for infinite)

        Yields:
            Message: Parsed message from StreamBus

        Example:
            >>> for msg in consumer.consume_iter():
            ...     print(f"Received: {msg.value_as_str()}")
        """
        message_count = 0

        try:
            while max_messages is None or message_count < max_messages:
                messages = self.fetch()

                if not messages:
                    time.sleep(0.1)
                    continue

                for msg in messages:
                    message_count += 1
                    yield msg

                    if max_messages is not None and message_count >= max_messages:
                        break

        except KeyboardInterrupt:
            logger.info("Consumer interrupted by user")
        finally:
            self.close()

    def consume_json(self, handler: Callable[[Message, Any], None], max_messages: Optional[int] = None):
        """
        Consume messages and automatically parse JSON values.

        Args:
            handler: Function to call with (message, parsed_json)
            max_messages: Maximum number of messages to consume

        Example:
            >>> def process_order(msg, data):
            ...     print(f"Order {data['id']}: ${data['amount']}")
            >>> consumer.consume_json(process_order)
        """
        def json_handler(msg: Message):
            try:
                data = json.loads(msg.value_as_str())
                handler(msg, data)
            except json.JSONDecodeError as e:
                logger.error(f"Failed to decode JSON: {e}")

        self.consume(json_handler, max_messages)

    def seek(self, offset: int):
        """
        Seek to a specific offset.

        Args:
            offset: Offset to seek to
        """
        self.offset = offset
        logger.info(f"Seeked to offset {offset}")

    def seek_to_beginning(self):
        """Seek to the beginning of the partition."""
        self.seek(0)

    def _recv_exactly(self, n: int) -> bytes:
        """Receive exactly n bytes."""
        data = bytearray()
        while len(data) < n:
            chunk = self._socket.recv(n - len(data))
            if not chunk:
                raise ConnectionError("Socket connection broken")
            data.extend(chunk)
        return bytes(data)

    def close(self):
        """Close the consumer."""
        if self._socket:
            try:
                self._socket.close()
            except OSError as e:
                logger.error(f"Error closing socket: {e}")
            finally:
                self._socket = None
                self._connected = False
                logger.info("Consumer closed")

    def __enter__(self):
        """Context manager entry."""
        self.connect()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit."""
        self.close()
