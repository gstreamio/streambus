package schema

import (
	"testing"
)

func TestErrorCode_String(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrorNone, "None"},
		{ErrorSchemaNotFound, "SchemaNotFound"},
		{ErrorSubjectNotFound, "SubjectNotFound"},
		{ErrorVersionNotFound, "VersionNotFound"},
		{ErrorInvalidSchema, "InvalidSchema"},
		{ErrorIncompatibleSchema, "IncompatibleSchema"},
		{ErrorSubjectAlreadyExists, "SubjectAlreadyExists"},
		{ErrorVersionAlreadyExists, "VersionAlreadyExists"},
		{ErrorInvalidCompatibilityMode, "InvalidCompatibilityMode"},
		{ErrorInvalidSubject, "InvalidSubject"},
		{ErrorInvalidVersion, "InvalidVersion"},
		{ErrorSchemaRegistryError, "SchemaRegistryError"},
		{ErrorCode(999), "Unknown"}, // Test unknown error code
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.code.String()
			if result != tt.expected {
				t.Errorf("ErrorCode(%d).String() = %s, want %s", tt.code, result, tt.expected)
			}
		})
	}
}

func TestErrorCode_Error(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrorNone, "None"},
		{ErrorSchemaNotFound, "SchemaNotFound"},
		{ErrorSubjectNotFound, "SubjectNotFound"},
		{ErrorVersionNotFound, "VersionNotFound"},
		{ErrorInvalidSchema, "InvalidSchema"},
		{ErrorIncompatibleSchema, "IncompatibleSchema"},
		{ErrorSubjectAlreadyExists, "SubjectAlreadyExists"},
		{ErrorVersionAlreadyExists, "VersionAlreadyExists"},
		{ErrorInvalidCompatibilityMode, "InvalidCompatibilityMode"},
		{ErrorInvalidSubject, "InvalidSubject"},
		{ErrorInvalidVersion, "InvalidVersion"},
		{ErrorSchemaRegistryError, "SchemaRegistryError"},
		{ErrorCode(999), "Unknown"}, // Test unknown error code
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.code.Error()
			if result != tt.expected {
				t.Errorf("ErrorCode(%d).Error() = %s, want %s", tt.code, result, tt.expected)
			}
		})
	}
}

func TestErrorCode_AsError(t *testing.T) {
	// Test that ErrorCode can be used as an error
	var err error = ErrorSchemaNotFound

	if err.Error() != "SchemaNotFound" {
		t.Errorf("expected error message 'SchemaNotFound', got '%s'", err.Error())
	}
}
