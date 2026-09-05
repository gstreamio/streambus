package server

import (
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/transaction"
)

// TransactionHandler routes transaction requests to a transaction coordinator
// and delegates everything else to the handler it wraps.
//
// Like CoordinationHandler, this is the bridge that was missing between the
// wire protocol and an already-implemented coordinator: without it,
// InitProducerID/AddPartitionsToTxn/EndTxn and friends fall through to
// "unknown request type" and the coordinator is reachable only from Go code
// that imports pkg/transaction directly.
type TransactionHandler struct {
	baseHandler RequestHandler
	coordinator *transaction.TransactionCoordinator
}

// NewTransactionHandler wraps baseHandler so transaction requests are served
// by coordinator. A nil coordinator makes transaction requests fail with
// ErrTransactionCoordinatorNotAvailable rather than being silently ignored.
func NewTransactionHandler(baseHandler RequestHandler, coordinator *transaction.TransactionCoordinator) *TransactionHandler {
	return &TransactionHandler{
		baseHandler: baseHandler,
		coordinator: coordinator,
	}
}

// Handle routes a request.
func (h *TransactionHandler) Handle(req *protocol.Request) *protocol.Response {
	switch req.Header.Type {
	case protocol.RequestTypeInitProducerID:
		return h.handleInitProducerID(req)
	case protocol.RequestTypeAddPartitionsToTxn:
		return h.handleAddPartitionsToTxn(req)
	case protocol.RequestTypeAddOffsetsToTxn:
		return h.handleAddOffsetsToTxn(req)
	case protocol.RequestTypeTxnOffsetCommit:
		return h.handleTxnOffsetCommit(req)
	case protocol.RequestTypeEndTxn:
		return h.handleEndTxn(req)
	default:
		return h.baseHandler.Handle(req)
	}
}

// available reports whether a coordinator is wired up, returning an error
// response if not.
func (h *TransactionHandler) available(req *protocol.Request) *protocol.Response {
	if h.coordinator != nil {
		return nil
	}
	return coordinationError(req.Header.RequestID, protocol.ErrTransactionCoordinatorNotAvailable,
		"this broker is not a transaction coordinator")
}

func (h *TransactionHandler) handleInitProducerID(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.InitProducerIDRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "InitProducerIDRequest")
	}

	result, err := h.coordinator.InitProducerID(&transaction.InitProducerIDRequest{
		TransactionID:      transaction.TransactionID(payload.TransactionID),
		TransactionTimeout: time.Duration(payload.TransactionTimeoutMs) * time.Millisecond,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	return okResponse(req.Header.RequestID, &protocol.InitProducerIDResponse{
		ProducerID:    int64(result.ProducerID),
		ProducerEpoch: int16(result.ProducerEpoch),
		ErrorCode:     txnErrorToProtocol(result.ErrorCode),
	})
}

func (h *TransactionHandler) handleAddPartitionsToTxn(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.AddPartitionsToTxnRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "AddPartitionsToTxnRequest")
	}

	partitions := make([]transaction.PartitionMetadata, 0, len(payload.Partitions))
	for _, p := range payload.Partitions {
		partitions = append(partitions, transaction.PartitionMetadata{Topic: p.Topic, Partition: p.Partition})
	}

	result, err := h.coordinator.AddPartitionsToTxn(&transaction.AddPartitionsToTxnRequest{
		TransactionID: transaction.TransactionID(payload.TransactionID),
		ProducerID:    transaction.ProducerID(payload.ProducerID),
		ProducerEpoch: transaction.ProducerEpoch(payload.ProducerEpoch),
		Partitions:    partitions,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	// Report results in the order the request listed them.
	results := make([]protocol.TxnPartitionResult, 0, len(payload.Partitions))
	for _, p := range payload.Partitions {
		results = append(results, protocol.TxnPartitionResult{
			Topic:     p.Topic,
			Partition: p.Partition,
			ErrorCode: txnErrorToProtocol(result.Errors[p.Topic][p.Partition]),
		})
	}

	return okResponse(req.Header.RequestID, &protocol.AddPartitionsToTxnResponse{Results: results})
}

func (h *TransactionHandler) handleAddOffsetsToTxn(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.AddOffsetsToTxnRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "AddOffsetsToTxnRequest")
	}

	result, err := h.coordinator.AddOffsetsToTxn(&transaction.AddOffsetsToTxnRequest{
		TransactionID: transaction.TransactionID(payload.TransactionID),
		ProducerID:    transaction.ProducerID(payload.ProducerID),
		ProducerEpoch: transaction.ProducerEpoch(payload.ProducerEpoch),
		GroupID:       payload.GroupID,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	return okResponse(req.Header.RequestID, &protocol.AddOffsetsToTxnResponse{
		ErrorCode: txnErrorToProtocol(result.ErrorCode),
	})
}

func (h *TransactionHandler) handleTxnOffsetCommit(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.TxnOffsetCommitRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "TxnOffsetCommitRequest")
	}

	offsets := make(map[string]map[int32]transaction.OffsetMetadata, len(payload.Topics))
	for _, topic := range payload.Topics {
		partitions := make(map[int32]transaction.OffsetMetadata, len(topic.Partitions))
		for _, p := range topic.Partitions {
			partitions[p.Partition] = transaction.OffsetMetadata{Offset: p.Offset, Metadata: p.Metadata}
		}
		offsets[topic.Topic] = partitions
	}

	result, err := h.coordinator.TxnOffsetCommit(&transaction.TxnOffsetCommitRequest{
		TransactionID: transaction.TransactionID(payload.TransactionID),
		GroupID:       payload.GroupID,
		ProducerID:    transaction.ProducerID(payload.ProducerID),
		ProducerEpoch: transaction.ProducerEpoch(payload.ProducerEpoch),
		Offsets:       offsets,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	topics := make([]protocol.OffsetCommitTopicResult, 0, len(payload.Topics))
	for _, topic := range payload.Topics {
		results := make([]protocol.OffsetCommitPartitionResult, 0, len(topic.Partitions))
		for _, p := range topic.Partitions {
			results = append(results, protocol.OffsetCommitPartitionResult{
				Partition: p.Partition,
				ErrorCode: txnErrorToProtocol(result.Errors[topic.Topic][p.Partition]),
			})
		}
		topics = append(topics, protocol.OffsetCommitTopicResult{Topic: topic.Topic, Partitions: results})
	}

	return okResponse(req.Header.RequestID, &protocol.TxnOffsetCommitResponse{Topics: topics})
}

func (h *TransactionHandler) handleEndTxn(req *protocol.Request) *protocol.Response {
	if resp := h.available(req); resp != nil {
		return resp
	}
	payload, ok := req.Payload.(*protocol.EndTxnRequest)
	if !ok {
		return badRequest(req.Header.RequestID, "EndTxnRequest")
	}

	result, err := h.coordinator.EndTxn(&transaction.EndTxnRequest{
		TransactionID: transaction.TransactionID(payload.TransactionID),
		ProducerID:    transaction.ProducerID(payload.ProducerID),
		ProducerEpoch: transaction.ProducerEpoch(payload.ProducerEpoch),
		Commit:        payload.Commit,
	})
	if err != nil {
		return coordinationError(req.Header.RequestID, protocol.ErrInvalidRequest, err.Error())
	}

	return okResponse(req.Header.RequestID, &protocol.EndTxnResponse{
		ErrorCode: txnErrorToProtocol(result.ErrorCode),
	})
}

// txnErrorToProtocol maps a transaction coordinator error code onto its
// protocol counterpart. An unrecognised non-zero code becomes
// ErrInvalidTransactionState rather than being reported as success, so a new
// coordinator error can never reach a client looking like ErrNone.
func txnErrorToProtocol(code transaction.ErrorCode) protocol.ErrorCode {
	switch code {
	case transaction.ErrorNone:
		return protocol.ErrNone
	case transaction.ErrorInvalidProducerEpoch:
		return protocol.ErrInvalidProducerEpoch
	case transaction.ErrorInvalidTransactionState:
		return protocol.ErrInvalidTransactionState
	case transaction.ErrorInvalidProducerIDMapping:
		return protocol.ErrInvalidProducerIDMapping
	case transaction.ErrorTransactionCoordinatorNotAvailable:
		return protocol.ErrTransactionCoordinatorNotAvailable
	case transaction.ErrorTransactionCoordinatorFenced:
		return protocol.ErrTransactionCoordinatorFenced
	case transaction.ErrorProducerFenced:
		return protocol.ErrProducerFenced
	case transaction.ErrorInvalidTransactionTimeout:
		return protocol.ErrInvalidTransactionTimeout
	case transaction.ErrorConcurrentTransactions:
		return protocol.ErrConcurrentTransactions
	case transaction.ErrorTransactionAborted:
		return protocol.ErrTransactionAborted
	case transaction.ErrorInvalidPartitionList:
		return protocol.ErrInvalidPartitionList
	default:
		return protocol.ErrInvalidTransactionState
	}
}
