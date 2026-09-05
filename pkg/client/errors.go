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

	// Security errors
	ErrInvalidTLSConfig     = errors.New("invalid TLS configuration")
	ErrInvalidSASLConfig    = errors.New("invalid SASL configuration")
	ErrTLSHandshakeFailed   = errors.New("TLS handshake failed")
	ErrAuthenticationFailed = errors.New("authentication failed")
)
