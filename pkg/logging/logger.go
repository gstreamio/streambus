package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level represents the severity level of a log message
type Level int

const (
	// LevelDebug is for detailed debugging information
	LevelDebug Level = iota
	// LevelInfo is for informational messages
	LevelInfo
	// LevelWarn is for warning messages
	LevelWarn
	// LevelError is for error messages
	LevelError
	// LevelFatal is for fatal errors
	LevelFatal
)

// String returns the string representation of the log level
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a string into a log level
func ParseLevel(s string) (Level, error) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return LevelDebug, nil
	case "INFO":
		return LevelInfo, nil
	case "WARN", "WARNING":
		return LevelWarn, nil
	case "ERROR":
		return LevelError, nil
	case "FATAL":
		return LevelFatal, nil
	default:
		return LevelInfo, fmt.Errorf("invalid log level: %s", s)
	}
}

// Fields represents structured log fields
type Fields map[string]interface{}

// Entry represents a single log entry
type Entry struct {
	Timestamp  time.Time              `json:"timestamp"`
	Level      string                 `json:"level"`
	Message    string                 `json:"message"`
	Fields     map[string]interface{} `json:"fields,omitempty"`
	Component  string                 `json:"component,omitempty"`
	Operation  string                 `json:"operation,omitempty"`
	Error      string                 `json:"error,omitempty"`
	StackTrace string                 `json:"stack_trace,omitempty"`
	File       string                 `json:"file,omitempty"`
	Line       int                    `json:"line,omitempty"`
}

// Logger is the main logging interface
type Logger struct {
	mu           sync.RWMutex
	level        Level
	output       io.Writer
	component    string
	defaultFields Fields
	includeTrace bool
	includeFile  bool
}

// Config holds logger configuration
type Config struct {
	Level        Level
	Output       io.Writer
	Component    string
	IncludeTrace bool
	IncludeFile  bool
}

// DefaultConfig returns a default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:        LevelInfo,
		Output:       os.Stdout,
		IncludeTrace: false,
		IncludeFile:  true,
	}
}

// New creates a new logger with the given configuration
func New(config *Config) *Logger {
	if config == nil {
		config = DefaultConfig()
	}
	if config.Output == nil {
		config.Output = os.Stdout
	}
	return &Logger{
		level:         config.Level,
		output:        config.Output,
		component:     config.Component,
		defaultFields: make(Fields),
		includeTrace:  config.IncludeTrace,
		includeFile:   config.IncludeFile,
	}
}

// WithComponent returns a new logger with the specified component
func (l *Logger) WithComponent(component string) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	newLogger := &Logger{
		level:         l.level,
		output:        l.output,
		component:     component,
		defaultFields: make(Fields),
		includeTrace:  l.includeTrace,
		includeFile:   l.includeFile,
	}
	// Copy default fields
	for k, v := range l.defaultFields {
		newLogger.defaultFields[k] = v
	}
	return newLogger
}

// WithFields returns a new logger with the specified default fields
func (l *Logger) WithFields(fields Fields) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	newLogger := &Logger{
		level:         l.level,
		output:        l.output,
		component:     l.component,
		defaultFields: make(Fields),
		includeTrace:  l.includeTrace,
		includeFile:   l.includeFile,
	}
	// Copy existing default fields
	for k, v := range l.defaultFields {
		newLogger.defaultFields[k] = v
	}
	// Add new fields
	for k, v := range fields {
		newLogger.defaultFields[k] = v
	}
	return newLogger
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel returns the current log level
func (l *Logger) GetLevel() Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields ...Fields) {
	l.log(LevelDebug, msg, nil, "", fields...)
}

// Info logs an info message
func (l *Logger) Info(msg string, fields ...Fields) {
	l.log(LevelInfo, msg, nil, "", fields...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields ...Fields) {
	l.log(LevelWarn, msg, nil, "", fields...)
}

// Error logs an error message
func (l *Logger) Error(msg string, err error, fields ...Fields) {
	l.log(LevelError, msg, err, "", fields...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, err error, fields ...Fields) {
	l.log(LevelFatal, msg, err, "", fields...)
	os.Exit(1)
}

// DebugOp logs a debug message with operation context
func (l *Logger) DebugOp(op, msg string, fields ...Fields) {
	l.log(LevelDebug, msg, nil, op, fields...)
}

// InfoOp logs an info message with operation context
func (l *Logger) InfoOp(op, msg string, fields ...Fields) {
	l.log(LevelInfo, msg, nil, op, fields...)
}

// WarnOp logs a warning message with operation context
func (l *Logger) WarnOp(op, msg string, fields ...Fields) {
	l.log(LevelWarn, msg, nil, op, fields...)
}

// ErrorOp logs an error message with operation context
func (l *Logger) ErrorOp(op, msg string, err error, fields ...Fields) {
	l.log(LevelError, msg, err, op, fields...)
}

// log is the internal logging method
func (l *Logger) log(level Level, msg string, err error, operation string, fields ...Fields) {
	l.mu.RLock()
	currentLevel := l.level
	l.mu.RUnlock()

	// Check if we should log at this level
	if level < currentLevel {
		return
	}

	// Build the entry
	entry := Entry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   msg,
		Component: l.component,
		Operation: operation,
		Fields:    make(map[string]interface{}),
	}

	// Add default fields
	for k, v := range l.defaultFields {
		entry.Fields[k] = v
	}

	// Add provided fields
	for _, fieldSet := range fields {
		for k, v := range fieldSet {
			entry.Fields[k] = v
		}
	}

	// Add error if present
	if err != nil {
		entry.Error = err.Error()
		if l.includeTrace {
			entry.StackTrace = getStackTrace()
		}
	}

	// Add file and line if configured
	if l.includeFile {
		file, line := getFileLine(3) // Skip this function and the caller
		entry.File = file
		entry.Line = line
	}

	// Write the log entry
	l.write(entry)
}

// write writes the log entry to the output
func (l *Logger) write(entry Entry) {
	l.mu.RLock()
	output := l.output
	l.mu.RUnlock()

	data, err := json.Marshal(entry)
	if err != nil {
		// Fallback to simple text output
		fmt.Fprintf(output, "ERROR: Failed to marshal log entry: %v\n", err)
		return
	}

	_, _ = output.Write(data)
	_, _ = output.Write([]byte("\n"))
}

// getFileLine returns the file and line number of the caller
func getFileLine(skip int) (string, int) {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown", 0
	}
	// Get just the filename, not the full path
	parts := strings.Split(file, "/")
	if len(parts) > 0 {
		file = parts[len(parts)-1]
	}
	return file, line
}

// getStackTrace returns a stack trace as a string
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// Global default logger
var defaultLogger = New(DefaultConfig())

// SetDefaultLogger sets the global default logger
func SetDefaultLogger(logger *Logger) {
	defaultLogger = logger
}

// Default returns the default logger
func Default() *Logger {
	return defaultLogger
}

// Debug logs a debug message using the default logger
func Debug(msg string, fields ...Fields) {
	defaultLogger.Debug(msg, fields...)
}

// Info logs an info message using the default logger
func Info(msg string, fields ...Fields) {
	defaultLogger.Info(msg, fields...)
}

// Warn logs a warning message using the default logger
func Warn(msg string, fields ...Fields) {
	defaultLogger.Warn(msg, fields...)
}

// Error logs an error message using the default logger
func Error(msg string, err error, fields ...Fields) {
	defaultLogger.Error(msg, err, fields...)
}

// DebugOp logs a debug message with operation context using the default logger
func DebugOp(op, msg string, fields ...Fields) {
	defaultLogger.DebugOp(op, msg, fields...)
}

// InfoOp logs an info message with operation context using the default logger
func InfoOp(op, msg string, fields ...Fields) {
	defaultLogger.InfoOp(op, msg, fields...)
}

// WarnOp logs a warning message with operation context using the default logger
func WarnOp(op, msg string, fields ...Fields) {
	defaultLogger.WarnOp(op, msg, fields...)
}

// ErrorOp logs an error message with operation context using the default logger
func ErrorOp(op, msg string, err error, fields ...Fields) {
	defaultLogger.ErrorOp(op, msg, err, fields...)
}
