package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gstreamio/streambus/pkg/logging"
	"github.com/gstreamio/streambus/pkg/replication/link"
)

// testReplicationLink builds a minimal link.ReplicationLink that passes
// Validate(), so CreateLink actually stores it instead of rejecting it.
func testReplicationLink(name string) *link.ReplicationLink {
	return &link.ReplicationLink{
		Name: name,
		Type: link.ReplicationTypeActivePassive,
		SourceCluster: link.ClusterConfig{
			ClusterID:         "source",
			Brokers:           []string{"localhost:9092"},
			ConnectionTimeout: link.DefaultClusterConfig().ConnectionTimeout,
			RequestTimeout:    link.DefaultClusterConfig().RequestTimeout,
		},
		TargetCluster: link.ClusterConfig{
			ClusterID:         "target",
			Brokers:           []string{"localhost:9093"},
			ConnectionTimeout: link.DefaultClusterConfig().ConnectionTimeout,
			RequestTimeout:    link.DefaultClusterConfig().RequestTimeout,
		},
		Config:         link.DefaultReplicationConfig(),
		FailoverConfig: link.DefaultFailoverConfig(),
	}
}

func testLogger() *logging.Logger {
	return logging.New(&logging.Config{Level: logging.LevelError, Component: "mirror-maker-test"})
}

// TestNewMirrorMaker_StorageBackend is the regression test for the bug where
// a configured StoragePath was silently ignored in favor of memory storage:
// links vanished on restart even though the operator asked for persistence.
func TestNewMirrorMaker_StorageBackend(t *testing.T) {
	tests := []struct {
		name        string
		useDiskPath bool
		wantPersist bool
	}{
		{
			name:        "empty storage path falls back to memory storage and does not persist",
			useDiskPath: false,
			wantPersist: false,
		},
		{
			name:        "configured storage path persists links across restarts",
			useDiskPath: true,
			wantPersist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{}
			if tt.useDiskPath {
				config.StoragePath = t.TempDir()
			}
			logger := testLogger()

			mm1, err := newMirrorMaker(config, logger)
			if err != nil {
				t.Fatalf("newMirrorMaker: %v", err)
			}
			if err := mm1.manager.CreateLink(testReplicationLink("link-1")); err != nil {
				t.Fatalf("CreateLink: %v", err)
			}

			// A second instance built from the same config simulates a
			// restart: file storage should see the link mm1 wrote, memory
			// storage should start empty.
			mm2, err := newMirrorMaker(config, logger)
			if err != nil {
				t.Fatalf("newMirrorMaker (second instance): %v", err)
			}
			links, err := mm2.manager.ListLinks()
			if err != nil {
				t.Fatalf("ListLinks: %v", err)
			}

			gotPersisted := len(links) == 1
			if gotPersisted != tt.wantPersist {
				t.Errorf("link persisted across instances = %v, want %v (links: %v)", gotPersisted, tt.wantPersist, links)
			}
		})
	}
}

// TestNewMirrorMaker_InvalidStoragePath ensures a storage path that cannot
// be created (e.g. a parent that's actually a file) surfaces as an error
// instead of silently falling back to memory storage.
func TestNewMirrorMaker_InvalidStoragePath(t *testing.T) {
	dir := t.TempDir()
	blockingFile := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Using a file as though it were a directory forces MkdirAll to fail.
	config := &Config{StoragePath: filepath.Join(blockingFile, "links")}

	_, err := newMirrorMaker(config, testLogger())
	if err == nil {
		t.Fatal("expected an error when the storage path cannot be created, got nil")
	}
}
