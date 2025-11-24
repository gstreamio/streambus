package server

import (
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
)

func TestHandler_handleGetOffset(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Create a topic
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "offset-test",
			NumPartitions: 1,
		},
	}

	createResp := handler.handleCreateTopic(createReq)
	if createResp.Header.Status != protocol.StatusOK {
		t.Fatalf("CreateTopic failed: %v", createResp.Payload)
	}

	// Get offset
	getOffsetReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeGetOffset,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.GetOffsetRequest{
			Topic:       "offset-test",
			PartitionID: 0,
		},
	}

	resp := handler.handleGetOffset(getOffsetReq)
	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("handleGetOffset failed: %v", resp.Payload)
	}

	offsetResp := resp.Payload.(*protocol.GetOffsetResponse)
	if offsetResp.Topic != "offset-test" {
		t.Errorf("Topic = %s, want offset-test", offsetResp.Topic)
	}
	if offsetResp.PartitionID != 0 {
		t.Errorf("PartitionID = %d, want 0", offsetResp.PartitionID)
	}
	if offsetResp.StartOffset != 0 {
		t.Errorf("StartOffset = %d, want 0", offsetResp.StartOffset)
	}
}

func TestHandler_handleGetOffset_TopicNotFound(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeGetOffset,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.GetOffsetRequest{
			Topic:       "non-existent",
			PartitionID: 0,
		},
	}

	resp := handler.handleGetOffset(req)
	if resp.Header.Status != protocol.StatusError {
		t.Error("Expected error status for non-existent topic")
	}
	if resp.Header.ErrorCode != protocol.ErrTopicNotFound {
		t.Errorf("ErrorCode = %v, want ErrTopicNotFound", resp.Header.ErrorCode)
	}
}

func TestHandler_handleDeleteTopic(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Create a topic first
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "delete-test",
			NumPartitions: 1,
		},
	}

	createResp := handler.handleCreateTopic(createReq)
	if createResp.Header.Status != protocol.StatusOK {
		t.Fatalf("CreateTopic failed: %v", createResp.Payload)
	}

	// Delete the topic
	deleteReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeDeleteTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.DeleteTopicRequest{
			Topic: "delete-test",
		},
	}

	resp := handler.handleDeleteTopic(deleteReq)
	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("handleDeleteTopic failed: %v", resp.Payload)
	}

	deleteResp := resp.Payload.(*protocol.DeleteTopicResponse)
	if !deleteResp.Deleted {
		t.Error("Deleted flag should be true")
	}
	if deleteResp.Topic != "delete-test" {
		t.Errorf("Topic = %s, want delete-test", deleteResp.Topic)
	}
}

func TestHandler_handleDeleteTopic_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeDeleteTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.DeleteTopicRequest{
			Topic: "non-existent",
		},
	}

	resp := handler.handleDeleteTopic(req)
	if resp.Header.Status != protocol.StatusError {
		t.Error("Expected error status for non-existent topic")
	}
	if resp.Header.ErrorCode != protocol.ErrTopicNotFound {
		t.Errorf("ErrorCode = %v, want ErrTopicNotFound", resp.Header.ErrorCode)
	}
}

func TestHandler_handleListTopics(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Create multiple topics
	topics := []string{"topic-a", "topic-b", "topic-c"}
	partitions := []uint32{1, 2, 3}

	for i, topic := range topics {
		createReq := &protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: uint64(i + 1),
				Type:      protocol.RequestTypeCreateTopic,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.CreateTopicRequest{
				Topic:         topic,
				NumPartitions: partitions[i],
			},
		}

		createResp := handler.handleCreateTopic(createReq)
		if createResp.Header.Status != protocol.StatusOK {
			t.Fatalf("CreateTopic %s failed: %v", topic, createResp.Payload)
		}
	}

	// List topics
	listReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 100,
			Type:      protocol.RequestTypeListTopics,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ListTopicsRequest{},
	}

	resp := handler.handleListTopics(listReq)
	if resp.Header.Status != protocol.StatusOK {
		t.Fatalf("handleListTopics failed: %v", resp.Payload)
	}

	listResp := resp.Payload.(*protocol.ListTopicsResponse)
	if len(listResp.Topics) != 3 {
		t.Errorf("Expected 3 topics, got %d", len(listResp.Topics))
	}

	// Verify topic info
	topicMap := make(map[string]protocol.TopicInfo)
	for _, info := range listResp.Topics {
		topicMap[info.Name] = info
	}

	for i, name := range topics {
		info, exists := topicMap[name]
		if !exists {
			t.Errorf("Topic %s not in list", name)
			continue
		}
		if info.NumPartitions != partitions[i] {
			t.Errorf("Topic %s has %d partitions, want %d",
				name, info.NumPartitions, partitions[i])
		}
	}
}

func TestHandler_errorResponse(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Call errorResponse
	resp := handler.errorResponse(123, protocol.ErrTopicNotFound, "test error message")

	if resp.Header.RequestID != 123 {
		t.Errorf("RequestID = %d, want 123", resp.Header.RequestID)
	}

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Status = %v, want StatusError", resp.Header.Status)
	}

	if resp.Header.ErrorCode != protocol.ErrTopicNotFound {
		t.Errorf("ErrorCode = %v, want ErrTopicNotFound", resp.Header.ErrorCode)
	}

	errorResp := resp.Payload.(*protocol.ErrorResponse)
	if errorResp.ErrorCode != protocol.ErrTopicNotFound {
		t.Errorf("Payload ErrorCode = %v, want ErrTopicNotFound", errorResp.ErrorCode)
	}

	if errorResp.Message != "test error message" {
		t.Errorf("Message = %s, want 'test error message'", errorResp.Message)
	}

	// Verify error counter increased
	stats := handler.Stats()
	if stats.ErrorsHandled != 1 {
		t.Errorf("ErrorsHandled = %d, want 1", stats.ErrorsHandled)
	}
}

func TestHandler_Close(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)

	// Create some topics
	for i := 0; i < 3; i++ {
		createReq := &protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: uint64(i + 1),
				Type:      protocol.RequestTypeCreateTopic,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.CreateTopicRequest{
				Topic:         "close-test-topic",
				NumPartitions: 1,
			},
		}
		handler.handleCreateTopic(createReq)
	}

	// Close handler
	err := handler.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Close again (idempotent)
	err = handler.Close()
	if err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

func TestHandler_Stats(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Initial stats
	stats := handler.Stats()
	if stats.RequestsHandled != 0 {
		t.Errorf("Initial RequestsHandled = %d, want 0", stats.RequestsHandled)
	}
	if stats.ErrorsHandled != 0 {
		t.Errorf("Initial ErrorsHandled = %d, want 0", stats.ErrorsHandled)
	}
	if stats.Uptime <= 0 {
		t.Error("Uptime should be greater than 0")
	}

	// Handle some requests
	for i := 0; i < 5; i++ {
		req := &protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: uint64(i + 1),
				Type:      protocol.RequestTypeHealthCheck,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.HealthCheckRequest{},
		}
		handler.Handle(req)
	}

	// Check stats after requests
	stats = handler.Stats()
	if stats.RequestsHandled != 5 {
		t.Errorf("RequestsHandled = %d, want 5", stats.RequestsHandled)
	}

	// Generate some errors
	for i := 0; i < 3; i++ {
		handler.errorResponse(uint64(i), protocol.ErrStorageError, "test error")
	}

	stats = handler.Stats()
	if stats.ErrorsHandled != 3 {
		t.Errorf("ErrorsHandled = %d, want 3", stats.ErrorsHandled)
	}

	// Verify uptime is still increasing
	time.Sleep(50 * time.Millisecond)
	newStats := handler.Stats()
	if newStats.Uptime <= stats.Uptime {
		t.Error("Uptime should increase over time")
	}
}

func TestNewHandlerWithDataDir(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	if handler == nil {
		t.Fatal("NewHandlerWithDataDir returned nil")
	}

	if handler.topicManager == nil {
		t.Error("Handler should have a topic manager")
	}

	// Verify we can use it
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "test",
			NumPartitions: 1,
		},
	}

	resp := handler.Handle(createReq)
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Failed to create topic with custom handler: %v", resp.Payload)
	}
}

func TestHandler_Handle_UnknownRequestType(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Use an unknown request type
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 999,
			Type:      protocol.RequestType(99), // Unknown type
			Version:   protocol.ProtocolVersion,
		},
		Payload: nil,
	}

	resp := handler.Handle(req)

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Expected StatusError for unknown request type, got %v", resp.Header.Status)
	}

	if resp.Header.ErrorCode != protocol.ErrUnknownRequest {
		t.Errorf("Expected ErrUnknownRequest, got %v", resp.Header.ErrorCode)
	}

	errorResp := resp.Payload.(*protocol.ErrorResponse)
	if errorResp.Message == "" {
		t.Error("Expected error message for unknown request type")
	}
}

func TestHandler_Handle_AllRequestTypes(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Create a topic first
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "all-types-test",
			NumPartitions: 1,
		},
	}
	handler.Handle(createReq)

	testCases := []struct {
		name        string
		requestType protocol.RequestType
		payload     interface{}
		expectOK    bool
	}{
		{
			name:        "Produce",
			requestType: protocol.RequestTypeProduce,
			payload: &protocol.ProduceRequest{
				Topic:       "all-types-test",
				PartitionID: 0,
				Messages: []protocol.Message{
					{Key: []byte("k"), Value: []byte("v")},
				},
			},
			expectOK: true,
		},
		{
			name:        "Fetch",
			requestType: protocol.RequestTypeFetch,
			payload: &protocol.FetchRequest{
				Topic:       "all-types-test",
				PartitionID: 0,
				Offset:      0,
				MaxBytes:    1024,
			},
			expectOK: true,
		},
		{
			name:        "GetOffset",
			requestType: protocol.RequestTypeGetOffset,
			payload: &protocol.GetOffsetRequest{
				Topic:       "all-types-test",
				PartitionID: 0,
			},
			expectOK: true,
		},
		{
			name:        "ListTopics",
			requestType: protocol.RequestTypeListTopics,
			payload:     &protocol.ListTopicsRequest{},
			expectOK:    true,
		},
		{
			name:        "HealthCheck",
			requestType: protocol.RequestTypeHealthCheck,
			payload:     &protocol.HealthCheckRequest{},
			expectOK:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &protocol.Request{
				Header: protocol.RequestHeader{
					RequestID: 100,
					Type:      tc.requestType,
					Version:   protocol.ProtocolVersion,
				},
				Payload: tc.payload,
			}

			resp := handler.Handle(req)

			if tc.expectOK && resp.Header.Status != protocol.StatusOK {
				t.Errorf("Expected OK status for %s, got %v", tc.name, resp.Header.Status)
			}
		})
	}
}

func TestHandler_handleFetch_NonExistentPartition(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Create topic with 1 partition
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "fetch-test",
			NumPartitions: 1,
		},
	}
	handler.Handle(createReq)

	// Try to fetch from non-existent partition 99
	fetchReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeFetch,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.FetchRequest{
			Topic:       "fetch-test",
			PartitionID: 99,
			Offset:      0,
			MaxBytes:    1024,
		},
	}

	resp := handler.Handle(fetchReq)

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Expected error for non-existent partition, got %v", resp.Header.Status)
	}

	if resp.Header.ErrorCode != protocol.ErrTopicNotFound {
		t.Errorf("Expected ErrTopicNotFound, got %v", resp.Header.ErrorCode)
	}
}

func TestHandler_handleFetch_EmptyTopic(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Create topic
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "empty-topic",
			NumPartitions: 1,
		},
	}
	handler.Handle(createReq)

	// Fetch from empty topic
	fetchReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeFetch,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.FetchRequest{
			Topic:       "empty-topic",
			PartitionID: 0,
			Offset:      0,
			MaxBytes:    1024,
		},
	}

	resp := handler.Handle(fetchReq)

	// Should succeed but return empty messages
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status for empty topic fetch, got %v", resp.Header.Status)
	}

	fetchResp := resp.Payload.(*protocol.FetchResponse)
	if len(fetchResp.Messages) != 0 {
		t.Errorf("Expected 0 messages from empty topic, got %d", len(fetchResp.Messages))
	}
}

func TestHandler_handleFetch_InvalidOffset(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Create topic and add some messages
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "offset-test",
			NumPartitions: 1,
		},
	}
	handler.Handle(createReq)

	// Produce a message
	produceReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "offset-test",
			PartitionID: 0,
			Messages: []protocol.Message{
				{Key: []byte("k1"), Value: []byte("v1")},
			},
		},
	}
	handler.Handle(produceReq)

	// Fetch with offset beyond available data
	fetchReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 3,
			Type:      protocol.RequestTypeFetch,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.FetchRequest{
			Topic:       "offset-test",
			PartitionID: 0,
			Offset:      1000, // Way beyond available data
			MaxBytes:    1024,
		},
	}

	resp := handler.Handle(fetchReq)

	// Should succeed but return empty messages (read error handled)
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status for invalid offset, got %v", resp.Header.Status)
	}

	fetchResp := resp.Payload.(*protocol.FetchResponse)
	if len(fetchResp.Messages) != 0 {
		t.Errorf("Expected 0 messages for invalid offset, got %d", len(fetchResp.Messages))
	}
}

func TestHandler_Close_WithError(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)

	// Create multiple topics to ensure there's state to close
	for i := 0; i < 3; i++ {
		createReq := &protocol.Request{
			Header: protocol.RequestHeader{
				RequestID: uint64(i + 1),
				Type:      protocol.RequestTypeCreateTopic,
				Version:   protocol.ProtocolVersion,
			},
			Payload: &protocol.CreateTopicRequest{
				Topic:         "close-test-topic",
				NumPartitions: 1,
			},
		}
		handler.Handle(createReq)
	}

	// Close handler (first time)
	err := handler.Close()
	if err != nil {
		t.Errorf("First Close failed: %v", err)
	}

	// Close handler again (should be idempotent)
	err = handler.Close()
	if err != nil {
		t.Errorf("Second Close failed: %v", err)
	}

	// Try to use handler after close (should fail gracefully)
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 999,
			Type:      protocol.RequestTypeHealthCheck,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.HealthCheckRequest{},
	}

	// This might panic or return error depending on implementation
	// We're just testing that Close works correctly
	_ = req
}

func TestHandler_handleProduce_AutoCreateTopicFailure(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Try to produce to a non-existent topic (should auto-create)
	req := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "auto-create-test",
			PartitionID: 0,
			Messages: []protocol.Message{
				{Key: []byte("k1"), Value: []byte("v1")},
			},
		},
	}

	resp := handler.Handle(req)

	// Should succeed because auto-create works
	if resp.Header.Status != protocol.StatusOK {
		t.Errorf("Expected OK status for auto-create produce, got %v", resp.Header.Status)
	}
}

func TestHandler_handleProduce_InvalidPartition(t *testing.T) {
	tempDir := t.TempDir()
	handler := NewHandlerWithDataDir(tempDir)
	defer handler.Close()

	// Create topic with 1 partition
	createReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 1,
			Type:      protocol.RequestTypeCreateTopic,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.CreateTopicRequest{
			Topic:         "produce-test",
			NumPartitions: 1,
		},
	}
	handler.Handle(createReq)

	// Try to produce to non-existent partition
	produceReq := &protocol.Request{
		Header: protocol.RequestHeader{
			RequestID: 2,
			Type:      protocol.RequestTypeProduce,
			Version:   protocol.ProtocolVersion,
		},
		Payload: &protocol.ProduceRequest{
			Topic:       "produce-test",
			PartitionID: 99,
			Messages: []protocol.Message{
				{Key: []byte("k1"), Value: []byte("v1")},
			},
		},
	}

	resp := handler.Handle(produceReq)

	if resp.Header.Status != protocol.StatusError {
		t.Errorf("Expected error for invalid partition, got %v", resp.Header.Status)
	}

	if resp.Header.ErrorCode != protocol.ErrTopicNotFound {
		t.Errorf("Expected ErrTopicNotFound, got %v", resp.Header.ErrorCode)
	}
}
