package schema

import (
	"testing"
)

func TestDataValidator_ValidateData_JSON(t *testing.T) {
	v := NewDataValidator()

	schemaDef := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number"},
			"active": {"type": "boolean"}
		},
		"required": ["name"]
	}`

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "valid complete object",
			data:    `{"name": "Alice", "age": 30, "active": true}`,
			wantErr: false,
		},
		{
			name:    "valid with only required field",
			data:    `{"name": "Bob"}`,
			wantErr: false,
		},
		{
			name:    "valid with extra fields",
			data:    `{"name": "Charlie", "email": "charlie@test.com"}`,
			wantErr: false,
		},
		{
			name:    "missing required field",
			data:    `{"age": 25}`,
			wantErr: true,
		},
		{
			name:    "not valid JSON",
			data:    `this is not json`,
			wantErr: true,
		},
		{
			name:    "wrong type for property",
			data:    `{"name": 123}`,
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    ``,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.ValidateData(FormatJSON, schemaDef, []byte(tc.data))
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateData() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDataValidator_ValidateData_JSONArrayType(t *testing.T) {
	v := NewDataValidator()

	schemaDef := `{"type": "array"}`

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{name: "valid array", data: `[1, 2, 3]`, wantErr: false},
		{name: "empty array", data: `[]`, wantErr: false},
		{name: "not an array", data: `{"key": "value"}`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.ValidateData(FormatJSON, schemaDef, []byte(tc.data))
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateData() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDataValidator_ValidateData_Avro(t *testing.T) {
	v := NewDataValidator()

	avroSchema := `{
		"type": "record",
		"name": "User",
		"fields": [
			{"name": "username", "type": "string"},
			{"name": "email", "type": "string"},
			{"name": "nickname", "type": "string", "default": ""}
		]
	}`

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "valid with all fields",
			data:    `{"username": "alice", "email": "alice@test.com", "nickname": "ali"}`,
			wantErr: false,
		},
		{
			name:    "valid with optional field missing",
			data:    `{"username": "bob", "email": "bob@test.com"}`,
			wantErr: false,
		},
		{
			name:    "missing required field",
			data:    `{"username": "charlie"}`,
			wantErr: true,
		},
		{
			name:    "not valid JSON",
			data:    `not json`,
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    ``,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.ValidateData(FormatAvro, avroSchema, []byte(tc.data))
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateData() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDataValidator_ValidateData_AvroPrimitive(t *testing.T) {
	v := NewDataValidator()

	tests := []struct {
		name       string
		schemaType string
		data       string
		wantErr    bool
	}{
		{name: "valid string", schemaType: "string", data: `"hello"`, wantErr: false},
		{name: "invalid string", schemaType: "string", data: `123`, wantErr: true},
		{name: "valid int", schemaType: "int", data: `42`, wantErr: false},
		{name: "invalid int", schemaType: "int", data: `"not a number"`, wantErr: true},
		{name: "valid boolean", schemaType: "boolean", data: `true`, wantErr: false},
		{name: "invalid boolean", schemaType: "boolean", data: `"yes"`, wantErr: true},
		{name: "valid null", schemaType: "null", data: `null`, wantErr: false},
		{name: "invalid null", schemaType: "null", data: `"not null"`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			schemaDef := `{"type": "` + tc.schemaType + `"}`
			err := v.ValidateData(FormatAvro, schemaDef, []byte(tc.data))
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateData() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDataValidator_ValidateData_Protobuf(t *testing.T) {
	v := NewDataValidator()

	protoDef := `syntax = "proto3";
message User {
  string name = 1;
  int32 age = 2;
}`

	// Protobuf data validation is a pass-through for now
	err := v.ValidateData(FormatProtobuf, protoDef, []byte("any data"))
	if err != nil {
		t.Errorf("expected nil for protobuf data validation, got %v", err)
	}
}

func TestDataValidator_ValidateData_UnsupportedFormat(t *testing.T) {
	v := NewDataValidator()

	err := v.ValidateData(SchemaFormat("THRIFT"), `{}`, []byte(`{}`))
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestDataValidator_ValidateData_EmptyData(t *testing.T) {
	v := NewDataValidator()

	err := v.ValidateData(FormatJSON, `{"type": "object"}`, []byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestRegistry_ValidateMessage(t *testing.T) {
	validator := NewDefaultValidator()
	registry := NewSchemaRegistry(validator, nil)

	// No schema registered -- should pass
	err := registry.ValidateMessage("test-topic-value", []byte("anything"))
	if err != nil {
		t.Errorf("expected nil when no schema registered, got %v", err)
	}

	// Register a schema
	_, regErr := registry.RegisterSchema(&RegisterSchemaRequest{
		Subject:    "test-topic-value",
		Format:     FormatJSON,
		Definition: `{"type": "object", "properties": {"name": {"type": "string"}}, "required": ["name"]}`,
	})
	if regErr != nil {
		t.Fatalf("failed to register schema: %v", regErr)
	}

	// Valid data
	err = registry.ValidateMessage("test-topic-value", []byte(`{"name": "Alice"}`))
	if err != nil {
		t.Errorf("expected nil for valid data, got %v", err)
	}

	// Invalid data
	err = registry.ValidateMessage("test-topic-value", []byte(`{"age": 30}`))
	if err == nil {
		t.Error("expected error for invalid data")
	}
}

func TestRegistry_HasSchema(t *testing.T) {
	validator := NewDefaultValidator()
	registry := NewSchemaRegistry(validator, nil)

	if registry.HasSchema("nonexistent") {
		t.Error("expected false for nonexistent subject")
	}

	_, _ = registry.RegisterSchema(&RegisterSchemaRequest{
		Subject:    "existing",
		Format:     FormatJSON,
		Definition: `{"type": "object"}`,
	})

	if !registry.HasSchema("existing") {
		t.Error("expected true for registered subject")
	}
}
