"""Wire protocol tests for the StreamBus Python SDK.

The response fixtures here are byte-for-byte output from the Go codec in
``pkg/protocol/codec.go``. They exist because the SDK previously disagreed
with the broker about field order - silently, since a mis-ordered frame still
parses into plausible-looking garbage - and only a fixture taken from the real
encoder can catch that.
"""

import io

import pytest

from streambus_sdk import _wire


def reader_for(blob: bytes):
    """Return a recv_exactly callable that reads from a fixed byte string."""
    buf = io.BytesIO(blob)

    def recv_exactly(n: int) -> bytes:
        data = buf.read(n)
        if len(data) != n:
            raise ConnectionError("short read")
        return data

    return recv_exactly


class TestMessageCodec:
    def test_message_round_trip(self):
        original = _wire.WireMessage(
            offset=42,
            timestamp=1234567890123456789,
            key=b"k1",
            value=b"hello",
            headers={"tenant_id": b"acme"},
        )
        encoded = _wire.encode_message(original)
        decoded, offset = _wire.decode_message(encoded, 0)

        assert offset == len(encoded)
        assert decoded == original

    def test_message_field_order_matches_go(self):
        """Offset and Timestamp lead, before Key and Value.

        Getting this backwards is exactly the bug that made the SDK
        incompatible with the broker.
        """
        msg = _wire.WireMessage(
            offset=1, timestamp=2, key=b"", value=b"", headers={}
        )
        encoded = _wire.encode_message(msg)

        assert encoded[0:8] == (1).to_bytes(8, "big")
        assert encoded[8:16] == (2).to_bytes(8, "big")
        # Then a zero-length key, a zero-length value, and a zero header count.
        assert encoded[16:28] == b"\x00" * 12

    def test_empty_key_decodes_as_none(self):
        msg = _wire.WireMessage(offset=0, timestamp=0, key=b"", value=b"v", headers={})
        decoded, _ = _wire.decode_message(_wire.encode_message(msg), 0)
        assert decoded.key is None

    def test_multiple_headers_round_trip(self):
        msg = _wire.WireMessage(
            offset=0, timestamp=0, key=None, value=b"v",
            headers={"a": b"1", "b": b"2", "c": b"3"},
        )
        decoded, _ = _wire.decode_message(_wire.encode_message(msg), 0)
        assert decoded.headers == {"a": b"1", "b": b"2", "c": b"3"}

    def test_truncated_message_raises(self):
        msg = _wire.WireMessage(offset=0, timestamp=0, key=b"k", value=b"v", headers={})
        encoded = _wire.encode_message(msg)
        with pytest.raises(_wire.ProtocolError):
            _wire.decode_message(encoded[:-3], 0)


class TestRequestFraming:
    def test_produce_request_header_layout(self):
        frame = _wire.encode_produce_request(7, "events", 2, [
            _wire.WireMessage(offset=0, timestamp=1, key=b"k", value=b"v", headers={})
        ])

        declared_length = int.from_bytes(frame[0:4], "big")
        # Length counts every byte after itself.
        assert declared_length == len(frame) - 4
        assert int.from_bytes(frame[4:12], "big") == 7
        assert frame[12] == _wire.REQUEST_PRODUCE
        assert frame[13] == _wire.PROTOCOL_VERSION

    def test_fetch_request_carries_isolation_level(self):
        frame = _wire.encode_fetch_request(
            9, "events", 0, 5, 1024, _wire.ISOLATION_READ_COMMITTED
        )
        # The isolation level is the final payload byte, just before the CRC.
        assert frame[-5] == _wire.ISOLATION_READ_COMMITTED

    def test_request_crc_covers_everything_after_length(self):
        import zlib

        frame = _wire.encode_produce_request(1, "t", 0, [
            _wire.WireMessage(offset=0, timestamp=0, key=None, value=b"x", headers={})
        ])
        body = frame[4:-4]
        assert int.from_bytes(frame[-4:], "big") == zlib.crc32(body) & 0xFFFFFFFF


class TestResponseParsing:
    # Emitted by the Go codec for ProduceResponse{BaseOffset:100,
    # NumMessages:3, HighWaterMark:103} with RequestID 7, StatusOK.
    PRODUCE_RESP = bytes.fromhex(
        "00000023000000000000000700000000000000000000640000000300000000000000677c00e331"
    )

    def test_produce_response(self):
        request_id, payload = _wire.read_response(reader_for(self.PRODUCE_RESP))
        result = _wire.decode_produce_response(payload)

        assert request_id == 7
        assert result.base_offset == 100
        assert result.num_messages == 3
        assert result.high_water_mark == 103

    def test_response_header_is_shorter_than_request_header(self):
        """A response header is 11 bytes past the length field, not 12.

        Reading it with the request layout swallows the first payload byte.
        """
        _, payload = _wire.read_response(reader_for(self.PRODUCE_RESP))
        # BaseOffset is the first payload field and must be intact.
        assert int.from_bytes(payload[0:8], "big") == 100

    def test_crc_mismatch_is_rejected(self):
        corrupted = bytearray(self.PRODUCE_RESP)
        corrupted[-1] ^= 0xFF
        with pytest.raises(_wire.ProtocolError, match="CRC mismatch"):
            _wire.read_response(reader_for(bytes(corrupted)))

    def test_error_status_raises_broker_error(self):
        # ErrorResponse{Message:"topic does not exist"}, StatusError,
        # ErrorCode 8 (ErrTopicNotFound), RequestID 11.
        import struct
        import zlib

        message = b"topic does not exist"
        payload = struct.pack(">I", len(message)) + message
        body = struct.pack(">QBH", 11, _wire.STATUS_ERROR, 8) + payload
        crc = zlib.crc32(body) & 0xFFFFFFFF
        body += struct.pack(">I", crc)
        frame = struct.pack(">I", len(body)) + body

        with pytest.raises(_wire.BrokerError) as excinfo:
            _wire.read_response(reader_for(frame))

        assert "topic does not exist" in str(excinfo.value)
        assert excinfo.value.status == _wire.STATUS_ERROR
        assert excinfo.value.error_code == 8


class TestFetchResponse:
    def _build(self, messages, high_water_mark=500, lso=480, next_offset=44):
        import struct
        import zlib

        payload = bytearray()
        payload.extend(struct.pack(">q", high_water_mark))
        payload.extend(struct.pack(">I", len(messages)))
        for msg in messages:
            payload.extend(_wire.encode_message(msg))
        payload.extend(struct.pack(">qq", lso, next_offset))

        body = struct.pack(">QBH", 9, _wire.STATUS_OK, 0) + bytes(payload)
        body += struct.pack(">I", zlib.crc32(body) & 0xFFFFFFFF)
        return struct.pack(">I", len(body)) + body

    def test_decodes_messages_and_trailing_offsets(self):
        frame = self._build([
            _wire.WireMessage(42, 1111, b"a", b"first", {"h": b"v"}),
            _wire.WireMessage(43, 2222, None, b"second", {}),
        ])
        _, payload = _wire.read_response(reader_for(frame))
        result = _wire.decode_fetch_response(payload)

        assert result.high_water_mark == 500
        assert result.last_stable_offset == 480
        assert result.next_offset == 44
        assert [m.offset for m in result.messages] == [42, 43]
        assert result.messages[0].headers == {"h": b"v"}
        assert result.messages[1].key is None

    def test_empty_fetch_still_reports_next_offset(self):
        """A window holding only a filtered control record returns no
        messages, but must still advance the consumer."""
        frame = self._build([], high_water_mark=10, lso=10, next_offset=7)
        _, payload = _wire.read_response(reader_for(frame))
        result = _wire.decode_fetch_response(payload)

        assert result.messages == []
        assert result.next_offset == 7

    def test_server_without_trailing_fields(self):
        """An older server sends a shorter payload; that is not an error."""
        import struct

        payload = struct.pack(">q", 99) + struct.pack(">I", 0)
        result = _wire.decode_fetch_response(payload)

        assert result.high_water_mark == 99
        assert result.last_stable_offset == 99
        assert result.next_offset == _wire.NEXT_OFFSET_UNSET
