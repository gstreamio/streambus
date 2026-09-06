"""StreamBus Producer implementation."""

import json
import logging
import socket
import time
from typing import Any, Dict, List, Optional

from . import _wire
from ._wire import BrokerError, ProduceResult, ProtocolError, WireMessage

logger = logging.getLogger(__name__)


class Producer:
    """
    High-level StreamBus producer.

    Provides simplified methods for sending messages to StreamBus topics.
    """

    def __init__(self, broker: str, port: int, connect_timeout: float = 10.0):
        """
        Initialize producer.

        Args:
            broker: StreamBus broker address
            port: StreamBus broker port
            connect_timeout: Socket timeout in seconds
        """
        self.broker = broker
        self.port = port
        self.connect_timeout = connect_timeout
        self._socket: Optional[socket.socket] = None
        self._connected = False
        self._request_id = 1

        logger.info(f"Producer initialized: {broker}:{port}")

    def connect(self):
        """Connect to StreamBus broker."""
        if self._connected:
            return

        try:
            self._socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            self._socket.settimeout(self.connect_timeout)
            self._socket.connect((self.broker, self.port))
            self._connected = True
            logger.info(f"Producer connected to {self.broker}:{self.port}")
        except socket.error as e:
            logger.error(f"Failed to connect: {e}")
            raise ConnectionError(f"Failed to connect to {self.broker}:{self.port}: {e}")

    def _next_request_id(self) -> int:
        request_id = self._request_id
        self._request_id += 1
        return request_id

    def send(
        self,
        topic: str,
        key: Optional[bytes],
        value: bytes,
        partition: int = 0,
        headers: Optional[Dict[str, bytes]] = None,
    ) -> int:
        """
        Send a message to a topic.

        Args:
            topic: Topic name
            key: Message key (may be None)
            value: Message value
            partition: Partition to write to (default: 0)
            headers: Optional message headers

        Returns:
            int: Offset assigned to the message

        Example:
            >>> producer.send("orders", b"order-1", b'{"amount": 100}')
        """
        result = self.send_batch(
            topic,
            [{"key": key, "value": value, "headers": headers}],
            partition=partition,
        )
        return result.base_offset

    def send_batch(
        self,
        topic: str,
        messages: List[Dict[str, Any]],
        partition: int = 0,
    ) -> ProduceResult:
        """
        Send several messages to one partition in a single request.

        Args:
            topic: Topic name
            messages: Dicts with 'value' and optionally 'key', 'headers',
                'timestamp' (nanoseconds since the epoch)
            partition: Partition to write to (default: 0)

        Returns:
            ProduceResult: base offset, count written, and high water mark
        """
        if not messages:
            raise ValueError("send_batch requires at least one message")

        if not self._connected:
            self.connect()

        now = time.time_ns()
        wire_messages = [
            WireMessage(
                # Server-assigned; the field must still occupy its 8 bytes.
                offset=0,
                timestamp=msg.get("timestamp") or now,
                key=msg.get("key") or b"",
                value=msg.get("value") or b"",
                headers=msg.get("headers") or {},
            )
            for msg in messages
        ]

        request = _wire.encode_produce_request(
            self._next_request_id(), topic, partition, wire_messages
        )

        try:
            self._socket.sendall(request)
            _, payload = _wire.read_response(self._recv_exactly)
        except (BrokerError, ProtocolError):
            # The broker answered; the connection is still usable.
            raise
        except (socket.error, ConnectionError):
            # Framing state is unknown after a transport failure - a later
            # request would read this one's leftover bytes.
            self.close()
            raise

        return _wire.decode_produce_response(payload)

    def send_json(
        self,
        topic: str,
        key: Optional[str],
        value: Any,
        partition: int = 0,
    ) -> int:
        """
        Send a JSON message to a topic.

        Args:
            topic: Topic name
            key: Message key (encoded as UTF-8)
            value: Python object to serialize as JSON
            partition: Partition to write to (default: 0)

        Returns:
            int: Offset assigned to the message

        Example:
            >>> producer.send_json("orders", "order-1", {"amount": 100})
        """
        key_bytes = key.encode("utf-8") if key else b""
        value_bytes = json.dumps(value).encode("utf-8")
        return self.send(topic, key_bytes, value_bytes, partition=partition)

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
        """Close the producer."""
        if self._socket:
            try:
                self._socket.close()
            except OSError as e:
                logger.error(f"Error closing socket: {e}")
            finally:
                self._socket = None
                self._connected = False
                logger.info("Producer closed")

    def __enter__(self):
        """Context manager entry."""
        self.connect()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit."""
        self.close()
