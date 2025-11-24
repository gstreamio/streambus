package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gstreamio/streambus/pkg/metadata"
)

// TestHandleTopicMessages_GetMessages tests GET /api/v1/topics/:name/messages
func TestHandleTopicMessages_GetMessages(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	// Create a test topic
	ctx := context.Background()
	_ = metaStore.CreateTopic(ctx, "test-topic", 3, 1, metadata.DefaultTopicConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/test-topic/messages", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var messages []MessageInfo
	if err := json.NewDecoder(w.Body).Decode(&messages); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Currently returns empty array as storage API is not fully implemented
	if len(messages) != 0 {
		t.Errorf("Expected 0 messages (not implemented), got %d", len(messages))
	}
}

// TestHandleTopicMessages_WithQueryParams tests messages endpoint with query parameters
func TestHandleTopicMessages_WithQueryParams(t *testing.T) {
	broker, metaStore := newTestBrokerWithMetaStore(t)

	// Create a test topic
	ctx := context.Background()
	_ = metaStore.CreateTopic(ctx, "test-topic", 3, 1, metadata.DefaultTopicConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/test-topic/messages?partition=0&offset=10&limit=100", nil)
	w := httptest.NewRecorder()

	broker.handleTopicOperations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
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
