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
