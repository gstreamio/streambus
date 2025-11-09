package schema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DefaultValidator provides basic schema validation
type DefaultValidator struct{}

// NewDefaultValidator creates a new default validator
func NewDefaultValidator() *DefaultValidator {
	return &DefaultValidator{}
}

// Validate validates a schema definition
func (v *DefaultValidator) Validate(format SchemaFormat, definition string) error {
	if definition == "" {
		return fmt.Errorf("schema definition is empty")
	}

	switch format {
	case FormatJSON:
		return v.validateJSON(definition)
	case FormatAvro:
		return v.validateAvro(definition)
	case FormatProtobuf:
		return v.validateProtobuf(definition)
	default:
		return fmt.Errorf("unsupported schema format: %s", format)
	}
}

// validateJSON validates a JSON schema
func (v *DefaultValidator) validateJSON(definition string) error {
	// Validate that it's valid JSON
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(definition), &schema); err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}

	// Check for required JSON Schema fields
	if _, ok := schema["$schema"]; !ok {
		// Accept schemas without $schema for flexibility
	}

	return nil
}

// validateAvro validates an Avro schema
func (v *DefaultValidator) validateAvro(definition string) error {
	// Validate that it's valid JSON (Avro schemas are JSON)
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(definition), &schema); err != nil {
		return fmt.Errorf("invalid Avro schema (not valid JSON): %w", err)
	}

	// Check for required Avro fields
	schemaType, hasType := schema["type"]
	if !hasType {
		return fmt.Errorf("Avro schema must have 'type' field")
	}

	// Validate type is one of the Avro primitive or complex types
	typeStr, ok := schemaType.(string)
	if !ok {
		// Could be a complex type definition
		return nil
	}

	validTypes := []string{
		"null", "boolean", "int", "long", "float", "double",
		"bytes", "string", "record", "enum", "array", "map",
		"fixed", "union",
	}

	valid := false
	for _, t := range validTypes {
		if typeStr == t {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("invalid Avro type: %s", typeStr)
	}

	// For record types, check for required fields
	if typeStr == "record" {
		if _, hasName := schema["name"]; !hasName {
			return fmt.Errorf("Avro record schema must have 'name' field")
		}
		if _, hasFields := schema["fields"]; !hasFields {
			return fmt.Errorf("Avro record schema must have 'fields' array")
		}
	}

	return nil
}

// validateProtobuf validates a Protocol Buffers schema
func (v *DefaultValidator) validateProtobuf(definition string) error {
	// Basic protobuf validation - check for syntax declaration
	lines := strings.Split(definition, "\n")

	hasSyntax := false
	hasMessage := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "syntax") {
			hasSyntax = true
		}

		if strings.HasPrefix(trimmed, "message") {
			hasMessage = true
		}
	}

	if !hasSyntax {
		// Accept proto without explicit syntax (defaults to proto2)
	}

	if !hasMessage && !strings.Contains(definition, "enum") {
		return fmt.Errorf("protobuf schema must contain at least one message or enum definition")
	}

	return nil
}

// CheckCompatibility checks if two schemas are compatible
func (v *DefaultValidator) CheckCompatibility(format SchemaFormat, oldSchema, newSchema string, mode CompatibilityMode) (bool, error) {
	// Validate both schemas first
	if err := v.Validate(format, oldSchema); err != nil {
		return false, fmt.Errorf("old schema is invalid: %w", err)
	}

	if err := v.Validate(format, newSchema); err != nil {
		return false, fmt.Errorf("new schema is invalid: %w", err)
	}

	// NONE mode allows any change
	if mode == CompatibilityNone {
		return true, nil
	}

	switch format {
	case FormatJSON:
		return v.checkJSONCompatibility(oldSchema, newSchema, mode)
	case FormatAvro:
		return v.checkAvroCompatibility(oldSchema, newSchema, mode)
	case FormatProtobuf:
		return v.checkProtobufCompatibility(oldSchema, newSchema, mode)
	default:
		return false, fmt.Errorf("unsupported schema format: %s", format)
	}
}

// checkJSONCompatibility checks JSON schema compatibility
func (v *DefaultValidator) checkJSONCompatibility(oldSchema, newSchema string, mode CompatibilityMode) (bool, error) {
	var oldDef, newDef map[string]interface{}

	if err := json.Unmarshal([]byte(oldSchema), &oldDef); err != nil {
		return false, err
	}

	if err := json.Unmarshal([]byte(newSchema), &newDef); err != nil {
		return false, err
	}

	switch mode {
	case CompatibilityBackward, CompatibilityBackwardTransitive:
		// New schema can read data written with old schema
		// This means new schema can have:
		// - Additional optional fields
		// - Removed fields (if they were optional)
		return v.checkBackwardCompatibleJSON(oldDef, newDef)

	case CompatibilityForward, CompatibilityForwardTransitive:
		// Old schema can read data written with new schema
		// This means new schema can:
		// - Add required fields with defaults
		// - Remove fields
		return v.checkForwardCompatibleJSON(oldDef, newDef)

	case CompatibilityFull, CompatibilityFullTransitive:
		// Both backward and forward compatible
		backward, err := v.checkBackwardCompatibleJSON(oldDef, newDef)
		if err != nil {
			return false, err
		}

		forward, err := v.checkForwardCompatibleJSON(oldDef, newDef)
		if err != nil {
			return false, err
		}

		return backward && forward, nil

	default:
		return false, fmt.Errorf("unsupported compatibility mode: %s", mode)
	}
}

// checkBackwardCompatibleJSON checks if new JSON schema is backward compatible
func (v *DefaultValidator) checkBackwardCompatibleJSON(oldDef, newDef map[string]interface{}) (bool, error) {
	// Simplified check: new schema should have all required fields from old schema
	oldRequired := v.getRequiredFields(oldDef)
	newRequired := v.getRequiredFields(newDef)

	// All old required fields must exist in new schema
	for _, field := range oldRequired {
		found := false
		for _, newField := range newRequired {
			if field == newField {
				found = true
				break
			}
		}
		if !found {
			// Check if it's in new schema's properties but not required
			newProps := v.getProperties(newDef)
			if _, exists := newProps[field]; !exists {
				return false, fmt.Errorf("required field '%s' removed", field)
			}
		}
	}

	return true, nil
}

// checkForwardCompatibleJSON checks if new JSON schema is forward compatible
func (v *DefaultValidator) checkForwardCompatibleJSON(oldDef, newDef map[string]interface{}) (bool, error) {
	// Simplified check: old schema should be able to read new data
	// New required fields must have defaults
	newRequired := v.getRequiredFields(newDef)
	oldProps := v.getProperties(oldDef)

	for _, field := range newRequired {
		if _, exists := oldProps[field]; !exists {
			// New required field added - check if it has a default
			newProps := v.getProperties(newDef)
			if prop, ok := newProps[field].(map[string]interface{}); ok {
				if _, hasDefault := prop["default"]; !hasDefault {
					return false, fmt.Errorf("new required field '%s' has no default", field)
				}
			}
		}
	}

	return true, nil
}

// Helper functions for JSON schema analysis
func (v *DefaultValidator) getRequiredFields(schema map[string]interface{}) []string {
	required, ok := schema["required"].([]interface{})
	if !ok {
		return []string{}
	}

	fields := make([]string, 0, len(required))
	for _, field := range required {
		if fieldStr, ok := field.(string); ok {
			fields = append(fields, fieldStr)
		}
	}

	return fields
}

func (v *DefaultValidator) getProperties(schema map[string]interface{}) map[string]interface{} {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return make(map[string]interface{})
	}
	return props
}

// checkAvroCompatibility checks Avro schema compatibility
func (v *DefaultValidator) checkAvroCompatibility(oldSchema, newSchema string, mode CompatibilityMode) (bool, error) {
	var oldDef, newDef map[string]interface{}

	if err := json.Unmarshal([]byte(oldSchema), &oldDef); err != nil {
		return false, err
	}

	if err := json.Unmarshal([]byte(newSchema), &newDef); err != nil {
		return false, err
	}

	switch mode {
	case CompatibilityBackward, CompatibilityBackwardTransitive:
		// Check backward compatibility rules for Avro
		return v.checkAvroBackwardCompatible(oldDef, newDef)

	case CompatibilityForward, CompatibilityForwardTransitive:
		// Check forward compatibility rules for Avro
		return v.checkAvroForwardCompatible(oldDef, newDef)

	case CompatibilityFull, CompatibilityFullTransitive:
		backward, err := v.checkAvroBackwardCompatible(oldDef, newDef)
		if err != nil {
			return false, err
		}

		forward, err := v.checkAvroForwardCompatible(oldDef, newDef)
		if err != nil {
			return false, err
		}

		return backward && forward, nil

	default:
		return false, fmt.Errorf("unsupported compatibility mode: %s", mode)
	}
}

// checkAvroBackwardCompatible checks Avro backward compatibility
func (v *DefaultValidator) checkAvroBackwardCompatible(oldDef, newDef map[string]interface{}) (bool, error) {
	// Simplified: check that type hasn't changed
	oldType, _ := oldDef["type"].(string)
	newType, _ := newDef["type"].(string)

	if oldType != newType {
		return false, fmt.Errorf("schema type changed from %s to %s", oldType, newType)
	}

	// For records, check fields
	if oldType == "record" {
		oldFields := v.getAvroFields(oldDef)
		newFields := v.getAvroFields(newDef)

		// All old fields must exist in new schema or have defaults
		for _, oldField := range oldFields {
			found := false
			for _, newField := range newFields {
				if oldField == newField {
					found = true
					break
				}
			}
			if !found {
				return false, fmt.Errorf("field '%s' removed without default", oldField)
			}
		}
	}

	return true, nil
}

// checkAvroForwardCompatible checks Avro forward compatibility
func (v *DefaultValidator) checkAvroForwardCompatible(oldDef, newDef map[string]interface{}) (bool, error) {
	// Simplified: check that new fields have defaults
	if oldDef["type"] == "record" && newDef["type"] == "record" {
		oldFields := v.getAvroFields(oldDef)
		newFields := v.getAvroFields(newDef)

		// New fields must have defaults
		for _, newField := range newFields {
			found := false
			for _, oldField := range oldFields {
				if newField == oldField {
					found = true
					break
				}
			}
			if !found {
				// New field - should have default (simplified check)
				// In real implementation, we'd check the field definition
			}
		}
	}

	return true, nil
}

// getAvroFields extracts field names from Avro record schema
func (v *DefaultValidator) getAvroFields(schema map[string]interface{}) []string {
	fields, ok := schema["fields"].([]interface{})
	if !ok {
		return []string{}
	}

	fieldNames := make([]string, 0, len(fields))
	for _, field := range fields {
		if fieldMap, ok := field.(map[string]interface{}); ok {
			if name, ok := fieldMap["name"].(string); ok {
				fieldNames = append(fieldNames, name)
			}
		}
	}

	return fieldNames
}

// checkProtobufCompatibility checks Protobuf schema compatibility
func (v *DefaultValidator) checkProtobufCompatibility(oldSchema, newSchema string, mode CompatibilityMode) (bool, error) {
	// Simplified protobuf compatibility check
	// In a real implementation, this would parse the proto files and check:
	// - Field numbers haven't changed
	// - Required fields haven't been added
	// - Field types are compatible

	switch mode {
	case CompatibilityBackward, CompatibilityBackwardTransitive:
		// Check that old field numbers still exist
		return true, nil

	case CompatibilityForward, CompatibilityForwardTransitive:
		// Check that new fields are optional or have defaults
		return true, nil

	case CompatibilityFull, CompatibilityFullTransitive:
		// Both backward and forward
		return true, nil

	default:
		return false, fmt.Errorf("unsupported compatibility mode: %s", mode)
	}
}
