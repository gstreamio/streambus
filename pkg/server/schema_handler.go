package server

import (
	"fmt"
	"sync/atomic"

	"github.com/gstreamio/streambus/pkg/protocol"
	"github.com/gstreamio/streambus/pkg/schema"
)

// SchemaHandler wraps a handler with schema validation on the produce path.
// When a topic has a registered schema, messages are validated before writing.
// Topics without schemas are unaffected (opt-in enforcement).
type SchemaHandler struct {
	baseHandler    RequestHandler
	schemaRegistry *schema.SchemaRegistry
	enabled        bool

	// Metrics
	requestsHandled    int64
	validationFailures int64
}

// NewSchemaHandler creates a new schema-enforcing handler.
func NewSchemaHandler(baseHandler RequestHandler, registry *schema.SchemaRegistry, enabled bool) *SchemaHandler {
	return &SchemaHandler{
		baseHandler:    baseHandler,
		schemaRegistry: registry,
		enabled:        enabled,
	}
}

// Handle handles a request, validating produce messages against registered schemas.
func (h *SchemaHandler) Handle(req *protocol.Request) *protocol.Response {
	atomic.AddInt64(&h.requestsHandled, 1)

	if !h.enabled || h.schemaRegistry == nil {
		return h.baseHandler.Handle(req)
	}

	// Only intercept produce requests
	if req.Header.Type != protocol.RequestTypeProduce {
		return h.baseHandler.Handle(req)
	}

	if err := h.validateProduceRequest(req); err != nil {
		atomic.AddInt64(&h.validationFailures, 1)
		return h.schemaErrorResponse(req.Header.RequestID, err)
	}

	return h.baseHandler.Handle(req)
}

// validateProduceRequest validates all messages in a produce request against registered schemas.
func (h *SchemaHandler) validateProduceRequest(req *protocol.Request) error {
	payload := req.Payload.(*protocol.ProduceRequest)

	// Build subject names (convention: "<topic>-value" and "<topic>-key")
	valueSubject := schema.Subject(payload.Topic + "-value")
	keySubject := schema.Subject(payload.Topic + "-key")

	hasValueSchema := h.schemaRegistry.HasSchema(valueSubject)
	hasKeySchema := h.schemaRegistry.HasSchema(keySubject)

	// No schemas registered for this topic -- skip validation
	if !hasValueSchema && !hasKeySchema {
		return nil
	}

	for i, msg := range payload.Messages {
		if err := h.validateMessage(i, msg, valueSubject, keySubject, hasValueSchema, hasKeySchema); err != nil {
			return err
		}
	}

	return nil
}

// validateMessage validates a single message's key and value against their respective schemas.
func (h *SchemaHandler) validateMessage(
	idx int,
	msg protocol.Message,
	valueSubject, keySubject schema.Subject,
	hasValueSchema, hasKeySchema bool,
) error {
	if hasValueSchema {
		if err := h.schemaRegistry.ValidateMessage(valueSubject, msg.Value); err != nil {
			return fmt.Errorf("message[%d] value schema validation failed: %w", idx, err)
		}
	}

	if hasKeySchema && len(msg.Key) > 0 {
		if err := h.schemaRegistry.ValidateMessage(keySubject, msg.Key); err != nil {
			return fmt.Errorf("message[%d] key schema validation failed: %w", idx, err)
		}
	}

	return nil
}

// schemaErrorResponse builds an error response for schema validation failures.
func (h *SchemaHandler) schemaErrorResponse(requestID uint64, err error) *protocol.Response {
	return &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: requestID,
			Status:    protocol.StatusError,
			ErrorCode: protocol.ErrSchemaValidationFailed,
		},
		Payload: &protocol.ErrorResponse{
			ErrorCode: protocol.ErrSchemaValidationFailed,
			Message:   err.Error(),
		},
	}
}

// GetStats returns schema handler statistics.
func (h *SchemaHandler) GetStats() map[string]int64 {
	return map[string]int64{
		"requests_handled":    atomic.LoadInt64(&h.requestsHandled),
		"validation_failures": atomic.LoadInt64(&h.validationFailures),
	}
}
