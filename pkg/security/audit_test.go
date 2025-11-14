package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawntherrien/streambus/pkg/logging"
)

func TestNewFileAuditLogger(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "streambus-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "audit.log")
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	auditLogger, err := NewFileAuditLogger(logFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	if auditLogger.filePath != logFile {
		t.Error("File path not set correctly")
	}

	if auditLogger.file == nil {
		t.Error("File not opened")
	}

	if auditLogger.encoder == nil {
		t.Error("Encoder not initialized")
	}
}

func TestNewFileAuditLogger_InvalidPath(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	_, err := NewFileAuditLogger("/nonexistent/dir/audit.log", logger)
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestFileAuditLogger_LogEvent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "streambus-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "audit.log")
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	auditLogger, err := NewFileAuditLogger(logFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	// Create test event
	event := &AuditEvent{
		Timestamp: time.Now(),
		Principal: &Principal{
			ID:   "user1",
			Type: PrincipalTypeUser,
		},
		Action: ActionTopicWrite,
		Resource: Resource{
			Type: ResourceTypeTopic,
			Name: "test-topic",
		},
		Result:   AuditResultSuccess,
		ClientIP: "192.168.1.100",
		Metadata: map[string]string{
			"partition": "0",
		},
	}

	// Log the event
	if err := auditLogger.LogEvent(event); err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Close to flush
	auditLogger.Close()

	// Read the file and verify
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("Log file is empty")
	}

	// Parse JSON
	var loggedEvent AuditEvent
	if err := json.Unmarshal(content, &loggedEvent); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify fields
	if loggedEvent.Principal.ID != "user1" {
		t.Error("Principal ID not logged correctly")
	}

	if loggedEvent.Action != ActionTopicWrite {
		t.Error("Action not logged correctly")
	}

	if loggedEvent.Resource.Name != "test-topic" {
		t.Error("Resource not logged correctly")
	}
}

func TestFileAuditLogger_LogEvent_SetTimestamp(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "streambus-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "audit.log")
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	auditLogger, err := NewFileAuditLogger(logFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	// Create event without timestamp
	event := &AuditEvent{
		Principal: &Principal{
			ID: "user1",
		},
		Action: ActionTopicWrite,
		Resource: Resource{
			Type: ResourceTypeTopic,
			Name: "test",
		},
		Result: AuditResultSuccess,
	}

	// Log the event
	if err := auditLogger.LogEvent(event); err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Timestamp should be set
	if event.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set automatically")
	}
}

func TestFileAuditLogger_Close(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "streambus-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "audit.log")
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})

	auditLogger, err := NewFileAuditLogger(logFile, logger)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}

	// Close should succeed
	if err := auditLogger.Close(); err != nil {
		t.Errorf("Failed to close: %v", err)
	}

	// Second close should be safe
	if err := auditLogger.Close(); err != nil {
		t.Error("Second close should not error")
	}
}

func TestNewMemoryAuditLogger(t *testing.T) {
	logger := NewMemoryAuditLogger()

	if logger == nil {
		t.Fatal("Expected non-nil logger")
	}

	if logger.events == nil {
		t.Error("Events slice not initialized")
	}
}

func TestMemoryAuditLogger_LogEvent(t *testing.T) {
	logger := NewMemoryAuditLogger()

	event1 := &AuditEvent{
		Principal: &Principal{ID: "user1"},
		Action:    ActionTopicWrite,
		Resource:  Resource{Type: ResourceTypeTopic, Name: "topic1"},
		Result:    AuditResultSuccess,
	}

	event2 := &AuditEvent{
		Principal: &Principal{ID: "user2"},
		Action:    ActionTopicRead,
		Resource:  Resource{Type: ResourceTypeTopic, Name: "topic2"},
		Result:    AuditResultFailure,
	}

	// Log events
	if err := logger.LogEvent(event1); err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	if err := logger.LogEvent(event2); err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Get events
	events := logger.GetEvents()
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	// Verify events
	if events[0].Principal.ID != "user1" {
		t.Error("First event not correct")
	}

	if events[1].Principal.ID != "user2" {
		t.Error("Second event not correct")
	}
}

func TestMemoryAuditLogger_GetEvents_Copy(t *testing.T) {
	logger := NewMemoryAuditLogger()

	event := &AuditEvent{
		Principal: &Principal{ID: "user1"},
		Action:    ActionTopicWrite,
		Resource:  Resource{Type: ResourceTypeTopic, Name: "topic1"},
		Result:    AuditResultSuccess,
	}

	logger.LogEvent(event)

	// Get events
	events1 := logger.GetEvents()
	events2 := logger.GetEvents()

	// Should be different slices
	if &events1[0] == &events2[0] {
		t.Error("Expected GetEvents to return a copy, not the same slice")
	}
}

func TestMemoryAuditLogger_Clear(t *testing.T) {
	logger := NewMemoryAuditLogger()

	event := &AuditEvent{
		Principal: &Principal{ID: "user1"},
		Action:    ActionTopicWrite,
		Resource:  Resource{Type: ResourceTypeTopic, Name: "topic1"},
		Result:    AuditResultSuccess,
	}

	logger.LogEvent(event)

	// Verify event was logged
	if len(logger.GetEvents()) != 1 {
		t.Error("Expected 1 event")
	}

	// Clear
	logger.Clear()

	// Verify cleared
	if len(logger.GetEvents()) != 0 {
		t.Error("Expected no events after clear")
	}
}

func TestNewMultiAuditLogger(t *testing.T) {
	logger1 := NewMemoryAuditLogger()
	logger2 := NewMemoryAuditLogger()

	multiLogger := NewMultiAuditLogger(logger1, logger2)

	if multiLogger == nil {
		t.Fatal("Expected non-nil logger")
	}

	if len(multiLogger.loggers) != 2 {
		t.Errorf("Expected 2 loggers, got %d", len(multiLogger.loggers))
	}
}

func TestMultiAuditLogger_LogEvent(t *testing.T) {
	logger1 := NewMemoryAuditLogger()
	logger2 := NewMemoryAuditLogger()

	multiLogger := NewMultiAuditLogger(logger1, logger2)

	event := &AuditEvent{
		Principal: &Principal{ID: "user1"},
		Action:    ActionTopicWrite,
		Resource:  Resource{Type: ResourceTypeTopic, Name: "topic1"},
		Result:    AuditResultSuccess,
	}

	// Log to multi logger
	if err := multiLogger.LogEvent(event); err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Verify both loggers received the event
	if len(logger1.GetEvents()) != 1 {
		t.Error("Logger 1 didn't receive event")
	}

	if len(logger2.GetEvents()) != 1 {
		t.Error("Logger 2 didn't receive event")
	}
}

func TestMultiAuditLogger_AddLogger(t *testing.T) {
	logger1 := NewMemoryAuditLogger()

	multiLogger := NewMultiAuditLogger(logger1)

	if len(multiLogger.loggers) != 1 {
		t.Error("Expected 1 logger initially")
	}

	logger2 := NewMemoryAuditLogger()
	multiLogger.AddLogger(logger2)

	if len(multiLogger.loggers) != 2 {
		t.Error("Expected 2 loggers after adding")
	}
}

func TestNewStructuredAuditLogger(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})
	auditLogger := NewStructuredAuditLogger(logger)

	if auditLogger == nil {
		t.Fatal("Expected non-nil logger")
	}

	if auditLogger.logger != logger {
		t.Error("Logger not set correctly")
	}
}

func TestStructuredAuditLogger_LogEvent(t *testing.T) {
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})
	auditLogger := NewStructuredAuditLogger(logger)

	tests := []struct {
		name   string
		event  *AuditEvent
		result AuditResult
	}{
		{
			name: "success event",
			event: &AuditEvent{
				Timestamp: time.Now(),
				Principal: &Principal{
					ID:         "user1",
					Type:       PrincipalTypeUser,
					AuthMethod: AuthMethodSASLPlain,
				},
				Action: ActionTopicWrite,
				Resource: Resource{
					Type: ResourceTypeTopic,
					Name: "orders",
				},
				Result:   AuditResultSuccess,
				ClientIP: "192.168.1.100",
				Metadata: map[string]string{
					"partition": "0",
				},
			},
		},
		{
			name: "failure event",
			event: &AuditEvent{
				Timestamp: time.Now(),
				Principal: &Principal{
					ID:   "user2",
					Type: PrincipalTypeUser,
				},
				Action: ActionTopicRead,
				Resource: Resource{
					Type: ResourceTypeTopic,
					Name: "sensitive",
				},
				Result:       AuditResultFailure,
				ErrorMessage: "insufficient permissions",
			},
		},
		{
			name: "denied event",
			event: &AuditEvent{
				Timestamp: time.Now(),
				Principal: &Principal{
					ID:   "user3",
					Type: PrincipalTypeUser,
				},
				Action: ActionTopicDelete,
				Resource: Resource{
					Type: ResourceTypeTopic,
					Name: "protected",
				},
				Result:       AuditResultDenied,
				ErrorMessage: "access denied",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := auditLogger.LogEvent(tt.event)
			if err != nil {
				t.Errorf("Failed to log event: %v", err)
			}
		})
	}
}

func TestNewAuditEventBuilder(t *testing.T) {
	builder := NewAuditEventBuilder()

	if builder == nil {
		t.Fatal("Expected non-nil builder")
	}

	if builder.event == nil {
		t.Error("Event not initialized")
	}

	if builder.event.Timestamp.IsZero() {
		t.Error("Timestamp not set")
	}

	if builder.event.Metadata == nil {
		t.Error("Metadata not initialized")
	}
}

func TestAuditEventBuilder(t *testing.T) {
	principal := &Principal{
		ID:       "user1",
		Type:     PrincipalTypeUser,
		ClientIP: "192.168.1.100",
	}

	event := NewAuditEventBuilder().
		WithPrincipal(principal).
		WithAction(ActionTopicWrite).
		WithResource(Resource{
			Type: ResourceTypeTopic,
			Name: "orders",
		}).
		WithResult(AuditResultSuccess).
		WithClientIP("192.168.1.100").
		WithMetadata("partition", "0").
		WithMetadata("size", "1024").
		Build()

	// Verify all fields
	if event.Principal != principal {
		t.Error("Principal not set")
	}

	if event.Action != ActionTopicWrite {
		t.Error("Action not set")
	}

	if event.Resource.Name != "orders" {
		t.Error("Resource not set")
	}

	if event.Result != AuditResultSuccess {
		t.Error("Result not set")
	}

	if event.ClientIP != "192.168.1.100" {
		t.Error("Client IP not set")
	}

	if event.Metadata["partition"] != "0" {
		t.Error("Metadata not set")
	}

	if event.Metadata["size"] != "1024" {
		t.Error("Second metadata not set")
	}
}

func TestAuditEventBuilder_WithError(t *testing.T) {
	testErr := fmt.Errorf("test error message")
	event := NewAuditEventBuilder().
		WithAction(ActionTopicWrite).
		WithResource(Resource{Type: ResourceTypeTopic, Name: "test"}).
		WithError(testErr).
		Build()

	if event.ErrorMessage == "" {
		t.Error("Error message not set")
	}

	if event.ErrorMessage != "test error message" {
		t.Errorf("Expected error message 'test error message', got '%s'", event.ErrorMessage)
	}

	if event.Result != AuditResultFailure {
		t.Error("Result not set to FAILURE when error provided")
	}
}

func TestAuditEventBuilder_WithNilError(t *testing.T) {
	event := NewAuditEventBuilder().
		WithAction(ActionTopicWrite).
		WithResource(Resource{Type: ResourceTypeTopic, Name: "test"}).
		WithResult(AuditResultSuccess).
		WithError(nil).
		Build()

	// Should not modify result if error is nil
	if event.Result != AuditResultSuccess {
		t.Error("Result should not change for nil error")
	}

	if event.ErrorMessage != "" {
		t.Error("Error message should be empty for nil error")
	}
}

func TestAuditEventBuilder_WithPrincipal_ClientIP(t *testing.T) {
	principal := &Principal{
		ID:       "user1",
		ClientIP: "10.0.0.1",
	}

	event := NewAuditEventBuilder().
		WithPrincipal(principal).
		WithAction(ActionTopicRead).
		WithResource(Resource{Type: ResourceTypeTopic, Name: "test"}).
		Build()

	// Client IP should be copied from principal
	if event.ClientIP != "10.0.0.1" {
		t.Error("Client IP not copied from principal")
	}
}

func TestAuditEventBuilder_WithNilPrincipal(t *testing.T) {
	event := NewAuditEventBuilder().
		WithPrincipal(nil).
		WithAction(ActionTopicRead).
		WithResource(Resource{Type: ResourceTypeTopic, Name: "test"}).
		Build()

	// Should not panic with nil principal
	if event.Principal != nil {
		t.Error("Expected nil principal")
	}

	if event.ClientIP != "" {
		t.Error("Client IP should not be set for nil principal")
	}
}

func TestAuditEvent_CompleteWorkflow(t *testing.T) {
	// This test demonstrates a complete audit workflow
	tmpDir, err := os.MkdirTemp("", "streambus-audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create loggers
	logger := logging.New(&logging.Config{Level: logging.LevelInfo})
	fileLogger, err := NewFileAuditLogger(filepath.Join(tmpDir, "audit.log"), logger)
	if err != nil {
		t.Fatalf("Failed to create file logger: %v", err)
	}
	defer fileLogger.Close()

	memoryLogger := NewMemoryAuditLogger()
	structuredLogger := NewStructuredAuditLogger(logger)

	// Create multi-logger
	multiLogger := NewMultiAuditLogger(fileLogger, memoryLogger, structuredLogger)

	// Create principal
	principal := &Principal{
		ID:         "user1",
		Type:       PrincipalTypeUser,
		AuthMethod: AuthMethodSASLPlain,
		ClientIP:   "192.168.1.100",
	}

	// Build and log event
	event := NewAuditEventBuilder().
		WithPrincipal(principal).
		WithAction(ActionTopicWrite).
		WithResource(Resource{
			Type: ResourceTypeTopic,
			Name: "audit-test-topic",
		}).
		WithResult(AuditResultSuccess).
		WithMetadata("test", "complete-workflow").
		Build()

	// Log to all loggers
	if err := multiLogger.LogEvent(event); err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Verify in memory logger
	events := memoryLogger.GetEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event in memory, got %d", len(events))
	}

	if events[0].Metadata["test"] != "complete-workflow" {
		t.Error("Event not logged correctly to memory")
	}

	// Close file logger to flush
	fileLogger.Close()

	// Verify file exists and has content
	content, err := os.ReadFile(filepath.Join(tmpDir, "audit.log"))
	if err != nil {
		t.Fatalf("Failed to read audit log: %v", err)
	}

	if len(content) == 0 {
		t.Error("Audit log file is empty")
	}
}

func TestConcurrentAuditLogging(t *testing.T) {
	logger := NewMemoryAuditLogger()

	// Log events concurrently
	done := make(chan bool)
	numGoroutines := 10
	eventsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < eventsPerGoroutine; j++ {
				event := &AuditEvent{
					Principal: &Principal{ID: "user1"},
					Action:    ActionTopicWrite,
					Resource:  Resource{Type: ResourceTypeTopic, Name: "test"},
					Result:    AuditResultSuccess,
				}
				logger.LogEvent(event)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all events were logged
	events := logger.GetEvents()
	expected := numGoroutines * eventsPerGoroutine
	if len(events) != expected {
		t.Errorf("Expected %d events, got %d", expected, len(events))
	}
}
