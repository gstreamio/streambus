package broker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/schema"
)

// registerSchemaAPI registers schema registry HTTP endpoints on the mux.
func (b *Broker) registerSchemaAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/schemas/subjects", b.handleSchemaSubjects)
	mux.HandleFunc("/api/v1/schemas/subjects/", b.handleSchemaSubjectOperations)
	mux.HandleFunc("/api/v1/schemas/ids/", b.handleSchemaByID)

	b.logger.Info("Registered schema registry API endpoints")
}

// ==================== Schema Subject Endpoints ====================

// SchemaRegistrationRequest is the JSON body for registering a schema.
type SchemaRegistrationRequest struct {
	Format     string `json:"format"`
	Definition string `json:"definition"`
}

// SchemaResponse is the JSON response for schema operations.
type SchemaResponse struct {
	ID         int32  `json:"id"`
	Subject    string `json:"subject,omitempty"`
	Version    int32  `json:"version,omitempty"`
	Format     string `json:"format,omitempty"`
	Definition string `json:"definition,omitempty"`
}

// handleSchemaSubjects handles GET /api/v1/schemas/subjects (list subjects).
func (b *Broker) handleSchemaSubjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.schemaRegistry == nil {
		http.Error(w, "Schema registry not available", http.StatusServiceUnavailable)
		return
	}

	resp, err := b.schemaRegistry.ListSubjects(&schema.ListSubjectsRequest{})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list subjects: %v", err), http.StatusInternalServerError)
		return
	}

	subjects := make([]string, len(resp.Subjects))
	for i, s := range resp.Subjects {
		subjects[i] = string(s)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(subjects)
}

// handleSchemaSubjectOperations routes subject-specific operations.
func (b *Broker) handleSchemaSubjectOperations(w http.ResponseWriter, r *http.Request) {
	if b.schemaRegistry == nil {
		http.Error(w, "Schema registry not available", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/schemas/subjects/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Subject name required", http.StatusBadRequest)
		return
	}

	subject := parts[0]

	// Route sub-resources
	if len(parts) > 1 {
		b.routeSubjectSubResource(w, r, subject, parts[1:])
		return
	}

	// Subject-level operations
	switch r.Method {
	case http.MethodPost:
		b.registerSchemaForSubject(w, r, subject)
	case http.MethodGet:
		b.listSubjectVersions(w, r, subject)
	case http.MethodDelete:
		b.deleteSubject(w, r, subject)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// routeSubjectSubResource routes sub-resource requests like /versions/1.
func (b *Broker) routeSubjectSubResource(w http.ResponseWriter, r *http.Request, subject string, subParts []string) {
	switch subParts[0] {
	case "versions":
		if len(subParts) > 1 && subParts[1] == "latest" {
			b.getLatestSchemaVersion(w, r, subject)
			return
		}
		if len(subParts) > 1 {
			b.getSchemaByVersion(w, r, subject, subParts[1])
			return
		}
		b.listSubjectVersions(w, r, subject)
	default:
		http.Error(w, "Unknown sub-resource", http.StatusNotFound)
	}
}

// registerSchemaForSubject handles POST /api/v1/schemas/subjects/:subject.
func (b *Broker) registerSchemaForSubject(w http.ResponseWriter, r *http.Request, subject string) {
	var req SchemaRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	format := schema.SchemaFormat(strings.ToUpper(req.Format))
	regResp, err := b.schemaRegistry.RegisterSchema(&schema.RegisterSchemaRequest{
		Subject:    schema.Subject(subject),
		Format:     format,
		Definition: req.Definition,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to register schema: %v", err), http.StatusInternalServerError)
		return
	}

	if regResp.ErrorCode != schema.ErrorNone {
		statusCode := schemaErrorToHTTPStatus(regResp.ErrorCode)
		http.Error(w, fmt.Sprintf("Schema registration failed: %s", regResp.ErrorCode), statusCode)
		return
	}

	b.logger.Info("Schema registered via API", logging.Fields{
		"subject": subject,
		"id":      regResp.ID,
		"format":  format,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SchemaResponse{ID: int32(regResp.ID)})
}

// listSubjectVersions handles GET /api/v1/schemas/subjects/:subject.
func (b *Broker) listSubjectVersions(w http.ResponseWriter, r *http.Request, subject string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := b.schemaRegistry.ListVersions(&schema.ListVersionsRequest{
		Subject: schema.Subject(subject),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list versions: %v", err), http.StatusInternalServerError)
		return
	}

	if resp.ErrorCode == schema.ErrorSubjectNotFound {
		http.Error(w, "Subject not found", http.StatusNotFound)
		return
	}

	versions := make([]int32, len(resp.Versions))
	for i, v := range resp.Versions {
		versions[i] = int32(v)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(versions)
}

// getSchemaByVersion handles GET /api/v1/schemas/subjects/:subject/versions/:version.
func (b *Broker) getSchemaByVersion(w http.ResponseWriter, r *http.Request, subject, versionStr string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	version, err := strconv.ParseInt(versionStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid version number", http.StatusBadRequest)
		return
	}

	resp, err := b.schemaRegistry.GetSchemaBySubjectVersion(&schema.GetSchemaBySubjectVersionRequest{
		Subject: schema.Subject(subject),
		Version: schema.Version(version),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get schema: %v", err), http.StatusInternalServerError)
		return
	}

	if resp.ErrorCode != schema.ErrorNone {
		statusCode := schemaErrorToHTTPStatus(resp.ErrorCode)
		http.Error(w, resp.ErrorCode.String(), statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SchemaResponse{
		ID:         int32(resp.Schema.ID),
		Subject:    string(resp.Schema.Subject),
		Version:    int32(resp.Schema.Version),
		Format:     string(resp.Schema.Format),
		Definition: resp.Schema.Definition,
	})
}

// getLatestSchemaVersion handles GET /api/v1/schemas/subjects/:subject/versions/latest.
func (b *Broker) getLatestSchemaVersion(w http.ResponseWriter, r *http.Request, subject string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := b.schemaRegistry.GetLatestSchema(&schema.GetLatestSchemaRequest{
		Subject: schema.Subject(subject),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get latest schema: %v", err), http.StatusInternalServerError)
		return
	}

	if resp.ErrorCode != schema.ErrorNone {
		statusCode := schemaErrorToHTTPStatus(resp.ErrorCode)
		http.Error(w, resp.ErrorCode.String(), statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SchemaResponse{
		ID:         int32(resp.Schema.ID),
		Subject:    string(resp.Schema.Subject),
		Version:    int32(resp.Schema.Version),
		Format:     string(resp.Schema.Format),
		Definition: resp.Schema.Definition,
	})
}

// deleteSubject handles DELETE /api/v1/schemas/subjects/:subject.
func (b *Broker) deleteSubject(w http.ResponseWriter, r *http.Request, subject string) {
	resp, err := b.schemaRegistry.DeleteSubject(&schema.DeleteSubjectRequest{
		Subject: schema.Subject(subject),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete subject: %v", err), http.StatusInternalServerError)
		return
	}

	if resp.ErrorCode == schema.ErrorSubjectNotFound {
		http.Error(w, "Subject not found", http.StatusNotFound)
		return
	}

	b.logger.Info("Subject deleted via API", logging.Fields{
		"subject":  subject,
		"versions": len(resp.Versions),
	})

	w.WriteHeader(http.StatusNoContent)
}

// handleSchemaByID handles GET /api/v1/schemas/ids/:id.
func (b *Broker) handleSchemaByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if b.schemaRegistry == nil {
		http.Error(w, "Schema registry not available", http.StatusServiceUnavailable)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/schemas/ids/")
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid schema ID", http.StatusBadRequest)
		return
	}

	resp, err := b.schemaRegistry.GetSchema(&schema.GetSchemaRequest{
		ID: schema.SchemaID(id),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get schema: %v", err), http.StatusInternalServerError)
		return
	}

	if resp.ErrorCode != schema.ErrorNone {
		statusCode := schemaErrorToHTTPStatus(resp.ErrorCode)
		http.Error(w, resp.ErrorCode.String(), statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SchemaResponse{
		ID:         int32(resp.Schema.ID),
		Subject:    string(resp.Schema.Subject),
		Version:    int32(resp.Schema.Version),
		Format:     string(resp.Schema.Format),
		Definition: resp.Schema.Definition,
	})
}

// schemaErrorToHTTPStatus maps schema error codes to HTTP status codes.
func schemaErrorToHTTPStatus(code schema.ErrorCode) int {
	switch code {
	case schema.ErrorNone:
		return http.StatusOK
	case schema.ErrorSchemaNotFound, schema.ErrorSubjectNotFound, schema.ErrorVersionNotFound:
		return http.StatusNotFound
	case schema.ErrorInvalidSchema, schema.ErrorInvalidSubject, schema.ErrorInvalidVersion,
		schema.ErrorInvalidCompatibilityMode:
		return http.StatusBadRequest
	case schema.ErrorIncompatibleSchema:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
