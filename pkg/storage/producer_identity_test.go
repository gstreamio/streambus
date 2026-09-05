package storage

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

// newFormatTestLog creates a log in a temp directory with the given write
// format version pinned via Config.MessageFormatVersion.
func newFormatTestLog(t *testing.T, version MessageFormatVersion) Log {
	t.Helper()

	config := *DefaultConfig()
	config.MessageFormatVersion = version

	log, err := NewLog(t.TempDir(), config)
	if err != nil {
		t.Fatalf("NewLog failed: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	return log
}

// TestLog_Append_StampsProducerIdentityFromBatch verifies the fix at the
// heart of this feature: MessageBatch.ProducerID/ProducerEpoch used to be
// read nowhere - Append wrote each message without ever copying them down
// from the batch, so the record format had nothing to carry and a
// read-committed fetch could never tell which transaction a record
// belonged to. Every message in a batch must come back stamped with the
// batch's producer identity.
func TestLog_Append_StampsProducerIdentityFromBatch(t *testing.T) {
	log := newHeaderTestLog(t)

	if _, err := log.Append(&MessageBatch{
		Messages: []Message{
			{Value: []byte("v1")},
			{Value: []byte("v2")},
		},
		Timestamp:     time.Now(),
		ProducerID:    4242,
		ProducerEpoch: 7,
	}); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	messages, err := log.ReadRange(0, 2)
	if err != nil {
		t.Fatalf("ReadRange failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("read %d messages, want 2", len(messages))
	}
	for i, msg := range messages {
		if msg.ProducerID != 4242 {
			t.Errorf("message %d ProducerID = %d, want 4242", i, msg.ProducerID)
		}
		if msg.ProducerEpoch != 7 {
			t.Errorf("message %d ProducerEpoch = %d, want 7", i, msg.ProducerEpoch)
		}
	}
}

// TestLog_Append_NonTransactionalBatchStampsZeroProducerID verifies the
// sentinel: a batch with no ProducerID set (the ordinary, non-transactional
// case) must persist ProducerID 0, not merely leave it unset in memory - a
// restart must see the same sentinel a live process would.
func TestLog_Append_NonTransactionalBatchStampsZeroProducerID(t *testing.T) {
	log := newHeaderTestLog(t)

	if _, err := log.Append(&MessageBatch{
		Messages:  []Message{{Value: []byte("v")}},
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	messages, err := log.ReadRange(0, 1)
	if err != nil {
		t.Fatalf("ReadRange failed: %v", err)
	}
	if messages[0].ProducerID != 0 || messages[0].ProducerEpoch != 0 {
		t.Errorf("ProducerID/Epoch = %d/%d, want 0/0", messages[0].ProducerID, messages[0].ProducerEpoch)
	}
}

// TestSerializeMessageV3_RoundTrip exercises the wire format directly:
// producer identity is appended after the header section, and must survive
// alongside key, value, timestamp and headers.
func TestSerializeMessageV3_RoundTrip(t *testing.T) {
	timestamp := time.Now().Truncate(time.Nanosecond)
	original := &Message{
		Key:           []byte("k"),
		Value:         []byte("v"),
		Headers:       map[string][]byte{"h": []byte("1")},
		Timestamp:     timestamp,
		ProducerID:    123456789,
		ProducerEpoch: -1, // producer epochs are signed; a fenced sentinel is legitimate input
	}

	data := serializeMessageV3(original)
	got := deserializeMessageV3(data)

	if !bytes.Equal(got.Key, original.Key) {
		t.Errorf("Key = %q, want %q", got.Key, original.Key)
	}
	if !bytes.Equal(got.Value, original.Value) {
		t.Errorf("Value = %q, want %q", got.Value, original.Value)
	}
	if !got.Timestamp.Equal(timestamp) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, timestamp)
	}
	if len(got.Headers) != 1 || !bytes.Equal(got.Headers["h"], []byte("1")) {
		t.Errorf("Headers = %v, want {h: 1}", got.Headers)
	}
	if got.ProducerID != original.ProducerID {
		t.Errorf("ProducerID = %d, want %d", got.ProducerID, original.ProducerID)
	}
	if got.ProducerEpoch != original.ProducerEpoch {
		t.Errorf("ProducerEpoch = %d, want %d", got.ProducerEpoch, original.ProducerEpoch)
	}
}

// TestDeserializeMessage_V2RecordHasZeroProducerIdentity is the backward
// compatibility half of the format change: a record written before v3
// existed has no producer fields on disk at all, and must not be
// misinterpreted as belonging to some producer - it must decode to the same
// zero sentinel as an ordinary non-transactional v3 record.
func TestDeserializeMessage_V2RecordHasZeroProducerIdentity(t *testing.T) {
	v2Data := serializeMessageV2(&Message{
		Key:       []byte("k"),
		Value:     []byte("v"),
		Timestamp: time.Now(),
	})

	log := &logImpl{}
	got := log.deserializeMessage(v2Data)

	if got.ProducerID != 0 || got.ProducerEpoch != 0 {
		t.Errorf("ProducerID/Epoch = %d/%d, want 0/0 for a v2 record", got.ProducerID, got.ProducerEpoch)
	}
	if !bytes.Equal(got.Key, []byte("k")) || !bytes.Equal(got.Value, []byte("v")) {
		t.Errorf("Key/Value = %q/%q, want k/v", got.Key, got.Value)
	}
}

// TestDeserializeMessageV3_TruncatedRecord mirrors
// TestDeserializeMessageV2_TruncatedRecord for the v3 format: every prefix of
// a v3 record must parse without panicking, including truncations that fall
// inside the trailing producer-identity fields that v2 doesn't have.
func TestDeserializeMessageV3_TruncatedRecord(t *testing.T) {
	full := serializeMessageV3(&Message{
		Key:           []byte("k"),
		Value:         []byte("v"),
		Headers:       map[string][]byte{"h": []byte("1")},
		Timestamp:     time.Now(),
		ProducerID:    99,
		ProducerEpoch: 3,
	})

	for cut := 0; cut < len(full); cut++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("parsing a %d-byte prefix panicked: %v", cut, r)
				}
			}()
			_ = deserializeMessageV3(full[:cut])
		}()
	}
}

// TestLog_Append_ProducerIdentitySurvivesReopen exercises the same
// close-then-reopen pattern as TestLog_Reopen, but for producer identity
// specifically: it is only meaningful for read-committed filtering if it
// actually reaches disk, not just the in-memory Message passed to Append.
func TestLog_Append_ProducerIdentitySurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	config := *DefaultConfig()

	log, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("NewLog failed: %v", err)
	}

	if _, err := log.Append(&MessageBatch{
		Messages:      []Message{{Value: []byte("v")}},
		Timestamp:     time.Now(),
		ProducerID:    777,
		ProducerEpoch: 2,
	}); err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	log2, err := NewLog(dir, config)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer log2.Close()

	messages, err := log2.ReadRange(0, 1)
	if err != nil {
		t.Fatalf("ReadRange after reopen failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("read %d messages after reopen, want 1", len(messages))
	}
	if messages[0].ProducerID != 777 || messages[0].ProducerEpoch != 2 {
		t.Errorf("ProducerID/Epoch after reopen = %d/%d, want 777/2",
			messages[0].ProducerID, messages[0].ProducerEpoch)
	}
}

// TestLogImpl_SerializeMessage_DefaultIsV3 pins the default write format:
// with Config.MessageFormatVersion left at its zero value (MessageFormatUnset,
// the only state a caller who never heard of this setting can be in), Append
// must keep writing v3. Regressing the default to v2 would silently reopen
// the transactional-isolation gap v3 was built to close, for every existing
// caller that never opts in to anything.
func TestLogImpl_SerializeMessage_DefaultIsV3(t *testing.T) {
	l := &logImpl{} // zero-value config: MessageFormatVersion == MessageFormatUnset
	if l.config.MessageFormatVersion != MessageFormatUnset {
		t.Fatalf("precondition failed: zero-value Config.MessageFormatVersion = %v, want MessageFormatUnset", l.config.MessageFormatVersion)
	}

	data := l.serializeMessage(&Message{Value: []byte("v"), Timestamp: time.Now()})

	if got := newFormatVersion(data); got != recordVersionV3 {
		t.Errorf("newFormatVersion(default-written record) = %d, want %d (v3)", got, recordVersionV3)
	}
}

// TestLogImpl_SerializeMessage_V2ConfiguredWritesV2ReadableFormat verifies
// the rolling-upgrade path this setting exists for: pinning
// Config.MessageFormatVersion to MessageFormatV2 makes Append write records
// an old broker - one that has never heard of v3 - can still decode, using
// exactly the v2 decoder (not merely "some format v3 also happens to read").
func TestLogImpl_SerializeMessage_V2ConfiguredWritesV2ReadableFormat(t *testing.T) {
	l := &logImpl{config: Config{MessageFormatVersion: MessageFormatV2}}

	msg := &Message{
		Key:       []byte("k"),
		Value:     []byte("v"),
		Headers:   map[string][]byte{"h": []byte("1")},
		Timestamp: time.Now().Truncate(time.Nanosecond),
	}
	data := l.serializeMessage(msg)

	if got := newFormatVersion(data); got != recordVersionV2 {
		t.Fatalf("newFormatVersion(v2-configured record) = %d, want %d (v2)", got, recordVersionV2)
	}

	// An old-reader decode, not the version-dispatched deserializeMessage -
	// this is what actually stands in for a pre-v3 broker reading the file.
	got := deserializeMessageV2(data)
	if !bytes.Equal(got.Key, msg.Key) || !bytes.Equal(got.Value, msg.Value) {
		t.Errorf("Key/Value = %q/%q, want %q/%q", got.Key, got.Value, msg.Key, msg.Value)
	}
	if len(got.Headers) != 1 || !bytes.Equal(got.Headers["h"], []byte("1")) {
		t.Errorf("Headers = %v, want {h: 1}", got.Headers)
	}

	// The full round trip through the log's own (version-dispatched) reader
	// must also come back clean, with the sentinel zero producer identity -
	// v2 has nowhere to have stored anything else.
	roundTripped := l.deserializeMessage(data)
	if roundTripped.ProducerID != 0 || roundTripped.ProducerEpoch != 0 {
		t.Errorf("ProducerID/Epoch = %d/%d, want 0/0 for a v2-written record", roundTripped.ProducerID, roundTripped.ProducerEpoch)
	}
}

// TestLog_Append_TransactionalRecordRejectedUnderV2 is the correctness trap
// this setting creates and must not paper over: v2 has no field for producer
// identity, so a transactional batch (nonzero ProducerID) cannot be written
// under v2 without silently losing what read_committed needs to keep hiding
// that transaction's records if it is later aborted. Append must refuse
// rather than write the record with its producer identity quietly dropped.
func TestLog_Append_TransactionalRecordRejectedUnderV2(t *testing.T) {
	log := newFormatTestLog(t, MessageFormatV2)

	_, err := log.Append(&MessageBatch{
		Messages:      []Message{{Value: []byte("v")}},
		Timestamp:     time.Now(),
		ProducerID:    555,
		ProducerEpoch: 1,
	})

	if !errors.Is(err, ErrTransactionalRecordNeedsV3) {
		t.Fatalf("Append error = %v, want ErrTransactionalRecordNeedsV3", err)
	}

	// Confirm the rejection is atomic: nothing from the rejected batch was
	// written, so the log is still empty at offset 0.
	messages, err := log.ReadRange(0, 1)
	if err != nil {
		t.Fatalf("ReadRange failed: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("read %d messages after a rejected Append, want 0", len(messages))
	}
}

// TestLog_Append_NonTransactionalBatchAllowedUnderV2 confirms the v2 gate is
// scoped to transactional records specifically: an ordinary batch (the
// ProducerID-0 sentinel) has no producer identity to lose, so it must still
// be writable while the log is pinned to v2.
func TestLog_Append_NonTransactionalBatchAllowedUnderV2(t *testing.T) {
	log := newFormatTestLog(t, MessageFormatV2)

	if _, err := log.Append(&MessageBatch{
		Messages:  []Message{{Value: []byte("v")}},
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("Append of a non-transactional batch under v2 failed: %v", err)
	}

	messages, err := log.ReadRange(0, 1)
	if err != nil {
		t.Fatalf("ReadRange failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("read %d messages, want 1", len(messages))
	}
	if messages[0].ProducerID != 0 || messages[0].ProducerEpoch != 0 {
		t.Errorf("ProducerID/Epoch = %d/%d, want 0/0", messages[0].ProducerID, messages[0].ProducerEpoch)
	}
}

// TestLog_Append_V3ConfiguredRoundTripsProducerIdentity mirrors the v2 tests
// above for the explicit (not merely default) v3 case: a log configured for
// MessageFormatV3 must round-trip producer identity exactly like the default.
func TestLog_Append_V3ConfiguredRoundTripsProducerIdentity(t *testing.T) {
	log := newFormatTestLog(t, MessageFormatV3)

	if _, err := log.Append(&MessageBatch{
		Messages:      []Message{{Value: []byte("v")}},
		Timestamp:     time.Now(),
		ProducerID:    9001,
		ProducerEpoch: 4,
	}); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	messages, err := log.ReadRange(0, 1)
	if err != nil {
		t.Fatalf("ReadRange failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("read %d messages, want 1", len(messages))
	}
	if messages[0].ProducerID != 9001 || messages[0].ProducerEpoch != 4 {
		t.Errorf("ProducerID/Epoch = %d/%d, want 9001/4", messages[0].ProducerID, messages[0].ProducerEpoch)
	}
}

// TestParseMessageFormatVersion covers the config-string parsing cmd/broker
// uses to validate storage.message_format_version at startup: valid spellings
// map to their version, an empty string means "unset" (use the default), and
// anything else - in particular a typo - is rejected rather than silently
// falling back to the default.
func TestParseMessageFormatVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    MessageFormatVersion
		wantErr bool
	}{
		{"", MessageFormatUnset, false},
		{"v2", MessageFormatV2, false},
		{"V2", MessageFormatV2, false},
		{" v2 ", MessageFormatV2, false},
		{"v3", MessageFormatV3, false},
		{"V3", MessageFormatV3, false},
		{"v4", MessageFormatUnset, true},
		{"3", MessageFormatUnset, true},
		{"vv3", MessageFormatUnset, true},
	}

	for _, tt := range tests {
		got, err := ParseMessageFormatVersion(tt.in)
		if tt.wantErr {
			if !errors.Is(err, ErrInvalidMessageFormatVersion) {
				t.Errorf("ParseMessageFormatVersion(%q) error = %v, want ErrInvalidMessageFormatVersion", tt.in, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMessageFormatVersion(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseMessageFormatVersion(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
