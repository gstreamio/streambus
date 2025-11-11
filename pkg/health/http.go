package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Handler provides HTTP handlers for health check endpoints
type Handler struct {
	registry *Registry
}

// NewHandler creates a new HTTP handler for health checks
func NewHandler(registry *Registry) *Handler {
	return &Handler{
		registry: registry,
	}
}

// RegisterRoutes registers all health check routes on the given mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/health/live", h.handleLiveness)
	mux.HandleFunc("/health/ready", h.handleReadiness)
}

// handleHealth returns detailed health status for all components
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	response := h.registry.CheckAll(ctx)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(h.statusCodeFromHealth(response.Status))

	if r.Method == http.MethodHead {
		return
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleLiveness returns a simple liveness check (is the process alive?)
func (h *Handler) handleLiveness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	check := LivenessCheck()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // Liveness is always OK if we can respond

	if r.Method == http.MethodHead {
		return
	}

	if err := json.NewEncoder(w).Encode(check); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// handleReadiness returns readiness status (can the service accept traffic?)
func (h *Handler) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	check := h.registry.ReadinessCheck(ctx)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(h.statusCodeFromHealth(check.Status))

	if r.Method == http.MethodHead {
		return
	}

	if err := json.NewEncoder(w).Encode(check); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// statusCodeFromHealth converts health status to HTTP status code
func (h *Handler) statusCodeFromHealth(status Status) int {
	switch status {
	case StatusHealthy:
		return http.StatusOK
	case StatusDegraded:
		return http.StatusOK // Degraded but still serving
	case StatusUnhealthy:
		return http.StatusServiceUnavailable
	case StatusUnknown:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// Server wraps an HTTP server for health checks
type Server struct {
	server   *http.Server
	registry *Registry
}

// NewServer creates a new health check HTTP server
func NewServer(addr string, registry *Registry) *Server {
	mux := http.NewServeMux()
	handler := NewHandler(registry)
	handler.RegisterRoutes(mux)

	return &Server{
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		registry: registry,
	}
}

// Start starts the health check server
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Stop gracefully stops the health check server
func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
