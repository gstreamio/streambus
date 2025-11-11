package streambus

import "errors"

// Common errors
var (
	// Connection errors
	ErrConnectionFailed   = errors.New("failed to connect to broker")
	ErrConnectionClosed   = errors.New("connection closed")
	ErrNoAvailableBrokers = errors.New("no available brokers")
	ErrBrokerNotAvailable = errors.New("broker not available")

	// Topic errors
	ErrTopicNotFound     = errors.New("topic not found")
	ErrTopicAlreadyExists = errors.New("topic already exists")
	ErrInvalidTopicName  = errors.New("invalid topic name")

	// Message errors
	ErrMessageTooLarge   = errors.New("message too large")
	ErrInvalidMessage    = errors.New("invalid message")
	ErrMessageSendFailed = errors.New("failed to send message")

	// Consumer errors
	ErrConsumerClosed     = errors.New("consumer closed")
	ErrInvalidOffset      = errors.New("invalid offset")
	ErrOffsetOutOfRange   = errors.New("offset out of range")
	ErrNoMessagesAvailable = errors.New("no messages available")

	// Producer errors
	ErrProducerClosed = errors.New("producer closed")
	ErrFlushTimeout   = errors.New("flush timeout")

	// Consumer group errors
	ErrGroupNotFound      = errors.New("consumer group not found")
	ErrGroupAlreadyExists = errors.New("consumer group already exists")
	ErrRebalanceInProgress = errors.New("rebalance in progress")

	// Configuration errors
	ErrInvalidConfig     = errors.New("invalid configuration")
	ErrMissingBrokers    = errors.New("no brokers specified")
	ErrInvalidPartition  = errors.New("invalid partition")

	// Authentication errors
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrUnauthorized        = errors.New("unauthorized")

	// Timeout errors
	ErrRequestTimeout = errors.New("request timeout")
	ErrSessionTimeout = errors.New("session timeout")

	// General errors
	ErrNotImplemented = errors.New("not implemented")
	ErrOperationFailed = errors.New("operation failed")
)