"""StreamBus Producer implementation."""

import json
import logging
import socket
import struct
import time
import zlib
from typing import Optional, Any, Dict, List

logger = logging.getLogger(__name__)


class Producer:
    """
    High-level StreamBus producer.

    Provides simplified methods for sending messages to StreamBus topics.
    """

    def __init__(self, broker: str, port: int):
        """
        Initialize producer.

        Args:
            broker: StreamBus broker address
            port: StreamBus broker port
        """
        self.broker = broker
        self.port = port
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
            self._socket.connect((self.broker, self.port))
            self._connected = True
            logger.info(f"Producer connected to {self.broker}:{self.port}")
        except socket.error as e:
            logger.error(f"Failed to connect: {e}")
            raise ConnectionError(f"Failed to connect to {self.broker}:{self.port}: {e}")

    def send(self, topic: str, key: bytes, value: bytes) -> int:
        """
        Send a message to a topic.

        Args:
            topic: Topic name
            key: Message key
            value: Message value

        Returns:
            int: Message offset

        Example:
            >>> producer.send("orders", b"order-1", b'{"amount": 100}')
        """
        if not self._connected:
            self.connect()

        # Build message
        timestamp = int(time.time() * 1_000_000_000)  # nanoseconds
        message = {
            'key': key or b'',
            'value': value,
            'timestamp': timestamp
        }

        # Encode produce request
        request_data = self._encode_produce_request(topic, 0, [message])

        # Send request
        self._socket.sendall(request_data)

        # Receive response
        response = self._recv_response()

        # Parse response to get offset
        if len(response) >= 14:
            # Skip partition_id (4 bytes)
            offset = struct.unpack('>Q', response[4:12])[0]
            return offset

        return -1

    def send_json(self, topic: str, key: str, value: Any) -> int:
        """
        Send a JSON message to a topic.

        Args:
            topic: Topic name
            key: Message key (will be encoded as UTF-8)
            value: Python object to serialize as JSON

        Returns:
            int: Message offset

        Example:
            >>> producer.send_json("orders", "order-1", {"amount": 100, "status": "pending"})
        """
        key_bytes = key.encode('utf-8') if key else b''
        value_bytes = json.dumps(value).encode('utf-8')
        return self.send(topic, key_bytes, value_bytes)

    def _encode_produce_request(self, topic: str, partition_id: int, messages: List[Dict]) -> bytes:
        """Encode a produce request."""
        # Encode payload
        payload = bytearray()

        # Topic (length-prefixed string)
        topic_bytes = topic.encode('utf-8')
        payload.extend(struct.pack('>I', len(topic_bytes)))
        payload.extend(topic_bytes)

        # Partition ID
        payload.extend(struct.pack('>I', partition_id))

        # Message count
        payload.extend(struct.pack('>I', len(messages)))

        # Messages
        for msg in messages:
            # Key
            key = msg.get('key', b'')
            payload.extend(struct.pack('>I', len(key)))
            if key:
                payload.extend(key)

            # Value
            value = msg.get('value', b'')
            payload.extend(struct.pack('>I', len(value)))
            if value:
                payload.extend(value)

            # Timestamp
            payload.extend(struct.pack('>Q', msg['timestamp']))

            # Headers count (always 0 for now)
            payload.extend(struct.pack('>I', 0))

        # Build header
        request_id = self._request_id
        self._request_id += 1

        total_length = 8 + 1 + 1 + 2 + len(payload) + 4

        header = struct.pack(
            '>IQBBH',
            total_length,
            request_id,
            0x01,  # PRODUCE request type
            1,     # Protocol version
            0      # Flags
        )

        # Calculate CRC32
        crc_data = header[4:] + payload
        crc32 = zlib.crc32(crc_data)

        return header + payload + struct.pack('>I', crc32)

    def _recv_response(self) -> bytes:
        """Receive response from broker."""
        # Receive header (16 bytes)
        header_data = self._recv_exactly(16)

        # Parse header
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
                raise RuntimeError(f"Produce request failed: {error_msg}")

            return payload_data

        return b''

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
            except Exception as e:
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
