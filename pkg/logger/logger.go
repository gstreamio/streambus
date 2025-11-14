package logger

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Global logger instance
	globalLogger *zap.Logger
	// Sugar logger for convenience methods
	globalSugar *zap.SugaredLogger
)

// LogLevel represents log levels
type LogLevel string

const (
	DebugLevel LogLevel = "debug"
	InfoLevel  LogLevel = "info"
	WarnLevel  LogLevel = "warn"
	ErrorLevel LogLevel = "error"
	FatalLevel LogLevel = "fatal"
)

func init() {
	// Initialize with production config by default
	// Can be overridden by calling Init()
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}
	Init(LogLevel(level))
}

// Init initializes the global logger with the specified log level
func Init(level LogLevel) {
	zapLevel := parseLogLevel(level)

	// Use production config for performance
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapLevel)

	// Use JSON encoding for structured logs
	config.Encoding = "json"

	// Optimize for performance
	config.DisableCaller = true
	config.DisableStacktrace = true

	// For better readability in development, check if we should use console encoding
	if os.Getenv("LOG_FORMAT") == "console" {
		config.Encoding = "console"
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	var err error
	globalLogger, err = config.Build(
		// Add sampling for high-frequency logs (after first 100, then 10% thereafter per second)
		zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return zapcore.NewSamplerWithOptions(
				core,
				time.Second, // Sample period
				100,         // Log first 100 per second
				10,          // Then only log 10% thereafter
			)
		}),
	)
	if err != nil {
		panic(err)
	}

	globalSugar = globalLogger.Sugar()
}

// parseLogLevel converts string to zapcore.Level
func parseLogLevel(level LogLevel) zapcore.Level {
	switch strings.ToLower(string(level)) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// Logger returns the global logger instance
func Logger() *zap.Logger {
	return globalLogger
}

// Sugar returns the global sugar logger instance
func Sugar() *zap.SugaredLogger {
	return globalSugar
}

// Debug logs a debug message
func Debug(msg string, fields ...zap.Field) {
	globalLogger.Debug(msg, fields...)
}

// Info logs an info message
func Info(msg string, fields ...zap.Field) {
	globalLogger.Info(msg, fields...)
}

// Warn logs a warning message
func Warn(msg string, fields ...zap.Field) {
	globalLogger.Warn(msg, fields...)
}

// Error logs an error message
func Error(msg string, fields ...zap.Field) {
	globalLogger.Error(msg, fields...)
}

// Fatal logs a fatal message and exits
func Fatal(msg string, fields ...zap.Field) {
	globalLogger.Fatal(msg, fields...)
}

// Sync flushes any buffered log entries
func Sync() error {
	return globalLogger.Sync()
}

// WithFields creates a new logger with the given fields
func WithFields(fields ...zap.Field) *zap.Logger {
	return globalLogger.With(fields...)
}
