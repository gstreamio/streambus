package client

import (
	"errors"
)

var (
	// Configuration errors
	ErrNoBrokers             = errors.New("no brokers specified")
	ErrInvalidTimeout        = errors.New("invalid timeout value")
	ErrInvalidMaxConnections = errors.New("invalid max connections value")
	ErrInvalidRetries        = errors.New("invalid retry count")

	// Connection errors
	ErrNoConnection         = errors.New("no connection available")
	ErrConnectionClosed     = errors.New("connection closed")
	ErrAllBrokersFailed     = errors.New("all brokers failed")
	ErrConnectionPoolClosed = errors.New("connection pool closed")

	// Request errors
	ErrRequestTimeout  = errors.New("request timeout")
	ErrInvalidResponse = errors.New("invalid response")
	ErrRequestFailed   = errors.New("request failed")

	// Client errors
	ErrClientClosed     = errors.New("client is closed")
	ErrInvalidTopic     = errors.New("invalid topic name")
	ErrInvalidPartition = errors.New("invalid partition")
	ErrInvalidOffset    = errors.New("invalid offset")

	// Producer errors
	ErrProducerClosed  = errors.New("producer is closed")
	ErrMessageTooLarge = errors.New("message too large")
	ErrBatchFull       = errors.New("batch is full")

	// Consumer errors
	ErrConsumerClosed = errors.New("consumer is closed")
	ErrNoMessages     = errors.New("no messages available")

	// Group consumer errors
	// ErrGroupCoordinationNotImplemented is returned by GroupConsumer instead of
	// silently simulating a successful join/sync/commit. Multi-partition consumer
	// group coordination (join/sync/heartbeat/offset-commit against a coordinator)
	// is not implemented yet; use the single-partition Consumer/PartitionConsumer
	// instead until this lands.
	ErrGroupCoordinationNotImplemented = errors.New("streambus: consumer group coordination is not implemented; use Consumer/PartitionConsumer instead")

	// Transactional producer errors
	// ErrTransactionCoordinationNotImplemented is returned by TransactionalProducer
	// instead of silently reporting a commit as successful. There is no
	// transaction-coordinator wiring yet, so CommitTransaction previously returned
	// nil without ever writing the transaction's messages to the broker - a silent
	// data-loss bug. Use the plain Producer for now.
	ErrTransactionCoordinationNotImplemented = errors.New("streambus: transaction coordination is not implemented; messages would not be durably written, use Producer instead")

	// Security errors
	ErrInvalidTLSConfig     = errors.New("invalid TLS configuration")
	ErrInvalidSASLConfig    = errors.New("invalid SASL configuration")
	ErrTLSHandshakeFailed   = errors.New("TLS handshake failed")
	ErrAuthenticationFailed = errors.New("authentication failed")
)
