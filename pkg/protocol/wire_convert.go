package protocol

import (
	"encoding/binary"
	"fmt"
)

// bigEndian is the byte order every StreamBus wire field uses.
var bigEndian = binary.BigEndian

// Typed field helpers for values whose wire width differs from their Go type.
//
// Writing an ErrorCode (uint16) through writeInt16, or a slice length (int)
// through writeInt32, works in practice but hides an unchecked narrowing
// conversion at every call site. These helpers do the conversion once, in one
// place, with the bound stated explicitly - so the safety argument lives next
// to the check instead of being repeated in a comment at each field.

// maxArrayLen bounds a collection length on the wire.
//
// The codec already refuses to encode or decode a message larger than
// MaxMessageSize, and every element costs at least one byte, so a legitimate
// collection can never approach this. The explicit cap is what turns an
// unchecked int-to-int32 narrowing into a checked one.
const maxArrayLen = MaxMessageSize

// writeArrayLen writes a collection length.
//
// A length beyond maxArrayLen cannot be produced by any valid payload, so it
// is written as zero rather than silently wrapping to a negative count that
// the reader would reject with a confusing error. Callers that could plausibly
// hit the bound should check it themselves first.
func (w *payloadWriter) writeArrayLen(n int) {
	if n < 0 || n > maxArrayLen {
		w.writeInt32(0)
		return
	}
	w.writeInt32(int32(n))
}

// writeErrorCode writes an ErrorCode in its natural unsigned width.
func (w *payloadWriter) writeErrorCode(code ErrorCode) {
	w.writeUint16(uint16(code))
}

// readErrorCode reads an ErrorCode.
func (r *payloadReader) readErrorCode() ErrorCode {
	return ErrorCode(r.readUint16())
}

// writeUint16 writes an unsigned 16-bit value.
func (w *payloadWriter) writeUint16(v uint16) {
	if b := w.reserve(2); b != nil {
		bigEndian.PutUint16(b, v)
	}
}

// readUint16 reads an unsigned 16-bit value.
func (r *payloadReader) readUint16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return bigEndian.Uint16(b)
}

// checkedInt32 narrows an int for callers that must surface an out-of-range
// value as an error rather than clamping it.
func checkedInt32(n int) (int32, error) {
	if n < 0 || n > maxArrayLen {
		return 0, fmt.Errorf("%w: value %d out of range", ErrMalformedPayload, n)
	}
	return int32(n), nil
}
