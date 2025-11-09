package errors

import (
	"errors"
	"fmt"
	"time"
)

// Category represents the category of an error
type Category int

const (
	// CategoryRetriable indicates the operation can be retried immediately
	CategoryRetriable Category = iota
	// CategoryTransient indicates a temporary failure, retry after delay
	CategoryTransient
	// CategoryFatal indicates an unrecoverable error
	CategoryFatal
	// CategoryInvalidInput indicates a client error (bad request)
	CategoryInvalidInput
	// CategoryUnknown indicates the category is unknown
	CategoryUnknown
)

// String returns the string representation of the category
func (c Category) String() string {
	switch c {
	case CategoryRetriable:
		return "retriable"
	case CategoryTransient:
		return "transient"
	case CategoryFatal:
		return "fatal"
	case CategoryInvalidInput:
		return "invalid_input"
	case CategoryUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// Error represents a structured error with context
type Error struct {
	// Category is the error category
	Category Category
	// Op is the operation that failed
	Op string
	// Err is the underlying error
	Err error
	// Message is a human-readable message
	Message string
	// Metadata contains additional context
	Metadata map[string]interface{}
	// Timestamp is when the error occurred
	Timestamp time.Time
	// Code is an optional error code
	Code string
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("[%s] %s: %s: %v", e.Category, e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %v", e.Category, e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Err
}

// Is implements error matching
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Category == t.Category && e.Code == t.Code
}

// New creates a new Error
func New(category Category, op string, err error) *Error {
	return &Error{
		Category:  category,
		Op:        op,
		Err:       err,
		Metadata:  make(map[string]interface{}),
		Timestamp: time.Now(),
	}
}

// Retriable creates a retriable error
func Retriable(op string, err error) *Error {
	return New(CategoryRetriable, op, err)
}

// Transient creates a transient error
func Transient(op string, err error) *Error {
	return New(CategoryTransient, op, err)
}

// Fatal creates a fatal error
func Fatal(op string, err error) *Error {
	return New(CategoryFatal, op, err)
}

// InvalidInput creates an invalid input error
func InvalidInput(op string, err error) *Error {
	return New(CategoryInvalidInput, op, err)
}

// WithMessage adds a message to the error
func (e *Error) WithMessage(msg string) *Error {
	e.Message = msg
	return e
}

// WithCode adds an error code
func (e *Error) WithCode(code string) *Error {
	e.Code = code
	return e
}

// WithMetadata adds metadata to the error
func (e *Error) WithMetadata(key string, value interface{}) *Error {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// GetCategory returns the category of an error
func GetCategory(err error) Category {
	if err == nil {
		return CategoryUnknown
	}

	var e *Error
	if errors.As(err, &e) {
		return e.Category
	}

	// Try to classify standard errors
	return classifyStandardError(err)
}

// IsRetriable checks if an error is retriable
func IsRetriable(err error) bool {
	return GetCategory(err) == CategoryRetriable
}

// IsTransient checks if an error is transient
func IsTransient(err error) bool {
	return GetCategory(err) == CategoryTransient
}

// IsFatal checks if an error is fatal
func IsFatal(err error) bool {
	return GetCategory(err) == CategoryFatal
}

// IsInvalidInput checks if an error is due to invalid input
func IsInvalidInput(err error) bool {
	return GetCategory(err) == CategoryInvalidInput
}

// classifyStandardError attempts to classify standard Go errors
func classifyStandardError(err error) Category {
	// Context errors
	if errors.Is(err, ErrTimeout) || errors.Is(err, ErrTemporaryFailure) {
		return CategoryTransient
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrAlreadyExists) {
		return CategoryInvalidInput
	}
	if errors.Is(err, ErrNotLeader) || errors.Is(err, ErrNoLeader) {
		return CategoryRetriable
	}

	return CategoryUnknown
}

// Common error types
var (
	// ErrNotFound indicates a resource was not found
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists indicates a resource already exists
	ErrAlreadyExists = errors.New("already exists")
	// ErrNotLeader indicates the node is not the leader
	ErrNotLeader = errors.New("not leader")
	// ErrNoLeader indicates there is no leader elected
	ErrNoLeader = errors.New("no leader")
	// ErrTimeout indicates an operation timed out
	ErrTimeout = errors.New("timeout")
	// ErrTemporaryFailure indicates a temporary failure
	ErrTemporaryFailure = errors.New("temporary failure")
	// ErrInvalidState indicates an invalid state
	ErrInvalidState = errors.New("invalid state")
	// ErrCanceled indicates the operation was canceled
	ErrCanceled = errors.New("canceled")
	// ErrShutdown indicates the component is shutting down
	ErrShutdown = errors.New("shutdown")
)

// Wrap wraps an error with operation context
func Wrap(op string, err error) error {
	if err == nil {
		return nil
	}

	// If already a structured error, just update the op
	var e *Error
	if errors.As(err, &e) {
		e.Op = op + "." + e.Op
		return e
	}

	// Classify and wrap
	category := classifyStandardError(err)
	return New(category, op, err)
}

// Wrapf wraps an error with a formatted message
func Wrapf(op string, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	wrapped := Wrap(op, err)
	if e, ok := wrapped.(*Error); ok {
		e.Message = fmt.Sprintf(format, args...)
	}
	return wrapped
}

// Join combines multiple errors
func Join(errs ...error) error {
	var result error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if result == nil {
			result = err
		} else {
			result = fmt.Errorf("%w; %v", result, err)
		}
	}
	return result
}

// MultiError represents multiple errors
type MultiError struct {
	Errors []error
	Op     string
}

// Error implements the error interface
func (m *MultiError) Error() string {
	if len(m.Errors) == 0 {
		return "no errors"
	}
	if len(m.Errors) == 1 {
		return m.Errors[0].Error()
	}
	return fmt.Sprintf("%s: %d errors occurred", m.Op, len(m.Errors))
}

// Unwrap returns the first error
func (m *MultiError) Unwrap() error {
	if len(m.Errors) > 0 {
		return m.Errors[0]
	}
	return nil
}

// Add adds an error to the multi-error
func (m *MultiError) Add(err error) {
	if err != nil {
		m.Errors = append(m.Errors, err)
	}
}

// ErrorOrNil returns nil if there are no errors
func (m *MultiError) ErrorOrNil() error {
	if len(m.Errors) == 0 {
		return nil
	}
	return m
}

// NewMultiError creates a new MultiError
func NewMultiError(op string) *MultiError {
	return &MultiError{
		Op:     op,
		Errors: make([]error, 0),
	}
}
