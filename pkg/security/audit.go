package security

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gstreamio/streambus/pkg/logging"
)

// FileAuditLogger logs audit events to a file
type FileAuditLogger struct {
	mu       sync.Mutex
	file     *os.File
	encoder  *json.Encoder
	logger   *logging.Logger
	filePath string
}

// NewFileAuditLogger creates a new file-based audit logger
func NewFileAuditLogger(filePath string, logger *logging.Logger) (*FileAuditLogger, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}

	return &FileAuditLogger{
		file:     file,
		encoder:  json.NewEncoder(file),
		logger:   logger,
		filePath: filePath,
	}, nil
}

// LogEvent logs a security audit event
func (l *FileAuditLogger) LogEvent(event *AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Encode event as JSON
	if err := l.encoder.Encode(event); err != nil {
		l.logger.Error("Failed to write audit event", err)
		return err
	}

	// Sync to ensure event is written
	if err := l.file.Sync(); err != nil {
		l.logger.Error("Failed to sync audit log", err)
		return err
	}

	return nil
}

// Close closes the audit logger
func (l *FileAuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		err := l.file.Close()
		l.file = nil // Set to nil after closing to allow multiple Close() calls
		return err
	}
	return nil
}

// MemoryAuditLogger stores audit events in memory (for testing)
type MemoryAuditLogger struct {
	mu     sync.RWMutex
	events []*AuditEvent
}

// NewMemoryAuditLogger creates a new memory-based audit logger
func NewMemoryAuditLogger() *MemoryAuditLogger {
	return &MemoryAuditLogger{
		events: make([]*AuditEvent, 0),
	}
}

// LogEvent logs a security audit event
func (l *MemoryAuditLogger) LogEvent(event *AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	l.events = append(l.events, event)
	return nil
}

// GetEvents returns all logged events
func (l *MemoryAuditLogger) GetEvents() []*AuditEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Return a copy
	events := make([]*AuditEvent, len(l.events))
	copy(events, l.events)
	return events
}

// Clear clears all events
func (l *MemoryAuditLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = make([]*AuditEvent, 0)
}

// MultiAuditLogger logs to multiple audit loggers
type MultiAuditLogger struct {
	loggers []AuditLogger
}

// NewMultiAuditLogger creates a new multi audit logger
func NewMultiAuditLogger(loggers ...AuditLogger) *MultiAuditLogger {
	return &MultiAuditLogger{
		loggers: loggers,
	}
}

// LogEvent logs to all configured loggers
func (l *MultiAuditLogger) LogEvent(event *AuditEvent) error {
	for _, logger := range l.loggers {
		if err := logger.LogEvent(event); err != nil {
			// Log error but continue to other loggers
			continue
		}
	}
	return nil
}

// AddLogger adds a logger
func (l *MultiAuditLogger) AddLogger(logger AuditLogger) {
	l.loggers = append(l.loggers, logger)
}

// StructuredAuditLogger logs to structured logging
type StructuredAuditLogger struct {
	logger *logging.Logger
}

// NewStructuredAuditLogger creates a new structured audit logger
func NewStructuredAuditLogger(logger *logging.Logger) *StructuredAuditLogger {
	return &StructuredAuditLogger{
		logger: logger,
	}
}

// LogEvent logs an audit event using structured logging
func (l *StructuredAuditLogger) LogEvent(event *AuditEvent) error {
	fields := logging.Fields{
		"audit_event":  true,
		"timestamp":    event.Timestamp,
		"action":       event.Action,
		"resource":     event.Resource,
		"result":       event.Result,
		"client_ip":    event.ClientIP,
	}

	if event.Principal != nil {
		fields["principal_id"] = event.Principal.ID
		fields["principal_type"] = event.Principal.Type
		fields["auth_method"] = event.Principal.AuthMethod
	}

	if event.ErrorMessage != "" {
		fields["error"] = event.ErrorMessage
	}

	// Add metadata
	for k, v := range event.Metadata {
		fields["meta_"+k] = v
	}

	message := fmt.Sprintf("Security audit: %s on %s.%s - %s",
		event.Action,
		event.Resource.Type,
		event.Resource.Name,
		event.Result)

	switch event.Result {
	case AuditResultSuccess:
		l.logger.Info(message, fields)
	case AuditResultFailure, AuditResultDenied:
		l.logger.Warn(message, fields)
	default:
		l.logger.Info(message, fields)
	}

	return nil
}

// AuditEventBuilder helps build audit events fluently
type AuditEventBuilder struct {
	event *AuditEvent
}

// NewAuditEventBuilder creates a new audit event builder
func NewAuditEventBuilder() *AuditEventBuilder {
	return &AuditEventBuilder{
		event: &AuditEvent{
			Timestamp: time.Now(),
			Metadata:  make(map[string]string),
		},
	}
}

// WithPrincipal sets the principal
func (b *AuditEventBuilder) WithPrincipal(principal *Principal) *AuditEventBuilder {
	b.event.Principal = principal
	if principal != nil {
		b.event.ClientIP = principal.ClientIP
	}
	return b
}

// WithAction sets the action
func (b *AuditEventBuilder) WithAction(action Action) *AuditEventBuilder {
	b.event.Action = action
	return b
}

// WithResource sets the resource
func (b *AuditEventBuilder) WithResource(resource Resource) *AuditEventBuilder {
	b.event.Resource = resource
	return b
}

// WithResult sets the result
func (b *AuditEventBuilder) WithResult(result AuditResult) *AuditEventBuilder {
	b.event.Result = result
	return b
}

// WithClientIP sets the client IP
func (b *AuditEventBuilder) WithClientIP(ip string) *AuditEventBuilder {
	b.event.ClientIP = ip
	return b
}

// WithError sets the error message
func (b *AuditEventBuilder) WithError(err error) *AuditEventBuilder {
	if err != nil {
		b.event.ErrorMessage = err.Error()
		b.event.Result = AuditResultFailure
	}
	return b
}

// WithMetadata adds metadata
func (b *AuditEventBuilder) WithMetadata(key, value string) *AuditEventBuilder {
	b.event.Metadata[key] = value
	return b
}

// Build builds the audit event
func (b *AuditEventBuilder) Build() *AuditEvent {
	return b.event
}
