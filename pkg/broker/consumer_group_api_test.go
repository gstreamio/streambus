package broker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/metadata"
	"github.com/gstreamio/streambus/pkg/storage"
)

// writeTestMessages creates a topic in broker storage and appends messages to
// partition 0, returning the messages it wrote.
func writeTestMessages(t *testing.T, b *Broker, topic string, partitions uint32, msgs []storage.Message) {
	t.Helper()

	if err := b.topicManager.CreateTopic(topic, partitions); err != nil {
		t.Fatalf("Failed to create topic in storage: %v", err)
	}

	partition, err := b.topicManager.GetPartition(topic, 0)
	if err != nil {
		t.Fatalf("Failed to get partition: %v", err)
	}

	if _, err := partition.Log().Append(&storage.MessageBatch{
		Messages:  msgs,
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("Failed to append messages: %v", err)
	}
}

// TestHandleTopicMessages_GetMessages tests that GET /api/v1/topics/:name/messages
// returns messages actually present in the partition log.
func TestHandleTopicMessages_GetMessages(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	ctx := context.Background()
	_ = metaStore.CreateTopic(ctx, "test-topic", 3, 1, metadata.DefaultTopicConfig())

	writeTestMessages(t, broker, "test-topic", 3, []storage.Message{
		{Key: []byte("k0"), Value: []byte("v0"), Timestamp: time.Now()},
		{Key: []byte("k1"), Value: []byte("v1"), Timestamp: time.Now()},
		{Key: []byte("k2"), Value: []byte("v2"), Timestamp: time.Now()},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/test-topic/messages", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d (%s)", w.Code, w.Body.String())
	}

	var messages []MessageInfo
	if err := json.NewDecoder(w.Body).Decode(&messages); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(messages))
	}

	for i, msg := range messages {
		wantKey := fmt.Sprintf("k%d", i)
		wantValue := fmt.Sprintf("v%d", i)
		if msg.Key != wantKey {
			t.Errorf("message %d: key = %q, want %q", i, msg.Key, wantKey)
		}
		if msg.Value != wantValue {
			t.Errorf("message %d: value = %q, want %q", i, msg.Value, wantValue)
		}
		if msg.Offset != int64(i) {
			t.Errorf("message %d: offset = %d, want %d", i, msg.Offset, i)
		}
		if msg.Timestamp == 0 {
			t.Errorf("message %d: timestamp not populated", i)
		}
	}
}

// TestHandleTopicMessages_EmptyTopic verifies an existing but empty partition
// returns an empty array rather than an error.
func TestHandleTopicMessages_EmptyTopic(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	ctx := context.Background()
	_ = metaStore.CreateTopic(ctx, "empty-topic", 1, 1, metadata.DefaultTopicConfig())
	writeTestMessages(t, broker, "empty-topic", 1, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/empty-topic/messages", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d (%s)", w.Code, w.Body.String())
	}

	var messages []MessageInfo
	if err := json.NewDecoder(w.Body).Decode(&messages); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(messages))
	}
}

// TestHandleTopicMessages_UnknownTopic verifies a topic with no partition log
// is reported as not found rather than as an empty result.
func TestHandleTopicMessages_UnknownTopic(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/nope/messages", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestHandleTopicMessages_WithQueryParams tests that partition, offset and
// limit are honoured rather than ignored.
func TestHandleTopicMessages_WithQueryParams(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	ctx := context.Background()
	_ = metaStore.CreateTopic(ctx, "test-topic", 3, 1, metadata.DefaultTopicConfig())

	msgs := make([]storage.Message, 0, 20)
	for i := 0; i < 20; i++ {
		msgs = append(msgs, storage.Message{
			Key:       []byte(fmt.Sprintf("k%d", i)),
			Value:     []byte(fmt.Sprintf("v%d", i)),
			Timestamp: time.Now(),
		})
	}
	writeTestMessages(t, broker, "test-topic", 3, msgs)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/topics/test-topic/messages?partition=0&offset=10&limit=5", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d (%s)", w.Code, w.Body.String())
	}

	var messages []MessageInfo
	if err := json.NewDecoder(w.Body).Decode(&messages); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(messages) != 5 {
		t.Fatalf("limit=5 should return 5 messages, got %d", len(messages))
	}
	if messages[0].Offset != 10 {
		t.Errorf("offset=10 should start at offset 10, got %d", messages[0].Offset)
	}
	if messages[4].Offset != 14 {
		t.Errorf("last message offset = %d, want 14", messages[4].Offset)
	}
}

// TestHandleTopicMessages_OffsetPastEnd verifies reading past the log end
// returns an empty array rather than an error.
func TestHandleTopicMessages_OffsetPastEnd(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	ctx := context.Background()
	_ = metaStore.CreateTopic(ctx, "test-topic", 1, 1, metadata.DefaultTopicConfig())
	writeTestMessages(t, broker, "test-topic", 1, []storage.Message{
		{Value: []byte("only"), Timestamp: time.Now()},
	})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/topics/test-topic/messages?offset=9999", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d (%s)", w.Code, w.Body.String())
	}

	var messages []MessageInfo
	if err := json.NewDecoder(w.Body).Decode(&messages); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages past the end of the log, got %d", len(messages))
	}
}

// TestHandleTopicMessages_BinaryPayload verifies non-UTF-8 payloads survive as
// base64 rather than being mangled into replacement characters.
func TestHandleTopicMessages_BinaryPayload(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	ctx := context.Background()
	_ = metaStore.CreateTopic(ctx, "bin-topic", 1, 1, metadata.DefaultTopicConfig())

	binary := []byte{0xff, 0xfe, 0x00, 0x01}
	writeTestMessages(t, broker, "bin-topic", 1, []storage.Message{
		{Value: binary, Timestamp: time.Now()},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/bin-topic/messages", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d (%s)", w.Code, w.Body.String())
	}

	var messages []MessageInfo
	if err := json.NewDecoder(w.Body).Decode(&messages); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0].Encoding != "base64" {
		t.Errorf("Expected encoding=base64 for binary payload, got %q", messages[0].Encoding)
	}
	decoded, err := base64.StdEncoding.DecodeString(messages[0].Value)
	if err != nil {
		t.Fatalf("Value was not valid base64: %v", err)
	}
	if !bytes.Equal(decoded, binary) {
		t.Errorf("Round-tripped payload = %v, want %v", decoded, binary)
	}
}

// TestHandleTopicMessages_InvalidQueryParams verifies malformed parameters are
// rejected rather than silently defaulted.
func TestHandleTopicMessages_InvalidQueryParams(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"non-numeric partition", "?partition=abc"},
		{"negative offset", "?offset=-1"},
		{"non-numeric limit", "?limit=lots"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker, metaStore := newTestBrokerWithMetaStore(t)
			ctx := context.Background()
			_ = metaStore.CreateTopic(ctx, "test-topic", 1, 1, metadata.DefaultTopicConfig())
			writeTestMessages(t, broker, "test-topic", 1, nil)

			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/topics/test-topic/messages"+tt.query, nil)
			w := httptest.NewRecorder()

			broker.handleTopicOperations(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

// TestHandleTopicOperations_MissingTopicName tests topic operations without topic name
func TestHandleTopicOperations_MissingTopicName(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// TestHandleTopics_Create_DefaultValues tests creating topic with default values
func TestHandleTopics_Create_DefaultValues(t *testing.T) {
	broker, _ := newTestBrokerWithMetaStore(t)

	// Create topic with 0 partitions and 0 replication factor (should use defaults)
	reqBody := `{"name":"default-topic","num_partitions":0,"replication_factor":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTopics(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should default to 1 partition
	if resp["num_partitions"].(float64) != 1 {
		t.Errorf("Expected default num_partitions=1, got %v", resp["num_partitions"])
	}

	// Should default to 1 replication factor
	if resp["replication_factor"].(float64) != 1 {
		t.Errorf("Expected default replication_factor=1, got %v", resp["replication_factor"])
	}
}

// TestListTopics_NilMetaStore tests listing topics with nil metadata store
func TestListTopics_NilMetaStore(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics", nil)
	w := httptest.NewRecorder()

	broker.handleTopics(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

// TestCreateTopic_NilMetaStore tests creating topic with nil metadata store
func TestCreateTopic_NilMetaStore(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	reqBody := `{"name":"test-topic","num_partitions":3,"replication_factor":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	broker.handleTopics(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

// TestGetTopic_NilMetaStore tests getting topic with nil metadata store
func TestGetTopic_NilMetaStore(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/test-topic", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

// TestDeleteTopic_NilMetaStore tests deleting topic with nil metadata store
func TestDeleteTopic_NilMetaStore(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/topics/test-topic", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

// TestHandleTopicPartitions_NilMetaStore tests partitions endpoint with nil metadata store
func TestHandleTopicPartitions_NilMetaStore(t *testing.T) {
	broker := newTestBrokerForAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/test-topic/partitions", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}
