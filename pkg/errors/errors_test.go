package errors

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCategory_String(t *testing.T) {
	tests := []struct {
		category Category
		expected string
	}{
		{CategoryRetriable, "retriable"},
		{CategoryTransient, "transient"},
		{CategoryFatal, "fatal"},
		{CategoryInvalidInput, "invalid_input"},
		{CategoryUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.category.String())
		})
	}
}

func TestNew(t *testing.T) {
	baseErr := errors.New("base error")
	err := New(CategoryRetriable, "test.operation", baseErr)

	require.NotNil(t, err)
	assert.Equal(t, CategoryRetriable, err.Category)
	assert.Equal(t, "test.operation", err.Op)
	assert.Equal(t, baseErr, err.Err)
	assert.NotNil(t, err.Metadata)
	assert.False(t, err.Timestamp.IsZero())
}

func TestRetriable(t *testing.T) {
	baseErr := errors.New("network timeout")
	err := Retriable("connect", baseErr)

	assert.Equal(t, CategoryRetriable, err.Category)
	assert.Equal(t, "connect", err.Op)
	assert.Equal(t, baseErr, err.Err)
}

func TestTransient(t *testing.T) {
	baseErr := errors.New("resource unavailable")
	err := Transient("allocate", baseErr)

	assert.Equal(t, CategoryTransient, err.Category)
	assert.Equal(t, "allocate", err.Op)
}

func TestFatal(t *testing.T) {
	baseErr := errors.New("data corruption")
	err := Fatal("read", baseErr)

	assert.Equal(t, CategoryFatal, err.Category)
	assert.Equal(t, "read", err.Op)
}

func TestInvalidInput(t *testing.T) {
	baseErr := errors.New("invalid parameter")
	err := InvalidInput("validate", baseErr)

	assert.Equal(t, CategoryInvalidInput, err.Category)
	assert.Equal(t, "validate", err.Op)
}

func TestError_WithMessage(t *testing.T) {
	baseErr := errors.New("base")
	err := Retriable("op", baseErr).WithMessage("custom message")

	assert.Equal(t, "custom message", err.Message)
	assert.Contains(t, err.Error(), "custom message")
}

func TestError_WithCode(t *testing.T) {
	baseErr := errors.New("base")
	err := Retriable("op", baseErr).WithCode("ERR_001")

	assert.Equal(t, "ERR_001", err.Code)
}

func TestError_WithMetadata(t *testing.T) {
	baseErr := errors.New("base")
	err := Retriable("op", baseErr).
		WithMetadata("key1", "value1").
		WithMetadata("key2", 42)

	assert.Equal(t, "value1", err.Metadata["key1"])
	assert.Equal(t, 42, err.Metadata["key2"])
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		contains []string
	}{
		{
			name: "basic error",
			err:  Retriable("test.op", errors.New("base error")),
			contains: []string{
				"retriable",
				"test.op",
				"base error",
			},
		},
		{
			name: "error with message",
			err:  Retriable("test.op", errors.New("base")).WithMessage("custom msg"),
			contains: []string{
				"retriable",
				"test.op",
				"custom msg",
				"base",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.err.Error()
			for _, substr := range tt.contains {
				assert.Contains(t, errStr, substr)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	baseErr := errors.New("base error")
	err := Retriable("op", baseErr)

	unwrapped := err.Unwrap()
	assert.Equal(t, baseErr, unwrapped)
}

func TestError_Is(t *testing.T) {
	err1 := Retriable("op", errors.New("base")).WithCode("ERR_001")
	err2 := Retriable("op", errors.New("other")).WithCode("ERR_001")
	err3 := Transient("op", errors.New("base")).WithCode("ERR_001")
	err4 := Retriable("op", errors.New("base")).WithCode("ERR_002")

	// Same category and code
	assert.True(t, err1.Is(err2))

	// Different category
	assert.False(t, err1.Is(err3))

	// Different code
	assert.False(t, err1.Is(err4))

	// Not an *Error
	assert.False(t, err1.Is(errors.New("other")))
}

func TestGetCategory(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected Category
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: CategoryUnknown,
		},
		{
			name:     "structured retriable",
			err:      Retriable("op", errors.New("base")),
			expected: CategoryRetriable,
		},
		{
			name:     "structured transient",
			err:      Transient("op", errors.New("base")),
			expected: CategoryTransient,
		},
		{
			name:     "structured fatal",
			err:      Fatal("op", errors.New("base")),
			expected: CategoryFatal,
		},
		{
			name:     "structured invalid input",
			err:      InvalidInput("op", errors.New("base")),
			expected: CategoryInvalidInput,
		},
		{
			name:     "ErrNotLeader",
			err:      ErrNotLeader,
			expected: CategoryRetriable,
		},
		{
			name:     "ErrTimeout",
			err:      ErrTimeout,
			expected: CategoryTransient,
		},
		{
			name:     "ErrNotFound",
			err:      ErrNotFound,
			expected: CategoryInvalidInput,
		},
		{
			name:     "unknown error",
			err:      errors.New("unknown"),
			expected: CategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := GetCategory(tt.err)
			assert.Equal(t, tt.expected, category)
		})
	}
}

func TestIsRetriable(t *testing.T) {
	assert.True(t, IsRetriable(Retriable("op", errors.New("base"))))
	assert.True(t, IsRetriable(ErrNotLeader))
	assert.False(t, IsRetriable(Fatal("op", errors.New("base"))))
	assert.False(t, IsRetriable(nil))
}

func TestIsTransient(t *testing.T) {
	assert.True(t, IsTransient(Transient("op", errors.New("base"))))
	assert.True(t, IsTransient(ErrTimeout))
	assert.False(t, IsTransient(Fatal("op", errors.New("base"))))
}

func TestIsFatal(t *testing.T) {
	assert.True(t, IsFatal(Fatal("op", errors.New("base"))))
	assert.False(t, IsFatal(Retriable("op", errors.New("base"))))
}

func TestIsInvalidInput(t *testing.T) {
	assert.True(t, IsInvalidInput(InvalidInput("op", errors.New("base"))))
	assert.True(t, IsInvalidInput(ErrNotFound))
	assert.True(t, IsInvalidInput(ErrAlreadyExists))
	assert.False(t, IsInvalidInput(Fatal("op", errors.New("base"))))
}

func TestWrap(t *testing.T) {
	tests := []struct {
		name     string
		op       string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			op:       "test",
			err:      nil,
			expected: "",
		},
		{
			name:     "standard error",
			op:       "operation",
			err:      errors.New("base error"),
			expected: "operation",
		},
		{
			name:     "already wrapped",
			op:       "outer",
			err:      Retriable("inner", errors.New("base")),
			expected: "outer.inner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := Wrap(tt.op, tt.err)
			if tt.err == nil {
				assert.Nil(t, wrapped)
			} else {
				assert.NotNil(t, wrapped)
				if tt.expected != "" {
					assert.Contains(t, wrapped.Error(), tt.expected)
				}
			}
		})
	}
}

func TestWrapf(t *testing.T) {
	baseErr := errors.New("base error")
	wrapped := Wrapf("operation", baseErr, "failed with code %d", 500)

	require.NotNil(t, wrapped)
	assert.Contains(t, wrapped.Error(), "operation")
	assert.Contains(t, wrapped.Error(), "failed with code 500")

	// Test with nil error
	nilWrapped := Wrapf("op", nil, "message")
	assert.Nil(t, nilWrapped)
}

func TestJoin(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	err3 := errors.New("error 3")

	tests := []struct {
		name     string
		errs     []error
		expected string
	}{
		{
			name:     "no errors",
			errs:     []error{},
			expected: "",
		},
		{
			name:     "single error",
			errs:     []error{err1},
			expected: "error 1",
		},
		{
			name:     "multiple errors",
			errs:     []error{err1, err2, err3},
			expected: "error 1",
		},
		{
			name:     "with nil errors",
			errs:     []error{err1, nil, err2},
			expected: "error 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Join(tt.errs...)
			if tt.expected == "" {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Contains(t, result.Error(), tt.expected)
			}
		})
	}
}

func TestMultiError(t *testing.T) {
	multi := NewMultiError("batch.operation")

	// Initially no errors
	assert.Nil(t, multi.ErrorOrNil())

	// Add errors
	multi.Add(errors.New("error 1"))
	multi.Add(errors.New("error 2"))
	multi.Add(nil) // Should be ignored

	assert.Len(t, multi.Errors, 2)
	assert.NotNil(t, multi.ErrorOrNil())
}

func TestMultiError_Error(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *MultiError
		contains string
	}{
		{
			name: "no errors",
			setup: func() *MultiError {
				return NewMultiError("op")
			},
			contains: "no errors",
		},
		{
			name: "single error",
			setup: func() *MultiError {
				m := NewMultiError("op")
				m.Add(errors.New("single error"))
				return m
			},
			contains: "single error",
		},
		{
			name: "multiple errors",
			setup: func() *MultiError {
				m := NewMultiError("batch")
				m.Add(errors.New("error 1"))
				m.Add(errors.New("error 2"))
				return m
			},
			contains: "2 errors occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multi := tt.setup()
			errStr := multi.Error()
			assert.Contains(t, errStr, tt.contains)
		})
	}
}

func TestMultiError_Unwrap(t *testing.T) {
	multi := NewMultiError("op")

	// No errors - should return nil
	assert.Nil(t, multi.Unwrap())

	// With errors - should return first
	err1 := errors.New("first error")
	err2 := errors.New("second error")
	multi.Add(err1)
	multi.Add(err2)

	unwrapped := multi.Unwrap()
	assert.Equal(t, err1, unwrapped)
}

func TestCommonErrors(t *testing.T) {
	// Verify common errors are defined
	assert.NotNil(t, ErrNotFound)
	assert.NotNil(t, ErrAlreadyExists)
	assert.NotNil(t, ErrNotLeader)
	assert.NotNil(t, ErrNoLeader)
	assert.NotNil(t, ErrTimeout)
	assert.NotNil(t, ErrTemporaryFailure)
	assert.NotNil(t, ErrInvalidState)
	assert.NotNil(t, ErrCanceled)
	assert.NotNil(t, ErrShutdown)
}

func TestError_ChainedOperations(t *testing.T) {
	baseErr := errors.New("base error")

	err := Retriable("operation", baseErr).
		WithMessage("operation failed").
		WithCode("ERR_RETRY").
		WithMetadata("attempt", 3).
		WithMetadata("timeout", "5s")

	assert.Equal(t, CategoryRetriable, err.Category)
	assert.Equal(t, "operation", err.Op)
	assert.Equal(t, "operation failed", err.Message)
	assert.Equal(t, "ERR_RETRY", err.Code)
	assert.Equal(t, 3, err.Metadata["attempt"])
	assert.Equal(t, "5s", err.Metadata["timeout"])
	assert.False(t, err.Timestamp.IsZero())
}

func TestError_NestedWrapping(t *testing.T) {
	base := errors.New("base error")
	level1 := Wrap("level1", base)
	level2 := Wrap("level2", level1)
	level3 := Wrap("level3", level2)

	errStr := level3.Error()
	assert.Contains(t, errStr, "level3.level2.level1")
	assert.Contains(t, errStr, "base error")

	// Verify we can unwrap
	assert.True(t, errors.Is(level3, base))
}

func TestError_ErrorsAs(t *testing.T) {
	base := errors.New("base")
	wrapped := Retriable("op", base)
	doubleWrapped := fmt.Errorf("outer: %w", wrapped)

	var structuredErr *Error
	assert.True(t, errors.As(doubleWrapped, &structuredErr))
	assert.Equal(t, CategoryRetriable, structuredErr.Category)
}

func TestClassifyStandardError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected Category
	}{
		{"timeout", ErrTimeout, CategoryTransient},
		{"temporary failure", ErrTemporaryFailure, CategoryTransient},
		{"not found", ErrNotFound, CategoryInvalidInput},
		{"already exists", ErrAlreadyExists, CategoryInvalidInput},
		{"not leader", ErrNotLeader, CategoryRetriable},
		{"no leader", ErrNoLeader, CategoryRetriable},
		{"unknown", errors.New("unknown error"), CategoryUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := classifyStandardError(tt.err)
			assert.Equal(t, tt.expected, category)
		})
	}
}

func BenchmarkNew(b *testing.B) {
	baseErr := errors.New("base error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = New(CategoryRetriable, "operation", baseErr)
	}
}

func BenchmarkWrap(b *testing.B) {
	baseErr := errors.New("base error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Wrap("operation", baseErr)
	}
}

func BenchmarkGetCategory(b *testing.B) {
	err := Retriable("op", errors.New("base"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetCategory(err)
	}
}

func ExampleRetriable() {
	err := Retriable("connect.database", ErrTimeout).
		WithMessage("failed to connect to database").
		WithMetadata("host", "db.example.com").
		WithMetadata("timeout", "5s")

	fmt.Println("Category:", err.Category)
	fmt.Println("Retriable:", IsRetriable(err))
	// Output:
	// Category: retriable
	// Retriable: true
}

func ExampleWrap() {
	baseErr := errors.New("connection refused")
	wrapped := Wrap("database.connect", baseErr)

	if IsRetriable(wrapped) {
		fmt.Println("Can retry")
	}

	// Error string contains operation context
	errStr := wrapped.Error()
	_ = strings.Contains(errStr, "database.connect")
}

func ExampleMultiError() {
	multi := NewMultiError("batch.process")

	for i := 0; i < 3; i++ {
		// Simulate processing
		if i == 1 {
			multi.Add(fmt.Errorf("item %d failed", i))
		}
	}

	if err := multi.ErrorOrNil(); err != nil {
		fmt.Println("Batch processing had errors")
	}
	// Output: Batch processing had errors
}
