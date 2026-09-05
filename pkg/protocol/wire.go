package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// ErrMalformedPayload is returned when a payload is truncated or declares a
// length that does not fit within the remaining bytes.
var ErrMalformedPayload = errors.New("protocol: malformed payload")

// payloadWriter writes protocol fields into a byte buffer.
//
// A writer with a nil buffer runs in measuring mode: it advances its position
// without writing anything, so the same encode function can produce both the
// exact payload size and the payload itself. That is what keeps the size
// calculation and the encoding from drifting apart as fields are added.
type payloadWriter struct {
	buf []byte
	pos int
}

// newSizer returns a writer that only measures.
func newSizer() *payloadWriter {
	return &payloadWriter{}
}

// newWriter returns a writer that fills buf starting at pos.
func newWriter(buf []byte, pos int) *payloadWriter {
	return &payloadWriter{buf: buf, pos: pos}
}

// Len returns the current write position.
func (w *payloadWriter) Len() int {
	return w.pos
}

// reserve advances the position by n and returns the slice to write into, or
// nil in measuring mode.
func (w *payloadWriter) reserve(n int) []byte {
	start := w.pos
	w.pos += n
	if w.buf == nil {
		return nil
	}
	return w.buf[start : start+n]
}

func (w *payloadWriter) writeInt8(v int8) {
	if b := w.reserve(1); b != nil {
		b[0] = byte(v)
	}
}

func (w *payloadWriter) writeBool(v bool) {
	var b int8
	if v {
		b = 1
	}
	w.writeInt8(b)
}

func (w *payloadWriter) writeInt16(v int16) {
	if b := w.reserve(2); b != nil {
		binary.BigEndian.PutUint16(b, uint16(v))
	}
}

func (w *payloadWriter) writeInt32(v int32) {
	if b := w.reserve(4); b != nil {
		binary.BigEndian.PutUint32(b, uint32(v))
	}
}

func (w *payloadWriter) writeInt64(v int64) {
	if b := w.reserve(8); b != nil {
		binary.BigEndian.PutUint64(b, uint64(v))
	}
}

// writeString writes a 4-byte length prefix followed by the string bytes.
func (w *payloadWriter) writeString(s string) {
	w.writeInt32(int32(len(s)))
	if b := w.reserve(len(s)); b != nil {
		copy(b, s)
	}
}

// writeBytes writes a 4-byte length prefix followed by the raw bytes. A nil
// slice is written as length -1 so it round-trips back to nil rather than to
// an empty slice, which matters for optional protocol fields.
func (w *payloadWriter) writeBytes(data []byte) {
	if data == nil {
		w.writeInt32(-1)
		return
	}
	w.writeInt32(int32(len(data)))
	if b := w.reserve(len(data)); b != nil {
		copy(b, data)
	}
}

// payloadReader reads protocol fields from a byte buffer.
//
// Errors are sticky: once a read fails, every subsequent read is a no-op and
// Err keeps reporting the first failure. Callers therefore only need to check
// Err once, after decoding a whole payload, instead of after every field.
type payloadReader struct {
	buf []byte
	pos int
	err error
}

func newReader(buf []byte) *payloadReader {
	return &payloadReader{buf: buf}
}

// Err returns the first error encountered, if any.
func (r *payloadReader) Err() error {
	return r.err
}

// fail records an error unless one is already recorded.
func (r *payloadReader) fail(format string, args ...interface{}) {
	if r.err == nil {
		r.err = fmt.Errorf("%w: %s", ErrMalformedPayload, fmt.Sprintf(format, args...))
	}
}

// take returns the next n bytes, or nil if they are not available.
func (r *payloadReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.buf) {
		r.fail("need %d bytes at offset %d, have %d", n, r.pos, len(r.buf)-r.pos)
		return nil
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *payloadReader) readInt8() int8 {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return int8(b[0])
}

func (r *payloadReader) readBool() bool {
	return r.readInt8() != 0
}

func (r *payloadReader) readInt16() int16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return int16(binary.BigEndian.Uint16(b))
}

func (r *payloadReader) readInt32() int32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return int32(binary.BigEndian.Uint32(b))
}

func (r *payloadReader) readInt64() int64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b))
}

func (r *payloadReader) readString() string {
	length := r.readInt32()
	if r.err != nil {
		return ""
	}
	if length < 0 {
		r.fail("negative string length %d", length)
		return ""
	}
	b := r.take(int(length))
	if b == nil {
		return ""
	}
	return string(b)
}

// readBytes reads a length-prefixed byte slice. A length of -1 decodes to nil,
// matching writeBytes.
func (r *payloadReader) readBytes() []byte {
	length := r.readInt32()
	if r.err != nil {
		return nil
	}
	if length == -1 {
		return nil
	}
	if length < 0 {
		r.fail("negative byte-slice length %d", length)
		return nil
	}
	b := r.take(int(length))
	if b == nil {
		return nil
	}
	// Copy so the decoded payload does not alias the connection read buffer,
	// which is pooled and reused for the next request.
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// readCount reads a collection length and rejects values that could not
// possibly be backed by the remaining bytes. Without this bound, a corrupt or
// hostile length would drive a huge allocation before the first element fails
// to decode.
func (r *payloadReader) readCount() int {
	n := r.readInt32()
	if r.err != nil {
		return 0
	}
	if n < 0 {
		r.fail("negative element count %d", n)
		return 0
	}
	// Every element occupies at least one byte on the wire.
	if int(n) > len(r.buf)-r.pos {
		r.fail("element count %d exceeds %d remaining bytes", n, len(r.buf)-r.pos)
		return 0
	}
	return int(n)
}

// sortedKeys returns a map's keys in sorted order, so encoders that walk a map
// produce byte-identical output for equal inputs.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
