package storage

import (
	"bytes"
	"testing"
	"time"
)

// newHeaderTestLog creates a log in a temp directory.
func newHeaderTestLog(t *testing.T) Log {
	t.Helper()

	log, err := NewLog(t.TempDir(), *DefaultConfig())
	if err != nil {
		t.Fatalf("NewLog failed: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })

	return log
}

func TestMessageHeaders_SurviveRoundTrip(t *testing.T) {
	log := newHeaderTestLog(t)

	headers := map[string][]byte{
		"tenant_id":         []byte("acme"),
		"streambus.control": []byte("txn-marker"),
		"binary":            {0xff, 0x00, 0x01},
	}

	if _, err := log.Append(&MessageBatch{
		Messages: []Message{{
			Key:       []byte("k"),
			Value:     []byte("v"),
			Headers:   headers,
			Timestamp: time.Now(),
		}},
		Timestamp: time.Now(),
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

	got := messages[0]
	if !bytes.Equal(got.Key, []byte("k")) {
		t.Errorf("key = %q, want k", got.Key)
	}
	if !bytes.Equal(got.Value, []byte("v")) {
		t.Errorf("value = %q, want v", got.Value)
	}
	if len(got.Headers) != len(headers) {
		t.Fatalf("read %d headers, want %d", len(got.Headers), len(headers))
	}
	for name, want := range headers {
		if !bytes.Equal(got.Headers[name], want) {
			t.Errorf("header %q = %v, want %v", name, got.Headers[name], want)
		}
	}
}

func TestMessageHeaders_AbsentWhenNoneWritten(t *testing.T) {
	log := newHeaderTestLog(t)

	if _, err := log.Append(&MessageBatch{
		Messages:  []Message{{Value: []byte("v"), Timestamp: time.Now()}},
		Timestamp: time.Now(),
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
	if messages[0].Headers != nil {
		t.Errorf("headers = %v, want nil for a message written without any", messages[0].Headers)
	}
}

func TestMessageHeaders_TimestampPreserved(t *testing.T) {
	log := newHeaderTestLog(t)

	timestamp := time.Now().Truncate(time.Nanosecond)

	if _, err := log.Append(&MessageBatch{
		Messages: []Message{{
			Value:     []byte("v"),
			Headers:   map[string][]byte{"h": []byte("1")},
			Timestamp: timestamp,
		}},
		Timestamp: timestamp,
	}); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	messages, err := log.ReadRange(0, 1)
	if err != nil {
		t.Fatalf("ReadRange failed: %v", err)
	}
	if !messages[0].Timestamp.Equal(timestamp) {
		t.Errorf("timestamp = %v, want %v", messages[0].Timestamp, timestamp)
	}
}

func TestExtractTimestamp_AllRecordFormats(t *testing.T) {
	timestamp := time.Now()

	withHeaders := serializeMessageV2(&Message{
		Value:     []byte("v"),
		Headers:   map[string][]byte{"h": []byte("1")},
		Timestamp: timestamp,
	})
	if got := extractTimestamp(withHeaders); !got.Equal(timestamp) {
		t.Errorf("v2 timestamp = %v, want %v", got, timestamp)
	}

	// A v1 record: timestamp, then key and value.
	log := &logImpl{}
	withoutHeaders := log.serializeMessage(&Message{Value: []byte("v"), Timestamp: timestamp})
	if got := extractTimestamp(withoutHeaders); !got.Equal(timestamp) {
		t.Errorf("v1 timestamp = %v, want %v", got, timestamp)
	}
}

func TestDeserializeMessageV2_TruncatedRecord(t *testing.T) {
	full := serializeMessageV2(&Message{
		Key:       []byte("k"),
		Value:     []byte("v"),
		Headers:   map[string][]byte{"h": []byte("1")},
		Timestamp: time.Now(),
	})

	// Every prefix must parse without panicking; a corrupt record yields
	// whatever was readable rather than crashing the broker.
	for cut := 0; cut < len(full); cut++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("parsing a %d-byte prefix panicked: %v", cut, r)
				}
			}()
			_ = deserializeMessageV2(full[:cut])
		}()
	}
}
