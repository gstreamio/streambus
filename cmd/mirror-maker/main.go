package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/replication/link"
)

const version = "1.0.0"

// Config represents the mirror maker configuration
type Config struct {
	// Links is the list of replication links to manage
	Links []*link.ReplicationLink `json:"links"`

	// MetricsPort is the port for exposing metrics
	MetricsPort int `json:"metrics_port"`

	// HealthPort is the port for health checks
	HealthPort int `json:"health_port"`

	// LogLevel is the logging level
	LogLevel string `json:"log_level"`

	// StoragePath is the path for persistent storage
	StoragePath string `json:"storage_path"`
}

type mirrorMaker struct {
	config  *Config
	manager link.Manager
	logger  *logging.Logger

	httpServer *http.Server
}

func main() {
	var (
		configFile  = flag.String("config", "mirror-maker.json", "Configuration file path")
		showVersion = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("StreamBus Mirror Maker v%s\n", version)
		os.Exit(0)
	}

	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logLevel, err := logging.ParseLevel(config.LogLevel)
	if err != nil {
		logLevel = logging.LevelInfo
	}
	logger := logging.New(&logging.Config{
		Level:       logLevel,
		Component:   "mirror-maker",
		IncludeFile: true,
	})
	logger.Info("Starting StreamBus Mirror Maker", logging.Fields{
		"version": version,
	})

	// Create mirror maker instance
	mm, err := newMirrorMaker(config, logger)
	if err != nil {
		logger.Error("Failed to create mirror maker", err)
		os.Exit(1)
	}

	// Start mirror maker
	if err := mm.Start(); err != nil {
		logger.Error("Failed to start mirror maker", err)
		os.Exit(1)
	}

	logger.Info("Mirror maker started successfully", logging.Fields{
		"links":        len(config.Links),
		"metrics_port": config.MetricsPort,
		"health_port":  config.HealthPort,
	})

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutdown signal received, stopping mirror maker", logging.Fields{})

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := mm.Stop(ctx); err != nil {
		logger.Error("Error during shutdown", err)
		os.Exit(1)
	}

	logger.Info("Mirror maker stopped successfully", logging.Fields{})
}

// loadConfig loads configuration from a file
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults
	if config.MetricsPort == 0 {
		config.MetricsPort = 9090
	}
	if config.HealthPort == 0 {
		config.HealthPort = 8080
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	return &config, nil
}

// newMirrorMaker creates a new mirror maker instance
func newMirrorMaker(config *Config, logger *logging.Logger) (*mirrorMaker, error) {
	// Create storage
	var storage link.Storage
	if config.StoragePath != "" {
		// TODO: Implement file-based storage
		// For now, use memory storage
		storage = link.NewMemoryStorage()
	} else {
		storage = link.NewMemoryStorage()
	}

	// Create replication manager
	manager := link.NewManager(storage)

	mm := &mirrorMaker{
		config:  config,
		manager: manager,
		logger:  logger,
	}

	return mm, nil
}

// Start starts the mirror maker
func (mm *mirrorMaker) Start() error {
	// Create all configured replication links
	for _, linkConfig := range mm.config.Links {
		mm.logger.Info("Creating replication link", logging.Fields{
			"id":   linkConfig.ID,
			"name": linkConfig.Name,
			"type": linkConfig.Type,
		})

		if err := mm.manager.CreateLink(linkConfig); err != nil {
			return fmt.Errorf("failed to create link %s: %w", linkConfig.ID, err)
		}

		// Start the link
		if err := mm.manager.StartLink(linkConfig.ID); err != nil {
			return fmt.Errorf("failed to start link %s: %w", linkConfig.ID, err)
		}

		mm.logger.Info("Replication link started", logging.Fields{
			"id":     linkConfig.ID,
			"source": linkConfig.SourceCluster.ClusterID,
			"target": linkConfig.TargetCluster.ClusterID,
		})
	}

	// Start HTTP server for metrics and health
	mux := http.NewServeMux()
	mm.registerHandlers(mux)

	mm.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", mm.config.MetricsPort),
		Handler: mux,
	}

	go func() {
		mm.logger.Info("Starting HTTP server", logging.Fields{
			"port": mm.config.MetricsPort,
		})
		if err := mm.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			mm.logger.Error("HTTP server error", err)
		}
	}()

	return nil
}

// Stop stops the mirror maker
func (mm *mirrorMaker) Stop(ctx context.Context) error {
	mm.logger.Info("Stopping mirror maker", logging.Fields{})

	// Stop all replication links
	links, err := mm.manager.ListLinks()
	if err == nil {
		for _, lnk := range links {
			if lnk.Status == link.ReplicationStatusActive {
				mm.logger.Info("Stopping replication link", logging.Fields{
					"id": lnk.ID,
				})
				if err := mm.manager.StopLink(lnk.ID); err != nil {
					mm.logger.Error("Failed to stop link", err, logging.Fields{
						"id": lnk.ID,
					})
				}
			}
		}
	}

	// Close manager
	if err := mm.manager.Close(); err != nil {
		mm.logger.Error("Failed to close manager", err)
	}

	// Shutdown HTTP server
	if mm.httpServer != nil {
		if err := mm.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown HTTP server: %w", err)
		}
	}

	return nil
}

// registerHandlers registers HTTP handlers
func (mm *mirrorMaker) registerHandlers(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("/health", mm.handleHealth)
	mux.HandleFunc("/health/live", mm.handleLiveness)
	mux.HandleFunc("/health/ready", mm.handleReadiness)

	// Metrics
	mux.HandleFunc("/metrics", mm.handleMetrics)

	// Links status
	mux.HandleFunc("/api/v1/links", mm.handleLinks)
	mux.HandleFunc("/api/v1/links/", mm.handleLinkDetails)
}

// handleHealth handles GET /health
func (mm *mirrorMaker) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	links, err := mm.manager.ListLinks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	healthyCount := 0
	for _, link := range links {
		if link.Health != nil && link.Health.Status == "healthy" {
			healthyCount++
		}
	}

	status := map[string]interface{}{
		"status":       "ok",
		"total_links":  len(links),
		"healthy_links": healthyCount,
		"version":      version,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// handleLiveness handles GET /health/live
func (mm *mirrorMaker) handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// handleReadiness handles GET /health/ready
func (mm *mirrorMaker) handleReadiness(w http.ResponseWriter, r *http.Request) {
	links, err := mm.manager.ListLinks()
	if err != nil {
		http.Error(w, "Not ready", http.StatusServiceUnavailable)
		return
	}

	// Ready if at least one link is active
	for _, lnk := range links {
		if lnk.Status == link.ReplicationStatusActive {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ready"))
			return
		}
	}

	http.Error(w, "No active links", http.StatusServiceUnavailable)
}

// handleMetrics handles GET /metrics
func (mm *mirrorMaker) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	links, err := mm.manager.ListLinks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")

	// Write Prometheus-style metrics
	for _, link := range links {
		metrics, err := mm.manager.GetMetrics(link.ID)
		if err != nil {
			continue
		}

		// Total messages replicated
		fmt.Fprintf(w, "# HELP streambus_replication_messages_total Total messages replicated\n")
		fmt.Fprintf(w, "# TYPE streambus_replication_messages_total counter\n")
		fmt.Fprintf(w, "streambus_replication_messages_total{link_id=\"%s\",link_name=\"%s\"} %d\n",
			link.ID, link.Name, metrics.TotalMessagesReplicated)

		// Total bytes replicated
		fmt.Fprintf(w, "# HELP streambus_replication_bytes_total Total bytes replicated\n")
		fmt.Fprintf(w, "# TYPE streambus_replication_bytes_total counter\n")
		fmt.Fprintf(w, "streambus_replication_bytes_total{link_id=\"%s\",link_name=\"%s\"} %d\n",
			link.ID, link.Name, metrics.TotalBytesReplicated)

		// Messages per second
		fmt.Fprintf(w, "# HELP streambus_replication_messages_per_second Current replication rate\n")
		fmt.Fprintf(w, "# TYPE streambus_replication_messages_per_second gauge\n")
		fmt.Fprintf(w, "streambus_replication_messages_per_second{link_id=\"%s\",link_name=\"%s\"} %.2f\n",
			link.ID, link.Name, metrics.MessagesPerSecond)

		// Replication lag
		fmt.Fprintf(w, "# HELP streambus_replication_lag_ms Replication lag in milliseconds\n")
		fmt.Fprintf(w, "# TYPE streambus_replication_lag_ms gauge\n")
		fmt.Fprintf(w, "streambus_replication_lag_ms{link_id=\"%s\",link_name=\"%s\"} %d\n",
			link.ID, link.Name, metrics.ReplicationLag)

		// Total errors
		fmt.Fprintf(w, "# HELP streambus_replication_errors_total Total replication errors\n")
		fmt.Fprintf(w, "# TYPE streambus_replication_errors_total counter\n")
		fmt.Fprintf(w, "streambus_replication_errors_total{link_id=\"%s\",link_name=\"%s\"} %d\n",
			link.ID, link.Name, metrics.TotalErrors)

		// Consecutive failures
		fmt.Fprintf(w, "# HELP streambus_replication_consecutive_failures Consecutive failures\n")
		fmt.Fprintf(w, "# TYPE streambus_replication_consecutive_failures gauge\n")
		fmt.Fprintf(w, "streambus_replication_consecutive_failures{link_id=\"%s\",link_name=\"%s\"} %d\n",
			link.ID, link.Name, metrics.ConsecutiveFailures)

		// Uptime
		fmt.Fprintf(w, "# HELP streambus_replication_uptime_seconds Replication uptime in seconds\n")
		fmt.Fprintf(w, "# TYPE streambus_replication_uptime_seconds counter\n")
		fmt.Fprintf(w, "streambus_replication_uptime_seconds{link_id=\"%s\",link_name=\"%s\"} %d\n",
			link.ID, link.Name, metrics.UptimeSeconds)

		// Per-partition metrics
		for partKey, partMetrics := range metrics.PartitionMetrics {
			fmt.Fprintf(w, "# HELP streambus_replication_partition_lag Partition replication lag\n")
			fmt.Fprintf(w, "# TYPE streambus_replication_partition_lag gauge\n")
			fmt.Fprintf(w, "streambus_replication_partition_lag{link_id=\"%s\",topic=\"%s\",partition=\"%d\"} %d\n",
				link.ID, partMetrics.Topic, partMetrics.Partition, partMetrics.Lag)

			_ = partKey // Avoid unused variable
		}
	}
}

// handleLinks handles GET /api/v1/links
func (mm *mirrorMaker) handleLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	links, err := mm.manager.ListLinks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(links)
}

// handleLinkDetails handles GET /api/v1/links/:id
func (mm *mirrorMaker) handleLinkDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract link ID from path
	linkID := r.URL.Path[len("/api/v1/links/"):]
	if linkID == "" {
		http.Error(w, "Link ID required", http.StatusBadRequest)
		return
	}

	link, err := mm.manager.GetLink(linkID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(link)
}
