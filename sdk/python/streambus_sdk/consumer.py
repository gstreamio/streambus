"""StreamBus Consumer implementation."""

import json
import logging
import socket
import struct
import time
import zlib
from datetime import datetime
from typing import Optional, Callable, Iterator, Any

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
        fetch_timeout: float = 5.0
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
        """
        self.broker = broker
        self.port = port
        self.topic = topic
        self.partition = partition
        self.offset = start_offset
        self.fetch_timeout = fetch_timeout

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

    def fetch(self, max_bytes: int = 1024 * 1024) -> list[Message]:
        """
        Fetch messages from the current offset.

        Args:
            max_bytes: Maximum bytes to fetch per request

        Returns:
            List of Message objects
        """
        if not self._connected:
            self.connect()

        try:
            # Build fetch request
            request_data = self._encode_fetch_request(
                self.topic,
                self.partition,
                self.offset,
                max_bytes
            )

            # Send request
            self._socket.sendall(request_data)

            # Receive response header
            header_data = self._recv_exactly(16)
            length, request_id, status, version, flags = struct.unpack('>IQBBH', header_data)

            # Receive rest of message
            remaining_length = length - 12
            if remaining_length > 4:
                rest_data = self._recv_exactly(remaining_length)
                payload_data = rest_data[:-4]
                received_crc = struct.unpack('>I', rest_data[-4:])[0]

                # Verify CRC32
                crc_data = header_data[4:] + payload_data
                calculated_crc = zlib.crc32(crc_data)
                if received_crc != calculated_crc:
                    raise ValueError(f"CRC mismatch: expected {calculated_crc}, got {received_crc}")

                # Check status
                if status != 0:  # StatusCode.OK
                    error_msg = "Unknown error"
                    if len(payload_data) >= 4:
                        msg_len = struct.unpack('>I', payload_data[:4])[0]
                        if len(payload_data) >= 4 + msg_len:
                            error_msg = payload_data[4:4+msg_len].decode('utf-8', errors='replace')
                    raise RuntimeError(f"Fetch request failed: {error_msg}")

                # Decode messages
                messages = self._decode_fetch_response(payload_data)

                # Update offset for next fetch
                if messages:
                    self.offset = messages[-1].offset + 1

                return messages

            return []

        except socket.timeout:
            logger.debug("Fetch timeout - no new messages")
            return []
        except Exception as e:
            logger.error(f"Fetch error: {e}")
            self.close()
            raise

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
        message_count = 0

        try:
            while True:
                if max_messages is not None and message_count >= max_messages:
                    break

                messages = self.fetch()

                if not messages:
                    time.sleep(0.1)
                    continue

                for msg in messages:
                    handler(msg)
                    message_count += 1

                    if max_messages is not None and message_count >= max_messages:
                        break

        except KeyboardInterrupt:
            logger.info("Consumer interrupted by user")
        finally:
            self.close()

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
            while True:
                if max_messages is not None and message_count >= max_messages:
                    break

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

    def _encode_fetch_request(self, topic: str, partition_id: int, offset: int, max_bytes: int) -> bytes:
        """Encode a fetch request."""
        payload = bytearray()

        # Topic
        topic_bytes = topic.encode('utf-8')
        payload.extend(struct.pack('>I', len(topic_bytes)))
        payload.extend(topic_bytes)

        # Partition ID
        payload.extend(struct.pack('>I', partition_id))

        # Offset
        payload.extend(struct.pack('>Q', offset))

        # Max bytes
        payload.extend(struct.pack('>I', max_bytes))

        # Build header
        request_id = self._request_id
        self._request_id += 1

        total_length = 8 + 1 + 1 + 2 + len(payload) + 4

        header = struct.pack(
            '>IQBBH',
            total_length,
            request_id,
            0x02,  # FETCH request type
            1,     # Protocol version
            0      # Flags
        )

        # Calculate CRC32
        crc_data = header[4:] + payload
        crc32 = zlib.crc32(crc_data)

        return header + payload + struct.pack('>I', crc32)

    def _decode_fetch_response(self, payload: bytes) -> list[Message]:
        """Decode a fetch response."""
        offset = 0
        messages = []

        # Message count
        if len(payload) < 4:
            return messages

        msg_count = struct.unpack('>I', payload[offset:offset+4])[0]
        offset += 4

        # Decode messages
        for _ in range(msg_count):
            # Offset
            msg_offset = struct.unpack('>Q', payload[offset:offset+8])[0]
            offset += 8

            # Key
            key_len = struct.unpack('>I', payload[offset:offset+4])[0]
            offset += 4
            key = payload[offset:offset+key_len] if key_len > 0 else None
            offset += key_len

            # Value
            val_len = struct.unpack('>I', payload[offset:offset+4])[0]
            offset += 4
            value = payload[offset:offset+val_len] if val_len > 0 else b''
            offset += val_len

            # Timestamp (nanoseconds)
            timestamp_ns = struct.unpack('>Q', payload[offset:offset+8])[0]
            offset += 8
            timestamp = datetime.fromtimestamp(timestamp_ns / 1_000_000_000)

            # Headers (skip for now)
            header_count = struct.unpack('>I', payload[offset:offset+4])[0]
            offset += 4

            for _ in range(header_count):
                hdr_key_len = struct.unpack('>I', payload[offset:offset+4])[0]
                offset += 4 + hdr_key_len
                hdr_val_len = struct.unpack('>I', payload[offset:offset+4])[0]
                offset += 4 + hdr_val_len

            messages.append(Message(
                topic=self.topic,
                partition=self.partition,
                offset=msg_offset,
                key=key,
                value=value,
                timestamp=timestamp
            ))

        return messages

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
            except Exception as e:
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
