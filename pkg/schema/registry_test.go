package schema

import (
	"testing"

	"github.com/gstreamio/streambus/pkg/logging"
)

func testLogger() *logging.Logger {
	return logging.New(&logging.Config{
		Level:  logging.LevelDebug,
		Output: nil, // Disable output for tests
	})
}

func TestNewSchemaRegistry(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()

	registry := NewSchemaRegistry(validator, logger)

	if registry == nil {
		t.Fatal("expected registry, got nil")
	}

	if registry.validator != validator {
		t.Error("validator not set correctly")
	}

	if registry.globalCompatibility != CompatibilityBackward {
		t.Errorf("expected default compatibility to be Backward, got %v", registry.globalCompatibility)
	}
}

func TestRegisterSchema(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Valid JSON schema
	jsonSchema := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number"}
		}
	}`

	req := &RegisterSchemaRequest{
		Subject:    "user-value",
		Format:     FormatJSON,
		Definition: jsonSchema,
	}

	resp, err := registry.RegisterSchema(req)
	if err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone, got %v", resp.ErrorCode)
	}

	if resp.ID == 0 {
		t.Error("expected non-zero schema ID")
	}

	// Check that schema was stored
	stats := registry.Stats()
	if stats.TotalSchemas != 1 {
		t.Errorf("expected 1 schema, got %d", stats.TotalSchemas)
	}

	if stats.TotalSubjects != 1 {
		t.Errorf("expected 1 subject, got %d", stats.TotalSubjects)
	}
}

func TestRegisterSchema_Duplicate(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	schema := `{"type": "string"}`

	req := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: schema,
	}

	// Register first time
	resp1, err := registry.RegisterSchema(req)
	if err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}

	// Register same schema again
	resp2, err := registry.RegisterSchema(req)
	if err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}

	// Should return same ID
	if resp1.ID != resp2.ID {
		t.Errorf("expected same ID for duplicate schema, got %d and %d", resp1.ID, resp2.ID)
	}

	// Should not create new version
	stats := registry.Stats()
	if stats.TotalSchemas != 1 {
		t.Errorf("expected 1 schema, got %d", stats.TotalSchemas)
	}
}

func TestRegisterSchema_Evolution(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register v1
	schemaV1 := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`

	req1 := &RegisterSchemaRequest{
		Subject:    "user-value",
		Format:     FormatJSON,
		Definition: schemaV1,
	}

	resp1, err := registry.RegisterSchema(req1)
	if err != nil {
		t.Fatalf("failed to register schema v1: %v", err)
	}

	// Register v2 with additional optional field (backward compatible)
	schemaV2 := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number"}
		},
		"required": ["name"]
	}`

	req2 := &RegisterSchemaRequest{
		Subject:    "user-value",
		Format:     FormatJSON,
		Definition: schemaV2,
	}

	resp2, err := registry.RegisterSchema(req2)
	if err != nil {
		t.Fatalf("failed to register schema v2: %v", err)
	}

	if resp2.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone, got %v", resp2.ErrorCode)
	}

	// Should have different IDs
	if resp1.ID == resp2.ID {
		t.Error("expected different IDs for different schemas")
	}

	// Should have 2 schemas
	stats := registry.Stats()
	if stats.TotalSchemas != 2 {
		t.Errorf("expected 2 schemas, got %d", stats.TotalSchemas)
	}

	// Should still be 1 subject
	if stats.TotalSubjects != 1 {
		t.Errorf("expected 1 subject, got %d", stats.TotalSubjects)
	}
}

func TestRegisterSchema_InvalidSubject(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &RegisterSchemaRequest{
		Subject:    "",
		Format:     FormatJSON,
		Definition: `{"type": "string"}`,
	}

	resp, err := registry.RegisterSchema(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorInvalidSubject {
		t.Errorf("expected ErrorInvalidSubject, got %v", resp.ErrorCode)
	}
}

func TestRegisterSchema_InvalidSchema(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: "not valid json",
	}

	resp, err := registry.RegisterSchema(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorInvalidSchema {
		t.Errorf("expected ErrorInvalidSchema, got %v", resp.ErrorCode)
	}
}

func TestGetSchema(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register a schema
	schema := `{"type": "string"}`
	req := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: schema,
	}

	registerResp, _ := registry.RegisterSchema(req)

	// Get the schema
	getReq := &GetSchemaRequest{
		ID: registerResp.ID,
	}

	getResp, err := registry.GetSchema(getReq)
	if err != nil {
		t.Fatalf("failed to get schema: %v", err)
	}

	if getResp.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone, got %v", getResp.ErrorCode)
	}

	if getResp.Schema == nil {
		t.Fatal("expected schema, got nil")
	}

	if getResp.Schema.Definition != schema {
		t.Errorf("schema definition mismatch: got %s", getResp.Schema.Definition)
	}
}

func TestGetSchema_NotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &GetSchemaRequest{
		ID: 999,
	}

	resp, err := registry.GetSchema(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorSchemaNotFound {
		t.Errorf("expected ErrorSchemaNotFound, got %v", resp.ErrorCode)
	}
}

func TestGetSchemaBySubjectVersion(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register schema
	req := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: `{"type": "string"}`,
	}

	_, _ = registry.RegisterSchema(req)

	// Get by subject and version
	getReq := &GetSchemaBySubjectVersionRequest{
		Subject: "test-subject",
		Version: 1,
	}

	resp, err := registry.GetSchemaBySubjectVersion(getReq)
	if err != nil {
		t.Fatalf("failed to get schema: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone, got %v", resp.ErrorCode)
	}

	if resp.Schema == nil {
		t.Fatal("expected schema, got nil")
	}

	if resp.Schema.Version != 1 {
		t.Errorf("expected version 1, got %d", resp.Schema.Version)
	}
}

func TestGetLatestSchema(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register multiple versions
	for i := 1; i <= 3; i++ {
		req := &RegisterSchemaRequest{
			Subject:    "test-subject",
			Format:     FormatJSON,
			Definition: `{"type": "string", "version": ` + string(rune('0'+i)) + `}`,
		}
		_, _ = registry.RegisterSchema(req)
	}

	// Get latest
	req := &GetLatestSchemaRequest{
		Subject: "test-subject",
	}

	resp, err := registry.GetLatestSchema(req)
	if err != nil {
		t.Fatalf("failed to get latest schema: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone, got %v", resp.ErrorCode)
	}

	if resp.Schema.Version != 3 {
		t.Errorf("expected version 3, got %d", resp.Schema.Version)
	}
}

func TestListSubjects(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register schemas for multiple subjects
	subjects := []string{"user-value", "order-value", "product-value"}
	for _, subject := range subjects {
		req := &RegisterSchemaRequest{
			Subject:    Subject(subject),
			Format:     FormatJSON,
			Definition: `{"type": "string"}`,
		}
		_, _ = registry.RegisterSchema(req)
	}

	// List all subjects
	req := &ListSubjectsRequest{}
	resp, err := registry.ListSubjects(req)
	if err != nil {
		t.Fatalf("failed to list subjects: %v", err)
	}

	if len(resp.Subjects) != 3 {
		t.Errorf("expected 3 subjects, got %d", len(resp.Subjects))
	}
}

func TestListSubjects_WithPrefix(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register schemas
	subjects := []string{"user-value", "user-key", "order-value"}
	for _, subject := range subjects {
		req := &RegisterSchemaRequest{
			Subject:    Subject(subject),
			Format:     FormatJSON,
			Definition: `{"type": "string"}`,
		}
		_, _ = registry.RegisterSchema(req)
	}

	// List with prefix
	req := &ListSubjectsRequest{
		Prefix: "user",
	}

	resp, err := registry.ListSubjects(req)
	if err != nil {
		t.Fatalf("failed to list subjects: %v", err)
	}

	if len(resp.Subjects) != 2 {
		t.Errorf("expected 2 subjects with prefix 'user', got %d", len(resp.Subjects))
	}
}

func TestListVersions(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register multiple versions
	for i := 1; i <= 3; i++ {
		req := &RegisterSchemaRequest{
			Subject:    "test-subject",
			Format:     FormatJSON,
			Definition: `{"type": "string", "v": ` + string(rune('0'+i)) + `}`,
		}
		_, _ = registry.RegisterSchema(req)
	}

	// List versions
	req := &ListVersionsRequest{
		Subject: "test-subject",
	}

	resp, err := registry.ListVersions(req)
	if err != nil {
		t.Fatalf("failed to list versions: %v", err)
	}

	if len(resp.Versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(resp.Versions))
	}
}

func TestDeleteSchema(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register schema
	req := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: `{"type": "string"}`,
	}

	_, _ = registry.RegisterSchema(req)

	// Delete schema
	deleteReq := &DeleteSchemaRequest{
		Subject: "test-subject",
		Version: 1,
	}

	resp, err := registry.DeleteSchema(deleteReq)
	if err != nil {
		t.Fatalf("failed to delete schema: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone, got %v", resp.ErrorCode)
	}

	// Verify deletion
	stats := registry.Stats()
	if stats.TotalSchemas != 0 {
		t.Errorf("expected 0 schemas after deletion, got %d", stats.TotalSchemas)
	}
}

func TestDeleteSubject(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register multiple versions
	for i := 1; i <= 3; i++ {
		req := &RegisterSchemaRequest{
			Subject:    "test-subject",
			Format:     FormatJSON,
			Definition: `{"type": "string", "v": ` + string(rune('0'+i)) + `}`,
		}
		_, _ = registry.RegisterSchema(req)
	}

	// Delete subject
	req := &DeleteSubjectRequest{
		Subject: "test-subject",
	}

	resp, err := registry.DeleteSubject(req)
	if err != nil {
		t.Fatalf("failed to delete subject: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone, got %v", resp.ErrorCode)
	}

	if len(resp.Versions) != 3 {
		t.Errorf("expected 3 deleted versions, got %d", len(resp.Versions))
	}

	// Verify deletion
	stats := registry.Stats()
	if stats.TotalSchemas != 0 {
		t.Errorf("expected 0 schemas after deletion, got %d", stats.TotalSchemas)
	}

	if stats.TotalSubjects != 0 {
		t.Errorf("expected 0 subjects after deletion, got %d", stats.TotalSubjects)
	}
}

func TestUpdateCompatibility(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &UpdateCompatibilityRequest{
		Subject:       "test-subject",
		Compatibility: CompatibilityFull,
	}

	resp, err := registry.UpdateCompatibility(req)
	if err != nil {
		t.Fatalf("failed to update compatibility: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone, got %v", resp.ErrorCode)
	}

	// Verify compatibility was set
	getReq := &GetCompatibilityRequest{
		Subject: "test-subject",
	}

	getResp, err := registry.GetCompatibility(getReq)
	if err != nil {
		t.Fatalf("failed to get compatibility: %v", err)
	}

	if getResp.Compatibility != CompatibilityFull {
		t.Errorf("expected CompatibilityFull, got %v", getResp.Compatibility)
	}
}

func TestGetCompatibility_UsesGlobal(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &GetCompatibilityRequest{
		Subject: "nonexistent-subject",
	}

	resp, err := registry.GetCompatibility(req)
	if err != nil {
		t.Fatalf("failed to get compatibility: %v", err)
	}

	// Should return global compatibility
	if resp.Compatibility != CompatibilityBackward {
		t.Errorf("expected global compatibility (Backward), got %v", resp.Compatibility)
	}
}

func TestTestCompatibility(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register base schema
	schemaV1 := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`

	regReq := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: schemaV1,
	}

	_, _ = registry.RegisterSchema(regReq)

	// Test compatibility with compatible schema
	schemaV2 := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "number"}
		},
		"required": ["name"]
	}`

	testReq := &TestCompatibilityRequest{
		Subject:    "test-subject",
		Version:    1,
		Format:     FormatJSON,
		Definition: schemaV2,
	}

	resp, err := registry.TestCompatibility(testReq)
	if err != nil {
		t.Fatalf("failed to test compatibility: %v", err)
	}

	if !resp.Compatible {
		t.Error("expected schema to be compatible")
	}
}

func TestSetGlobalCompatibility(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	err := registry.SetGlobalCompatibility(CompatibilityNone)
	if err != nil {
		t.Fatalf("failed to set global compatibility: %v", err)
	}

	// Verify it was set
	req := &GetCompatibilityRequest{
		Subject: "any-subject",
	}

	resp, _ := registry.GetCompatibility(req)
	if resp.Compatibility != CompatibilityNone {
		t.Errorf("expected CompatibilityNone, got %v", resp.Compatibility)
	}
}

func TestStats(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Initially empty
	stats := registry.Stats()
	if stats.TotalSchemas != 0 {
		t.Errorf("expected 0 schemas, got %d", stats.TotalSchemas)
	}

	// Register some schemas
	for i := 0; i < 3; i++ {
		req := &RegisterSchemaRequest{
			Subject:    Subject("subject-" + string(rune('0'+i))),
			Format:     FormatJSON,
			Definition: `{"type": "string"}`,
		}
		_, _ = registry.RegisterSchema(req)
	}

	stats = registry.Stats()
	if stats.TotalSchemas != 3 {
		t.Errorf("expected 3 schemas, got %d", stats.TotalSchemas)
	}

	if stats.TotalSubjects != 3 {
		t.Errorf("expected 3 subjects, got %d", stats.TotalSubjects)
	}
}

func TestNewSchemaRegistry_WithNilLogger(t *testing.T) {
	validator := NewDefaultValidator()
	registry := NewSchemaRegistry(validator, nil)

	if registry == nil {
		t.Fatal("expected registry, got nil")
	}

	if registry.logger == nil {
		t.Error("expected default logger to be created")
	}
}

func TestNewSchemaRegistry_WithNilValidator(t *testing.T) {
	logger := testLogger()
	registry := NewSchemaRegistry(nil, logger)

	if registry == nil {
		t.Fatal("expected registry, got nil")
	}

	// Registry should work without validator (just skips validation)
	req := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: `{"type": "string"}`,
	}

	resp, err := registry.RegisterSchema(req)
	if err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone, got %v", resp.ErrorCode)
	}
}

func TestIsValidCompatibilityMode(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	tests := []struct {
		mode  CompatibilityMode
		valid bool
	}{
		{CompatibilityNone, true},
		{CompatibilityBackward, true},
		{CompatibilityForward, true},
		{CompatibilityFull, true},
		{CompatibilityBackwardTransitive, true},
		{CompatibilityForwardTransitive, true},
		{CompatibilityFullTransitive, true},
		{"INVALID_MODE", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			valid := registry.isValidCompatibilityMode(tt.mode)
			if valid != tt.valid {
				t.Errorf("expected isValidCompatibilityMode(%s) = %v, got %v", tt.mode, tt.valid, valid)
			}
		})
	}
}

func TestUpdateCompatibility_InvalidMode(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &UpdateCompatibilityRequest{
		Subject:       "test-subject",
		Compatibility: "INVALID_MODE",
	}

	resp, err := registry.UpdateCompatibility(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorInvalidCompatibilityMode {
		t.Errorf("expected ErrorInvalidCompatibilityMode, got %v", resp.ErrorCode)
	}
}

func TestSetGlobalCompatibility_InvalidMode(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	err := registry.SetGlobalCompatibility("INVALID_MODE")
	if err == nil {
		t.Error("expected error for invalid compatibility mode")
	}
}

func TestTestCompatibility_SubjectNotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &TestCompatibilityRequest{
		Subject:    "nonexistent-subject",
		Version:    1,
		Format:     FormatJSON,
		Definition: `{"type": "string"}`,
	}

	resp, err := registry.TestCompatibility(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorSubjectNotFound {
		t.Errorf("expected ErrorSubjectNotFound, got %v", resp.ErrorCode)
	}

	if resp.Compatible {
		t.Error("expected compatible to be false")
	}
}

func TestTestCompatibility_VersionNotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register a schema
	req := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: `{"type": "string"}`,
	}
	_, _ = registry.RegisterSchema(req)

	// Test against nonexistent version
	testReq := &TestCompatibilityRequest{
		Subject:    "test-subject",
		Version:    999,
		Format:     FormatJSON,
		Definition: `{"type": "number"}`,
	}

	resp, err := registry.TestCompatibility(testReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorVersionNotFound {
		t.Errorf("expected ErrorVersionNotFound, got %v", resp.ErrorCode)
	}

	if resp.Compatible {
		t.Error("expected compatible to be false")
	}
}

func TestCheckCompatibility_WithNoneMode(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Set compatibility to NONE
	err := registry.SetGlobalCompatibility(CompatibilityNone)
	if err != nil {
		t.Fatalf("failed to set global compatibility: %v", err)
	}

	// Register base schema
	req1 := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: `{"type": "string"}`,
	}
	_, _ = registry.RegisterSchema(req1)

	// Register completely different schema - should be allowed with NONE mode
	req2 := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: `{"type": "number"}`,
	}

	resp, err := registry.RegisterSchema(req2)
	if err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}

	if resp.ErrorCode != ErrorNone {
		t.Errorf("expected ErrorNone with NONE compatibility mode, got %v", resp.ErrorCode)
	}
}

func TestGetSchemaBySubjectVersion_SubjectNotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &GetSchemaBySubjectVersionRequest{
		Subject: "nonexistent-subject",
		Version: 1,
	}

	resp, err := registry.GetSchemaBySubjectVersion(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorSubjectNotFound {
		t.Errorf("expected ErrorSubjectNotFound, got %v", resp.ErrorCode)
	}
}

func TestGetSchemaBySubjectVersion_VersionNotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register a schema
	req := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: `{"type": "string"}`,
	}
	_, _ = registry.RegisterSchema(req)

	// Try to get nonexistent version
	getReq := &GetSchemaBySubjectVersionRequest{
		Subject: "test-subject",
		Version: 999,
	}

	resp, err := registry.GetSchemaBySubjectVersion(getReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorVersionNotFound {
		t.Errorf("expected ErrorVersionNotFound, got %v", resp.ErrorCode)
	}
}

func TestGetLatestSchema_SubjectNotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &GetLatestSchemaRequest{
		Subject: "nonexistent-subject",
	}

	resp, err := registry.GetLatestSchema(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorSubjectNotFound {
		t.Errorf("expected ErrorSubjectNotFound, got %v", resp.ErrorCode)
	}
}

func TestListVersions_SubjectNotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &ListVersionsRequest{
		Subject: "nonexistent-subject",
	}

	resp, err := registry.ListVersions(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorSubjectNotFound {
		t.Errorf("expected ErrorSubjectNotFound, got %v", resp.ErrorCode)
	}
}

func TestDeleteSchema_SubjectNotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &DeleteSchemaRequest{
		Subject: "nonexistent-subject",
		Version: 1,
	}

	resp, err := registry.DeleteSchema(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorSubjectNotFound {
		t.Errorf("expected ErrorSubjectNotFound, got %v", resp.ErrorCode)
	}
}

func TestDeleteSchema_VersionNotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register a schema
	req := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: `{"type": "string"}`,
	}
	_, _ = registry.RegisterSchema(req)

	// Try to delete nonexistent version
	deleteReq := &DeleteSchemaRequest{
		Subject: "test-subject",
		Version: 999,
	}

	resp, err := registry.DeleteSchema(deleteReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorVersionNotFound {
		t.Errorf("expected ErrorVersionNotFound, got %v", resp.ErrorCode)
	}
}

func TestDeleteSchema_UpdatesLatestVersion(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register multiple versions
	for i := 1; i <= 3; i++ {
		req := &RegisterSchemaRequest{
			Subject:    "test-subject",
			Format:     FormatJSON,
			Definition: `{"type": "string", "v": ` + string(rune('0'+i)) + `}`,
		}
		_, _ = registry.RegisterSchema(req)
	}

	// Delete latest version (v3)
	deleteReq := &DeleteSchemaRequest{
		Subject: "test-subject",
		Version: 3,
	}

	_, err := registry.DeleteSchema(deleteReq)
	if err != nil {
		t.Fatalf("failed to delete schema: %v", err)
	}

	// Verify latest version is now v2
	latestReq := &GetLatestSchemaRequest{
		Subject: "test-subject",
	}

	resp, err := registry.GetLatestSchema(latestReq)
	if err != nil {
		t.Fatalf("failed to get latest schema: %v", err)
	}

	if resp.Schema.Version != 2 {
		t.Errorf("expected latest version to be 2 after deletion, got %d", resp.Schema.Version)
	}
}

func TestDeleteSubject_SubjectNotFound(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	req := &DeleteSubjectRequest{
		Subject: "nonexistent-subject",
	}

	resp, err := registry.DeleteSubject(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ErrorCode != ErrorSubjectNotFound {
		t.Errorf("expected ErrorSubjectNotFound, got %v", resp.ErrorCode)
	}
}

func TestRegisterSchema_IncompatibleSchema(t *testing.T) {
	validator := NewDefaultValidator()
	logger := testLogger()
	registry := NewSchemaRegistry(validator, logger)

	// Register base schema with required field
	schemaV1 := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`

	req1 := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: schemaV1,
	}
	_, _ = registry.RegisterSchema(req1)

	// Try to register incompatible schema (removes required field)
	schemaV2 := `{
		"type": "object",
		"properties": {
			"age": {"type": "number"}
		},
		"required": ["age"]
	}`

	req2 := &RegisterSchemaRequest{
		Subject:    "test-subject",
		Format:     FormatJSON,
		Definition: schemaV2,
	}

	resp, err := registry.RegisterSchema(req2)
	// The compatibility check returns an error when schemas are incompatible
	// This is expected - the error contains the incompatibility reason
	if err != nil {
		// Verify it's a compatibility error
		if resp == nil || (resp.ErrorCode != ErrorSchemaRegistryError && resp.ErrorCode != ErrorIncompatibleSchema) {
			t.Errorf("expected compatibility error, got: %v", err)
		}
		return
	}

	if resp.ErrorCode != ErrorIncompatibleSchema {
		t.Errorf("expected ErrorIncompatibleSchema, got %v", resp.ErrorCode)
	}
}
