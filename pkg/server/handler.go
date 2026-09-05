package server

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/logger"
	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/storage"
	"go.uber.org/zap"
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
	return NewHandlerWithTopicManager(NewTopicManager(dataDir))
}

// NewHandlerWithTopicManager creates a new request handler backed by an
// existing TopicManager. Use this when another component (such as a broker's
// admin API) needs to read the same storage the wire-protocol path writes to,
// instead of opening a second, independent view of the same directory.
func NewHandlerWithTopicManager(tm *TopicManager) *Handler {
	return &Handler{
		topicManager: tm,
		startTime:    time.Now(),
	}
}

// TopicManager returns the topic manager backing this handler.
func (h *Handler) TopicManager() *TopicManager {
	return h.topicManager
}

// Handle handles a request and returns a response
func (h *Handler) Handle(req *protocol.Request) *protocol.Response {
	atomic.AddInt64(&h.requestsHandled, 1)

	// Log at debug level for troubleshooting
	logger.Debug("handling request",
		zap.Int("type", int(req.Header.Type)),
		zap.Uint64("requestID", req.Header.RequestID),
		zap.String("payload", fmt.Sprintf("%T", req.Payload)))

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
		Messages:      storageMessages,
		Timestamp:     time.Now(),
		ProducerID:    payload.ProducerID,
		ProducerEpoch: payload.ProducerEpoch,
	}
	offsets, err := partition.log.Append(batch)
	if err != nil {
		return h.errorResponse(req.Header.RequestID, protocol.ErrStorageError,
			fmt.Sprintf("failed to write messages: %v", err))
	}

	// Get base offset and high water mark
	baseOffset := int64(offsets[0])
	highWaterMark := int64(partition.log.HighWaterMark())

	// A transactional batch opens (or extends) an in-flight transaction on
	// this partition: read-committed fetches must not read past its first
	// record until the coordinator's marker resolves it (see
	// transaction_bridge.go's logMarkerWriter, which calls EndTransaction).
	partition.BeginTransaction(payload.ProducerID, payload.ProducerEpoch, baseOffset)

	// TODO: When replication is implemented, validate LeaderEpoch from request
	// against current partition leader epoch. If mismatch, return ErrFencedLeaderEpoch.
	// For now, we return epoch 0 (single broker mode).
	var currentLeaderEpoch int64 = 0

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
			LeaderEpoch:   currentLeaderEpoch,
		},
	}

	return resp
}

// handleFetch handles a fetch request
func (h *Handler) handleFetch(req *protocol.Request) *protocol.Response {
	payload := req.Payload.(*protocol.FetchRequest)

	// Log fetch request at debug level
	logger.Debug("fetch request",
		zap.String("topic", payload.Topic),
		zap.Uint32("partition", payload.PartitionID),
		zap.Int64("offset", payload.Offset),
		zap.Uint32("maxBytes", payload.MaxBytes))

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
		logger.Debug("read error",
			zap.String("topic", payload.Topic),
			zap.Uint32("partition", payload.PartitionID),
			zap.Int64("offset", payload.Offset),
			zap.Error(err))
		// A genuinely invalid offset (before retention start, negative, etc.)
		// must be reported to the client - silently returning an empty list
		// here made it indistinguishable from "no new messages yet" and
		// masked real offset-tracking bugs on the client side.
		if errors.Is(err, storage.ErrOffsetOutOfRange) {
			return h.errorResponse(req.Header.RequestID, protocol.ErrOffsetOutOfRange, err.Error())
		}
		return h.errorResponse(req.Header.RequestID, protocol.ErrStorageError, err.Error())
	}

	highWaterMark := int64(partition.log.HighWaterMark())
	lastStableOffset := partition.LastStableOffset()

	// read_uncommitted may see anything up to the high water mark;
	// read_committed must stop at the last stable offset instead, so it
	// never returns a record from a transaction still in flight.
	readLimit := highWaterMark
	var aborted []abortedRange
	if payload.IsolationLevel == protocol.IsolationReadCommitted {
		readLimit = lastStableOffset
		// Snapshot once per fetch rather than having visibleMessages call
		// partition.IsAborted per record - see abortedRanges' doc comment.
		// read_uncommitted skips this entirely: it never filters on
		// abortedTxns, so there is nothing to snapshot.
		aborted = partition.abortedRanges()
	}

	messages, nextOffset := visibleMessages(storageMessages, payload.Offset, readLimit, aborted)

	resp := &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
			ErrorCode: protocol.ErrNone,
		},
		Payload: &protocol.FetchResponse{
			Topic:            payload.Topic,
			PartitionID:      payload.PartitionID,
			HighWaterMark:    highWaterMark,
			LastStableOffset: lastStableOffset,
			Messages:         messages,
			NextOffset:       nextOffset,
		},
	}

	return resp
}

// visibleMessages turns a raw log read into what a fetch response may
// actually return to a consumer.
//
// Three things are filtered out here rather than left to the client: control
// records (transaction markers) are never returned regardless of isolation
// level; nothing at or past readLimit is returned (the caller has already
// picked readLimit according to the request's isolation level); and a record
// whose producer identity and offset match one of aborted is skipped even
// though it is before readLimit - the abort marker already lifted the
// LastStableOffset barrier, so without this check the record would
// otherwise be indistinguishable from ordinary committed data. aborted is a
// snapshot from Partition.abortedRanges, taken once by the caller for the
// whole fetch (see handleFetch) - checking it here never touches txnMu; a
// read_uncommitted fetch passes nil, under which isAbortedInRanges always
// reports false, so it never filters on aborts at all.
//
// nextOffset is not simply "the last returned message's offset + 1" - a
// fetch window that contained only a filtered record returns zero messages,
// but the client must still be told to resume past it, or it would re-fetch
// the same apparently-empty window forever.
func visibleMessages(storageMessages []*storage.Message, startOffset, readLimit int64, aborted []abortedRange) ([]protocol.Message, int64) {
	messages := make([]protocol.Message, 0, len(storageMessages))
	nextOffset := startOffset

	for _, msg := range storageMessages {
		offset := int64(msg.Offset)
		if offset >= readLimit {
			break
		}
		nextOffset = offset + 1

		if protocol.IsControlRecord(msg.Headers) {
			continue
		}
		if isAbortedInRanges(aborted, msg.ProducerID, msg.ProducerEpoch, offset) {
			continue
		}

		messages = append(messages, protocol.Message{
			Offset:    offset,
			Key:       msg.Key,
			Value:     msg.Value,
			Timestamp: msg.Timestamp.UnixNano(),
		})
	}

	return messages, nextOffset
}

// handleGetOffset handles a get offset request
// Supports timestamp-based offset lookup (Kafka ListOffsets compatible)
func (h *Handler) handleGetOffset(req *protocol.Request) *protocol.Response {
	payload := req.Payload.(*protocol.GetOffsetRequest)

	// Get partition
	partition, err := h.topicManager.GetPartition(payload.Topic, payload.PartitionID)
	if err != nil {
		return h.errorResponse(req.Header.RequestID, protocol.ErrTopicNotFound, err.Error())
	}

	// Get base offsets from log
	startOffset := int64(partition.log.StartOffset())
	endOffset := int64(partition.log.EndOffset())
	highWaterMark := int64(partition.log.HighWaterMark())

	var resultOffset int64
	var resultTimestamp int64

	// Handle timestamp-based queries
	switch payload.Timestamp {
	case protocol.OffsetLatest, 0:
		// Return latest offset (log end offset)
		// Note: 0 is treated as OffsetLatest for backward compatibility
		if payload.Timestamp == 0 && startOffset < endOffset {
			// For backward compatibility, timestamp 0 returns earliest
			resultOffset = startOffset
		} else {
			resultOffset = endOffset
		}
	case protocol.OffsetEarliest:
		// Return earliest offset (log start offset)
		resultOffset = startOffset
	case protocol.OffsetMaxTimestamp:
		// Not yet implemented - return end offset
		resultOffset = endOffset
	default:
		// Timestamp-based query: find first offset >= timestamp
		if payload.Timestamp > 0 {
			offset, timestamp, err := partition.log.FindOffsetByTimestamp(payload.Timestamp)
			if err != nil {
				return h.errorResponse(req.Header.RequestID, protocol.ErrStorageError,
					fmt.Sprintf("failed to find offset by timestamp: %v", err))
			}
			resultOffset = int64(offset)
			resultTimestamp = timestamp
		} else {
			// Invalid timestamp, return earliest
			resultOffset = startOffset
		}
	}

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
			Offset:        resultOffset,
			Timestamp:     resultTimestamp,
			LeaderEpoch:   0, // TODO: Get from partition metadata when replication is implemented
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
