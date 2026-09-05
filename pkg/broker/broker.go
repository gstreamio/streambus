package broker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gstreamio/streambus/pkg/cluster"
	"github.com/gstreamio/streambus/pkg/consensus"
	"github.com/gstreamio/streambus/pkg/consumer/group"
	"github.com/gstreamio/streambus/pkg/health"
	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/metadata"
	"github.com/gstreamio/streambus/pkg/metrics"
	"github.com/gstreamio/streambus/pkg/replication/link"
	"github.com/gstreamio/streambus/pkg/schema"
	"github.com/gstreamio/streambus/pkg/security"
	"github.com/gstreamio/streambus/pkg/server"
	"github.com/gstreamio/streambus/pkg/tenancy"
	"github.com/gstreamio/streambus/pkg/transaction"
)

// Config holds broker configuration
type Config struct {
	// Broker identity
	BrokerID int32
	Host     string
	Port     int

	// Network ports
	GRPCPort int
	HTTPPort int

	// Storage configuration
	DataDir string

	// Raft configuration
	RaftDataDir string
	RaftPeers   []consensus.Peer

	// Server configuration
	Server *server.Config

	// Security configuration
	Security *security.SecurityConfig

	// Multi-tenancy configuration
	EnableMultiTenancy bool

	// Logging
	LogLevel string
}

// Broker represents a StreamBus broker instance
type Broker struct {
	config *Config
	logger *logging.Logger

	// Core components
	//
	// topicManager owns the broker's partition logs on disk. It is created by
	// initStorage and shared with the wire-protocol handler (initServer) so
	// the admin API reads exactly the data producers write, rather than
	// opening a second, independent view of the same directory.
	topicManager *server.TopicManager
	server       *server.Server
	raftNode     *consensus.RaftNode
	fsm          *metadata.FSM
	metaStore    *metadata.Store
	adapter      *metadata.ClusterMetadataStore

	// Cluster components
	registry    *cluster.BrokerRegistry
	coordinator *cluster.ClusterCoordinator
	heartbeat   *cluster.HeartbeatService

	// Consumer group coordinator
	groupCoordinator *group.GroupCoordinator

	// Transaction coordinator
	txnCoordinator *transaction.TransactionCoordinator

	// Cross-datacenter replication link manager
	replicationManager link.Manager

	// Schema registry
	schemaRegistry *schema.SchemaRegistry

	// Security
	securityManager *security.Manager

	// Multi-tenancy
	tenancyManager *tenancy.Manager

	// Observability
	healthRegistry *health.Registry
	metricsReg     *metrics.Registry
	httpServer     *http.Server

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	status BrokerStatus
}

// BrokerStatus represents the broker's current status
type BrokerStatus int

const (
	StatusStopped BrokerStatus = iota
	StatusStarting
	StatusRunning
	StatusStopping
)

// New creates a new broker instance
func New(config *Config) (*Broker, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Setup logger
	logLevel, err := logging.ParseLevel(config.LogLevel)
	if err != nil {
		logLevel = logging.LevelInfo
	}
	logger := logging.New(&logging.Config{
		Level:        logLevel,
		Output:       os.Stdout,
		Component:    "broker",
		IncludeTrace: false,
		IncludeFile:  true,
	})

	ctx, cancel := context.WithCancel(context.Background())

	b := &Broker{
		config:     config,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		status:     StatusStopped,
		metricsReg: metrics.NewRegistry(),
	}

	return b, nil
}

// Start starts the broker
func (b *Broker) Start() error {
	b.mu.Lock()
	if b.status != StatusStopped {
		b.mu.Unlock()
		return fmt.Errorf("broker is already running")
	}
	b.status = StatusStarting
	b.mu.Unlock()

	b.logger.Info("Starting StreamBus Broker", logging.Fields{
		"broker_id": b.config.BrokerID,
		"host":      b.config.Host,
		"port":      b.config.Port,
	})

	// Initialize components in order
	if err := b.initStorage(); err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize multi-tenancy BEFORE observability so the tenancy manager
	// exists when HTTP API routes are registered
	if err := b.initTenancy(); err != nil {
		return fmt.Errorf("failed to initialize multi-tenancy: %w", err)
	}

	// Start observability (HTTP/health endpoints) early so Docker health checks work
	// This must happen before initCluster() which may block waiting for Raft consensus
	if err := b.initObservability(); err != nil {
		return fmt.Errorf("failed to initialize observability: %w", err)
	}

	if err := b.initConsensus(); err != nil {
		return fmt.Errorf("failed to initialize consensus: %w", err)
	}

	if err := b.initCluster(); err != nil {
		return fmt.Errorf("failed to initialize cluster: %w", err)
	}

	if err := b.initConsumerGroups(); err != nil {
		return fmt.Errorf("failed to initialize consumer groups: %w", err)
	}

	if err := b.initTransactions(); err != nil {
		return fmt.Errorf("failed to initialize transactions: %w", err)
	}

	if err := b.initReplication(); err != nil {
		return fmt.Errorf("failed to initialize replication: %w", err)
	}

	if err := b.initSchemaRegistry(); err != nil {
		return fmt.Errorf("failed to initialize schema registry: %w", err)
	}

	if err := b.initSecurity(); err != nil {
		return fmt.Errorf("failed to initialize security: %w", err)
	}

	if err := b.initServer(); err != nil {
		return fmt.Errorf("failed to initialize server: %w", err)
	}

	b.mu.Lock()
	b.status = StatusRunning
	b.mu.Unlock()

	b.logger.Info("StreamBus Broker started successfully", logging.Fields{
		"broker_id": b.config.BrokerID,
		"address":   fmt.Sprintf("%s:%d", b.config.Host, b.config.Port),
	})

	return nil
}

// Stop stops the broker gracefully
// SecurityManager returns the security manager
func (b *Broker) SecurityManager() *security.Manager {
	return b.securityManager
}

// TenancyManager returns the tenancy manager
func (b *Broker) TenancyManager() *tenancy.Manager {
	return b.tenancyManager
}

// Stop stops the broker
func (b *Broker) Stop() error {
	b.mu.Lock()
	if b.status != StatusRunning {
		b.mu.Unlock()
		return fmt.Errorf("broker is not running")
	}
	b.status = StatusStopping
	b.mu.Unlock()

	b.logger.Info("Stopping StreamBus Broker")

	// Stop components in reverse order
	if b.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = b.httpServer.Shutdown(shutdownCtx)
		cancel()
	}

	if b.heartbeat != nil {
		_ = b.heartbeat.Stop()
	}

	if b.coordinator != nil {
		_ = b.coordinator.Stop()
	}

	if b.groupCoordinator != nil {
		_ = b.groupCoordinator.Stop()
	}

	if b.txnCoordinator != nil {
		b.txnCoordinator.Stop()
	}

	if b.replicationManager != nil {
		if err := b.replicationManager.Close(); err != nil {
			b.logger.Error("Failed to close replication manager", err)
		}
	}

	if b.server != nil {
		_ = b.server.Stop()
	}

	if b.securityManager != nil {
		if err := b.securityManager.Close(); err != nil {
			b.logger.Error("Failed to close security manager", err)
		}
	}

	if b.raftNode != nil {
		_ = b.raftNode.Stop()
	}

	if b.topicManager != nil {
		if err := b.topicManager.Close(); err != nil {
			b.logger.Error("Failed to close storage", err)
		}
	}

	// Cancel context
	b.cancel()

	// Wait for goroutines
	b.wg.Wait()

	b.mu.Lock()
	b.status = StatusStopped
	b.mu.Unlock()

	b.logger.Info("StreamBus Broker stopped")
	return nil
}

// WaitForShutdown blocks until the broker is stopped
func (b *Broker) WaitForShutdown() {
	<-b.ctx.Done()
}

// Status returns the current broker status
func (b *Broker) Status() BrokerStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

// initStorage initializes the storage engine.
//
// The TopicManager it creates is the broker's single owner of partition logs:
// initServer hands it to the wire-protocol handler, and the admin API and
// per-tenant storage accounting read through it. Creating it here (rather than
// letting initServer create its own) is what lets those readers observe the
// same bytes producers write.
func (b *Broker) initStorage() error {
	b.logger.Info("Initializing storage engine", logging.Fields{
		"data_dir": b.config.DataDir,
	})

	b.topicManager = server.NewTopicManager(b.config.DataDir)

	topics := b.topicManager.ListTopics()
	b.logger.Info("Storage engine initialized", logging.Fields{
		"data_dir": b.config.DataDir,
		"topics":   len(topics),
	})

	return nil
}

// initConsensus initializes Raft consensus
func (b *Broker) initConsensus() error {
	b.logger.Info("Initializing Raft consensus", logging.Fields{
		"node_id":  b.config.BrokerID,
		"raft_dir": b.config.RaftDataDir,
		"peers":    len(b.config.RaftPeers),
	})

	// Create FSM
	b.fsm = metadata.NewFSM()

	// Create Raft configuration
	raftConfig := consensus.DefaultConfig()
	raftConfig.NodeID = uint64(b.config.BrokerID)
	raftConfig.DataDir = b.config.RaftDataDir
	raftConfig.Peers = b.config.RaftPeers

	// Create Raft node
	node, err := consensus.NewNode(raftConfig, b.fsm)
	if err != nil {
		return fmt.Errorf("failed to create raft node: %w", err)
	}

	// Set bind address from peers
	for _, peer := range b.config.RaftPeers {
		if peer.ID == uint64(b.config.BrokerID) {
			node.SetBindAddr(peer.Addr)
			break
		}
	}

	// Start Raft node
	if err := node.Start(); err != nil {
		return fmt.Errorf("failed to start raft node: %w", err)
	}

	b.raftNode = node

	// Create metadata store
	b.metaStore = metadata.NewStore(b.fsm, b.raftNode)

	// Create cluster metadata adapter
	b.adapter = metadata.NewClusterMetadataStore(b.metaStore)

	b.logger.Info("Raft consensus initialized")

	return nil
}

// initCluster initializes cluster components
func (b *Broker) initCluster() error {
	b.logger.Info("Initializing cluster components")

	// Wait for Raft leader election (with timeout)
	b.logger.Info("Waiting for Raft leader election...")
	deadline := time.Now().Add(30 * time.Second)
	leaderElected := false
	for time.Now().Before(deadline) {
		leaderID := b.raftNode.Leader()
		if b.metaStore.IsLeader() || leaderID != 0 {
			leaderElected = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !leaderElected {
		b.logger.Warn("Leader not elected yet, continuing anyway")
	} else {
		leaderID := b.raftNode.Leader()
		b.logger.Info("Raft leader elected", logging.Fields{
			"leader_id": leaderID,
			"is_leader": b.metaStore.IsLeader(),
		})
	}

	// Create broker registry
	b.registry = cluster.NewBrokerRegistry(b.adapter)

	// Register this broker
	brokerMetadata := &cluster.BrokerMetadata{
		ID:     b.config.BrokerID,
		Host:   b.config.Host,
		Port:   b.config.Port,
		Rack:   "default",
		Status: cluster.BrokerStatusStarting,
	}

	if err := b.registry.RegisterBroker(b.ctx, brokerMetadata); err != nil {
		b.logger.Warn("Failed to register broker initially", logging.Fields{
			"error": err.Error(),
		})
	}

	// Start heartbeat service
	b.heartbeat = cluster.NewHeartbeatService(b.config.BrokerID, b.registry)
	b.heartbeat.SetInterval(3 * time.Second)
	if err := b.heartbeat.Start(); err != nil {
		return fmt.Errorf("failed to start heartbeat service: %w", err)
	}

	// Create cluster coordinator (only start if leader)
	strategy := cluster.NewRoundRobinStrategy()
	b.coordinator = cluster.NewClusterCoordinator(b.registry, strategy, b.adapter)

	// Start coordinator if this node is leader
	if b.metaStore.IsLeader() {
		b.logger.Info("Starting cluster coordinator (this node is leader)")
		if err := b.coordinator.Start(); err != nil {
			b.logger.Error("Failed to start coordinator", err)
		}
	}

	b.logger.Info("Cluster components initialized")

	return nil
}

// initConsumerGroups initializes the consumer group coordinator
func (b *Broker) initConsumerGroups() error {
	b.logger.Info("Initializing consumer group coordinator")

	// Persist offsets under the broker's data directory so committed offsets
	// survive a restart. Some tests construct a Broker struct literal with
	// no DataDir, so fall back to memory storage rather than failing.
	var offsetStorage group.OffsetStorage
	if b.config.DataDir == "" {
		offsetStorage = group.NewMemoryOffsetStorage()
	} else {
		fileStorage, err := group.NewFileOffsetStorage(filepath.Join(b.config.DataDir, "consumer-offsets"))
		if err != nil {
			return fmt.Errorf("failed to initialize offset storage: %w", err)
		}
		offsetStorage = fileStorage
	}

	// Create coordinator configuration
	coordinatorConfig := group.DefaultCoordinatorConfig()

	// Create group coordinator
	b.groupCoordinator = group.NewGroupCoordinator(offsetStorage, coordinatorConfig)

	b.logger.Info("Consumer group coordinator initialized")

	return nil
}

// initTransactions initializes the transaction coordinator
func (b *Broker) initTransactions() error {
	b.logger.Info("Initializing transaction coordinator")

	// Create transaction log (using memory storage for now)
	txnLog := transaction.NewMemoryTransactionLog()

	// Create coordinator configuration
	coordinatorConfig := transaction.DefaultCoordinatorConfig()

	// Create transaction coordinator
	b.txnCoordinator = transaction.NewTransactionCoordinator(txnLog, coordinatorConfig, b.logger)

	// Give the coordinator somewhere real to write transaction markers and
	// publish transactional offsets. Without these it cannot honour a commit
	// and EndTxn refuses rather than reporting a commit it did not perform.
	b.txnCoordinator.SetMarkerWriter(newLogMarkerWriter(b.topicManager))
	b.txnCoordinator.SetOffsetCommitter(newGroupOffsetCommitter(b.groupCoordinator))

	b.logger.Info("Transaction coordinator initialized")

	return nil
}

// initReplication initializes the cross-datacenter replication link manager
// that backs the /api/v1/replication/links admin endpoints.
//
// Link definitions, checkpoints and offset mappings are persisted under the
// broker's data directory so links survive a restart instead of having to be
// recreated. Some tests construct a Broker struct literal with no DataDir,
// so fall back to in-memory storage rather than failing in that case.
func (b *Broker) initReplication() error {
	b.logger.Info("Initializing replication link manager")

	var linkStorage link.Storage
	if b.config.DataDir == "" {
		linkStorage = link.NewMemoryStorage()
	} else {
		fileStorage, err := link.NewFileStorage(filepath.Join(b.config.DataDir, "replication-links"))
		if err != nil {
			return fmt.Errorf("failed to initialize replication link storage: %w", err)
		}
		linkStorage = fileStorage
	}

	b.replicationManager = link.NewManager(linkStorage)

	b.logger.Info("Replication link manager initialized")

	return nil
}

// initSchemaRegistry initializes the schema registry
func (b *Broker) initSchemaRegistry() error {
	b.logger.Info("Initializing schema registry")

	// Create schema validator
	validator := schema.NewDefaultValidator()

	// Create schema registry
	b.schemaRegistry = schema.NewSchemaRegistry(validator, b.logger)

	b.logger.Info("Schema registry initialized")

	return nil
}

// initSecurity initializes the security manager
func (b *Broker) initSecurity() error {
	b.logger.Info("Initializing security")

	// Use default security config if none provided
	securityConfig := b.config.Security
	if securityConfig == nil {
		securityConfig = security.DefaultSecurityConfig()
		b.logger.Info("Using default security configuration (all security features disabled)")
	}

	// Create security manager
	secManager, err := security.NewManager(securityConfig, b.logger)
	if err != nil {
		return fmt.Errorf("failed to create security manager: %w", err)
	}

	b.securityManager = secManager

	// Log security configuration
	b.logger.Info("Security manager initialized", logging.Fields{
		"authentication": secManager.IsAuthenticationEnabled(),
		"authorization":  secManager.IsAuthorizationEnabled(),
		"audit":          secManager.IsAuditEnabled(),
		"encryption":     secManager.IsEncryptionEnabled(),
	})

	return nil
}

// initTenancy initializes the multi-tenancy manager
func (b *Broker) initTenancy() error {
	b.logger.Info("Initializing multi-tenancy")

	if !b.config.EnableMultiTenancy {
		b.logger.Info("Multi-tenancy disabled")
		return nil
	}

	// Create tenancy manager
	b.tenancyManager = tenancy.NewManager()

	// Create default tenant for backward compatibility
	defaultQuotas := tenancy.DefaultQuotas()
	_, err := b.tenancyManager.CreateTenant("default", "Default Tenant", defaultQuotas)
	if err != nil {
		b.logger.Warn("Failed to create default tenant", logging.Fields{
			"error": err.Error(),
		})
	}

	b.logger.Info("Multi-tenancy initialized", logging.Fields{
		"enabled": true,
	})

	// Start storage tracking goroutine
	b.wg.Add(1)
	go b.trackTenantStorage()

	return nil
}

// trackTenantStorage periodically updates tenant storage usage
func (b *Broker) trackTenantStorage() {
	defer b.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.updateTenantStorageUsage()
		}
	}
}

// updateTenantStorageUsage recomputes each tenant's storage usage from the
// bytes its topics actually occupy on disk and reports it to the quota
// tracker.
//
// A tenant owns a topic when it created it through the tenancy-aware handler
// (see tenancy.Manager.RegisterTopic). Topics with no ownership record - those
// created before multi-tenancy was enabled, or directly on disk - are not
// counted against any tenant; they are reported as unattributed so an operator
// can see the gap rather than silently under-billing a tenant.
func (b *Broker) updateTenantStorageUsage() {
	if b.tenancyManager == nil || b.topicManager == nil {
		return
	}

	usageByTopic := b.topicManager.DiskUsageByTopic()
	owners := b.tenancyManager.TopicOwners()

	usageByTenant := make(map[tenancy.TenantID]int64)
	var unattributed int64
	for topic, bytes := range usageByTopic {
		owner, ok := owners[topic]
		if !ok {
			unattributed += bytes
			continue
		}
		usageByTenant[owner] += bytes
	}

	for _, tenant := range b.tenancyManager.ListTenants() {
		storageBytes := usageByTenant[tenant.ID]

		if err := b.tenancyManager.UpdateStorageUsage(tenant.ID, storageBytes); err != nil {
			b.logger.Error("Failed to update tenant storage usage", err, logging.Fields{
				"tenant_id": string(tenant.ID),
			})
			continue
		}

		b.logger.Debug("Updated storage usage for tenant", logging.Fields{
			"tenant_id":     string(tenant.ID),
			"storage_bytes": storageBytes,
		})
	}

	if unattributed > 0 {
		b.logger.Debug("Storage not attributed to any tenant", logging.Fields{
			"bytes": unattributed,
		})
	}
}

// initServer initializes the network server
func (b *Broker) initServer() error {
	b.logger.Info("Initializing network server", logging.Fields{
		"address": fmt.Sprintf("%s:%d", b.config.Host, b.config.Port),
	})

	// Create base server handler over the broker's storage. initStorage runs
	// first, so b.topicManager is set; fall back to opening one here only if a
	// caller drove initServer directly (as some tests do).
	if b.topicManager == nil {
		b.topicManager = server.NewTopicManager(b.config.DataDir)
	}
	baseHandler := server.NewHandlerWithTopicManager(b.topicManager)

	// Route consumer group and transaction requests to their coordinators.
	// These sit closest to the base handler so the tenancy, schema and
	// security wrappers below still see every request.
	var handler server.RequestHandler = baseHandler
	handler = server.NewCoordinationHandler(handler, b.groupCoordinator, newRegistryCoordinatorLocator(b.registry))
	handler = server.NewTransactionHandler(handler, b.txnCoordinator)

	// Wrap with tenancy handler if enabled
	if b.config.EnableMultiTenancy && b.tenancyManager != nil {
		handler = server.NewTenancyHandler(handler, b.tenancyManager, true)
		b.logger.Info("Multi-tenancy enabled for request handling")
	}

	// Wrap with schema validation handler if schema registry is available
	if b.schemaRegistry != nil {
		schemaHandler := server.NewSchemaHandler(handler, b.schemaRegistry, true)
		handler = schemaHandler
		b.logger.Info("Schema validation enabled for produce path")
	}

	// Wrap with security handler if security is enabled
	if b.securityManager != nil {
		securityEnabled := b.securityManager.IsAuthenticationEnabled() || b.securityManager.IsAuthorizationEnabled()
		securityHandler := server.NewSecurityHandler(handler, b.securityManager, securityEnabled)
		handler = securityHandler
		if securityEnabled {
			b.logger.Info("Security enabled for request handling", logging.Fields{
				"authentication": b.securityManager.IsAuthenticationEnabled(),
				"authorization":  b.securityManager.IsAuthorizationEnabled(),
			})
		}
	}

	// Create server
	serverConfig := b.config.Server
	if serverConfig == nil {
		serverConfig = server.DefaultConfig()
	}
	serverConfig.Address = fmt.Sprintf("%s:%d", b.config.Host, b.config.Port)

	srv, err := server.New(serverConfig, handler)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Start server
	if err := srv.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	b.server = srv

	b.logger.Info("Network server started")

	return nil
}

// initObservability initializes health checks and metrics
func (b *Broker) initObservability() error {
	b.logger.Info("Initializing observability", logging.Fields{
		"http_port": b.config.HTTPPort,
	})

	// Create health registry
	b.healthRegistry = health.NewRegistry()

	// Register health checkers
	if b.raftNode != nil {
		b.healthRegistry.Register(health.NewRaftNodeHealthChecker("raft", b.raftNode))
	}

	// Create HTTP handler for health
	healthHandler := health.NewHandler(b.healthRegistry)

	// Create HTTP server
	mux := http.NewServeMux()
	healthHandler.RegisterRoutes(mux)

	// Register metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("# Metrics endpoint placeholder\n"))
	})

	// Register tenancy management API endpoints
	b.registerTenancyAPI(mux)

	// Register admin management API endpoints
	b.registerAdminAPI(mux)

	// Register schema registry API endpoints
	b.registerSchemaAPI(mux)

	httpAddr := fmt.Sprintf(":%d", b.config.HTTPPort)
	b.httpServer = &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}

	// Start HTTP server in background
	go func() {
		b.logger.Info("HTTP server starting", logging.Fields{
			"address": httpAddr,
		})
		if err := b.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			b.logger.Error("HTTP server error", err)
		}
	}()

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	b.logger.Info("Observability initialized", logging.Fields{
		"health_endpoint":  fmt.Sprintf("http://0.0.0.0:%d/health", b.config.HTTPPort),
		"metrics_endpoint": fmt.Sprintf("http://0.0.0.0:%d/metrics", b.config.HTTPPort),
	})

	return nil
}

// IsReady returns true if the broker is ready to accept requests
func (b *Broker) IsReady() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status == StatusRunning
}

// IsAlive returns true if the broker is alive (not stopped)
func (b *Broker) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status != StatusStopped
}

// IsLeader returns true if this broker is the Raft leader
func (b *Broker) IsLeader() bool {
	if b.raftNode == nil {
		return false
	}
	return b.raftNode.IsLeader()
}

// validateConfig validates broker configuration
func validateConfig(config *Config) error {
	if config.BrokerID <= 0 {
		return fmt.Errorf("broker_id must be positive")
	}

	if config.Host == "" {
		return fmt.Errorf("host is required")
	}

	if config.Port <= 0 {
		return fmt.Errorf("port must be positive")
	}

	if config.DataDir == "" {
		return fmt.Errorf("data_dir is required")
	}

	if config.RaftDataDir == "" {
		return fmt.Errorf("raft_data_dir is required")
	}

	if len(config.RaftPeers) == 0 {
		return fmt.Errorf("raft_peers is required")
	}

	return nil
}
