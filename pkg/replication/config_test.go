package replication

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Verify reasonable defaults
	if cfg.FetchMinBytes != 1 {
		t.Errorf("Expected FetchMinBytes=1, got %d", cfg.FetchMinBytes)
	}
	if cfg.FetchMaxBytes != 1024*1024 {
		t.Errorf("Expected FetchMaxBytes=1MB, got %d", cfg.FetchMaxBytes)
	}
	if cfg.FetchMaxWaitMs != 500 {
		t.Errorf("Expected FetchMaxWaitMs=500, got %d", cfg.FetchMaxWaitMs)
	}
	if cfg.FetcherInterval != 50*time.Millisecond {
		t.Errorf("Expected FetcherInterval=50ms, got %v", cfg.FetcherInterval)
	}
	if cfg.ReplicaLagMaxMessages != 4000 {
		t.Errorf("Expected ReplicaLagMaxMessages=4000, got %d", cfg.ReplicaLagMaxMessages)
	}
	if cfg.ReplicaLagTimeMaxMs != 10000 {
		t.Errorf("Expected ReplicaLagTimeMaxMs=10000, got %d", cfg.ReplicaLagTimeMaxMs)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "negative FetchMinBytes",
			modify: func(c *Config) {
				c.FetchMinBytes = -1
			},
			wantErr: true,
		},
		{
			name: "zero FetchMaxBytes",
			modify: func(c *Config) {
				c.FetchMaxBytes = 0
			},
			wantErr: true,
		},
		{
			name: "FetchMaxBytes < FetchMinBytes",
			modify: func(c *Config) {
				c.FetchMinBytes = 1000
				c.FetchMaxBytes = 500
			},
			wantErr: true,
		},
		{
			name: "negative FetchMaxWaitMs",
			modify: func(c *Config) {
				c.FetchMaxWaitMs = -1
			},
			wantErr: true,
		},
		{
			name: "zero FetcherInterval",
			modify: func(c *Config) {
				c.FetcherInterval = 0
			},
			wantErr: true,
		},
		{
			name: "zero ReplicaLagMaxMessages",
			modify: func(c *Config) {
				c.ReplicaLagMaxMessages = 0
			},
			wantErr: true,
		},
		{
			name: "zero ReplicaLagTimeMaxMs",
			modify: func(c *Config) {
				c.ReplicaLagTimeMaxMs = 0
			},
			wantErr: true,
		},
		{
			name: "zero NumFetcherThreads",
			modify: func(c *Config) {
				c.NumFetcherThreads = 0
			},
			wantErr: true,
		},
		{
			name: "zero MaxInflightFetches",
			modify: func(c *Config) {
				c.MaxInflightFetches = 0
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigClone(t *testing.T) {
	original := DefaultConfig()
	original.FetchMaxBytes = 2048
	original.ReplicaIDForFollower = 42

	clone := original.Clone()

	if clone == nil {
		t.Fatal("Clone returned nil")
	}

	// Verify values are equal
	if clone.FetchMaxBytes != original.FetchMaxBytes {
		t.Errorf("FetchMaxBytes not cloned correctly")
	}
	if clone.ReplicaIDForFollower != original.ReplicaIDForFollower {
		t.Errorf("ReplicaIDForFollower not cloned correctly")
	}

	// Verify it's a different instance
	clone.FetchMaxBytes = 4096
	if original.FetchMaxBytes == clone.FetchMaxBytes {
		t.Error("Clone is not independent")
	}
}

func TestConfigCloneNil(t *testing.T) {
	var cfg *Config
	clone := cfg.Clone()

	if clone != nil {
		t.Error("Clone of nil should return nil")
	}
}
