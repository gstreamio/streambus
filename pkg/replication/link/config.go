package link

import (
	"fmt"
	"time"
)

// DefaultReplicationConfig returns default replication configuration
func DefaultReplicationConfig() ReplicationConfig {
	return ReplicationConfig{
		MaxBytes:                1024 * 1024 * 10, // 10 MB
		MaxMessages:             10000,
		FetchWaitMaxMs:          500,
		MinBytes:                1,
		BatchSize:               1000,
		BufferSize:              100000,
		ConcurrentPartitions:    10,
		EnableCompression:       true,
		CompressionType:         "snappy",
		ThrottleRateBytesPerSec: 0, // unlimited
		CheckpointIntervalMs:    5000,
		SyncIntervalMs:          30000,
		HeartbeatIntervalMs:     3000,
		EnableExactlyOnce:       false,
		EnableIdempotence:       true,
	}
}

// DefaultClusterConfig returns default cluster configuration
func DefaultClusterConfig() ClusterConfig {
	return ClusterConfig{
		ConnectionTimeout: 30 * time.Second,
		RequestTimeout:    10 * time.Second,
		RetryBackoff:      500 * time.Millisecond,
		MaxRetries:        3,
		Security:          nil, // no security by default
	}
}

// DefaultFailoverConfig returns default failover configuration
func DefaultFailoverConfig() *FailoverConfig {
	return &FailoverConfig{
		Enabled:                false,
		FailoverThreshold:      100000, // 100k messages lag
		FailoverTimeoutMs:      60000,  // 60 seconds
		MaxConsecutiveFailures: 3,
		AutoFailback:           false,
		FailbackDelayMs:        300000, // 5 minutes
	}
}

// Validate validates the replication link configuration
func (rl *ReplicationLink) Validate() error {
	if rl.ID == "" {
		return fmt.Errorf("replication link ID cannot be empty")
	}

	if rl.Name == "" {
		return fmt.Errorf("replication link name cannot be empty")
	}

	if rl.Type != ReplicationTypeActivePassive &&
		rl.Type != ReplicationTypeActiveActive &&
		rl.Type != ReplicationTypeStar {
		return fmt.Errorf("invalid replication type: %s", rl.Type)
	}

	if err := rl.SourceCluster.Validate(); err != nil {
		return fmt.Errorf("invalid source cluster config: %w", err)
	}

	if err := rl.TargetCluster.Validate(); err != nil {
		return fmt.Errorf("invalid target cluster config: %w", err)
	}

	if rl.SourceCluster.ClusterID == rl.TargetCluster.ClusterID {
		return fmt.Errorf("source and target cluster IDs must be different")
	}

	// Validate topic configurations
	for topicName, topicConfig := range rl.TopicConfig {
		if topicConfig.SourceTopic == "" {
			topicConfig.SourceTopic = topicName
		}
		if topicConfig.TargetTopic == "" {
			topicConfig.TargetTopic = topicName
		}
	}

	// Validate replication config
	if err := rl.Config.Validate(); err != nil {
		return fmt.Errorf("invalid replication config: %w", err)
	}

	return nil
}

// Validate validates the cluster configuration
func (cc *ClusterConfig) Validate() error {
	if cc.ClusterID == "" {
		return fmt.Errorf("cluster ID cannot be empty")
	}

	if len(cc.Brokers) == 0 && cc.BootstrapServers == "" {
		return fmt.Errorf("brokers or bootstrap servers must be specified")
	}

	if cc.ConnectionTimeout <= 0 {
		return fmt.Errorf("connection timeout must be positive")
	}

	if cc.RequestTimeout <= 0 {
		return fmt.Errorf("request timeout must be positive")
	}

	if cc.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}

	// Validate security config if present
	if cc.Security != nil {
		if err := cc.Security.Validate(); err != nil {
			return fmt.Errorf("invalid security config: %w", err)
		}
	}

	return nil
}

// Validate validates the security configuration
func (sc *SecurityConfig) Validate() error {
	if sc.EnableTLS {
		if sc.TLSCertFile == "" {
			return fmt.Errorf("TLS cert file must be specified when TLS is enabled")
		}
		if sc.TLSKeyFile == "" {
			return fmt.Errorf("TLS key file must be specified when TLS is enabled")
		}
	}

	if sc.SASLMechanism != "" {
		if sc.SASLMechanism != "PLAIN" &&
			sc.SASLMechanism != "SCRAM-SHA-256" &&
			sc.SASLMechanism != "SCRAM-SHA-512" {
			return fmt.Errorf("unsupported SASL mechanism: %s", sc.SASLMechanism)
		}
		if sc.SASLUsername == "" {
			return fmt.Errorf("SASL username must be specified")
		}
		if sc.SASLPassword == "" {
			return fmt.Errorf("SASL password must be specified")
		}
	}

	return nil
}

// Validate validates the replication configuration
func (rc *ReplicationConfig) Validate() error {
	if rc.MaxBytes <= 0 {
		return fmt.Errorf("max bytes must be positive")
	}

	if rc.MaxMessages <= 0 {
		return fmt.Errorf("max messages must be positive")
	}

	if rc.BatchSize <= 0 {
		return fmt.Errorf("batch size must be positive")
	}

	if rc.BufferSize <= 0 {
		return fmt.Errorf("buffer size must be positive")
	}

	if rc.ConcurrentPartitions <= 0 {
		return fmt.Errorf("concurrent partitions must be positive")
	}

	if rc.CompressionType != "" &&
		rc.CompressionType != "none" &&
		rc.CompressionType != "gzip" &&
		rc.CompressionType != "snappy" &&
		rc.CompressionType != "lz4" &&
		rc.CompressionType != "zstd" {
		return fmt.Errorf("unsupported compression type: %s", rc.CompressionType)
	}

	if rc.ThrottleRateBytesPerSec < 0 {
		return fmt.Errorf("throttle rate cannot be negative")
	}

	if rc.CheckpointIntervalMs <= 0 {
		return fmt.Errorf("checkpoint interval must be positive")
	}

	if rc.SyncIntervalMs <= 0 {
		return fmt.Errorf("sync interval must be positive")
	}

	if rc.HeartbeatIntervalMs <= 0 {
		return fmt.Errorf("heartbeat interval must be positive")
	}

	return nil
}

// Clone creates a deep copy of the replication link
func (rl *ReplicationLink) Clone() *ReplicationLink {
	clone := &ReplicationLink{
		ID:            rl.ID,
		Name:          rl.Name,
		Type:          rl.Type,
		SourceCluster: rl.SourceCluster.Clone(),
		TargetCluster: rl.TargetCluster.Clone(),
		Topics:        make([]string, len(rl.Topics)),
		TopicPrefix:   rl.TopicPrefix,
		TopicConfig:   make(map[string]*TopicReplicationConfig),
		Config:        rl.Config,
		Status:        rl.Status,
		CreatedAt:     rl.CreatedAt,
		UpdatedAt:     rl.UpdatedAt,
		StartedAt:     rl.StartedAt,
		StoppedAt:     rl.StoppedAt,
	}

	copy(clone.Topics, rl.Topics)

	for k, v := range rl.TopicConfig {
		clone.TopicConfig[k] = &TopicReplicationConfig{
			SourceTopic:            v.SourceTopic,
			TargetTopic:            v.TargetTopic,
			Enabled:                v.Enabled,
			ReplicateDeletes:       v.ReplicateDeletes,
			PreservePartitionCount: v.PreservePartitionCount,
			PreserveTimestamps:     v.PreserveTimestamps,
			CompressionType:        v.CompressionType,
			Priority:               v.Priority,
		}
	}

	if rl.Filter != nil {
		clone.Filter = &FilterConfig{
			Enabled:         rl.Filter.Enabled,
			IncludePatterns: append([]string{}, rl.Filter.IncludePatterns...),
			ExcludePatterns: append([]string{}, rl.Filter.ExcludePatterns...),
			FilterByHeader:  make(map[string]string),
			MinTimestamp:    rl.Filter.MinTimestamp,
			MaxTimestamp:    rl.Filter.MaxTimestamp,
		}
		for k, v := range rl.Filter.FilterByHeader {
			clone.Filter.FilterByHeader[k] = v
		}
	}

	if rl.Transform != nil {
		clone.Transform = &TransformConfig{
			Enabled:          rl.Transform.Enabled,
			HeaderTransforms: make(map[string]string),
			KeyTransform:     rl.Transform.KeyTransform,
			ValueTransform:   rl.Transform.ValueTransform,
			AddHeaders:       make(map[string]string),
			RemoveHeaders:    append([]string{}, rl.Transform.RemoveHeaders...),
		}
		for k, v := range rl.Transform.HeaderTransforms {
			clone.Transform.HeaderTransforms[k] = v
		}
		for k, v := range rl.Transform.AddHeaders {
			clone.Transform.AddHeaders[k] = v
		}
	}

	if rl.Metrics != nil {
		clone.Metrics = &ReplicationMetrics{
			TotalMessagesReplicated:   rl.Metrics.TotalMessagesReplicated,
			TotalBytesReplicated:      rl.Metrics.TotalBytesReplicated,
			MessagesPerSecond:         rl.Metrics.MessagesPerSecond,
			BytesPerSecond:            rl.Metrics.BytesPerSecond,
			ReplicationLag:            rl.Metrics.ReplicationLag,
			AverageReplicationLag:     rl.Metrics.AverageReplicationLag,
			MaxReplicationLag:         rl.Metrics.MaxReplicationLag,
			TotalErrors:               rl.Metrics.TotalErrors,
			ErrorsPerSecond:           rl.Metrics.ErrorsPerSecond,
			LastCheckpoint:            rl.Metrics.LastCheckpoint,
			LastSuccessfulReplication: rl.Metrics.LastSuccessfulReplication,
			PartitionMetrics:          make(map[string]*PartitionReplicationMetrics),
			ConsecutiveFailures:       rl.Metrics.ConsecutiveFailures,
			UptimeSeconds:             rl.Metrics.UptimeSeconds,
		}
		for k, v := range rl.Metrics.PartitionMetrics {
			clone.Metrics.PartitionMetrics[k] = &PartitionReplicationMetrics{
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
	}

	if rl.Health != nil {
		clone.Health = &ReplicationHealth{
			Status:                 rl.Health.Status,
			LastHealthCheck:        rl.Health.LastHealthCheck,
			SourceClusterReachable: rl.Health.SourceClusterReachable,
			TargetClusterReachable: rl.Health.TargetClusterReachable,
			ReplicationLagHealthy:  rl.Health.ReplicationLagHealthy,
			ErrorRateHealthy:       rl.Health.ErrorRateHealthy,
			CheckpointHealthy:      rl.Health.CheckpointHealthy,
			Issues:                 append([]string{}, rl.Health.Issues...),
			Warnings:               append([]string{}, rl.Health.Warnings...),
		}
	}

	if rl.FailoverConfig != nil {
		clone.FailoverConfig = &FailoverConfig{
			Enabled:                rl.FailoverConfig.Enabled,
			FailoverThreshold:      rl.FailoverConfig.FailoverThreshold,
			FailoverTimeoutMs:      rl.FailoverConfig.FailoverTimeoutMs,
			MaxConsecutiveFailures: rl.FailoverConfig.MaxConsecutiveFailures,
			AutoFailback:           rl.FailoverConfig.AutoFailback,
			FailbackDelayMs:        rl.FailoverConfig.FailbackDelayMs,
			NotificationWebhook:    rl.FailoverConfig.NotificationWebhook,
			NotificationEmail:      rl.FailoverConfig.NotificationEmail,
		}
	}

	return clone
}

// Clone creates a deep copy of the cluster configuration
func (cc *ClusterConfig) Clone() ClusterConfig {
	clone := ClusterConfig{
		ClusterID:         cc.ClusterID,
		Brokers:           append([]string{}, cc.Brokers...),
		BootstrapServers:  cc.BootstrapServers,
		ConnectionTimeout: cc.ConnectionTimeout,
		RequestTimeout:    cc.RequestTimeout,
		RetryBackoff:      cc.RetryBackoff,
		MaxRetries:        cc.MaxRetries,
	}

	if cc.Security != nil {
		clone.Security = &SecurityConfig{
			EnableTLS:      cc.Security.EnableTLS,
			TLSCertFile:    cc.Security.TLSCertFile,
			TLSKeyFile:     cc.Security.TLSKeyFile,
			TLSCAFile:      cc.Security.TLSCAFile,
			TLSSkipVerify:  cc.Security.TLSSkipVerify,
			SASLMechanism:  cc.Security.SASLMechanism,
			SASLUsername:   cc.Security.SASLUsername,
			SASLPassword:   cc.Security.SASLPassword,
		}
	}

	return clone
}
