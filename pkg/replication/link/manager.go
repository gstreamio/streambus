package link

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// manager implements the Manager interface
type manager struct {
	mu sync.RWMutex

	// links stores all replication links by ID
	links map[string]*ReplicationLink

	// streamHandlers stores active stream handlers by link ID
	streamHandlers map[string]*StreamHandler

	// checkpoints stores checkpoints by link ID -> topic -> partition
	checkpoints map[string]map[string]map[int32]*Checkpoint

	// offsetMappings stores offset mappings by link ID -> topic -> partition
	offsetMappings map[string]map[string]map[int32]*OffsetMapping

	// storage is the persistence layer (optional)
	storage Storage

	// ctx is the manager context
	ctx context.Context

	// cancel cancels the manager context
	cancel context.CancelFunc

	// wg tracks background goroutines
	wg sync.WaitGroup
}

// Storage interface for persisting replication state
type Storage interface {
	// SaveLink saves a replication link
	SaveLink(link *ReplicationLink) error

	// LoadLink loads a replication link
	LoadLink(linkID string) (*ReplicationLink, error)

	// DeleteLink deletes a replication link
	DeleteLink(linkID string) error

	// ListLinks lists all replication links
	ListLinks() ([]*ReplicationLink, error)

	// SaveCheckpoint saves a checkpoint
	SaveCheckpoint(checkpoint *Checkpoint) error

	// LoadCheckpoint loads a checkpoint
	LoadCheckpoint(linkID, topic string, partition int32) (*Checkpoint, error)

	// SaveOffsetMapping saves an offset mapping
	SaveOffsetMapping(mapping *OffsetMapping) error

	// LoadOffsetMapping loads an offset mapping
	LoadOffsetMapping(linkID, topic string, partition int32) (*OffsetMapping, error)
}

// NewManager creates a new replication link manager
func NewManager(storage Storage) Manager {
	ctx, cancel := context.WithCancel(context.Background())

	mgr := &manager{
		links:          make(map[string]*ReplicationLink),
		streamHandlers: make(map[string]*StreamHandler),
		checkpoints:    make(map[string]map[string]map[int32]*Checkpoint),
		offsetMappings: make(map[string]map[string]map[int32]*OffsetMapping),
		storage:        storage,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Load existing links from storage if available
	if storage != nil {
		mgr.loadLinksFromStorage()
	}

	// Start health monitoring goroutine
	mgr.wg.Add(1)
	go mgr.healthMonitorLoop()

	return mgr
}

// CreateLink creates a new replication link
func (m *manager) CreateLink(config *ReplicationLink) error {
	if config.ID == "" {
		config.ID = uuid.New().String()
	}

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid link configuration: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if link already exists
	if _, exists := m.links[config.ID]; exists {
		return fmt.Errorf("replication link %s already exists", config.ID)
	}

	// Set creation timestamps
	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now
	config.Status = ReplicationStatusStopped

	// Initialize metrics and health
	config.Metrics = &ReplicationMetrics{
		PartitionMetrics: make(map[string]*PartitionReplicationMetrics),
	}
	config.Health = &ReplicationHealth{
		Status: "unhealthy",
	}

	// Apply defaults if not set
	if config.Config.MaxBytes == 0 {
		config.Config = DefaultReplicationConfig()
	}

	if config.FailoverConfig == nil {
		config.FailoverConfig = DefaultFailoverConfig()
	}

	// Store the link
	m.links[config.ID] = config.Clone()

	// Initialize checkpoint and offset mapping storage
	m.checkpoints[config.ID] = make(map[string]map[int32]*Checkpoint)
	m.offsetMappings[config.ID] = make(map[string]map[int32]*OffsetMapping)

	// Persist to storage if available
	if m.storage != nil {
		if err := m.storage.SaveLink(config); err != nil {
			delete(m.links, config.ID)
			return fmt.Errorf("failed to persist link: %w", err)
		}
	}

	return nil
}

// DeleteLink deletes a replication link
func (m *manager) DeleteLink(linkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, exists := m.links[linkID]
	if !exists {
		return fmt.Errorf("replication link %s not found", linkID)
	}

	// Don't allow deletion of active links
	if link.Status == ReplicationStatusActive || link.Status == ReplicationStatusInitializing {
		return fmt.Errorf("cannot delete active replication link, stop it first")
	}

	// Delete from memory
	delete(m.links, linkID)
	delete(m.checkpoints, linkID)
	delete(m.offsetMappings, linkID)

	// Delete from storage if available
	if m.storage != nil {
		if err := m.storage.DeleteLink(linkID); err != nil {
			return fmt.Errorf("failed to delete link from storage: %w", err)
		}
	}

	return nil
}

// UpdateLink updates a replication link configuration
func (m *manager) UpdateLink(linkID string, config *ReplicationLink) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid link configuration: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existingLink, exists := m.links[linkID]
	if !exists {
		return fmt.Errorf("replication link %s not found", linkID)
	}

	// Don't allow updates to active links (they must be paused first)
	if existingLink.Status == ReplicationStatusActive {
		return fmt.Errorf("cannot update active replication link, pause it first")
	}

	// Preserve immutable fields
	config.ID = linkID
	config.CreatedAt = existingLink.CreatedAt
	config.UpdatedAt = time.Now()
	config.Status = existingLink.Status
	config.Metrics = existingLink.Metrics
	config.Health = existingLink.Health

	// Update the link
	m.links[linkID] = config.Clone()

	// Persist to storage if available
	if m.storage != nil {
		if err := m.storage.SaveLink(config); err != nil {
			return fmt.Errorf("failed to persist link update: %w", err)
		}
	}

	return nil
}

// GetLink retrieves a replication link
func (m *manager) GetLink(linkID string) (*ReplicationLink, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	link, exists := m.links[linkID]
	if !exists {
		return nil, fmt.Errorf("replication link %s not found", linkID)
	}

	return link.Clone(), nil
}

// ListLinks lists all replication links
func (m *manager) ListLinks() ([]*ReplicationLink, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	links := make([]*ReplicationLink, 0, len(m.links))
	for _, link := range m.links {
		links = append(links, link.Clone())
	}

	return links, nil
}

// StartLink starts a replication link
func (m *manager) StartLink(linkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, exists := m.links[linkID]
	if !exists {
		return fmt.Errorf("replication link %s not found", linkID)
	}

	if link.Status == ReplicationStatusActive {
		return fmt.Errorf("replication link %s is already active", linkID)
	}

	// Create stream handler
	handler, err := NewStreamHandler(link, m.storage)
	if err != nil {
		return fmt.Errorf("failed to create stream handler: %w", err)
	}

	// Start the stream handler
	if err := handler.Start(); err != nil {
		return fmt.Errorf("failed to start stream handler: %w", err)
	}

	// Store the handler
	m.streamHandlers[linkID] = handler

	// Update status
	link.Status = ReplicationStatusActive
	link.StartedAt = time.Now()
	link.UpdatedAt = time.Now()

	// Reset consecutive failures
	if link.Metrics != nil {
		link.Metrics.ConsecutiveFailures = 0
	}

	// Update health
	if link.Health != nil {
		link.Health.Status = "healthy"
		link.Health.Issues = nil
		link.Health.Warnings = nil
	}

	// Persist status change
	if m.storage != nil {
		if err := m.storage.SaveLink(link); err != nil {
			// Stop the handler if we can't persist
			_ = handler.Stop()
			delete(m.streamHandlers, linkID)
			return fmt.Errorf("failed to persist link status: %w", err)
		}
	}

	return nil
}

// StopLink stops a replication link
func (m *manager) StopLink(linkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, exists := m.links[linkID]
	if !exists {
		return fmt.Errorf("replication link %s not found", linkID)
	}

	if link.Status == ReplicationStatusStopped {
		return nil // Already stopped
	}

	// Stop the stream handler if running
	if handler, exists := m.streamHandlers[linkID]; exists {
		if err := handler.Stop(); err != nil {
			return fmt.Errorf("failed to stop stream handler: %w", err)
		}
		delete(m.streamHandlers, linkID)
	}

	// Update status
	link.Status = ReplicationStatusStopped
	link.StoppedAt = time.Now()
	link.UpdatedAt = time.Now()

	// Persist status change
	if m.storage != nil {
		if err := m.storage.SaveLink(link); err != nil {
			return fmt.Errorf("failed to persist link status: %w", err)
		}
	}

	return nil
}

// PauseLink pauses a replication link
func (m *manager) PauseLink(linkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, exists := m.links[linkID]
	if !exists {
		return fmt.Errorf("replication link %s not found", linkID)
	}

	if link.Status != ReplicationStatusActive {
		return fmt.Errorf("can only pause active replication links")
	}

	// Update status
	link.Status = ReplicationStatusPaused
	link.UpdatedAt = time.Now()

	// Persist status change
	if m.storage != nil {
		if err := m.storage.SaveLink(link); err != nil {
			return fmt.Errorf("failed to persist link status: %w", err)
		}
	}

	return nil
}

// ResumeLink resumes a paused replication link
func (m *manager) ResumeLink(linkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, exists := m.links[linkID]
	if !exists {
		return fmt.Errorf("replication link %s not found", linkID)
	}

	if link.Status != ReplicationStatusPaused {
		return fmt.Errorf("can only resume paused replication links")
	}

	// Update status
	link.Status = ReplicationStatusActive
	link.UpdatedAt = time.Now()

	// Persist status change
	if m.storage != nil {
		if err := m.storage.SaveLink(link); err != nil {
			return fmt.Errorf("failed to persist link status: %w", err)
		}
	}

	return nil
}

// GetMetrics retrieves metrics for a replication link
func (m *manager) GetMetrics(linkID string) (*ReplicationMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	link, exists := m.links[linkID]
	if !exists {
		return nil, fmt.Errorf("replication link %s not found", linkID)
	}

	if link.Metrics == nil {
		return &ReplicationMetrics{
			PartitionMetrics: make(map[string]*PartitionReplicationMetrics),
		}, nil
	}

	// Clone metrics to prevent modification
	metrics := &ReplicationMetrics{
		TotalMessagesReplicated:   link.Metrics.TotalMessagesReplicated,
		TotalBytesReplicated:      link.Metrics.TotalBytesReplicated,
		MessagesPerSecond:         link.Metrics.MessagesPerSecond,
		BytesPerSecond:            link.Metrics.BytesPerSecond,
		ReplicationLag:            link.Metrics.ReplicationLag,
		AverageReplicationLag:     link.Metrics.AverageReplicationLag,
		MaxReplicationLag:         link.Metrics.MaxReplicationLag,
		TotalErrors:               link.Metrics.TotalErrors,
		ErrorsPerSecond:           link.Metrics.ErrorsPerSecond,
		LastCheckpoint:            link.Metrics.LastCheckpoint,
		LastSuccessfulReplication: link.Metrics.LastSuccessfulReplication,
		PartitionMetrics:          make(map[string]*PartitionReplicationMetrics),
		ConsecutiveFailures:       link.Metrics.ConsecutiveFailures,
		UptimeSeconds:             link.Metrics.UptimeSeconds,
	}

	for k, v := range link.Metrics.PartitionMetrics {
		metrics.PartitionMetrics[k] = &PartitionReplicationMetrics{
			Topic:              v.Topic,
			Partition:          v.Partition,
			SourceOffset:       v.SourceOffset,
			TargetOffset:       v.TargetOffset,
			Lag:                v.Lag,
			MessagesReplicated: v.MessagesReplicated,
			BytesReplicated:    v.BytesReplicated,
			LastReplicatedAt:   v.LastReplicatedAt,
			Errors:             v.Errors,
		}
	}

	return metrics, nil
}

// GetHealth retrieves health status for a replication link
func (m *manager) GetHealth(linkID string) (*ReplicationHealth, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	link, exists := m.links[linkID]
	if !exists {
		return nil, fmt.Errorf("replication link %s not found", linkID)
	}

	if link.Health == nil {
		return &ReplicationHealth{
			Status: "unknown",
		}, nil
	}

	// Clone health to prevent modification
	health := &ReplicationHealth{
		Status:                 link.Health.Status,
		LastHealthCheck:        link.Health.LastHealthCheck,
		SourceClusterReachable: link.Health.SourceClusterReachable,
		TargetClusterReachable: link.Health.TargetClusterReachable,
		ReplicationLagHealthy:  link.Health.ReplicationLagHealthy,
		ErrorRateHealthy:       link.Health.ErrorRateHealthy,
		CheckpointHealthy:      link.Health.CheckpointHealthy,
		Issues:                 append([]string{}, link.Health.Issues...),
		Warnings:               append([]string{}, link.Health.Warnings...),
	}

	return health, nil
}

// Failover triggers a manual failover
func (m *manager) Failover(linkID string) (*FailoverEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, exists := m.links[linkID]
	if !exists {
		return nil, fmt.Errorf("replication link %s not found", linkID)
	}

	// Only allow failover for active-passive or active-active links
	if link.Type != ReplicationTypeActivePassive && link.Type != ReplicationTypeActiveActive {
		return nil, fmt.Errorf("failover not supported for replication type %s", link.Type)
	}

	startTime := time.Now()

	// Create failover event
	event := &FailoverEvent{
		ID:              fmt.Sprintf("failover-%s-%d", linkID, time.Now().Unix()),
		LinkID:          linkID,
		Type:            "failover",
		Reason:          "manual",
		SourceClusterID: link.SourceCluster.ClusterID,
		TargetClusterID: link.TargetCluster.ClusterID,
		Timestamp:       startTime,
		OffsetMappings:  make(map[string]*OffsetMapping),
	}

	// Stop the current replication
	handler, handlerExists := m.streamHandlers[linkID]
	if handlerExists {
		if err := handler.Stop(); err != nil {
			event.Success = false
			event.ErrorMessage = fmt.Sprintf("failed to stop stream handler: %v", err)
			event.Duration = time.Since(startTime)
			return event, fmt.Errorf("failover failed: %w", err)
		}
		delete(m.streamHandlers, linkID)
	}

	// Capture current offset mappings for all topics
	if linkCheckpoints, exists := m.checkpoints[linkID]; exists {
		for topic, partitionCheckpoints := range linkCheckpoints {
			for partition, checkpoint := range partitionCheckpoints {
				key := fmt.Sprintf("%s-%d", topic, partition)
				mapping := &OffsetMapping{
					LinkID:      linkID,
					Topic:       topic,
					Partition:   partition,
					Mappings:    map[int64]int64{checkpoint.SourceOffset: checkpoint.TargetOffset},
					LastUpdated: time.Now(),
				}
				event.OffsetMappings[key] = mapping

				// Store the mapping
				if m.offsetMappings[linkID] == nil {
					m.offsetMappings[linkID] = make(map[string]map[int32]*OffsetMapping)
				}
				if m.offsetMappings[linkID][topic] == nil {
					m.offsetMappings[linkID][topic] = make(map[int32]*OffsetMapping)
				}
				m.offsetMappings[linkID][topic][partition] = mapping

				// Persist if storage available
				if m.storage != nil {
					_ = m.storage.SaveOffsetMapping(mapping)
				}
			}
		}
	}

	// Swap source and target clusters to reverse replication direction
	originalSource := link.SourceCluster
	originalTarget := link.TargetCluster

	link.SourceCluster = originalTarget
	link.TargetCluster = originalSource
	link.UpdatedAt = time.Now()

	// Update status
	link.Status = ReplicationStatusStopped

	// Persist the updated link
	if m.storage != nil {
		if err := m.storage.SaveLink(link); err != nil {
			event.Success = false
			event.ErrorMessage = fmt.Sprintf("failed to persist failover state: %v", err)
			event.Duration = time.Since(startTime)
			return event, fmt.Errorf("failover failed: %w", err)
		}
	}

	// Send notifications if configured
	if link.FailoverConfig != nil && link.FailoverConfig.NotificationWebhook != "" {
		go m.sendFailoverNotification(link.FailoverConfig.NotificationWebhook, event)
	}

	event.Success = true
	event.Duration = time.Since(startTime)

	return event, nil
}

// Failback triggers a manual failback
func (m *manager) Failback(linkID string) (*FailoverEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, exists := m.links[linkID]
	if !exists {
		return nil, fmt.Errorf("replication link %s not found", linkID)
	}

	// Only allow failback for active-passive links
	if link.Type != ReplicationTypeActivePassive {
		return nil, fmt.Errorf("failback only supported for active-passive replication")
	}

	startTime := time.Now()

	// Create failback event
	event := &FailoverEvent{
		ID:              fmt.Sprintf("failback-%s-%d", linkID, time.Now().Unix()),
		LinkID:          linkID,
		Type:            "failback",
		Reason:          "manual",
		SourceClusterID: link.SourceCluster.ClusterID,
		TargetClusterID: link.TargetCluster.ClusterID,
		Timestamp:       startTime,
		OffsetMappings:  make(map[string]*OffsetMapping),
	}

	// Verify failback delay if configured
	if link.FailoverConfig != nil && link.FailoverConfig.FailbackDelayMs > 0 {
		lastFailover := link.StoppedAt
		if !lastFailover.IsZero() {
			minFailbackTime := lastFailover.Add(time.Duration(link.FailoverConfig.FailbackDelayMs) * time.Millisecond)
			if time.Now().Before(minFailbackTime) {
				event.Success = false
				event.ErrorMessage = "failback delay not yet elapsed"
				event.Duration = time.Since(startTime)
				return event, fmt.Errorf("failback not allowed yet, must wait until %v", minFailbackTime)
			}
		}
	}

	// Stop current replication if running
	handler, handlerExists := m.streamHandlers[linkID]
	if handlerExists {
		if err := handler.Stop(); err != nil {
			event.Success = false
			event.ErrorMessage = fmt.Sprintf("failed to stop stream handler: %v", err)
			event.Duration = time.Since(startTime)
			return event, fmt.Errorf("failback failed: %w", err)
		}
		delete(m.streamHandlers, linkID)
	}

	// Capture current offset mappings
	if linkCheckpoints, exists := m.checkpoints[linkID]; exists {
		for topic, partitionCheckpoints := range linkCheckpoints {
			for partition, checkpoint := range partitionCheckpoints {
				key := fmt.Sprintf("%s-%d", topic, partition)
				mapping := &OffsetMapping{
					LinkID:      linkID,
					Topic:       topic,
					Partition:   partition,
					Mappings:    map[int64]int64{checkpoint.SourceOffset: checkpoint.TargetOffset},
					LastUpdated: time.Now(),
				}
				event.OffsetMappings[key] = mapping
			}
		}
	}

	// Swap back source and target clusters to restore original direction
	originalSource := link.SourceCluster
	originalTarget := link.TargetCluster

	link.SourceCluster = originalTarget
	link.TargetCluster = originalSource
	link.UpdatedAt = time.Now()
	link.Status = ReplicationStatusStopped

	// Persist the updated link
	if m.storage != nil {
		if err := m.storage.SaveLink(link); err != nil {
			event.Success = false
			event.ErrorMessage = fmt.Sprintf("failed to persist failback state: %v", err)
			event.Duration = time.Since(startTime)
			return event, fmt.Errorf("failback failed: %w", err)
		}
	}

	// Send notifications if configured
	if link.FailoverConfig != nil && link.FailoverConfig.NotificationWebhook != "" {
		go m.sendFailoverNotification(link.FailoverConfig.NotificationWebhook, event)
	}

	event.Success = true
	event.Duration = time.Since(startTime)

	return event, nil
}

// sendFailoverNotification sends a failover notification to a webhook
func (m *manager) sendFailoverNotification(webhookURL string, event *FailoverEvent) {
	// TODO: Implement webhook notification
	// This would typically make an HTTP POST request to the webhook URL
	// with the failover event details
}

// GetCheckpoint retrieves the checkpoint for a topic-partition
func (m *manager) GetCheckpoint(linkID, topic string, partition int32) (*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	linkCheckpoints, exists := m.checkpoints[linkID]
	if !exists {
		return nil, fmt.Errorf("no checkpoints for link %s", linkID)
	}

	topicCheckpoints, exists := linkCheckpoints[topic]
	if !exists {
		return nil, fmt.Errorf("no checkpoints for topic %s", topic)
	}

	checkpoint, exists := topicCheckpoints[partition]
	if !exists {
		return nil, fmt.Errorf("no checkpoint for partition %d", partition)
	}

	return checkpoint, nil
}

// SetCheckpoint sets the checkpoint for a topic-partition
func (m *manager) SetCheckpoint(checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure the link exists
	if _, exists := m.links[checkpoint.LinkID]; !exists {
		return fmt.Errorf("replication link %s not found", checkpoint.LinkID)
	}

	// Initialize nested maps if needed
	if m.checkpoints[checkpoint.LinkID] == nil {
		m.checkpoints[checkpoint.LinkID] = make(map[string]map[int32]*Checkpoint)
	}
	if m.checkpoints[checkpoint.LinkID][checkpoint.Topic] == nil {
		m.checkpoints[checkpoint.LinkID][checkpoint.Topic] = make(map[int32]*Checkpoint)
	}

	// Store checkpoint
	m.checkpoints[checkpoint.LinkID][checkpoint.Topic][checkpoint.Partition] = checkpoint

	// Update metrics
	if link, exists := m.links[checkpoint.LinkID]; exists && link.Metrics != nil {
		link.Metrics.LastCheckpoint = checkpoint.Timestamp
	}

	// Persist to storage if available
	if m.storage != nil {
		if err := m.storage.SaveCheckpoint(checkpoint); err != nil {
			return fmt.Errorf("failed to persist checkpoint: %w", err)
		}
	}

	return nil
}

// Close closes the replication manager
func (m *manager) Close() error {
	m.mu.Lock()

	// Stop all stream handlers
	for linkID, handler := range m.streamHandlers {
		_ = handler.Stop()
		delete(m.streamHandlers, linkID)
	}

	m.mu.Unlock()

	// Cancel context and wait for background goroutines
	m.cancel()
	m.wg.Wait()

	return nil
}

// loadLinksFromStorage loads existing links from persistent storage
func (m *manager) loadLinksFromStorage() {
	if m.storage == nil {
		return
	}

	links, err := m.storage.ListLinks()
	if err != nil {
		// Log error but don't fail
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, link := range links {
		m.links[link.ID] = link
		m.checkpoints[link.ID] = make(map[string]map[int32]*Checkpoint)
		m.offsetMappings[link.ID] = make(map[string]map[int32]*OffsetMapping)
	}
}

// healthMonitorLoop periodically checks health of all active replication links
func (m *manager) healthMonitorLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAllLinksHealth()
		}
	}
}

// checkAllLinksHealth checks health of all active links
func (m *manager) checkAllLinksHealth() {
	m.mu.RLock()
	linkIDs := make([]string, 0, len(m.links))
	for linkID, link := range m.links {
		if link.Status == ReplicationStatusActive {
			linkIDs = append(linkIDs, linkID)
		}
	}
	m.mu.RUnlock()

	// Check health for each active link
	for _, linkID := range linkIDs {
		m.checkLinkHealth(linkID)
	}
}

// checkLinkHealth checks health of a single link
func (m *manager) checkLinkHealth(linkID string) {
	m.mu.RLock()
	link, exists := m.links[linkID]
	handler, handlerExists := m.streamHandlers[linkID]
	m.mu.RUnlock()

	if !exists {
		return
	}

	// If handler exists, its health is already being monitored
	// Here we just sync the health back to the link
	if handlerExists && handler.health != nil {
		m.mu.Lock()
		link.Health = handler.health
		link.Metrics = handler.metrics
		m.mu.Unlock()

		// Persist updated metrics/health if storage is available
		if m.storage != nil {
			_ = m.storage.SaveLink(link)
		}

		// Check if automatic failover should be triggered
		if link.FailoverConfig != nil && link.FailoverConfig.Enabled {
			m.checkAutomaticFailover(linkID, link)
		}
	}
}

// checkAutomaticFailover checks if automatic failover should be triggered
func (m *manager) checkAutomaticFailover(linkID string, link *ReplicationLink) {
	if link.Metrics == nil || link.Health == nil || link.FailoverConfig == nil {
		return
	}

	shouldFailover := false
	reason := ""

	// Check replication lag threshold
	if link.Metrics.ReplicationLag > link.FailoverConfig.FailoverThreshold {
		shouldFailover = true
		reason = fmt.Sprintf("replication lag (%d ms) exceeds threshold (%d ms)",
			link.Metrics.ReplicationLag, link.FailoverConfig.FailoverThreshold)
	}

	// Check consecutive failures
	if link.Metrics.ConsecutiveFailures > link.FailoverConfig.MaxConsecutiveFailures {
		shouldFailover = true
		reason = fmt.Sprintf("consecutive failures (%d) exceed threshold (%d)",
			link.Metrics.ConsecutiveFailures, link.FailoverConfig.MaxConsecutiveFailures)
	}

	// Check source cluster reachability
	if !link.Health.SourceClusterReachable {
		shouldFailover = true
		reason = "source cluster is unreachable"
	}

	// Trigger failover if conditions are met
	if shouldFailover {
		m.mu.Unlock() // Unlock before calling Failover which acquires the lock
		event, err := m.Failover(linkID)
		m.mu.Lock() // Re-acquire lock

		if err != nil {
			// Log error but don't fail - health check continues
			if link.Health != nil {
				link.Health.Issues = append(link.Health.Issues,
					fmt.Sprintf("automatic failover failed: %v", err))
			}
		} else if event != nil && event.Success {
			// Failover succeeded
			if link.Health != nil {
				link.Health.Warnings = append(link.Health.Warnings,
					fmt.Sprintf("automatic failover triggered: %s", reason))
			}

			// If auto-failback is enabled, schedule it
			if link.FailoverConfig.AutoFailback {
				go m.scheduleAutoFailback(linkID, link.FailoverConfig.FailbackDelayMs)
			}
		}
	}
}

// scheduleAutoFailback schedules an automatic failback after a delay
func (m *manager) scheduleAutoFailback(linkID string, delayMs int64) {
	time.Sleep(time.Duration(delayMs) * time.Millisecond)

	// Check if primary cluster is healthy before failing back
	m.mu.RLock()
	link, exists := m.links[linkID]
	m.mu.RUnlock()

	if !exists {
		return
	}

	// Only failback if the current target (original source) is healthy
	// and the current source (original target) is the active cluster
	if link.Health != nil && link.Health.TargetClusterReachable {
		event, err := m.Failback(linkID)
		if err != nil || event == nil || !event.Success {
			// Log error - automatic failback failed
			return
		}

		// Restart replication after failback
		_ = m.StartLink(linkID)
	}
}
