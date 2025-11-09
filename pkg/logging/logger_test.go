package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{Level(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.level.String())
		})
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
		hasError bool
	}{
		{"DEBUG", LevelDebug, false},
		{"debug", LevelDebug, false},
		{"INFO", LevelInfo, false},
		{"info", LevelInfo, false},
		{"WARN", LevelWarn, false},
		{"WARNING", LevelWarn, false},
		{"warn", LevelWarn, false},
		{"ERROR", LevelError, false},
		{"error", LevelError, false},
		{"FATAL", LevelFatal, false},
		{"fatal", LevelFatal, false},
		{"INVALID", LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := ParseLevel(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, level)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.Equal(t, LevelInfo, config.Level)
	assert.NotNil(t, config.Output)
	assert.False(t, config.IncludeTrace)
	assert.True(t, config.IncludeFile)
}

func TestNew(t *testing.T) {
	buf := &bytes.Buffer{}
	config := &Config{
		Level:     LevelDebug,
		Output:    buf,
		Component: "test",
	}

	logger := New(config)

	assert.NotNil(t, logger)
	assert.Equal(t, LevelDebug, logger.level)
	assert.Equal(t, "test", logger.component)
}

func TestNew_NilConfig(t *testing.T) {
	logger := New(nil)
	assert.NotNil(t, logger)
	assert.Equal(t, LevelInfo, logger.level)
}

func TestLogger_SetLevel(t *testing.T) {
	logger := New(DefaultConfig())

	logger.SetLevel(LevelDebug)
	assert.Equal(t, LevelDebug, logger.GetLevel())

	logger.SetLevel(LevelError)
	assert.Equal(t, LevelError, logger.GetLevel())
}

func TestLogger_WithComponent(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{Output: buf, Component: "parent"})

	childLogger := logger.WithComponent("child")

	assert.Equal(t, "parent", logger.component)
	assert.Equal(t, "child", childLogger.component)
}

func TestLogger_WithFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{Output: buf})

	fields := Fields{"key1": "value1", "key2": 42}
	newLogger := logger.WithFields(fields)

	newLogger.Info("test message")

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "value1", entry.Fields["key1"])
	assert.Equal(t, float64(42), entry.Fields["key2"]) // JSON unmarshals numbers as float64
}

func TestLogger_Debug(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelDebug,
		Output: buf,
	})

	logger.Debug("debug message", Fields{"key": "value"})

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "DEBUG", entry.Level)
	assert.Equal(t, "debug message", entry.Message)
	assert.Equal(t, "value", entry.Fields["key"])
	assert.False(t, entry.Timestamp.IsZero())
}

func TestLogger_Info(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelInfo,
		Output: buf,
	})

	logger.Info("info message")

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "INFO", entry.Level)
	assert.Equal(t, "info message", entry.Message)
}

func TestLogger_Warn(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelWarn,
		Output: buf,
	})

	logger.Warn("warning message")

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "WARN", entry.Level)
	assert.Equal(t, "warning message", entry.Message)
}

func TestLogger_Error(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelError,
		Output: buf,
	})

	testErr := errors.New("test error")
	logger.Error("error message", testErr)

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "ERROR", entry.Level)
	assert.Equal(t, "error message", entry.Message)
	assert.Equal(t, "test error", entry.Error)
}

func TestLogger_ErrorWithStackTrace(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:        LevelError,
		Output:       buf,
		IncludeTrace: true,
	})

	testErr := errors.New("test error")
	logger.Error("error message", testErr)

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "ERROR", entry.Level)
	assert.NotEmpty(t, entry.StackTrace)
	assert.Contains(t, entry.StackTrace, "goroutine")
}

func TestLogger_WithOperation(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelInfo,
		Output: buf,
	})

	logger.InfoOp("test.operation", "operation message")

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "INFO", entry.Level)
	assert.Equal(t, "operation message", entry.Message)
	assert.Equal(t, "test.operation", entry.Operation)
}

func TestLogger_ComponentField(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:     LevelInfo,
		Output:    buf,
		Component: "test-component",
	})

	logger.Info("test message")

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "test-component", entry.Component)
}

func TestLogger_LevelFiltering(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelWarn,
		Output: buf,
	})

	// These should not be logged
	logger.Debug("debug message")
	logger.Info("info message")

	// These should be logged
	logger.Warn("warn message")
	logger.Error("error message", nil)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should only have 2 lines (warn and error)
	assert.Len(t, lines, 2)

	var entry1 Entry
	json.Unmarshal([]byte(lines[0]), &entry1)
	assert.Equal(t, "WARN", entry1.Level)

	var entry2 Entry
	json.Unmarshal([]byte(lines[1]), &entry2)
	assert.Equal(t, "ERROR", entry2.Level)
}

func TestLogger_MultipleFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelInfo,
		Output: buf,
	})

	fields1 := Fields{"key1": "value1"}
	fields2 := Fields{"key2": "value2"}

	logger.Info("test message", fields1, fields2)

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "value1", entry.Fields["key1"])
	assert.Equal(t, "value2", entry.Fields["key2"])
}

func TestLogger_DefaultFieldsInherited(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelInfo,
		Output: buf,
	})

	parentLogger := logger.WithFields(Fields{"parent_key": "parent_value"})
	childLogger := parentLogger.WithFields(Fields{"child_key": "child_value"})

	childLogger.Info("test message")

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "parent_value", entry.Fields["parent_key"])
	assert.Equal(t, "child_value", entry.Fields["child_key"])
}

func TestLogger_FileAndLine(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:       LevelInfo,
		Output:      buf,
		IncludeFile: true,
	})

	logger.Info("test message")

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.NotEmpty(t, entry.File)
	assert.Contains(t, entry.File, "logger_test.go")
	assert.Greater(t, entry.Line, 0)
}

func TestLogger_NoFileAndLine(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:       LevelInfo,
		Output:      buf,
		IncludeFile: false,
	})

	logger.Info("test message")

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Empty(t, entry.File)
	assert.Equal(t, 0, entry.Line)
}

func TestDefaultLogger(t *testing.T) {
	// Test that default logger exists and works
	defaultLogger := Default()
	assert.NotNil(t, defaultLogger)

	// Test global functions
	buf := &bytes.Buffer{}
	testLogger := New(&Config{
		Level:  LevelInfo,
		Output: buf,
	})
	SetDefaultLogger(testLogger)

	Info("test message", Fields{"key": "value"})

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "INFO", entry.Level)
	assert.Equal(t, "test message", entry.Message)
}

func TestGlobalFunctions(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelDebug,
		Output: buf,
	})
	SetDefaultLogger(logger)

	// Test all global functions
	Debug("debug")
	Info("info")
	Warn("warn")
	Error("error", errors.New("test"))
	DebugOp("op", "debug op")
	InfoOp("op", "info op")
	WarnOp("op", "warn op")
	ErrorOp("op", "error op", errors.New("test"))

	output := buf.String()
	assert.Contains(t, output, "debug")
	assert.Contains(t, output, "info")
	assert.Contains(t, output, "warn")
	assert.Contains(t, output, "error")
}

func TestLogger_JSONFormatting(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:       LevelInfo,
		Output:      buf,
		Component:   "test",
		IncludeFile: true,
	})

	logger.InfoOp("test.operation", "test message", Fields{
		"string": "value",
		"number": 42,
		"bool":   true,
	})

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	// Verify all fields are present and valid JSON
	assert.False(t, entry.Timestamp.IsZero())
	assert.Equal(t, "INFO", entry.Level)
	assert.Equal(t, "test message", entry.Message)
	assert.Equal(t, "test", entry.Component)
	assert.Equal(t, "test.operation", entry.Operation)
	assert.Equal(t, "value", entry.Fields["string"])
	assert.Equal(t, float64(42), entry.Fields["number"])
	assert.Equal(t, true, entry.Fields["bool"])
	assert.NotEmpty(t, entry.File)
	assert.Greater(t, entry.Line, 0)
}

func TestLogger_ErrorOperationMethods(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelDebug,
		Output: buf,
	})

	testErr := errors.New("test error")

	logger.DebugOp("debug.op", "debug message")
	buf.Reset()

	logger.InfoOp("info.op", "info message")
	buf.Reset()

	logger.WarnOp("warn.op", "warn message")
	buf.Reset()

	logger.ErrorOp("error.op", "error message", testErr)

	var entry Entry
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "ERROR", entry.Level)
	assert.Equal(t, "error.op", entry.Operation)
	assert.Equal(t, "test error", entry.Error)
}

func BenchmarkLogger_Info(b *testing.B) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelInfo,
		Output: buf,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message")
	}
}

func BenchmarkLogger_InfoWithFields(b *testing.B) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelInfo,
		Output: buf,
	})

	fields := Fields{"key1": "value1", "key2": 42, "key3": true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", fields)
	}
}

func BenchmarkLogger_Error(b *testing.B) {
	buf := &bytes.Buffer{}
	logger := New(&Config{
		Level:  LevelError,
		Output: buf,
	})

	testErr := errors.New("test error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Error("benchmark error", testErr)
	}
}

func ExampleLogger() {
	logger := New(&Config{
		Level:     LevelInfo,
		Component: "example",
	})

	logger.Info("Application started")
	logger.InfoOp("database.connect", "Connected to database", Fields{
		"host": "localhost",
		"port": 5432,
	})
	logger.Error("Failed to process request", errors.New("connection timeout"))
}

func ExampleLogger_WithComponent() {
	logger := New(DefaultConfig())

	dbLogger := logger.WithComponent("database")
	dbLogger.Info("Database initialized")

	apiLogger := logger.WithComponent("api")
	apiLogger.Info("API server started")
}

func ExampleLogger_WithFields() {
	logger := New(DefaultConfig())

	requestLogger := logger.WithFields(Fields{
		"request_id": "abc123",
		"user_id":    "user456",
	})

	requestLogger.Info("Processing request")
	requestLogger.Info("Request completed")
}
