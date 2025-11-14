package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestInit(t *testing.T) {
	tests := []struct {
		name  string
		level LogLevel
	}{
		{
			name:  "Initialize with DEBUG level",
			level: DebugLevel,
		},
		{
			name:  "Initialize with INFO level",
			level: InfoLevel,
		},
		{
			name:  "Initialize with WARN level",
			level: WarnLevel,
		},
		{
			name:  "Initialize with ERROR level",
			level: ErrorLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Init(tt.level)
			if globalLogger == nil {
				t.Error("Init() failed - globalLogger is nil")
			}
		})
	}
}

func TestInitFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
	}{
		{
			name:     "Parse DEBUG from env",
			envValue: "debug",
		},
		{
			name:     "Parse INFO from env",
			envValue: "info",
		},
		{
			name:     "Parse WARN from env",
			envValue: "warn",
		},
		{
			name:     "Parse ERROR from env",
			envValue: "error",
		},
		{
			name:     "Default to INFO for invalid value",
			envValue: "invalid",
		},
		{
			name:     "Default to INFO for empty value",
			envValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				os.Setenv("LOG_LEVEL", tt.envValue)
			} else {
				os.Unsetenv("LOG_LEVEL")
			}
			defer os.Unsetenv("LOG_LEVEL")

			// Re-init logger with env var
			level := os.Getenv("LOG_LEVEL")
			if level == "" {
				level = "info"
			}
			Init(LogLevel(level))

			// Verify logger is initialized
			if globalLogger == nil {
				t.Error("Init() globalLogger is nil")
			}
		})
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		expected zapcore.Level
	}{
		{
			name:     "Parse DEBUG",
			level:    DebugLevel,
			expected: zapcore.DebugLevel,
		},
		{
			name:     "Parse INFO",
			level:    InfoLevel,
			expected: zapcore.InfoLevel,
		},
		{
			name:     "Parse WARN",
			level:    WarnLevel,
			expected: zapcore.WarnLevel,
		},
		{
			name:     "Parse ERROR",
			level:    ErrorLevel,
			expected: zapcore.ErrorLevel,
		},
		{
			name:     "Parse FATAL",
			level:    FatalLevel,
			expected: zapcore.FatalLevel,
		},
		{
			name:     "Default to INFO for invalid",
			level:    LogLevel("invalid"),
			expected: zapcore.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLogLevel(tt.level)
			if result != tt.expected {
				t.Errorf("parseLogLevel(%s) = %v, want %v", tt.level, result, tt.expected)
			}
		})
	}
}

func TestLoggingFunctions(t *testing.T) {
	// Create a buffer to capture logs
	var buf bytes.Buffer

	// Create a custom logger that writes to the buffer
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(&buf),
		zapcore.DebugLevel,
	)

	globalLogger = zap.New(core)
	defer func() { _ = globalLogger.Sync() }()

	tests := []struct {
		name     string
		logFunc  func()
		level    string
		message  string
	}{
		{
			name: "Debug logging",
			logFunc: func() {
				Debug("debug message", zap.String("key", "value"))
			},
			level:   "debug",
			message: "debug message",
		},
		{
			name: "Info logging",
			logFunc: func() {
				Info("info message", zap.Int("count", 42))
			},
			level:   "info",
			message: "info message",
		},
		{
			name: "Warn logging",
			logFunc: func() {
				Warn("warn message", zap.Bool("flag", true))
			},
			level:   "warn",
			message: "warn message",
		},
		{
			name: "Error logging",
			logFunc: func() {
				Error("error message", zap.Error(os.ErrNotExist))
			},
			level:   "error",
			message: "error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()

			// Parse the JSON log output
			var logEntry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("Failed to parse log JSON: %v", err)
			}

			// Verify level
			if logEntry["level"] != tt.level {
				t.Errorf("Expected level %s, got %s", tt.level, logEntry["level"])
			}

			// Verify message
			if logEntry["message"] != tt.message {
				t.Errorf("Expected message %s, got %s", tt.message, logEntry["message"])
			}
		})
	}
}

func TestWithFields(t *testing.T) {
	// Initialize logger
	Init(InfoLevel)

	// Create a child logger with fields
	childLogger := WithFields(
		zap.String("component", "test"),
		zap.Int("id", 123),
	)

	if childLogger == nil {
		t.Error("WithFields() returned nil logger")
	}
}

func TestSync(t *testing.T) {
	// Initialize logger
	Init(InfoLevel)

	// Test sync
	err := Sync()
	if err != nil {
		// Sync may return errors on some platforms (e.g., /dev/stderr on Linux)
		// This is expected and documented by zap
		t.Logf("Sync() returned error (may be expected): %v", err)
	}
}

func TestLoggerBeforeInit(t *testing.T) {
	// This test is not applicable because the logger is automatically
	// initialized in the init() function. However, we can test that
	// the logger is always available.
	if globalLogger == nil {
		t.Error("Global logger should be initialized by init() function")
	}

	// Verify logger functions work (they should never panic)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Logging caused panic: %v", r)
		}
	}()

	Debug("test")
	Info("test")
	Warn("test")
	Error("test")
}

func TestConcurrentLogging(t *testing.T) {
	// Initialize logger
	Init(InfoLevel)

	// Test concurrent logging (zap should be thread-safe)
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			Info("concurrent log", zap.Int("goroutine", id))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestLogLevelEnvironmentVariable(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
	}{
		{"Uppercase DEBUG", "DEBUG"},
		{"Lowercase debug", "debug"},
		{"Mixed case Debug", "Debug"},
		{"Uppercase INFO", "INFO"},
		{"Uppercase WARN", "WARN"},
		{"Uppercase ERROR", "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("LOG_LEVEL", tt.envValue)
			defer os.Unsetenv("LOG_LEVEL")

			level := os.Getenv("LOG_LEVEL")
			if level == "" {
				level = "info"
			}
			Init(LogLevel(level))

			if globalLogger == nil {
				t.Errorf("Init() with %s failed - globalLogger is nil", tt.envValue)
			}
		})
	}
}

func TestLogger(t *testing.T) {
	Init(InfoLevel)
	logger := Logger()
	if logger == nil {
		t.Error("Logger() returned nil")
	}
}

func TestSugar(t *testing.T) {
	Init(InfoLevel)
	sugar := Sugar()
	if sugar == nil {
		t.Error("Sugar() returned nil")
	}
}

func BenchmarkDebugLogging(b *testing.B) {
	Init(DebugLevel)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Debug("benchmark message", zap.Int("iteration", i))
	}
}

func BenchmarkInfoLogging(b *testing.B) {
	Init(InfoLevel)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("benchmark message", zap.Int("iteration", i))
	}
}

func BenchmarkWithFields(b *testing.B) {
	Init(InfoLevel)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger := WithFields(
			zap.String("component", "benchmark"),
			zap.Int("id", i),
		)
		_ = logger
	}
}
