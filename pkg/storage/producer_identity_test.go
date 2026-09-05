package storage

import (
	"bytes"
	"testing"
	"time"
)

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
