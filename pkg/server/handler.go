package server

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/shawntherrien/streambus/pkg/protocol"
	"github.com/shawntherrien/streambus/pkg/storage"
)

// Handler handles incoming requests
type Handler struct {
	topicManager *TopicManager

	// Metrics
	requestsHandled int64
	errorsHandled   int64
	startTime       time.Time
}

// NewHandler creates a new request handler
func NewHandler() *Handler {
	return &Handler{
		topicManager: NewTopicManager("./data"),
		startTime:    time.Now(),
	}
}

// NewHandlerWithDataDir creates a new request handler with custom data directory
func NewHandlerWithDataDir(dataDir string) *Handler {
	return &Handler{
		topicManager: NewTopicManager(dataDir),
		startTime:    time.Now(),
	}
}

// Handle handles a request and returns a response
func (h *Handler) Handle(req *protocol.Request) *protocol.Response {
	atomic.AddInt64(&h.requestsHandled, 1)

	fmt.Printf("Handle: type=%d requestID=%d payload=%T\n", req.Header.Type, req.Header.RequestID, req.Payload)

	// Route based on request type
	switch req.Header.Type {
	case protocol.RequestTypeProduce:
		return h.handleProduce(req)
	case protocol.RequestTypeFetch:
		return h.handleFetch(req)
	case protocol.RequestTypeGetOffset:
		return h.handleGetOffset(req)
	case protocol.RequestTypeCreateTopic:
		return h.handleCreateTopic(req)
	case protocol.RequestTypeDeleteTopic:
		return h.handleDeleteTopic(req)
	case protocol.RequestTypeListTopics:
		return h.handleListTopics(req)
	case protocol.RequestTypeHealthCheck:
		return h.handleHealthCheck(req)
	default:
		return h.errorResponse(req.Header.RequestID, protocol.ErrUnknownRequest, "unknown request type")
	}
}

// handleProduce handles a produce request
func (h *Handler) handleProduce(req *protocol.Request) *protocol.Response {
	payload := req.Payload.(*protocol.ProduceRequest)

	// Auto-create topic if it doesn't exist
	if !h.topicManager.TopicExists(payload.Topic) {
		// Create topic with default 1 partition
		if err := h.topicManager.CreateTopic(payload.Topic, 1); err != nil {
			return h.errorResponse(req.Header.RequestID, protocol.ErrTopicExists,
				fmt.Sprintf("failed to auto-create topic: %v", err))
		}
	}

	// Get partition
	partition, err := h.topicManager.GetPartition(payload.Topic, payload.PartitionID)
	if err != nil {
		return h.errorResponse(req.Header.RequestID, protocol.ErrTopicNotFound, err.Error())
	}

	// Convert protocol messages to storage messages
	storageMessages := make([]storage.Message, len(payload.Messages))
	for i, msg := range payload.Messages {
		storageMessages[i] = storage.Message{
			Key:       msg.Key,
			Value:     msg.Value,
			Timestamp: time.Unix(0, msg.Timestamp),
		}
	}

	// Create batch and append to log
	batch := &storage.MessageBatch{
		Messages:  storageMessages,
		Timestamp: time.Now(),
	}
	offsets, err := partition.log.Append(batch)
	if err != nil {
		return h.errorResponse(req.Header.RequestID, protocol.ErrStorageError,
			fmt.Sprintf("failed to write messages: %v", err))
	}

	// Get base offset and high water mark
	baseOffset := int64(offsets[0])
	highWaterMark := int64(partition.log.HighWaterMark())

	resp := &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
			ErrorCode: protocol.ErrNone,
		},
		Payload: &protocol.ProduceResponse{
			Topic:         payload.Topic,
			PartitionID:   payload.PartitionID,
			BaseOffset:    baseOffset,
			NumMessages:   uint32(len(payload.Messages)),
			HighWaterMark: highWaterMark,
		},
	}

	return resp
}

// handleFetch handles a fetch request
func (h *Handler) handleFetch(req *protocol.Request) *protocol.Response {
	payload := req.Payload.(*protocol.FetchRequest)

	fmt.Printf("FETCH REQUEST: topic='%s' partition=%d offset=%d maxBytes=%d\n",
		payload.Topic, payload.PartitionID, payload.Offset, payload.MaxBytes)

	// Auto-create topic if it doesn't exist
	if !h.topicManager.TopicExists(payload.Topic) {
		// Create topic with default 1 partition
		if err := h.topicManager.CreateTopic(payload.Topic, 1); err != nil {
			return h.errorResponse(req.Header.RequestID, protocol.ErrTopicExists,
				fmt.Sprintf("failed to auto-create topic: %v", err))
		}
	}

	// Get partition
	partition, err := h.topicManager.GetPartition(payload.Topic, payload.PartitionID)
	if err != nil {
		return h.errorResponse(req.Header.RequestID, protocol.ErrTopicNotFound, err.Error())
	}

	// Read messages from log starting at offset
	storageMessages, err := partition.log.Read(storage.Offset(payload.Offset), int(payload.MaxBytes))
	if err != nil {
		// Log the error for debugging
		fmt.Printf("Read error for topic=%s partition=%d offset=%d: %v\n",
			payload.Topic, payload.PartitionID, payload.Offset, err)
		// No messages available, return empty list
		storageMessages = []*storage.Message{}
	}

	// Convert storage messages to protocol messages
	messages := make([]protocol.Message, len(storageMessages))
	for i, msg := range storageMessages {
		messages[i] = protocol.Message{
			Offset:    int64(msg.Offset),
			Key:       msg.Key,
			Value:     msg.Value,
			Timestamp: msg.Timestamp.UnixNano(),
		}
	}

	resp := &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
			ErrorCode: protocol.ErrNone,
		},
		Payload: &protocol.FetchResponse{
			Topic:         payload.Topic,
			PartitionID:   payload.PartitionID,
			HighWaterMark: int64(partition.log.HighWaterMark()),
			Messages:      messages,
		},
	}

	return resp
}

// handleGetOffset handles a get offset request
func (h *Handler) handleGetOffset(req *protocol.Request) *protocol.Response {
	payload := req.Payload.(*protocol.GetOffsetRequest)

	// Get partition
	partition, err := h.topicManager.GetPartition(payload.Topic, payload.PartitionID)
	if err != nil {
		return h.errorResponse(req.Header.RequestID, protocol.ErrTopicNotFound, err.Error())
	}

	// Get start and end offsets from log
	startOffset := int64(partition.log.StartOffset())
	endOffset := int64(partition.log.EndOffset())
	highWaterMark := int64(partition.log.HighWaterMark())

	resp := &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
			ErrorCode: protocol.ErrNone,
		},
		Payload: &protocol.GetOffsetResponse{
			Topic:         payload.Topic,
			PartitionID:   payload.PartitionID,
			StartOffset:   startOffset,
			EndOffset:     endOffset,
			HighWaterMark: highWaterMark,
		},
	}

	return resp
}

// handleCreateTopic handles a create topic request
func (h *Handler) handleCreateTopic(req *protocol.Request) *protocol.Response {
	payload := req.Payload.(*protocol.CreateTopicRequest)

	// Create topic
	err := h.topicManager.CreateTopic(payload.Topic, payload.NumPartitions)
	if err != nil {
		return h.errorResponse(req.Header.RequestID, protocol.ErrTopicExists,
			fmt.Sprintf("failed to create topic: %v", err))
	}

	resp := &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
			ErrorCode: protocol.ErrNone,
		},
		Payload: &protocol.CreateTopicResponse{
			Topic:     payload.Topic,
			Created:   true,
			ErrorCode: protocol.ErrNone,
		},
	}

	return resp
}

// handleDeleteTopic handles a delete topic request
func (h *Handler) handleDeleteTopic(req *protocol.Request) *protocol.Response {
	payload := req.Payload.(*protocol.DeleteTopicRequest)

	// Delete topic
	err := h.topicManager.DeleteTopic(payload.Topic)
	if err != nil {
		return h.errorResponse(req.Header.RequestID, protocol.ErrTopicNotFound,
			fmt.Sprintf("failed to delete topic: %v", err))
	}

	resp := &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
			ErrorCode: protocol.ErrNone,
		},
		Payload: &protocol.DeleteTopicResponse{
			Topic:     payload.Topic,
			Deleted:   true,
			ErrorCode: protocol.ErrNone,
		},
	}

	return resp
}

// handleListTopics handles a list topics request
func (h *Handler) handleListTopics(req *protocol.Request) *protocol.Response {
	// Get list of topics
	topics := h.topicManager.ListTopics()

	// Convert to protocol.TopicInfo
	protocolTopics := make([]protocol.TopicInfo, len(topics))
	for i, topic := range topics {
		protocolTopics[i] = protocol.TopicInfo{
			Name:          topic.Name,
			NumPartitions: topic.NumPartitions,
		}
	}

	resp := &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
			ErrorCode: protocol.ErrNone,
		},
		Payload: &protocol.ListTopicsResponse{
			Topics: protocolTopics,
		},
	}

	return resp
}

// handleHealthCheck handles a health check request
func (h *Handler) handleHealthCheck(req *protocol.Request) *protocol.Response {
	uptime := time.Since(h.startTime).Seconds()

	resp := &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
			ErrorCode: protocol.ErrNone,
		},
		Payload: &protocol.HealthCheckResponse{
			Status: "healthy",
			Uptime: int64(uptime),
		},
	}

	return resp
}

// errorResponse creates an error response
func (h *Handler) errorResponse(requestID uint64, errorCode protocol.ErrorCode, message string) *protocol.Response {
	atomic.AddInt64(&h.errorsHandled, 1)

	return &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: requestID,
			Status:    protocol.StatusError,
			ErrorCode: errorCode,
		},
		Payload: &protocol.ErrorResponse{
			ErrorCode: errorCode,
			Message:   message,
		},
	}
}

// Close closes the handler and releases resources
func (h *Handler) Close() error {
	if h.topicManager != nil {
		return h.topicManager.Close()
	}
	return nil
}

// Stats returns handler statistics
func (h *Handler) Stats() HandlerStats {
	return HandlerStats{
		RequestsHandled: atomic.LoadInt64(&h.requestsHandled),
		ErrorsHandled:   atomic.LoadInt64(&h.errorsHandled),
		Uptime:          time.Since(h.startTime),
	}
}

// HandlerStats holds handler statistics
type HandlerStats struct {
	RequestsHandled int64
	ErrorsHandled   int64
	Uptime          time.Duration
}
