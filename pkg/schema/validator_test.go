package schema

import (
	"testing"
)

func TestDefaultValidator_ValidateJSON(t *testing.T) {
	validator := NewDefaultValidator()

	tests := []struct {
		name        string
		schema      string
		expectError bool
	}{
		{
			name: "valid JSON schema",
			schema: `{
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"age": {"type": "number"}
				}
			}`,
			expectError: false,
		},
		{
			name:        "invalid JSON",
			schema:      `{invalid json`,
			expectError: true,
		},
		{
			name:        "empty schema",
			schema:      "",
			expectError: true,
		},
		{
			name:        "simple type",
			schema:      `{"type": "string"}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(FormatJSON, tt.schema)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDefaultValidator_ValidateAvro(t *testing.T) {
	validator := NewDefaultValidator()

	tests := []struct {
		name        string
		schema      string
		expectError bool
	}{
		{
			name: "valid Avro record",
			schema: `{
				"type": "record",
				"name": "User",
				"fields": [
					{"name": "name", "type": "string"},
					{"name": "age", "type": "int"}
				]
			}`,
			expectError: false,
		},
		{
			name: "valid Avro primitive",
			schema: `{
				"type": "string"
			}`,
			expectError: false,
		},
		{
			name: "record without name",
			schema: `{
				"type": "record",
				"fields": [
					{"name": "name", "type": "string"}
				]
			}`,
			expectError: true,
		},
		{
			name: "record without fields",
			schema: `{
				"type": "record",
				"name": "User"
			}`,
			expectError: true,
		},
		{
			name: "invalid type",
			schema: `{
				"type": "invalid_type"
			}`,
			expectError: true,
		},
		{
			name:        "empty schema",
			schema:      "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(FormatAvro, tt.schema)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDefaultValidator_ValidateProtobuf(t *testing.T) {
	validator := NewDefaultValidator()

	tests := []struct {
		name        string
		schema      string
		expectError bool
	}{
		{
			name: "valid protobuf with syntax",
			schema: `syntax = "proto3";

message User {
  string name = 1;
  int32 age = 2;
}`,
			expectError: false,
		},
		{
			name: "valid protobuf without syntax",
			schema: `message User {
  string name = 1;
  int32 age = 2;
}`,
			expectError: false,
		},
		{
			name: "valid protobuf with enum",
			schema: `syntax = "proto3";

enum Status {
  UNKNOWN = 0;
  ACTIVE = 1;
  INACTIVE = 2;
}`,
			expectError: false,
		},
		{
			name:        "empty schema",
			schema:      "",
			expectError: true,
		},
		{
			name: "no message or enum",
			schema: `syntax = "proto3";
// Just comments`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(FormatProtobuf, tt.schema)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDefaultValidator_CheckCompatibility_JSON_Backward(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`

	tests := []struct {
		name        string
		newSchema   string
		expectMatch bool
	}{
		{
			name: "add optional field",
			newSchema: `{
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"age": {"type": "number"}
				},
				"required": ["name"]
			}`,
			expectMatch: true,
		},
		{
			name: "remove required field",
			newSchema: `{
				"type": "object",
				"properties": {
					"age": {"type": "number"}
				},
				"required": ["age"]
			}`,
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compatible, err := validator.CheckCompatibility(FormatJSON, oldSchema, tt.newSchema, CompatibilityBackward)

			// If we expect incompatibility, an error is acceptable
			if !tt.expectMatch && err != nil {
				// Incompatible - this is expected
				return
			}

			if err != nil && tt.expectMatch {
				t.Fatalf("compatibility check failed: %v", err)
			}

			if compatible != tt.expectMatch {
				t.Errorf("expected compatible=%v, got %v", tt.expectMatch, compatible)
			}
		})
	}
}

func TestDefaultValidator_CheckCompatibility_JSON_Forward(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`

	tests := []struct {
		name        string
		newSchema   string
		expectMatch bool
	}{
		{
			name: "add required field with default",
			newSchema: `{
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"age": {"type": "number", "default": 0}
				},
				"required": ["name", "age"]
			}`,
			expectMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compatible, err := validator.CheckCompatibility(FormatJSON, oldSchema, tt.newSchema, CompatibilityForward)
			if err != nil {
				t.Fatalf("compatibility check failed: %v", err)
			}

			if compatible != tt.expectMatch {
				t.Errorf("expected compatible=%v, got %v", tt.expectMatch, compatible)
			}
		})
	}
}

func TestDefaultValidator_CheckCompatibility_JSON_Full(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`

	newSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number"}
		},
		"required": ["name"]
	}`

	compatible, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityFull)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be fully compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Avro(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"}
		]
	}`

	tests := []struct {
		name        string
		newSchema   string
		mode        CompatibilityMode
		expectMatch bool
	}{
		{
			name: "add field - backward compatible",
			newSchema: `{
				"type": "record",
				"name": "User",
				"fields": [
					{"name": "name", "type": "string"},
					{"name": "age", "type": "int", "default": 0}
				]
			}`,
			mode:        CompatibilityBackward,
			expectMatch: true,
		},
		{
			name: "change type - incompatible",
			newSchema: `{
				"type": "string"
			}`,
			mode:        CompatibilityBackward,
			expectMatch: false,
		},
		{
			name: "remove field - backward incompatible",
			newSchema: `{
				"type": "record",
				"name": "User",
				"fields": []
			}`,
			mode:        CompatibilityBackward,
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compatible, err := validator.CheckCompatibility(FormatAvro, oldSchema, tt.newSchema, tt.mode)

			// If we expect incompatibility, an error is acceptable
			if !tt.expectMatch && err != nil {
				// Incompatible - this is expected
				return
			}

			if err != nil && tt.expectMatch {
				t.Fatalf("compatibility check failed: %v", err)
			}

			if compatible != tt.expectMatch {
				t.Errorf("expected compatible=%v, got %v", tt.expectMatch, compatible)
			}
		})
	}
}

func TestDefaultValidator_CheckCompatibility_Protobuf(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `syntax = "proto3";

message User {
  string name = 1;
}`

	newSchema := `syntax = "proto3";

message User {
  string name = 1;
  int32 age = 2;
}`

	// Protobuf compatibility check is simplified - should pass
	compatible, err := validator.CheckCompatibility(FormatProtobuf, oldSchema, newSchema, CompatibilityBackward)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_None(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{"type": "string"}`
	newSchema := `{"type": "number"}`

	// NONE mode should always be compatible
	compatible, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityNone)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected NONE mode to be always compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_InvalidOldSchema(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `invalid json`
	newSchema := `{"type": "string"}`

	_, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityBackward)
	if err == nil {
		t.Error("expected error for invalid old schema")
	}
}

func TestDefaultValidator_CheckCompatibility_InvalidNewSchema(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{"type": "string"}`
	newSchema := `invalid json`

	_, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityBackward)
	if err == nil {
		t.Error("expected error for invalid new schema")
	}
}

func TestDefaultValidator_CheckCompatibility_UnsupportedFormat(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `schema`
	newSchema := `schema`

	_, err := validator.CheckCompatibility("UNKNOWN", oldSchema, newSchema, CompatibilityBackward)
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}
