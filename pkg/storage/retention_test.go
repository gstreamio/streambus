package storage

import (
	"context"
	"testing"
	"time"
)

// testConfig returns a minimal storage config for tests.
func testConfig() Config {
	return Config{
		WAL: WALConfig{
			SegmentSize:   1024 * 1024,
			FsyncPolicy:   FsyncNever,
			FsyncInterval: time.Second,
		},
		MemTable: MemTableConfig{
			MaxSize:      1024 * 1024,
			NumImmutable: 2,
		},
	}
}

// appendMessages is a test helper that appends messages with specific timestamps.
func appendMessages(t *testing.T, log Log, timestamps []time.Time) {
	t.Helper()
	for _, ts := range timestamps {
		batch := &MessageBatch{
			Messages: []Message{
				{
					Key:       []byte("key"),
					Value:     []byte("value-data-padding-for-size"),
					Timestamp: ts,
				},
			},
		}
		if _, err := log.Append(batch); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}
}

func TestRetentionManager_TimeBasedRetention(t *testing.T) {
	tests := []struct {
		name             string
		retentionMs      int64
		messageAges      []time.Duration // age relative to now (negative = in the past)
		wantStartOffset  Offset
		wantDeleted      bool
		wantStatsDeleted int64
	}{
		{
			name:        "deletes messages older than retention",
			retentionMs: 1000, // 1 second
			messageAges: []time.Duration{
				-5 * time.Second,
				-4 * time.Second,
				-3 * time.Second,
				-500 * time.Millisecond,
				-100 * time.Millisecond,
			},
			wantStartOffset:  3, // first 3 messages are > 1s old
			wantDeleted:      true,
			wantStatsDeleted: 3,
		},
		{
			name:        "no deletion when all messages within retention",
			retentionMs: 60000, // 60 seconds
			messageAges: []time.Duration{
				-1 * time.Second,
				-500 * time.Millisecond,
			},
			wantStartOffset:  0,
			wantDeleted:      false,
			wantStatsDeleted: 0,
		},
		{
			name:        "deletes all messages when all expired",
			retentionMs: 100, // 100ms
			messageAges: []time.Duration{
				-5 * time.Second,
				-4 * time.Second,
				-3 * time.Second,
			},
			wantStartOffset:  3,
			wantDeleted:      true,
			wantStatsDeleted: 3,
		},
		{
			name:        "disabled when retentionMs is negative",
			retentionMs: -1,
			messageAges: []time.Duration{
				-5 * time.Second,
			},
			wantStartOffset:  0,
			wantDeleted:      false,
			wantStatsDeleted: 0,
		},
		{
			name:        "disabled when retentionMs is zero",
			retentionMs: 0,
			messageAges: []time.Duration{
				-5 * time.Second,
			},
			wantStartOffset:  0,
			wantDeleted:      false,
			wantStatsDeleted: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			log, err := NewLog(dir, testConfig())
			if err != nil {
				t.Fatalf("NewLog failed: %v", err)
			}
			defer log.Close()

			now := time.Now()
			timestamps := make([]time.Time, len(tt.messageAges))
			for i, age := range tt.messageAges {
				timestamps[i] = now.Add(age)
			}
			appendMessages(t, log, timestamps)

			rm := NewRetentionManager(RetentionConfig{
				CheckInterval:  time.Minute,
				RetentionMs:    tt.retentionMs,
				RetentionBytes: -1,
			})
			rm.RegisterLog("test:0", log)

			stats := rm.EnforceNow()

			if log.StartOffset() != tt.wantStartOffset {
				t.Errorf("StartOffset = %d, want %d", log.StartOffset(), tt.wantStartOffset)
			}
			if tt.wantDeleted && stats.SegmentsDeleted == 0 {
				t.Error("expected segments to be deleted, but none were")
			}
			if !tt.wantDeleted && stats.SegmentsDeleted != 0 {
				t.Errorf("expected no deletions, got %d", stats.SegmentsDeleted)
			}
			if stats.SegmentsDeleted != tt.wantStatsDeleted {
				t.Errorf("SegmentsDeleted = %d, want %d", stats.SegmentsDeleted, tt.wantStatsDeleted)
			}
		})
	}
}

func TestRetentionManager_SizeBasedRetention(t *testing.T) {
	tests := []struct {
		name            string
		retentionBytes  int64
		numMessages     int
		wantSomeDeleted bool
	}{
		{
			name:            "deletes oldest when size exceeded",
			retentionBytes:  100, // very small
			numMessages:     20,
			wantSomeDeleted: true,
		},
		{
			name:            "no deletion when within size limit",
			retentionBytes:  1024 * 1024, // 1MB
			numMessages:     5,
			wantSomeDeleted: false,
		},
		{
			name:            "disabled when retentionBytes is negative",
			retentionBytes:  -1,
			numMessages:     20,
			wantSomeDeleted: false,
		},
		{
			name:            "disabled when retentionBytes is zero",
			retentionBytes:  0,
			numMessages:     20,
			wantSomeDeleted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			log, err := NewLog(dir, testConfig())
			if err != nil {
				t.Fatalf("NewLog failed: %v", err)
			}
			defer log.Close()

			now := time.Now()
			timestamps := make([]time.Time, tt.numMessages)
			for i := range timestamps {
				timestamps[i] = now.Add(time.Duration(i) * time.Second)
			}
			appendMessages(t, log, timestamps)

			rm := NewRetentionManager(RetentionConfig{
				CheckInterval:  time.Minute,
				RetentionMs:    -1,
				RetentionBytes: tt.retentionBytes,
			})
			rm.RegisterLog("test:0", log)

			stats := rm.EnforceNow()

			if tt.wantSomeDeleted && stats.SegmentsDeleted == 0 {
				t.Error("expected some segments deleted, got 0")
			}
			if !tt.wantSomeDeleted && stats.SegmentsDeleted != 0 {
				t.Errorf("expected no deletions, got %d", stats.SegmentsDeleted)
			}
			if tt.wantSomeDeleted && log.StartOffset() == 0 {
				t.Error("expected StartOffset to advance, but it is still 0")
			}
		})
	}
}

func TestRetentionManager_CombinedRetention(t *testing.T) {
	dir := t.TempDir()
	log, err := NewLog(dir, testConfig())
	if err != nil {
		t.Fatalf("NewLog failed: %v", err)
	}
	defer log.Close()

	now := time.Now()
	timestamps := []time.Time{
		now.Add(-10 * time.Second),
		now.Add(-9 * time.Second),
		now.Add(-8 * time.Second),
		now.Add(-7 * time.Second),
		now.Add(-100 * time.Millisecond),
	}
	appendMessages(t, log, timestamps)

	// Time retention should delete first 4 (older than 1s).
	// Size retention is unlimited so won't kick in.
	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  time.Minute,
		RetentionMs:    1000, // 1 second
		RetentionBytes: -1,
	})
	rm.RegisterLog("test:0", log)

	stats := rm.EnforceNow()

	if stats.SegmentsDeleted == 0 {
		t.Error("expected combined retention to delete some segments")
	}
	if log.StartOffset() < 4 {
		t.Errorf("expected StartOffset >= 4, got %d", log.StartOffset())
	}
}

func TestRetentionManager_EmptyLog(t *testing.T) {
	dir := t.TempDir()
	log, err := NewLog(dir, testConfig())
	if err != nil {
		t.Fatalf("NewLog failed: %v", err)
	}
	defer log.Close()

	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  time.Minute,
		RetentionMs:    1000,
		RetentionBytes: 100,
	})
	rm.RegisterLog("test:0", log)

	stats := rm.EnforceNow()

	if stats.SegmentsDeleted != 0 {
		t.Errorf("expected 0 segments deleted on empty log, got %d", stats.SegmentsDeleted)
	}
	if stats.PartitionsChecked != 1 {
		t.Errorf("expected 1 partition checked, got %d", stats.PartitionsChecked)
	}
}

func TestRetentionManager_MultiplePartitions(t *testing.T) {
	logs := make(map[string]Log)
	for _, pid := range []string{"topic:0", "topic:1", "topic:2"} {
		dir := t.TempDir()
		log, err := NewLog(dir, testConfig())
		if err != nil {
			t.Fatalf("NewLog failed: %v", err)
		}
		defer log.Close()

		now := time.Now()
		appendMessages(t, log, []time.Time{
			now.Add(-5 * time.Second),
			now.Add(-4 * time.Second),
			now.Add(-100 * time.Millisecond),
		})
		logs[pid] = log
	}

	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  time.Minute,
		RetentionMs:    1000,
		RetentionBytes: -1,
	})
	for pid, log := range logs {
		rm.RegisterLog(pid, log)
	}

	stats := rm.EnforceNow()

	if stats.PartitionsChecked != 3 {
		t.Errorf("expected 3 partitions checked, got %d", stats.PartitionsChecked)
	}
	if stats.SegmentsDeleted == 0 {
		t.Error("expected some segments deleted across partitions")
	}

	// Each partition should have first 2 messages deleted.
	for pid, log := range logs {
		if log.StartOffset() < 2 {
			t.Errorf("partition %s: expected StartOffset >= 2, got %d", pid, log.StartOffset())
		}
	}
}

func TestRetentionManager_RegisterUnregister(t *testing.T) {
	dir := t.TempDir()
	log, err := NewLog(dir, testConfig())
	if err != nil {
		t.Fatalf("NewLog failed: %v", err)
	}
	defer log.Close()

	now := time.Now()
	appendMessages(t, log, []time.Time{now.Add(-5 * time.Second)})

	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  time.Minute,
		RetentionMs:    1000,
		RetentionBytes: -1,
	})

	rm.RegisterLog("test:0", log)
	rm.UnregisterLog("test:0")

	stats := rm.EnforceNow()

	if stats.PartitionsChecked != 0 {
		t.Errorf("expected 0 partitions checked after unregister, got %d", stats.PartitionsChecked)
	}
	if log.StartOffset() != 0 {
		t.Errorf("expected StartOffset 0 (no enforcement), got %d", log.StartOffset())
	}
}

func TestRetentionManager_StartStop(t *testing.T) {
	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  50 * time.Millisecond,
		RetentionMs:    -1,
		RetentionBytes: -1,
	})

	ctx := context.Background()
	go rm.Start(ctx)

	// Give it a moment to run a few ticks.
	time.Sleep(200 * time.Millisecond)

	// Stop should return without hanging.
	done := make(chan struct{})
	go func() {
		rm.Stop()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2 seconds")
	}
}

func TestRetentionManager_ContextCancellation(t *testing.T) {
	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  50 * time.Millisecond,
		RetentionMs:    -1,
		RetentionBytes: -1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		rm.Start(ctx)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}

func TestRetentionManager_CumulativeStats(t *testing.T) {
	dir := t.TempDir()
	log, err := NewLog(dir, testConfig())
	if err != nil {
		t.Fatalf("NewLog failed: %v", err)
	}
	defer log.Close()

	now := time.Now()
	appendMessages(t, log, []time.Time{
		now.Add(-5 * time.Second),
		now.Add(-4 * time.Second),
		now.Add(-100 * time.Millisecond),
	})

	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  time.Minute,
		RetentionMs:    1000,
		RetentionBytes: -1,
	})
	rm.RegisterLog("test:0", log)

	// First enforcement.
	stats1 := rm.EnforceNow()
	if stats1.SegmentsDeleted == 0 {
		t.Fatal("expected first enforcement to delete segments")
	}

	// Add more old messages.
	appendMessages(t, log, []time.Time{
		now.Add(-3 * time.Second),
	})

	// Second enforcement.
	stats2 := rm.EnforceNow()

	cumulative := rm.Stats()
	expectedTotal := stats1.SegmentsDeleted + stats2.SegmentsDeleted
	if cumulative.SegmentsDeleted != expectedTotal {
		t.Errorf("cumulative SegmentsDeleted = %d, want %d",
			cumulative.SegmentsDeleted, expectedTotal)
	}
}

func TestRetentionManager_SizeBasedFIFOOrder(t *testing.T) {
	dir := t.TempDir()
	log, err := NewLog(dir, testConfig())
	if err != nil {
		t.Fatalf("NewLog failed: %v", err)
	}
	defer log.Close()

	now := time.Now()
	// Append messages with increasing timestamps to verify FIFO order.
	for i := 0; i < 10; i++ {
		batch := &MessageBatch{
			Messages: []Message{
				{
					Key:       []byte("key"),
					Value:     make([]byte, 100), // 100-byte values
					Timestamp: now.Add(time.Duration(i) * time.Second),
				},
			},
		}
		if _, err := log.Append(batch); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  time.Minute,
		RetentionMs:    -1,
		RetentionBytes: 300, // allow roughly 3 messages
	})
	rm.RegisterLog("test:0", log)

	stats := rm.EnforceNow()

	if stats.SegmentsDeleted == 0 {
		t.Fatal("expected some segments to be deleted for size-based retention")
	}

	// Oldest messages should be deleted first (FIFO).
	startOffset := log.StartOffset()
	if startOffset == 0 {
		t.Error("expected StartOffset > 0 after size-based retention")
	}

	// Remaining messages should still be readable.
	endOffset := log.EndOffset()
	for offset := startOffset; offset < endOffset; offset++ {
		msgs, err := log.Read(offset, 1024)
		if err != nil {
			t.Errorf("failed to read offset %d after retention: %v", offset, err)
			continue
		}
		if len(msgs) == 0 {
			t.Errorf("no messages at offset %d after retention", offset)
		}
	}
}

func TestRetentionManager_DefaultConfig(t *testing.T) {
	cfg := DefaultRetentionConfig()

	if cfg.CheckInterval != 5*time.Minute {
		t.Errorf("default CheckInterval = %v, want 5m", cfg.CheckInterval)
	}
	if cfg.RetentionMs != -1 {
		t.Errorf("default RetentionMs = %d, want -1", cfg.RetentionMs)
	}
	if cfg.RetentionBytes != -1 {
		t.Errorf("default RetentionBytes = %d, want -1", cfg.RetentionBytes)
	}
}

func TestRetentionManager_ZeroCheckInterval(t *testing.T) {
	rm := NewRetentionManager(RetentionConfig{
		CheckInterval: 0, // should default to 5 minutes
	})
	if rm.config.CheckInterval != 5*time.Minute {
		t.Errorf("expected default check interval 5m, got %v", rm.config.CheckInterval)
	}
}

func TestRetentionManager_NegativeCheckInterval(t *testing.T) {
	rm := NewRetentionManager(RetentionConfig{
		CheckInterval: -1 * time.Second,
	})
	if rm.config.CheckInterval != 5*time.Minute {
		t.Errorf("expected default check interval 5m, got %v", rm.config.CheckInterval)
	}
}

func TestRetentionManager_NoRegisteredLogs(t *testing.T) {
	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  time.Minute,
		RetentionMs:    1000,
		RetentionBytes: 100,
	})

	stats := rm.EnforceNow()

	if stats.PartitionsChecked != 0 {
		t.Errorf("expected 0 partitions checked, got %d", stats.PartitionsChecked)
	}
	if stats.SegmentsDeleted != 0 {
		t.Errorf("expected 0 segments deleted, got %d", stats.SegmentsDeleted)
	}
}

func TestRetentionManager_BackgroundEnforcement(t *testing.T) {
	dir := t.TempDir()
	log, err := NewLog(dir, testConfig())
	if err != nil {
		t.Fatalf("NewLog failed: %v", err)
	}
	defer log.Close()

	now := time.Now()
	appendMessages(t, log, []time.Time{
		now.Add(-5 * time.Second),
		now.Add(-4 * time.Second),
		now.Add(-100 * time.Millisecond),
	})

	rm := NewRetentionManager(RetentionConfig{
		CheckInterval:  100 * time.Millisecond,
		RetentionMs:    1000,
		RetentionBytes: -1,
	})
	rm.RegisterLog("test:0", log)

	ctx := context.Background()
	go rm.Start(ctx)

	// Wait for at least one tick.
	time.Sleep(300 * time.Millisecond)
	rm.Stop()

	// After background enforcement, old messages should be deleted.
	if log.StartOffset() < 2 {
		t.Errorf("expected StartOffset >= 2 after background enforcement, got %d",
			log.StartOffset())
	}

	cumStats := rm.Stats()
	if cumStats.SegmentsDeleted == 0 {
		t.Error("expected cumulative stats to show deletions after background run")
	}
}

func TestEstimateBytes(t *testing.T) {
	tests := []struct {
		name string
		from Offset
		to   Offset
		want int64
	}{
		{"positive range", 0, 10, 2560},
		{"zero range", 5, 5, 0},
		{"negative range", 10, 5, 0},
		{"single message", 0, 1, 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateBytes(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("estimateBytes(%d, %d) = %d, want %d", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestRetentionManager_StopWithoutStart(t *testing.T) {
	rm := NewRetentionManager(RetentionConfig{
		CheckInterval: time.Minute,
	})

	// Stop without Start should not panic.
	rm.Stop()
}
