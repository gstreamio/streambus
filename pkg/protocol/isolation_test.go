package protocol

import (
	"bytes"
	"testing"
)

func TestCodec_FetchRequest_IsolationLevelRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		level IsolationLevel
	}{
		{"read_uncommitted", IsolationReadUncommitted},
		{"read_committed", IsolationReadCommitted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec := NewCodec()
			req := &Request{
				Header: RequestHeader{
					RequestID: 1,
					Type:      RequestTypeFetch,
					Version:   ProtocolVersion,
				},
				Payload: &FetchRequest{
					Topic:          "orders",
					PartitionID:    0,
					Offset:         42,
					MaxBytes:       1024,
					IsolationLevel: tt.level,
				},
			}

			buf := &bytes.Buffer{}
			if err := codec.EncodeRequest(buf, req); err != nil {
				t.Fatalf("EncodeRequest failed: %v", err)
			}

			decoded, err := codec.DecodeRequest(buf)
			if err != nil {
				t.Fatalf("DecodeRequest failed: %v", err)
			}

			got := decoded.Payload.(*FetchRequest)
			if got.IsolationLevel != tt.level {
				t.Errorf("IsolationLevel = %v, want %v", got.IsolationLevel, tt.level)
			}
		})
	}
}

// TestCodec_FetchRequest_DecodeToleratesMissingIsolationLevel verifies that a
// FetchRequest payload from a peer that predates isolation levels (one byte
// shorter than the current layout) decodes as IsolationReadUncommitted
// instead of panicking or erroring.
func TestCodec_FetchRequest_DecodeToleratesMissingIsolationLevel(t *testing.T) {
	codec := NewCodec()

	full := &Request{
		Header: RequestHeader{Type: RequestTypeFetch},
		Payload: &FetchRequest{
			Topic:          "orders",
			PartitionID:    0,
			Offset:         42,
			MaxBytes:       1024,
			IsolationLevel: IsolationReadCommitted,
		},
	}
	size, err := codec.calculateRequestPayloadSize(full)
	if err != nil {
		t.Fatalf("calculateRequestPayloadSize failed: %v", err)
	}

	buf := make([]byte, size)
	if _, err := codec.encodeRequestPayload(buf, 0, full); err != nil {
		t.Fatalf("encodeRequestPayload failed: %v", err)
	}

	// Truncate the trailing IsolationLevel byte to simulate an older peer's
	// shorter payload.
	shortened := buf[:len(buf)-1]

	decoded, err := codec.decodeRequestPayload(shortened, RequestTypeFetch)
	if err != nil {
		t.Fatalf("decodeRequestPayload failed on shortened payload: %v", err)
	}

	got := decoded.(*FetchRequest)
	if got.IsolationLevel != IsolationReadUncommitted {
		t.Errorf("IsolationLevel = %v, want IsolationReadUncommitted for a payload missing the field", got.IsolationLevel)
	}
	if got.Topic != "orders" || got.Offset != 42 || got.MaxBytes != 1024 {
		t.Errorf("unexpected fields decoded from shortened payload: %+v", got)
	}
}

func TestCodec_FetchResponse_LastStableOffsetAndNextOffsetRoundTrip(t *testing.T) {
	codec := NewCodec()
	resp := &Response{
		Header: ResponseHeader{RequestID: 1, Status: StatusOK},
		Payload: &FetchResponse{
			Topic:            "orders",
			PartitionID:      0,
			HighWaterMark:    100,
			LastStableOffset: 80,
			NextOffset:       55,
			Messages: []Message{
				{Offset: 50, Value: []byte("v")},
			},
		},
	}

	buf := &bytes.Buffer{}
	if err := codec.EncodeResponse(buf, resp); err != nil {
		t.Fatalf("EncodeResponse failed: %v", err)
	}

	decoded, err := codec.DecodeResponse(buf)
	if err != nil {
		t.Fatalf("DecodeResponse failed: %v", err)
	}
	if err := codec.DecodeResponsePayload(decoded, RequestTypeFetch); err != nil {
		t.Fatalf("DecodeResponsePayload failed: %v", err)
	}

	got := decoded.Payload.(*FetchResponse)
	if got.LastStableOffset != 80 {
		t.Errorf("LastStableOffset = %d, want 80", got.LastStableOffset)
	}
	if got.NextOffset != 55 {
		t.Errorf("NextOffset = %d, want 55", got.NextOffset)
	}
}

// TestCodec_FetchResponse_DecodeToleratesMissingTrailingFields verifies that
// a FetchResponse payload from a server that predates LastStableOffset and
// NextOffset decodes with the documented safe defaults instead of panicking:
// LastStableOffset falls back to HighWaterMark (no additional constraint),
// and NextOffset falls back to -1 (a sentinel telling the caller to use its
// legacy last-message-plus-one rule).
func TestCodec_FetchResponse_DecodeToleratesMissingTrailingFields(t *testing.T) {
	codec := NewCodec()

	full := &Response{
		Header: ResponseHeader{RequestID: 1, Status: StatusOK},
		Payload: &FetchResponse{
			HighWaterMark:    100,
			LastStableOffset: 80,
			NextOffset:       55,
			Messages:         []Message{{Offset: 50, Value: []byte("v")}},
		},
	}
	size, err := codec.calculateResponsePayloadSize(full)
	if err != nil {
		t.Fatalf("calculateResponsePayloadSize failed: %v", err)
	}
	buf := make([]byte, size)
	if _, err := codec.encodeResponsePayload(buf, 0, full); err != nil {
		t.Fatalf("encodeResponsePayload failed: %v", err)
	}

	// Drop both trailing 8-byte fields to simulate an older server's
	// shorter payload.
	shortened := buf[:len(buf)-16]

	decoded := &Response{
		Header:  ResponseHeader{Status: StatusOK},
		Payload: shortened,
	}
	if err := codec.DecodeResponsePayload(decoded, RequestTypeFetch); err != nil {
		t.Fatalf("DecodeResponsePayload failed on shortened payload: %v", err)
	}

	got := decoded.Payload.(*FetchResponse)
	if got.HighWaterMark != 100 {
		t.Fatalf("HighWaterMark = %d, want 100", got.HighWaterMark)
	}
	if got.LastStableOffset != got.HighWaterMark {
		t.Errorf("LastStableOffset = %d, want %d (HighWaterMark fallback)", got.LastStableOffset, got.HighWaterMark)
	}
	if got.NextOffset != -1 {
		t.Errorf("NextOffset = %d, want -1 (legacy-fallback sentinel)", got.NextOffset)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("Messages = %d, want 1", len(got.Messages))
	}
}

func TestCodec_ProduceRequest_ProducerIDRoundTrip(t *testing.T) {
	codec := NewCodec()
	req := &Request{
		Header: RequestHeader{Type: RequestTypeProduce},
		Payload: &ProduceRequest{
			Topic:         "orders",
			PartitionID:   0,
			Messages:      []Message{{Value: []byte("v")}},
			ProducerID:    1234,
			ProducerEpoch: 7,
		},
	}

	buf := &bytes.Buffer{}
	if err := codec.EncodeRequest(buf, req); err != nil {
		t.Fatalf("EncodeRequest failed: %v", err)
	}
	decoded, err := codec.DecodeRequest(buf)
	if err != nil {
		t.Fatalf("DecodeRequest failed: %v", err)
	}

	got := decoded.Payload.(*ProduceRequest)
	if got.ProducerID != 1234 {
		t.Errorf("ProducerID = %d, want 1234", got.ProducerID)
	}
	if got.ProducerEpoch != 7 {
		t.Errorf("ProducerEpoch = %d, want 7", got.ProducerEpoch)
	}
}

// TestCodec_ProduceRequest_DecodeToleratesMissingProducerFields verifies a
// ProduceRequest payload from a peer that predates ProducerID/ProducerEpoch
// decodes both as zero - the sentinel this codebase uses everywhere for "not
// a transactional batch" - rather than panicking on the shorter buffer.
func TestCodec_ProduceRequest_DecodeToleratesMissingProducerFields(t *testing.T) {
	codec := NewCodec()

	full := &Request{
		Header: RequestHeader{Type: RequestTypeProduce},
		Payload: &ProduceRequest{
			Topic:         "orders",
			PartitionID:   0,
			Messages:      []Message{{Value: []byte("v")}},
			ProducerID:    1234,
			ProducerEpoch: 7,
		},
	}
	size, err := codec.calculateRequestPayloadSize(full)
	if err != nil {
		t.Fatalf("calculateRequestPayloadSize failed: %v", err)
	}
	buf := make([]byte, size)
	if _, err := codec.encodeRequestPayload(buf, 0, full); err != nil {
		t.Fatalf("encodeRequestPayload failed: %v", err)
	}

	// Drop the trailing ProducerID (8 bytes) + ProducerEpoch (2 bytes).
	shortened := buf[:len(buf)-10]

	decoded, err := codec.decodeRequestPayload(shortened, RequestTypeProduce)
	if err != nil {
		t.Fatalf("decodeRequestPayload failed on shortened payload: %v", err)
	}

	got := decoded.(*ProduceRequest)
	if got.ProducerID != 0 {
		t.Errorf("ProducerID = %d, want 0 for a payload missing the field", got.ProducerID)
	}
	if got.ProducerEpoch != 0 {
		t.Errorf("ProducerEpoch = %d, want 0 for a payload missing the field", got.ProducerEpoch)
	}
	if len(got.Messages) != 1 {
		t.Errorf("Messages = %d, want 1", len(got.Messages))
	}
}

func TestIsControlRecord(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string][]byte
		want    bool
	}{
		{"nil headers", nil, false},
		{"no control header", map[string][]byte{"foo": []byte("bar")}, false},
		{
			"txn marker",
			map[string][]byte{ControlHeaderKey: []byte(ControlTypeTxnMarker)},
			true,
		},
		{
			"unrecognized control type",
			map[string][]byte{ControlHeaderKey: []byte("something-else")},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsControlRecord(tt.headers); got != tt.want {
				t.Errorf("IsControlRecord(%v) = %v, want %v", tt.headers, got, tt.want)
			}
		})
	}
}

// TestParseTransactionMarker_RoundTrip verifies ParseTransactionMarker
// inverts TransactionMarkerHeaders for both outcomes: partition transaction
// recovery (pkg/server) depends on this round trip to tell an aborted
// marker from a committed one when replaying a partition's log at startup.
func TestParseTransactionMarker_RoundTrip(t *testing.T) {
	tests := []struct {
		name          string
		producerID    int64
		producerEpoch int16
		commit        bool
	}{
		{"commit", 1000, 0, true},
		{"abort", 1000, 0, false},
		{"negative epoch", 42, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := TransactionMarkerHeaders(tt.producerID, tt.producerEpoch, tt.commit)

			gotID, gotEpoch, gotCommit, ok := ParseTransactionMarker(headers)
			if !ok {
				t.Fatalf("ParseTransactionMarker(%v) ok = false, want true", headers)
			}
			if gotID != tt.producerID {
				t.Errorf("producerID = %d, want %d", gotID, tt.producerID)
			}
			if gotEpoch != tt.producerEpoch {
				t.Errorf("producerEpoch = %d, want %d", gotEpoch, tt.producerEpoch)
			}
			if gotCommit != tt.commit {
				t.Errorf("commit = %v, want %v", gotCommit, tt.commit)
			}
		})
	}
}

// TestParseTransactionMarker_RejectsNonMarkersAndCorruptHeaders verifies ok
// is false - never a panic or a zero-value marker mistaken for a real one -
// whenever headers isn't an actual, well-formed transaction marker. Recovery
// (pkg/server's rebuildTransactionState) replays whatever is on disk, so a
// corrupt record must be recognized as unparseable rather than silently
// producing a plausible-looking (0, 0, false) marker.
func TestParseTransactionMarker_RejectsNonMarkersAndCorruptHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string][]byte
	}{
		{"nil headers", nil},
		{"ordinary data record", map[string][]byte{"tenant_id": []byte("acme")}},
		{
			"unparseable producer id",
			map[string][]byte{
				ControlHeaderKey:          []byte(ControlTypeTxnMarker),
				TxnProducerIDHeaderKey:    []byte("not-a-number"),
				TxnProducerEpochHeaderKey: []byte("0"),
				TxnCommitHeaderKey:        []byte("true"),
			},
		},
		{
			"unparseable commit flag",
			map[string][]byte{
				ControlHeaderKey:          []byte(ControlTypeTxnMarker),
				TxnProducerIDHeaderKey:    []byte("1000"),
				TxnProducerEpochHeaderKey: []byte("0"),
				TxnCommitHeaderKey:        []byte("not-a-bool"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, _, ok := ParseTransactionMarker(tt.headers); ok {
				t.Errorf("ParseTransactionMarker(%v) ok = true, want false", tt.headers)
			}
		})
	}
}
