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

func TestDefaultValidator_CheckCompatibility_JSON_UnsupportedMode(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{"type": "string"}`
	newSchema := `{"type": "number"}`

	_, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, "INVALID_MODE")
	if err == nil {
		t.Error("expected error for unsupported compatibility mode")
	}
}

func TestDefaultValidator_CheckCompatibility_Avro_Full(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"}
		]
	}`

	newSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"},
			{"name": "age", "type": "int", "default": 0}
		]
	}`

	compatible, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, CompatibilityFull)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be fully compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Avro_Forward(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"}
		]
	}`

	newSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"},
			{"name": "age", "type": "int", "default": 0}
		]
	}`

	compatible, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, CompatibilityForward)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be forward compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Avro_ForwardTransitive(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"}
		]
	}`

	newSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"},
			{"name": "email", "type": "string", "default": ""}
		]
	}`

	compatible, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, CompatibilityForwardTransitive)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be forward transitive compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Avro_FullTransitive(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"}
		]
	}`

	newSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"},
			{"name": "email", "type": "string", "default": ""}
		]
	}`

	compatible, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, CompatibilityFullTransitive)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be full transitive compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Avro_BackwardTransitive(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"}
		]
	}`

	newSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"},
			{"name": "email", "type": "string", "default": ""}
		]
	}`

	compatible, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, CompatibilityBackwardTransitive)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be backward transitive compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Avro_UnsupportedMode(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"}
		]
	}`

	newSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"}
		]
	}`

	_, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, "INVALID_MODE")
	if err == nil {
		t.Error("expected error for unsupported compatibility mode")
	}
}

func TestDefaultValidator_CheckCompatibility_Protobuf_Forward(t *testing.T) {
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

	compatible, err := validator.CheckCompatibility(FormatProtobuf, oldSchema, newSchema, CompatibilityForward)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be forward compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Protobuf_Full(t *testing.T) {
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

	compatible, err := validator.CheckCompatibility(FormatProtobuf, oldSchema, newSchema, CompatibilityFull)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be fully compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Protobuf_BackwardTransitive(t *testing.T) {
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

	compatible, err := validator.CheckCompatibility(FormatProtobuf, oldSchema, newSchema, CompatibilityBackwardTransitive)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be backward transitive compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Protobuf_ForwardTransitive(t *testing.T) {
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

	compatible, err := validator.CheckCompatibility(FormatProtobuf, oldSchema, newSchema, CompatibilityForwardTransitive)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be forward transitive compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Protobuf_FullTransitive(t *testing.T) {
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

	compatible, err := validator.CheckCompatibility(FormatProtobuf, oldSchema, newSchema, CompatibilityFullTransitive)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be full transitive compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_Protobuf_UnsupportedMode(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `syntax = "proto3";

message User {
  string name = 1;
}`

	newSchema := `syntax = "proto3";

message User {
  string name = 1;
}`

	_, err := validator.CheckCompatibility(FormatProtobuf, oldSchema, newSchema, "INVALID_MODE")
	if err == nil {
		t.Error("expected error for unsupported compatibility mode")
	}
}

func TestDefaultValidator_CheckCompatibility_JSON_ForwardTransitive(t *testing.T) {
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
			"age": {"type": "number", "default": 0}
		},
		"required": ["name", "age"]
	}`

	compatible, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityForwardTransitive)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be forward transitive compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_JSON_BackwardTransitive(t *testing.T) {
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

	compatible, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityBackwardTransitive)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be backward transitive compatible")
	}
}

func TestDefaultValidator_CheckCompatibility_JSON_FullTransitive(t *testing.T) {
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

	compatible, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityFullTransitive)
	if err != nil {
		t.Fatalf("compatibility check failed: %v", err)
	}

	if !compatible {
		t.Error("expected schemas to be full transitive compatible")
	}
}

func TestDefaultValidator_GetProperties_NilSchema(t *testing.T) {
	validator := NewDefaultValidator()

	schema := map[string]interface{}{
		"type": "object",
	}

	props := validator.getProperties(schema)
	if len(props) != 0 {
		t.Errorf("expected empty properties map, got %d properties", len(props))
	}
}

func TestDefaultValidator_ValidateAvro_ComplexType(t *testing.T) {
	validator := NewDefaultValidator()

	// Test with complex type (not a string)
	schema := `{
		"type": {
			"type": "record",
			"name": "ComplexType",
			"fields": [
				{"name": "id", "type": "int"}
			]
		}
	}`

	err := validator.Validate(FormatAvro, schema)
	if err != nil {
		t.Errorf("unexpected error for complex type: %v", err)
	}
}

func TestDefaultValidator_ValidateAvro_NoTypeField(t *testing.T) {
	validator := NewDefaultValidator()

	schema := `{
		"name": "User",
		"fields": []
	}`

	err := validator.Validate(FormatAvro, schema)
	if err == nil {
		t.Error("expected error for Avro schema without type field")
	}
}

func TestDefaultValidator_CheckCompatibility_JSON_ForwardIncompatible(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`

	// Add required field without default - forward incompatible
	newSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number"}
		},
		"required": ["name", "age"]
	}`

	compatible, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityForward)
	// If error is returned due to incompatibility, that's also acceptable
	if err != nil {
		return
	}

	if compatible {
		t.Error("expected schemas to be forward incompatible (new required field without default)")
	}
}

func TestDefaultValidator_CheckCompatibility_Avro_UnsupportedModeString(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{"type": "string"}`
	newSchema := `{"type": "string"}`

	_, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, CompatibilityMode("COMPLETELY_INVALID"))
	if err == nil {
		t.Error("expected error for completely invalid compatibility mode")
	}
}

func TestDefaultValidator_CheckCompatibility_AvroFullTransitive_Incompatible(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "name", "type": "string"}
		]
	}`

	// Change type - should be incompatible
	newSchema := `{
		"type": "enum",
		"name": "UserType",
		"symbols": ["ADMIN", "USER"]
	}`

	compatible, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, CompatibilityFullTransitive)
	// If error is returned, that's also a sign of incompatibility
	if err != nil {
		return
	}

	if compatible {
		t.Error("expected schemas to be incompatible (type changed)")
	}
}

func TestDefaultValidator_GetAvroFields_InvalidFieldStructure(t *testing.T) {
	validator := NewDefaultValidator()

	// Fields array with non-map entries
	schema := map[string]interface{}{
		"type": "record",
		"name": "Test",
		"fields": []interface{}{
			"not a map",
			map[string]interface{}{
				"name": "validField",
			},
			map[string]interface{}{
				"noname": "value",
			},
		},
	}

	fields := validator.getAvroFields(schema)
	// Should only return the valid field
	if len(fields) != 1 {
		t.Errorf("expected 1 valid field, got %d", len(fields))
	}
	if len(fields) > 0 && fields[0] != "validField" {
		t.Errorf("expected field 'validField', got '%s'", fields[0])
	}
}

func TestDefaultValidator_CheckAvroCompatibility_InvalidJSON(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{invalid json`
	newSchema := `{"type": "string"}`

	_, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, CompatibilityBackward)
	if err == nil {
		t.Error("expected error for invalid old schema JSON")
	}
}

func TestDefaultValidator_CheckAvroCompatibility_InvalidNewJSON(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{"type": "string"}`
	newSchema := `{invalid json`

	_, err := validator.CheckCompatibility(FormatAvro, oldSchema, newSchema, CompatibilityBackward)
	if err == nil {
		t.Error("expected error for invalid new schema JSON")
	}
}

func TestDefaultValidator_CheckJSONCompatibility_InvalidOldJSON(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{invalid json`
	newSchema := `{"type": "string"}`

	_, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityBackward)
	if err == nil {
		t.Error("expected error for invalid old schema JSON")
	}
}

func TestDefaultValidator_CheckJSONCompatibility_InvalidNewJSON(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{"type": "string"}`
	newSchema := `{invalid json`

	_, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityBackward)
	if err == nil {
		t.Error("expected error for invalid new schema JSON")
	}
}

func TestDefaultValidator_CheckForwardCompatibleJSON_InvalidProperty(t *testing.T) {
	validator := NewDefaultValidator()

	oldSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		}
	}`

	// New schema with non-map property
	newSchema := `{
		"type": "object",
		"properties": {
			"name": "not a map"
		},
		"required": ["name", "newfield"]
	}`

	_, err := validator.CheckCompatibility(FormatJSON, oldSchema, newSchema, CompatibilityForward)
	// Should handle this case gracefully
	if err != nil {
		// This is acceptable - parsing error
		return
	}
}
