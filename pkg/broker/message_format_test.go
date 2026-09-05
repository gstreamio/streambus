package broker

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/storage"
)

// recordMagicAndVersion mirrors the leading bytes of every v2/v3 record
// (see the format documentation in pkg/storage/log.go): a 4-byte magic
// prefix (0xFFFFFFFF) immediately followed by a 1-byte format version. It is
// redefined here, rather than imported, because the constants themselves are
// unexported package-storage internals - this end-to-end test deliberately
// works only with what actually reaches disk, the same way an operator
// inspecting a WAL file with no access to StreamBus internals would.
func recordMagicAndVersion(version byte) []byte {
	return []byte{0xFF, 0xFF, 0xFF, 0xFF, version}
}

// findWALSegmentBytes locates the single WAL segment file under dataDir for
// topic/partition 0 and returns its raw contents. There is exactly one
// segment because these tests write far less than WALConfig's segment size.
func findWALSegmentBytes(t *testing.T, dataDir, topic string) []byte {
	t.Helper()

	walDir := filepath.Join(dataDir, "topics", topic, "partition-0", "wal")
	matches, err := filepath.Glob(filepath.Join(walDir, "*.wal"))
	if err != nil {
		t.Fatalf("globbing WAL segments in %s: %v", walDir, err)
	}
	if len(matches) != 1 {
		t.Fatalf("found %d WAL segment files in %s, want 1: %v", len(matches), walDir, matches)
	}

	data, err := os.ReadFile(matches[0]) // #nosec G304 -- test-owned temp dir
	if err != nil {
		t.Fatalf("reading WAL segment %s: %v", matches[0], err)
	}
	return data
}

// TestBroker_InitStorage_MessageFormatVersionReachesDisk is the end-to-end
// proof that storage.message_format_version actually traverses every hop it
// is supposed to: cmd/broker's resolved value -> broker.Config.
// MessageFormatVersion -> initStorage -> server.
// NewTopicManagerWithMessageFormatVersion -> the storage.Config built for
// every partition -> logImpl.serializeMessage -> the bytes written to the
// WAL. A unit test at either end cannot catch a break introduced at any of
// the hops in between; this test appends a real record through the real
// broker code path and inspects the actual bytes that landed on disk.
//
// It is deliberately not a full broker.New/Start: that requires a cluster
// (Raft peers, ports) this setting has nothing to do with. initStorage is
// the exact unit that turns Config.MessageFormatVersion into a TopicManager,
// so calling it directly on a minimally-constructed Broker exercises the
// real production wiring without dragging in consensus.
func TestBroker_InitStorage_MessageFormatVersionReachesDisk(t *testing.T) {
	tests := []struct {
		name          string
		version       storage.MessageFormatVersion
		wantMagic     byte
		dontWantMagic byte
	}{
		{
			name:          "v2 configured",
			version:       storage.MessageFormatV2,
			wantMagic:     2,
			dontWantMagic: 3,
		},
		{
			name:          "v3 configured explicitly",
			version:       storage.MessageFormatV3,
			wantMagic:     3,
			dontWantMagic: 2,
		},
		{
			name:          "unset resolves to the v3 default",
			version:       storage.MessageFormatUnset,
			wantMagic:     3,
			dontWantMagic: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataDir := t.TempDir()
			const topic = "format-gate-topic"

			b := &Broker{
				logger: newTestLogger(),
				config: &Config{
					BrokerID:             1,
					DataDir:              dataDir,
					MessageFormatVersion: tt.version,
				},
				status: StatusRunning,
			}

			if err := b.initStorage(); err != nil {
				t.Fatalf("initStorage failed: %v", err)
			}

			if err := b.topicManager.CreateTopic(topic, 1); err != nil {
				t.Fatalf("CreateTopic failed: %v", err)
			}

			partition, err := b.topicManager.GetPartition(topic, 0)
			if err != nil {
				t.Fatalf("GetPartition failed: %v", err)
			}

			// A non-transactional batch: the format gate under test only
			// governs the write format, not the separate transactional-vs-v2
			// refusal (covered in pkg/storage), so ProducerID is left at its
			// zero sentinel here.
			if _, err := partition.Log().Append(&storage.MessageBatch{
				Messages:  []storage.Message{{Value: []byte("payload"), Timestamp: time.Now()}},
				Timestamp: time.Now(),
			}); err != nil {
				t.Fatalf("Append failed: %v", err)
			}

			// Close before inspecting the file: the WAL segment writer is
			// buffered, and Close is what flushes it.
			if err := b.topicManager.Close(); err != nil {
				t.Fatalf("Close failed: %v", err)
			}

			raw := findWALSegmentBytes(t, dataDir, topic)

			if !bytes.Contains(raw, recordMagicAndVersion(tt.wantMagic)) {
				t.Errorf("WAL segment does not contain a v%d record magic; MessageFormatVersion=%v did not reach disk", tt.wantMagic, tt.version)
			}
			if bytes.Contains(raw, recordMagicAndVersion(tt.dontWantMagic)) {
				t.Errorf("WAL segment unexpectedly contains a v%d record magic for MessageFormatVersion=%v", tt.dontWantMagic, tt.version)
			}
		})
	}
}
