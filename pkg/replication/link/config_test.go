package link

import (
	"testing"
	"time"
)

func TestDefaultReplicationConfig(t *testing.T) {
	config := DefaultReplicationConfig()

	if config.MaxBytes != 1024*1024*10 {
		t.Errorf("MaxBytes = %d, want %d", config.MaxBytes, 1024*1024*10)
	}

	if config.MaxMessages != 10000 {
		t.Errorf("MaxMessages = %d, want 10000", config.MaxMessages)
	}

	if config.FetchWaitMaxMs != 500 {
		t.Errorf("FetchWaitMaxMs = %d, want 500", config.FetchWaitMaxMs)
	}

	if !config.EnableCompression {
		t.Error("EnableCompression should be true by default")
	}

	if config.CompressionType != "snappy" {
		t.Errorf("CompressionType = %s, want snappy", config.CompressionType)
	}
}

func TestDefaultClusterConfig(t *testing.T) {
	config := DefaultClusterConfig()

	if config.ConnectionTimeout != 30*time.Second {
		t.Errorf("ConnectionTimeout = %v, want 30s", config.ConnectionTimeout)
	}

	if config.RequestTimeout != 10*time.Second {
		t.Errorf("RequestTimeout = %v, want 10s", config.RequestTimeout)
	}

	if config.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", config.MaxRetries)
	}

	if config.Security != nil {
		t.Error("Security should be nil by default")
	}
}

func TestDefaultFailoverConfig(t *testing.T) {
	config := DefaultFailoverConfig()

	if config.Enabled {
		t.Error("Failover should be disabled by default")
	}

	if config.FailoverThreshold != 100000 {
		t.Errorf("FailoverThreshold = %d, want 100000", config.FailoverThreshold)
	}

	if config.AutoFailback {
		t.Error("AutoFailback should be false by default")
	}
}

func TestReplicationLink_Validate(t *testing.T) {
	tests := []struct {
		name    string
		link    *ReplicationLink
		wantErr bool
	}{
		{
			name: "valid link",
			link: &ReplicationLink{
				ID:   "link-1",
				Name: "test-link",
				Type: ReplicationTypeActivePassive,
				SourceCluster: ClusterConfig{
					ClusterID:         "source",
					BootstrapServers:  "localhost:9092",
					ConnectionTimeout: 10 * time.Second,
					RequestTimeout:    5 * time.Second,
					MaxRetries:        3,
				},
				TargetCluster: ClusterConfig{
					ClusterID:         "target",
					BootstrapServers:  "localhost:9093",
					ConnectionTimeout: 10 * time.Second,
					RequestTimeout:    5 * time.Second,
					MaxRetries:        3,
				},
				Config: ReplicationConfig{
					MaxBytes:             1024,
					MaxMessages:          100,
					BatchSize:            10,
					BufferSize:           100,
					ConcurrentPartitions: 1,
					CheckpointIntervalMs: 1000,
					SyncIntervalMs:       1000,
					HeartbeatIntervalMs:  1000,
				},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			link: &ReplicationLink{
				Name: "test-link",
				Type: ReplicationTypeActivePassive,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			link: &ReplicationLink{
				ID:   "link-1",
				Type: ReplicationTypeActivePassive,
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			link: &ReplicationLink{
				ID:   "link-1",
				Name: "test-link",
				Type: "invalid",
			},
			wantErr: true,
		},
		{
			name: "same cluster IDs",
			link: &ReplicationLink{
				ID:   "link-1",
				Name: "test-link",
				Type: ReplicationTypeActivePassive,
				SourceCluster: ClusterConfig{
					ClusterID:         "same",
					BootstrapServers:  "localhost:9092",
					ConnectionTimeout: 10 * time.Second,
					RequestTimeout:    5 * time.Second,
					MaxRetries:        3,
				},
				TargetCluster: ClusterConfig{
					ClusterID:         "same",
					BootstrapServers:  "localhost:9093",
					ConnectionTimeout: 10 * time.Second,
					RequestTimeout:    5 * time.Second,
					MaxRetries:        3,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.link.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClusterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *ClusterConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &ClusterConfig{
				ClusterID:         "cluster-1",
				BootstrapServers:  "localhost:9092",
				ConnectionTimeout: 10 * time.Second,
				RequestTimeout:    5 * time.Second,
				MaxRetries:        3,
			},
			wantErr: false,
		},
		{
			name: "missing cluster ID",
			config: &ClusterConfig{
				BootstrapServers:  "localhost:9092",
				ConnectionTimeout: 10 * time.Second,
				RequestTimeout:    5 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "missing brokers and bootstrap servers",
			config: &ClusterConfig{
				ClusterID:         "cluster-1",
				ConnectionTimeout: 10 * time.Second,
				RequestTimeout:    5 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative connection timeout",
			config: &ClusterConfig{
				ClusterID:         "cluster-1",
				BootstrapServers:  "localhost:9092",
				ConnectionTimeout: -1 * time.Second,
				RequestTimeout:    5 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative request timeout",
			config: &ClusterConfig{
				ClusterID:         "cluster-1",
				BootstrapServers:  "localhost:9092",
				ConnectionTimeout: 10 * time.Second,
				RequestTimeout:    -1 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative max retries",
			config: &ClusterConfig{
				ClusterID:         "cluster-1",
				BootstrapServers:  "localhost:9092",
				ConnectionTimeout: 10 * time.Second,
				RequestTimeout:    5 * time.Second,
				MaxRetries:        -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *SecurityConfig
		wantErr bool
	}{
		{
			name: "valid TLS config",
			config: &SecurityConfig{
				EnableTLS:   true,
				TLSCertFile: "/path/to/cert",
				TLSKeyFile:  "/path/to/key",
			},
			wantErr: false,
		},
		{
			name: "TLS enabled without cert",
			config: &SecurityConfig{
				EnableTLS:  true,
				TLSKeyFile: "/path/to/key",
			},
			wantErr: true,
		},
		{
			name: "TLS enabled without key",
			config: &SecurityConfig{
				EnableTLS:   true,
				TLSCertFile: "/path/to/cert",
			},
			wantErr: true,
		},
		{
			name: "valid SASL config",
			config: &SecurityConfig{
				SASLMechanism: "PLAIN",
				SASLUsername:  "user",
				SASLPassword:  "pass",
			},
			wantErr: false,
		},
		{
			name: "unsupported SASL mechanism",
			config: &SecurityConfig{
				SASLMechanism: "INVALID",
				SASLUsername:  "user",
				SASLPassword:  "pass",
			},
			wantErr: true,
		},
		{
			name: "SASL without username",
			config: &SecurityConfig{
				SASLMechanism: "PLAIN",
				SASLPassword:  "pass",
			},
			wantErr: true,
		},
		{
			name: "SASL without password",
			config: &SecurityConfig{
				SASLMechanism: "PLAIN",
				SASLUsername:  "user",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReplicationConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *ReplicationConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &ReplicationConfig{
				MaxBytes:             1024,
				MaxMessages:          100,
				BatchSize:            10,
				BufferSize:           100,
				ConcurrentPartitions: 1,
				CheckpointIntervalMs: 1000,
				SyncIntervalMs:       1000,
				HeartbeatIntervalMs:  1000,
			},
			wantErr: false,
		},
		{
			name: "negative max bytes",
			config: &ReplicationConfig{
				MaxBytes: -1,
			},
			wantErr: true,
		},
		{
			name: "negative max messages",
			config: &ReplicationConfig{
				MaxBytes:    1024,
				MaxMessages: -1,
			},
			wantErr: true,
		},
		{
			name: "negative batch size",
			config: &ReplicationConfig{
				MaxBytes:    1024,
				MaxMessages: 100,
				BatchSize:   -1,
			},
			wantErr: true,
		},
		{
			name: "unsupported compression type",
			config: &ReplicationConfig{
				MaxBytes:             1024,
				MaxMessages:          100,
				BatchSize:            10,
				BufferSize:           100,
				ConcurrentPartitions: 1,
				CompressionType:      "invalid",
				CheckpointIntervalMs: 1000,
				SyncIntervalMs:       1000,
				HeartbeatIntervalMs:  1000,
			},
			wantErr: true,
		},
		{
			name: "negative throttle rate",
			config: &ReplicationConfig{
				MaxBytes:                1024,
				MaxMessages:             100,
				BatchSize:               10,
				BufferSize:              100,
				ConcurrentPartitions:    1,
				ThrottleRateBytesPerSec: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClusterConfig_Clone(t *testing.T) {
	original := ClusterConfig{
		ClusterID:         "cluster-1",
		Brokers:           []string{"broker1", "broker2"},
		BootstrapServers:  "localhost:9092",
		ConnectionTimeout: 10 * time.Second,
		RequestTimeout:    5 * time.Second,
		MaxRetries:        3,
		Security: &SecurityConfig{
			EnableTLS:   true,
			TLSCertFile: "/path/to/cert",
			TLSKeyFile:  "/path/to/key",
		},
	}

	clone := original.Clone()

	// Verify fields are equal
	if clone.ClusterID != original.ClusterID {
		t.Error("ClusterID not cloned correctly")
	}

	// Verify deep copy of slices
	if len(clone.Brokers) != len(original.Brokers) {
		t.Error("Brokers not cloned correctly")
	}

	// Modify clone to ensure it's independent
	clone.Brokers[0] = "modified"
	if original.Brokers[0] == "modified" {
		t.Error("Clone is not independent - modifying clone affected original")
	}

	// Verify deep copy of security config
	if clone.Security == nil {
		t.Error("Security config not cloned")
	}
	if clone.Security.TLSCertFile != original.Security.TLSCertFile {
		t.Error("Security config not cloned correctly")
	}
}
