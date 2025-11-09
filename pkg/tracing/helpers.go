package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SpanOption is a functional option for configuring spans
type SpanOption func(*spanConfig)

type spanConfig struct {
	attributes []attribute.KeyValue
	kind       trace.SpanKind
}

// WithAttributes adds attributes to a span
func WithAttributes(attrs ...attribute.KeyValue) SpanOption {
	return func(c *spanConfig) {
		c.attributes = append(c.attributes, attrs...)
	}
}

// WithSpanKind sets the span kind
func WithSpanKind(kind trace.SpanKind) SpanOption {
	return func(c *spanConfig) {
		c.kind = kind
	}
}

// StartSpan starts a new span with the given name and options
func StartSpan(ctx context.Context, tracer trace.Tracer, name string, opts ...SpanOption) (context.Context, trace.Span) {
	config := &spanConfig{
		kind: trace.SpanKindInternal,
	}

	for _, opt := range opts {
		opt(config)
	}

	spanOpts := []trace.SpanStartOption{
		trace.WithSpanKind(config.kind),
	}

	if len(config.attributes) > 0 {
		spanOpts = append(spanOpts, trace.WithAttributes(config.attributes...))
	}

	return tracer.Start(ctx, name, spanOpts...)
}

// RecordError records an error on the span
func RecordError(span trace.Span, err error) {
	if err != nil && span != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetSpanAttributes sets multiple attributes on a span
func SetSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	if span != nil {
		span.SetAttributes(attrs...)
	}
}

// AddSpanEvent adds an event to a span
func AddSpanEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	if span != nil {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// SetSpanStatus sets the status of a span
func SetSpanStatus(span trace.Span, code codes.Code, description string) {
	if span != nil {
		span.SetStatus(code, description)
	}
}

// Common attribute keys for StreamBus operations
const (
	// Broker attributes
	AttrBrokerID   = attribute.Key("streambus.broker.id")
	AttrBrokerHost = attribute.Key("streambus.broker.host")
	AttrBrokerPort = attribute.Key("streambus.broker.port")

	// Topic attributes
	AttrTopicName       = attribute.Key("streambus.topic.name")
	AttrTopicPartitions = attribute.Key("streambus.topic.partitions")
	AttrTopicReplicas   = attribute.Key("streambus.topic.replicas")

	// Partition attributes
	AttrPartitionID     = attribute.Key("streambus.partition.id")
	AttrPartitionOffset = attribute.Key("streambus.partition.offset")

	// Message attributes
	AttrMessageKey      = attribute.Key("streambus.message.key")
	AttrMessageSize     = attribute.Key("streambus.message.size")
	AttrMessageCount    = attribute.Key("streambus.message.count")
	AttrMessageOffset   = attribute.Key("streambus.message.offset")
	AttrMessageBatch    = attribute.Key("streambus.message.batch")

	// Consumer attributes
	AttrConsumerGroup  = attribute.Key("streambus.consumer.group")
	AttrConsumerID     = attribute.Key("streambus.consumer.id")
	AttrConsumerMember = attribute.Key("streambus.consumer.member")

	// Transaction attributes
	AttrTransactionID     = attribute.Key("streambus.transaction.id")
	AttrTransactionStatus = attribute.Key("streambus.transaction.status")

	// Request attributes
	AttrRequestID   = attribute.Key("streambus.request.id")
	AttrRequestType = attribute.Key("streambus.request.type")

	// Schema attributes
	AttrSchemaID      = attribute.Key("streambus.schema.id")
	AttrSchemaVersion = attribute.Key("streambus.schema.version")
	AttrSchemaType    = attribute.Key("streambus.schema.type")

	// Error attributes
	AttrErrorType    = attribute.Key("streambus.error.type")
	AttrErrorMessage = attribute.Key("streambus.error.message")
	AttrErrorCode    = attribute.Key("streambus.error.code")
)

// Helper functions for creating common attributes

// BrokerAttributes creates broker-related attributes
func BrokerAttributes(brokerID uint64, host string, port uint16) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrBrokerID.Int64(int64(brokerID)),
		AttrBrokerHost.String(host),
		AttrBrokerPort.Int64(int64(port)),
	}
}

// TopicAttributes creates topic-related attributes
func TopicAttributes(name string, partitions, replicas uint32) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrTopicName.String(name),
		AttrTopicPartitions.Int64(int64(partitions)),
		AttrTopicReplicas.Int64(int64(replicas)),
	}
}

// PartitionAttributes creates partition-related attributes
func PartitionAttributes(partitionID uint32, offset int64) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrPartitionID.Int64(int64(partitionID)),
		AttrPartitionOffset.Int64(offset),
	}
}

// MessageAttributes creates message-related attributes
func MessageAttributes(key string, size int, offset int64) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrMessageKey.String(key),
		AttrMessageSize.Int(size),
		AttrMessageOffset.Int64(offset),
	}
}

// ConsumerAttributes creates consumer-related attributes
func ConsumerAttributes(groupID, consumerID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrConsumerGroup.String(groupID),
		AttrConsumerID.String(consumerID),
	}
}

// TransactionAttributes creates transaction-related attributes
func TransactionAttributes(txID, status string) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrTransactionID.String(txID),
		AttrTransactionStatus.String(status),
	}
}

// RequestAttributes creates request-related attributes
func RequestAttributes(requestID uint64, requestType string) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrRequestID.Int64(int64(requestID)),
		AttrRequestType.String(requestType),
	}
}

// SchemaAttributes creates schema-related attributes
func SchemaAttributes(schemaID uint64, version uint32, schemaType string) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrSchemaID.Int64(int64(schemaID)),
		AttrSchemaVersion.Int64(int64(version)),
		AttrSchemaType.String(schemaType),
	}
}

// ErrorAttributes creates error-related attributes
func ErrorAttributes(errorType, message string, code int) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrErrorType.String(errorType),
		AttrErrorMessage.String(message),
		AttrErrorCode.Int(code),
	}
}

// ContextWithSpan returns a context with the given span
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// SpanFromContext returns the span from the context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// SpanContextFromContext returns the span context from the context
func SpanContextFromContext(ctx context.Context) trace.SpanContext {
	return trace.SpanContextFromContext(ctx)
}
