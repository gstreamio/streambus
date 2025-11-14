package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gstreamio/streambus/pkg/logging"
)

// SchemaRegistry manages schemas and their versions
type SchemaRegistry struct {
	mu sync.RWMutex

	// Schema storage
	schemas        map[SchemaID]*Schema                  // id -> schema
	subjectSchemas map[Subject]map[Version]*Schema       // subject -> version -> schema
	subjectLatest  map[Subject]Version                   // subject -> latest version
	fingerprints   map[Subject]map[string]SchemaID       // subject -> fingerprint -> id

	// Compatibility configuration
	globalCompatibility CompatibilityMode
	subjectCompatibility map[Subject]CompatibilityMode

	// ID generation
	nextID int32

	// Validator
	validator SchemaValidator

	logger *logging.Logger
}

// SchemaValidator validates schemas and checks compatibility
type SchemaValidator interface {
	Validate(format SchemaFormat, definition string) error
	CheckCompatibility(format SchemaFormat, oldSchema, newSchema string, mode CompatibilityMode) (bool, error)
}

// NewSchemaRegistry creates a new schema registry
func NewSchemaRegistry(validator SchemaValidator, logger *logging.Logger) *SchemaRegistry {
	if logger == nil {
		logger = logging.New(&logging.Config{
			Level:  logging.LevelInfo,
			Output: nil,
		})
	}

	return &SchemaRegistry{
		schemas:              make(map[SchemaID]*Schema),
		subjectSchemas:       make(map[Subject]map[Version]*Schema),
		subjectLatest:        make(map[Subject]Version),
		fingerprints:         make(map[Subject]map[string]SchemaID),
		globalCompatibility:  CompatibilityBackward, // Default
		subjectCompatibility: make(map[Subject]CompatibilityMode),
		nextID:               1,
		validator:            validator,
		logger:               logger,
	}
}

// RegisterSchema registers a new schema
func (sr *SchemaRegistry) RegisterSchema(req *RegisterSchemaRequest) (*RegisterSchemaResponse, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// Validate request
	if req.Subject == "" {
		return &RegisterSchemaResponse{
			ErrorCode: ErrorInvalidSubject,
		}, nil
	}

	// Validate schema definition
	if sr.validator != nil {
		if err := sr.validator.Validate(req.Format, req.Definition); err != nil {
			sr.logger.Warn("Schema validation failed", logging.Fields{
				"subject": req.Subject,
				"error":   err.Error(),
			})
			return &RegisterSchemaResponse{
				ErrorCode: ErrorInvalidSchema,
			}, nil
		}
	}

	// Calculate fingerprint
	fingerprint := sr.calculateFingerprint(req.Definition)

	// Check if schema already exists for this subject
	if subjectFingerprints, exists := sr.fingerprints[req.Subject]; exists {
		if existingID, found := subjectFingerprints[fingerprint]; found {
			// Schema already registered
			return &RegisterSchemaResponse{
				ID:        existingID,
				ErrorCode: ErrorNone,
			}, nil
		}
	}

	// Check compatibility with latest version
	if latestVersion, exists := sr.subjectLatest[req.Subject]; exists {
		latestSchema := sr.subjectSchemas[req.Subject][latestVersion]
		compatible, err := sr.checkCompatibility(req.Subject, latestSchema, req.Format, req.Definition)
		if err != nil {
			sr.logger.Error("Compatibility check failed", err)
			return &RegisterSchemaResponse{
				ErrorCode: ErrorSchemaRegistryError,
			}, err
		}
		if !compatible {
			sr.logger.Warn("Schema incompatible with latest version", logging.Fields{
				"subject": req.Subject,
				"version": latestVersion,
			})
			return &RegisterSchemaResponse{
				ErrorCode: ErrorIncompatibleSchema,
			}, nil
		}
	}

	// Assign new ID
	id := SchemaID(atomic.AddInt32(&sr.nextID, 1))

	// Determine next version
	nextVersion := Version(1)
	if latestVersion, exists := sr.subjectLatest[req.Subject]; exists {
		nextVersion = latestVersion + 1
	}

	// Create schema
	now := time.Now()
	schema := &Schema{
		ID:         id,
		Subject:    req.Subject,
		Version:    nextVersion,
		Format:     req.Format,
		Definition: req.Definition,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	// Store schema
	sr.schemas[id] = schema

	if sr.subjectSchemas[req.Subject] == nil {
		sr.subjectSchemas[req.Subject] = make(map[Version]*Schema)
	}
	sr.subjectSchemas[req.Subject][nextVersion] = schema

	sr.subjectLatest[req.Subject] = nextVersion

	if sr.fingerprints[req.Subject] == nil {
		sr.fingerprints[req.Subject] = make(map[string]SchemaID)
	}
	sr.fingerprints[req.Subject][fingerprint] = id

	sr.logger.Info("Schema registered", logging.Fields{
		"id":      id,
		"subject": req.Subject,
		"version": nextVersion,
		"format":  req.Format,
	})

	return &RegisterSchemaResponse{
		ID:        id,
		ErrorCode: ErrorNone,
	}, nil
}

// GetSchema retrieves a schema by ID
func (sr *SchemaRegistry) GetSchema(req *GetSchemaRequest) (*GetSchemaResponse, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	schema, exists := sr.schemas[req.ID]
	if !exists {
		return &GetSchemaResponse{
			ErrorCode: ErrorSchemaNotFound,
		}, nil
	}

	// Return a copy
	schemaCopy := *schema
	return &GetSchemaResponse{
		Schema:    &schemaCopy,
		ErrorCode: ErrorNone,
	}, nil
}

// GetSchemaBySubjectVersion retrieves a schema by subject and version
func (sr *SchemaRegistry) GetSchemaBySubjectVersion(req *GetSchemaBySubjectVersionRequest) (*GetSchemaBySubjectVersionResponse, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	subjectVersions, exists := sr.subjectSchemas[req.Subject]
	if !exists {
		return &GetSchemaBySubjectVersionResponse{
			ErrorCode: ErrorSubjectNotFound,
		}, nil
	}

	schema, exists := subjectVersions[req.Version]
	if !exists {
		return &GetSchemaBySubjectVersionResponse{
			ErrorCode: ErrorVersionNotFound,
		}, nil
	}

	// Return a copy
	schemaCopy := *schema
	return &GetSchemaBySubjectVersionResponse{
		Schema:    &schemaCopy,
		ErrorCode: ErrorNone,
	}, nil
}

// GetLatestSchema retrieves the latest schema for a subject
func (sr *SchemaRegistry) GetLatestSchema(req *GetLatestSchemaRequest) (*GetLatestSchemaResponse, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	latestVersion, exists := sr.subjectLatest[req.Subject]
	if !exists {
		return &GetLatestSchemaResponse{
			ErrorCode: ErrorSubjectNotFound,
		}, nil
	}

	schema := sr.subjectSchemas[req.Subject][latestVersion]

	// Return a copy
	schemaCopy := *schema
	return &GetLatestSchemaResponse{
		Schema:    &schemaCopy,
		ErrorCode: ErrorNone,
	}, nil
}

// ListSubjects lists all subjects
func (sr *SchemaRegistry) ListSubjects(req *ListSubjectsRequest) (*ListSubjectsResponse, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	subjects := make([]Subject, 0, len(sr.subjectSchemas))
	for subject := range sr.subjectSchemas {
		// Apply prefix filter if specified
		if req.Prefix != "" {
			if len(subject) >= len(req.Prefix) && string(subject)[:len(req.Prefix)] == req.Prefix {
				subjects = append(subjects, subject)
			}
		} else {
			subjects = append(subjects, subject)
		}
	}

	return &ListSubjectsResponse{
		Subjects:  subjects,
		ErrorCode: ErrorNone,
	}, nil
}

// ListVersions lists all versions for a subject
func (sr *SchemaRegistry) ListVersions(req *ListVersionsRequest) (*ListVersionsResponse, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	subjectVersions, exists := sr.subjectSchemas[req.Subject]
	if !exists {
		return &ListVersionsResponse{
			ErrorCode: ErrorSubjectNotFound,
		}, nil
	}

	versions := make([]Version, 0, len(subjectVersions))
	for version := range subjectVersions {
		versions = append(versions, version)
	}

	return &ListVersionsResponse{
		Versions:  versions,
		ErrorCode: ErrorNone,
	}, nil
}

// DeleteSchema deletes a specific schema version
func (sr *SchemaRegistry) DeleteSchema(req *DeleteSchemaRequest) (*DeleteSchemaResponse, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	subjectVersions, exists := sr.subjectSchemas[req.Subject]
	if !exists {
		return &DeleteSchemaResponse{
			ErrorCode: ErrorSubjectNotFound,
		}, nil
	}

	schema, exists := subjectVersions[req.Version]
	if !exists {
		return &DeleteSchemaResponse{
			ErrorCode: ErrorVersionNotFound,
		}, nil
	}

	// Delete from maps
	delete(sr.schemas, schema.ID)
	delete(subjectVersions, req.Version)

	// Update latest version if necessary
	if sr.subjectLatest[req.Subject] == req.Version {
		// Find new latest version
		var maxVersion Version
		for version := range subjectVersions {
			if version > maxVersion {
				maxVersion = version
			}
		}
		if maxVersion > 0 {
			sr.subjectLatest[req.Subject] = maxVersion
		} else {
			delete(sr.subjectLatest, req.Subject)
		}
	}

	// Clean up if no versions left
	if len(subjectVersions) == 0 {
		delete(sr.subjectSchemas, req.Subject)
		delete(sr.fingerprints, req.Subject)
	}

	sr.logger.Info("Schema version deleted", logging.Fields{
		"subject": req.Subject,
		"version": req.Version,
	})

	return &DeleteSchemaResponse{
		ErrorCode: ErrorNone,
	}, nil
}

// DeleteSubject deletes all versions of a subject
func (sr *SchemaRegistry) DeleteSubject(req *DeleteSubjectRequest) (*DeleteSubjectResponse, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	subjectVersions, exists := sr.subjectSchemas[req.Subject]
	if !exists {
		return &DeleteSubjectResponse{
			ErrorCode: ErrorSubjectNotFound,
		}, nil
	}

	// Collect deleted versions
	deletedVersions := make([]Version, 0, len(subjectVersions))

	// Delete all schemas
	for version, schema := range subjectVersions {
		delete(sr.schemas, schema.ID)
		deletedVersions = append(deletedVersions, version)
	}

	// Delete subject
	delete(sr.subjectSchemas, req.Subject)
	delete(sr.subjectLatest, req.Subject)
	delete(sr.fingerprints, req.Subject)
	delete(sr.subjectCompatibility, req.Subject)

	sr.logger.Info("Subject deleted", logging.Fields{
		"subject": req.Subject,
		"versions": len(deletedVersions),
	})

	return &DeleteSubjectResponse{
		Versions:  deletedVersions,
		ErrorCode: ErrorNone,
	}, nil
}

// UpdateCompatibility updates compatibility mode for a subject
func (sr *SchemaRegistry) UpdateCompatibility(req *UpdateCompatibilityRequest) (*UpdateCompatibilityResponse, error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// Validate compatibility mode
	if !sr.isValidCompatibilityMode(req.Compatibility) {
		return &UpdateCompatibilityResponse{
			ErrorCode: ErrorInvalidCompatibilityMode,
		}, nil
	}

	sr.subjectCompatibility[req.Subject] = req.Compatibility

	sr.logger.Info("Compatibility mode updated", logging.Fields{
		"subject":       req.Subject,
		"compatibility": req.Compatibility,
	})

	return &UpdateCompatibilityResponse{
		ErrorCode: ErrorNone,
	}, nil
}

// GetCompatibility retrieves compatibility mode for a subject
func (sr *SchemaRegistry) GetCompatibility(req *GetCompatibilityRequest) (*GetCompatibilityResponse, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	compatibility, exists := sr.subjectCompatibility[req.Subject]
	if !exists {
		// Return global compatibility
		compatibility = sr.globalCompatibility
	}

	return &GetCompatibilityResponse{
		Compatibility: compatibility,
		ErrorCode:     ErrorNone,
	}, nil
}

// TestCompatibility tests if a schema is compatible
func (sr *SchemaRegistry) TestCompatibility(req *TestCompatibilityRequest) (*TestCompatibilityResponse, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	// Get the schema to test against
	subjectVersions, exists := sr.subjectSchemas[req.Subject]
	if !exists {
		return &TestCompatibilityResponse{
			Compatible: false,
			ErrorCode:  ErrorSubjectNotFound,
		}, nil
	}

	existingSchema, exists := subjectVersions[req.Version]
	if !exists {
		return &TestCompatibilityResponse{
			Compatible: false,
			ErrorCode:  ErrorVersionNotFound,
		}, nil
	}

	// Check compatibility
	compatible, err := sr.checkCompatibility(req.Subject, existingSchema, req.Format, req.Definition)
	if err != nil {
		return &TestCompatibilityResponse{
			Compatible: false,
			ErrorCode:  ErrorSchemaRegistryError,
			Message:    err.Error(),
		}, nil
	}

	return &TestCompatibilityResponse{
		Compatible: compatible,
		ErrorCode:  ErrorNone,
	}, nil
}

// Internal methods

func (sr *SchemaRegistry) checkCompatibility(subject Subject, existingSchema *Schema, newFormat SchemaFormat, newDefinition string) (bool, error) {
	// Get compatibility mode
	compatibilityMode, exists := sr.subjectCompatibility[subject]
	if !exists {
		compatibilityMode = sr.globalCompatibility
	}

	// NONE mode allows any change
	if compatibilityMode == CompatibilityNone {
		return true, nil
	}

	// Use validator if available
	if sr.validator != nil {
		return sr.validator.CheckCompatibility(newFormat, existingSchema.Definition, newDefinition, compatibilityMode)
	}

	// Without validator, default to compatible for non-NONE modes
	// In production, this should use format-specific validators
	return true, nil
}

func (sr *SchemaRegistry) calculateFingerprint(definition string) string {
	hash := sha256.Sum256([]byte(definition))
	return hex.EncodeToString(hash[:])
}

func (sr *SchemaRegistry) isValidCompatibilityMode(mode CompatibilityMode) bool {
	switch mode {
	case CompatibilityNone, CompatibilityBackward, CompatibilityForward, CompatibilityFull,
		CompatibilityBackwardTransitive, CompatibilityForwardTransitive, CompatibilityFullTransitive:
		return true
	}
	return false
}

// Stats returns registry statistics
func (sr *SchemaRegistry) Stats() RegistryStats {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	return RegistryStats{
		TotalSchemas:  len(sr.schemas),
		TotalSubjects: len(sr.subjectSchemas),
	}
}

// RegistryStats holds registry statistics
type RegistryStats struct {
	TotalSchemas  int
	TotalSubjects int
}

// SetGlobalCompatibility sets the global compatibility mode
func (sr *SchemaRegistry) SetGlobalCompatibility(mode CompatibilityMode) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if !sr.isValidCompatibilityMode(mode) {
		return fmt.Errorf("invalid compatibility mode: %s", mode)
	}

	sr.globalCompatibility = mode
	return nil
}
