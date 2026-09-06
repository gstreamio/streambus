"""Binary wire protocol for StreamBus.

This module is the single place the Python SDK encodes or decodes frames.
Producer and Consumer previously carried their own copies of the framing
code, which is how they came to disagree with each other and with the
broker.

Layouts mirror ``pkg/protocol/codec.go`` in the StreamBus repository.
Request and response headers are *not* the same shape:

    Request:   Length(4) RequestID(8) Type(1) Version(1) Flags(2)
    Response:  Length(4) RequestID(8) Status(1) ErrorCode(2)

In both cases ``Length`` counts every byte after itself, including the
trailing CRC32, and the CRC32 covers everything after the length field.
"""

import struct
import zlib
from typing import Dict, List, NamedTuple, Optional, Tuple

# Request types (pkg/protocol/types.go)
REQUEST_PRODUCE = 0x01
REQUEST_FETCH = 0x02
REQUEST_GET_OFFSET = 0x03
REQUEST_CREATE_TOPIC = 0x04
REQUEST_DELETE_TOPIC = 0x05

PROTOCOL_VERSION = 1

# Response status codes
STATUS_OK = 0
STATUS_ERROR = 1
STATUS_PARTIAL_SUCCESS = 2

# Isolation levels (pkg/protocol/isolation.go)
ISOLATION_READ_UNCOMMITTED = 0
ISOLATION_READ_COMMITTED = 1

# Header sizes excluding the 4-byte length prefix, and excluding the CRC.
_REQUEST_HEADER_REST = 12  # RequestID(8) Type(1) Version(1) Flags(2)
_RESPONSE_HEADER_REST = 11  # RequestID(8) Status(1) ErrorCode(2)
_CRC_SIZE = 4

# A fetch response whose server predates the NextOffset field reports this
# sentinel, meaning "fall back to last-message-offset + 1".
NEXT_OFFSET_UNSET = -1


class ProtocolError(Exception):
    """Raised when a frame cannot be parsed or fails its checksum."""


class BrokerError(Exception):
    """Raised when the broker returns a non-OK status."""

    def __init__(self, message: str, status: int, error_code: int):
        super().__init__(message)
        self.status = status
        self.error_code = error_code


class WireMessage(NamedTuple):
    """A message exactly as it appears on the wire."""

    offset: int
    timestamp: int  # nanoseconds since the Unix epoch
    key: Optional[bytes]
    value: bytes
    headers: Dict[str, bytes]


class ProduceResult(NamedTuple):
    base_offset: int
    num_messages: int
    high_water_mark: int


class FetchResult(NamedTuple):
    high_water_mark: int
    messages: List[WireMessage]
    last_stable_offset: int
    next_offset: int


def _put_bytes(buf: bytearray, data: bytes) -> None:
    """Append a 4-byte big-endian length followed by the bytes themselves."""
    buf.extend(struct.pack(">I", len(data)))
    buf.extend(data)


def _take_bytes(buf: bytes, offset: int) -> Tuple[bytes, int]:
    """Read a 4-byte length-prefixed byte string, returning it and the new offset."""
    if offset + 4 > len(buf):
        raise ProtocolError("truncated frame: no room for a length prefix")
    (n,) = struct.unpack_from(">I", buf, offset)
    offset += 4
    if offset + n > len(buf):
        raise ProtocolError(f"truncated frame: declared {n} bytes, {len(buf) - offset} remain")
    return buf[offset:offset + n], offset + n


def encode_message(msg: WireMessage) -> bytes:
    """Encode one message.

    Field order is Offset, Timestamp, Key, Value, Headers - matching
    ``Codec.encodeMessage``. The offset is server-assigned and ignored on
    produce, but the field is still present on the wire and omitting it
    shifts every subsequent field.
    """
    buf = bytearray()
    buf.extend(struct.pack(">q", msg.offset))
    buf.extend(struct.pack(">q", msg.timestamp))
    _put_bytes(buf, msg.key or b"")
    _put_bytes(buf, msg.value or b"")
    headers = msg.headers or {}
    buf.extend(struct.pack(">I", len(headers)))
    for key, value in headers.items():
        _put_bytes(buf, key.encode("utf-8") if isinstance(key, str) else key)
        _put_bytes(buf, value.encode("utf-8") if isinstance(value, str) else value)
    return bytes(buf)


def decode_message(buf: bytes, offset: int) -> Tuple[WireMessage, int]:
    """Decode one message, returning it and the offset just past it."""
    if offset + 16 > len(buf):
        raise ProtocolError("truncated message: no room for offset and timestamp")
    (msg_offset,) = struct.unpack_from(">q", buf, offset)
    offset += 8
    (timestamp,) = struct.unpack_from(">q", buf, offset)
    offset += 8

    key, offset = _take_bytes(buf, offset)
    value, offset = _take_bytes(buf, offset)

    if offset + 4 > len(buf):
        raise ProtocolError("truncated message: no room for header count")
    (num_headers,) = struct.unpack_from(">I", buf, offset)
    offset += 4

    headers: Dict[str, bytes] = {}
    for _ in range(num_headers):
        header_key, offset = _take_bytes(buf, offset)
        header_value, offset = _take_bytes(buf, offset)
        headers[header_key.decode("utf-8", errors="replace")] = header_value

    return WireMessage(
        offset=msg_offset,
        timestamp=timestamp,
        key=key if key else None,
        value=value,
        headers=headers,
    ), offset


def encode_request(request_id: int, request_type: int, payload: bytes) -> bytes:
    """Wrap a payload in a request frame with its length prefix and CRC32."""
    length = _REQUEST_HEADER_REST + len(payload) + _CRC_SIZE
    header = struct.pack(
        ">IQBBH",
        length,
        request_id,
        request_type,
        PROTOCOL_VERSION,
        0,  # flags
    )
    crc = zlib.crc32(header[4:] + payload) & 0xFFFFFFFF
    return header + payload + struct.pack(">I", crc)


def encode_produce_request(
    request_id: int,
    topic: str,
    partition: int,
    messages: List[WireMessage],
) -> bytes:
    payload = bytearray()
    _put_bytes(payload, topic.encode("utf-8"))
    payload.extend(struct.pack(">I", partition))
    payload.extend(struct.pack(">I", len(messages)))
    for msg in messages:
        payload.extend(encode_message(msg))
    return encode_request(request_id, REQUEST_PRODUCE, bytes(payload))


def encode_fetch_request(
    request_id: int,
    topic: str,
    partition: int,
    offset: int,
    max_bytes: int,
    isolation_level: int = ISOLATION_READ_UNCOMMITTED,
) -> bytes:
    payload = bytearray()
    _put_bytes(payload, topic.encode("utf-8"))
    payload.extend(struct.pack(">I", partition))
    payload.extend(struct.pack(">q", offset))
    payload.extend(struct.pack(">I", max_bytes))
    payload.extend(struct.pack(">b", isolation_level))
    return encode_request(request_id, REQUEST_FETCH, bytes(payload))


def read_response(recv_exactly) -> Tuple[int, bytes]:
    """Read one response frame.

    ``recv_exactly(n)`` must return exactly n bytes or raise. Returns the
    request ID and the response payload, having already verified the CRC and
    raised BrokerError for a non-OK status.
    """
    length_bytes = recv_exactly(4)
    (length,) = struct.unpack(">I", length_bytes)
    if length < _RESPONSE_HEADER_REST + _CRC_SIZE:
        raise ProtocolError(f"response frame too short: {length} bytes")

    body = recv_exactly(length)

    received_crc = struct.unpack(">I", body[-_CRC_SIZE:])[0]
    calculated_crc = zlib.crc32(body[:-_CRC_SIZE]) & 0xFFFFFFFF
    if received_crc != calculated_crc:
        raise ProtocolError(
            f"CRC mismatch: computed {calculated_crc}, frame declared {received_crc}"
        )

    request_id, status, error_code = struct.unpack_from(">QBH", body, 0)
    payload = body[_RESPONSE_HEADER_REST:-_CRC_SIZE]

    if status != STATUS_OK:
        message = "unknown error"
        try:
            text, _ = _take_bytes(payload, 0)
            message = text.decode("utf-8", errors="replace")
        except ProtocolError:
            pass
        raise BrokerError(message, status, error_code)

    return request_id, payload


def decode_produce_response(payload: bytes) -> ProduceResult:
    """Decode a produce response: BaseOffset, NumMessages, HighWaterMark."""
    if len(payload) < 20:
        raise ProtocolError(f"produce response too short: {len(payload)} bytes")
    base_offset, num_messages, high_water_mark = struct.unpack_from(">qIq", payload, 0)
    return ProduceResult(base_offset, num_messages, high_water_mark)


def decode_fetch_response(payload: bytes) -> FetchResult:
    """Decode a fetch response.

    Layout is HighWaterMark, NumMessages, the messages, then
    LastStableOffset and NextOffset. The trailing pair was added after the
    original layout; a server that predates it simply sends a shorter
    payload, which is reported as no additional constraint.
    """
    if len(payload) < 12:
        raise ProtocolError(f"fetch response too short: {len(payload)} bytes")

    (high_water_mark,) = struct.unpack_from(">q", payload, 0)
    offset = 8
    (num_messages,) = struct.unpack_from(">I", payload, offset)
    offset += 4

    messages: List[WireMessage] = []
    for _ in range(num_messages):
        msg, offset = decode_message(payload, offset)
        messages.append(msg)

    last_stable_offset = high_water_mark
    next_offset = NEXT_OFFSET_UNSET
    if offset + 16 <= len(payload):
        last_stable_offset, next_offset = struct.unpack_from(">qq", payload, offset)

    return FetchResult(high_water_mark, messages, last_stable_offset, next_offset)
