package schema

import (
	"time"
)

// SchemaID is a unique identifier for a schema
type SchemaID int32

// Subject is a namespace for schemas (typically topic-key or topic-value)
type Subject string

// Version is a schema version number
type Version int32

// SchemaFormat represents the schema format/type
type SchemaFormat string

const (
	// FormatJSON represents JSON schema format
	FormatJSON SchemaFormat = "JSON"

	// FormatAvro represents Apache Avro schema format
	FormatAvro SchemaFormat = "AVRO"

	// FormatProtobuf represents Protocol Buffers schema format
	FormatProtobuf SchemaFormat = "PROTOBUF"
)

// CompatibilityMode defines how schema evolution is validated
type CompatibilityMode string

const (
	// CompatibilityNone allows any schema change
	CompatibilityNone CompatibilityMode = "NONE"

	// CompatibilityBackward allows deletion of fields and addition of optional fields
	// New schema can read data written with old schema
	CompatibilityBackward CompatibilityMode = "BACKWARD"

	// CompatibilityForward allows addition of fields and deletion of optional fields
	// Old schema can read data written with new schema
	CompatibilityForward CompatibilityMode = "FORWARD"

	// CompatibilityFull is both backward and forward compatible
	CompatibilityFull CompatibilityMode = "FULL"

	// CompatibilityBackwardTransitive checks compatibility against all previous versions
	CompatibilityBackwardTransitive CompatibilityMode = "BACKWARD_TRANSITIVE"

	// CompatibilityForwardTransitive checks compatibility against all previous versions
	CompatibilityForwardTransitive CompatibilityMode = "FORWARD_TRANSITIVE"

	// CompatibilityFullTransitive checks compatibility against all previous versions
	CompatibilityFullTransitive CompatibilityMode = "FULL_TRANSITIVE"
)

// Schema represents a schema definition
type Schema struct {
	ID         SchemaID
	Subject    Subject
	Version    Version
	Format     SchemaFormat
	Definition string // The actual schema definition (JSON, Avro, Proto)
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SchemaMetadata contains metadata about a schema
type SchemaMetadata struct {
	ID          SchemaID
	Subject     Subject
	Version     Version
	Format      SchemaFormat
	Fingerprint string // Hash of schema definition
	CreatedAt   time.Time
}

// SubjectVersion represents a subject and version pair
type SubjectVersion struct {
	Subject Subject
	Version Version
}

// ErrorCode represents schema registry error codes
type ErrorCode int

const (
	ErrorNone ErrorCode = iota
	ErrorSchemaNotFound
	ErrorSubjectNotFound
	ErrorVersionNotFound
	ErrorInvalidSchema
	ErrorIncompatibleSchema
	ErrorSubjectAlreadyExists
	ErrorVersionAlreadyExists
	ErrorInvalidCompatibilityMode
	ErrorInvalidSubject
	ErrorInvalidVersion
	ErrorSchemaRegistryError
)

// String returns the string representation of ErrorCode
func (e ErrorCode) String() string {
	switch e {
	case ErrorNone:
		return "None"
	case ErrorSchemaNotFound:
		return "SchemaNotFound"
	case ErrorSubjectNotFound:
		return "SubjectNotFound"
	case ErrorVersionNotFound:
		return "VersionNotFound"
	case ErrorInvalidSchema:
		return "InvalidSchema"
	case ErrorIncompatibleSchema:
		return "IncompatibleSchema"
	case ErrorSubjectAlreadyExists:
		return "SubjectAlreadyExists"
	case ErrorVersionAlreadyExists:
		return "VersionAlreadyExists"
	case ErrorInvalidCompatibilityMode:
		return "InvalidCompatibilityMode"
	case ErrorInvalidSubject:
		return "InvalidSubject"
	case ErrorInvalidVersion:
		return "InvalidVersion"
	case ErrorSchemaRegistryError:
		return "SchemaRegistryError"
	default:
		return "Unknown"
	}
}

// Error returns the error message
func (e ErrorCode) Error() string {
	return e.String()
}

// RegisterSchemaRequest registers a new schema
type RegisterSchemaRequest struct {
	Subject    Subject
	Format     SchemaFormat
	Definition string
}

// RegisterSchemaResponse returns the registered schema ID
type RegisterSchemaResponse struct {
	ID        SchemaID
	ErrorCode ErrorCode
}

// GetSchemaRequest retrieves a schema by ID
type GetSchemaRequest struct {
	ID SchemaID
}

// GetSchemaResponse returns the schema
type GetSchemaResponse struct {
	Schema    *Schema
	ErrorCode ErrorCode
}

// GetSchemaBySubjectVersionRequest retrieves a schema by subject and version
type GetSchemaBySubjectVersionRequest struct {
	Subject Subject
	Version Version
}

// GetSchemaBySubjectVersionResponse returns the schema
type GetSchemaBySubjectVersionResponse struct {
	Schema    *Schema
	ErrorCode ErrorCode
}

// GetLatestSchemaRequest retrieves the latest schema for a subject
type GetLatestSchemaRequest struct {
	Subject Subject
}

// GetLatestSchemaResponse returns the latest schema
type GetLatestSchemaResponse struct {
	Schema    *Schema
	ErrorCode ErrorCode
}

// ListSubjectsRequest lists all subjects
type ListSubjectsRequest struct {
	Prefix string // Optional prefix filter
}

// ListSubjectsResponse returns list of subjects
type ListSubjectsResponse struct {
	Subjects  []Subject
	ErrorCode ErrorCode
}

// ListVersionsRequest lists all versions for a subject
type ListVersionsRequest struct {
	Subject Subject
}

// ListVersionsResponse returns list of versions
type ListVersionsResponse struct {
	Versions  []Version
	ErrorCode ErrorCode
}

// DeleteSchemaRequest deletes a schema version
type DeleteSchemaRequest struct {
	Subject Subject
	Version Version
}

// DeleteSchemaResponse confirms deletion
type DeleteSchemaResponse struct {
	ErrorCode ErrorCode
}

// DeleteSubjectRequest deletes all versions of a subject
type DeleteSubjectRequest struct {
	Subject Subject
}

// DeleteSubjectResponse confirms deletion
type DeleteSubjectResponse struct {
	Versions  []Version // Deleted versions
	ErrorCode ErrorCode
}

// UpdateCompatibilityRequest updates compatibility mode for a subject
type UpdateCompatibilityRequest struct {
	Subject       Subject
	Compatibility CompatibilityMode
}

// UpdateCompatibilityResponse confirms update
type UpdateCompatibilityResponse struct {
	ErrorCode ErrorCode
}

// GetCompatibilityRequest retrieves compatibility mode for a subject
type GetCompatibilityRequest struct {
	Subject Subject
}

// GetCompatibilityResponse returns compatibility mode
type GetCompatibilityResponse struct {
	Compatibility CompatibilityMode
	ErrorCode     ErrorCode
}

// TestCompatibilityRequest tests if a schema is compatible
type TestCompatibilityRequest struct {
	Subject    Subject
	Version    Version // Test against this version
	Format     SchemaFormat
	Definition string
}

// TestCompatibilityResponse returns compatibility result
type TestCompatibilityResponse struct {
	Compatible bool
	ErrorCode  ErrorCode
	Message    string // Error/warning message
}

// SchemaReference represents a reference to another schema
type SchemaReference struct {
	Name    string
	Subject Subject
	Version Version
}

// SchemaWithReferences represents a schema with references to other schemas
type SchemaWithReferences struct {
	Schema     *Schema
	References []SchemaReference
}
