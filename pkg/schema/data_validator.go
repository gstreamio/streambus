package schema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DataValidator validates message data against a schema definition.
type DataValidator struct{}

// NewDataValidator creates a new data validator.
func NewDataValidator() *DataValidator {
	return &DataValidator{}
}

// ValidateData validates data against a schema definition and format.
// Returns nil if validation passes or the schema format is unsupported for data validation.
func (v *DataValidator) ValidateData(format SchemaFormat, definition string, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("message data is empty")
	}

	switch format {
	case FormatJSON:
		return v.validateJSONData(definition, data)
	case FormatAvro:
		return v.validateAvroData(definition, data)
	case FormatProtobuf:
		// Protobuf data validation requires compiled descriptors;
		// skip runtime validation for now.
		return nil
	default:
		return fmt.Errorf("unsupported schema format for data validation: %s", format)
	}
}

// validateJSONData validates JSON data against a JSON schema.
func (v *DataValidator) validateJSONData(definition string, data []byte) error {
	// Parse the schema definition
	var schemaDef map[string]interface{}
	if err := json.Unmarshal([]byte(definition), &schemaDef); err != nil {
		return fmt.Errorf("invalid JSON schema definition: %w", err)
	}

	// Parse the message data
	var messageData interface{}
	if err := json.Unmarshal(data, &messageData); err != nil {
		return fmt.Errorf("message is not valid JSON: %w", err)
	}

	return v.validateJSONValue(schemaDef, messageData)
}

// validateJSONValue validates a parsed JSON value against a schema definition.
func (v *DataValidator) validateJSONValue(schemaDef map[string]interface{}, value interface{}) error {
	// Check type constraint
	if err := v.checkJSONType(schemaDef, value); err != nil {
		return err
	}

	// For object types, check required fields and properties
	if obj, ok := value.(map[string]interface{}); ok {
		return v.checkJSONObjectFields(schemaDef, obj)
	}

	return nil
}

// checkJSONType validates that the value matches the expected JSON schema type.
func (v *DataValidator) checkJSONType(schemaDef map[string]interface{}, value interface{}) error {
	expectedType, hasType := schemaDef["type"].(string)
	if !hasType {
		return nil // No type constraint
	}

	return matchJSONType(expectedType, value)
}

// matchJSONType checks if a Go value matches the expected JSON schema type.
func matchJSONType(expectedType string, value interface{}) error {
	switch expectedType {
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("expected object, got %T", value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("expected array, got %T", value)
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "number", "integer":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("expected %s, got %T", expectedType, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("expected null, got %T", value)
		}
	}
	return nil
}

// checkJSONObjectFields validates required fields and property types for a JSON object.
func (v *DataValidator) checkJSONObjectFields(schemaDef map[string]interface{}, obj map[string]interface{}) error {
	// Check required fields
	if requiredRaw, ok := schemaDef["required"].([]interface{}); ok {
		for _, r := range requiredRaw {
			fieldName, ok := r.(string)
			if !ok {
				continue
			}
			if _, exists := obj[fieldName]; !exists {
				return fmt.Errorf("missing required field: %s", fieldName)
			}
		}
	}

	// Validate property types
	properties, hasProps := schemaDef["properties"].(map[string]interface{})
	if !hasProps {
		return nil
	}

	for fieldName, fieldValue := range obj {
		propSchema, exists := properties[fieldName]
		if !exists {
			continue // Extra fields are allowed unless additionalProperties is false
		}
		propDef, ok := propSchema.(map[string]interface{})
		if !ok {
			continue
		}
		if err := v.validateJSONValue(propDef, fieldValue); err != nil {
			return fmt.Errorf("field '%s': %w", fieldName, err)
		}
	}

	return nil
}

// validateAvroData validates JSON-encoded data against an Avro schema.
// Avro messages in StreamBus are expected to be JSON-encoded for simplicity.
func (v *DataValidator) validateAvroData(definition string, data []byte) error {
	var schemaDef map[string]interface{}
	if err := json.Unmarshal([]byte(definition), &schemaDef); err != nil {
		return fmt.Errorf("invalid Avro schema definition: %w", err)
	}

	schemaType, _ := schemaDef["type"].(string)

	// For record types, validate fields
	if schemaType == "record" {
		return v.validateAvroRecord(schemaDef, data)
	}

	// For primitive types, validate the value directly
	return v.validateAvroPrimitive(schemaType, data)
}

// validateAvroRecord validates JSON-encoded data against an Avro record schema.
func (v *DataValidator) validateAvroRecord(schemaDef map[string]interface{}, data []byte) error {
	var record map[string]interface{}
	if err := json.Unmarshal(data, &record); err != nil {
		return fmt.Errorf("message is not a valid JSON object for Avro record: %w", err)
	}

	fieldsRaw, ok := schemaDef["fields"].([]interface{})
	if !ok {
		return nil
	}

	// Check that all required fields (those without defaults) are present
	for _, fieldRaw := range fieldsRaw {
		field, ok := fieldRaw.(map[string]interface{})
		if !ok {
			continue
		}
		fieldName, _ := field["name"].(string)
		if fieldName == "" {
			continue
		}

		_, hasValue := record[fieldName]
		_, hasDefault := field["default"]

		if !hasValue && !hasDefault {
			return fmt.Errorf("missing required Avro field: %s", fieldName)
		}
	}

	return nil
}

// validateAvroPrimitive validates a value against an Avro primitive type.
func (v *DataValidator) validateAvroPrimitive(schemaType string, data []byte) error {
	trimmed := strings.TrimSpace(string(data))

	switch schemaType {
	case "string":
		if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' {
			return fmt.Errorf("expected Avro string value")
		}
	case "int", "long", "float", "double":
		var num json.Number
		if err := json.Unmarshal(data, &num); err != nil {
			return fmt.Errorf("expected Avro numeric value: %w", err)
		}
	case "boolean":
		if trimmed != "true" && trimmed != "false" {
			return fmt.Errorf("expected Avro boolean value")
		}
	case "null":
		if trimmed != "null" {
			return fmt.Errorf("expected Avro null value")
		}
	}

	return nil
}
