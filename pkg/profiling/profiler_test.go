package profiling

import (
	"net/http"
	"runtime"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   string
	}{
		{
			name:   "default listen addr",
			config: &Config{Enabled: true},
			want:   ":6060",
		},
		{
			name:   "custom listen addr",
			config: &Config{Enabled: true, ListenAddr: ":8080"},
			want:   ":8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.config)
			if p.config.ListenAddr != tt.want {
				t.Errorf("ListenAddr = %v, want %v", p.config.ListenAddr, tt.want)
			}
		})
	}
}

func TestProfiler_StartStop(t *testing.T) {
	p := New(&Config{
		Enabled:    true,
		ListenAddr: ":16060", // Use different port to avoid conflicts
	})

	// Start profiler
	err := p.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test that server is running
	resp, err := http.Get("http://localhost:16060/debug/pprof/")
	if err != nil {
		t.Fatalf("Failed to connect to profiling server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Stop profiler
	err = p.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestProfiler_DisabledDoesNotStart(t *testing.T) {
	p := New(&Config{
		Enabled:    false,
		ListenAddr: ":16061",
	})

	err := p.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Server should not be running
	_, err = http.Get("http://localhost:16061/debug/pprof/")
	if err == nil {
		t.Error("Server should not be running when disabled")
	}
}

func TestGetMemStats(t *testing.T) {
	stats := GetMemStats()
	if stats == nil {
		t.Error("GetMemStats() returned nil")
		return
	}

	if stats.Alloc == 0 {
		t.Error("GetMemStats() returned zero Alloc")
	}
}

func TestGetGoroutineCount(t *testing.T) {
	count := GetGoroutineCount()
	if count <= 0 {
		t.Errorf("GetGoroutineCount() = %d, want > 0", count)
	}
}

func TestTriggerGC(t *testing.T) {
	var beforeStats runtime.MemStats
	runtime.ReadMemStats(&beforeStats)
	before := beforeStats.NumGC

	// Trigger GC
	TriggerGC()

	var afterStats runtime.MemStats
	runtime.ReadMemStats(&afterStats)
	after := afterStats.NumGC
	if after <= before {
		t.Errorf("GC count did not increase: before=%d, after=%d", before, after)
	}
}

func TestFreeOSMemory(t *testing.T) {
	// Should not panic
	FreeOSMemory()
}

func TestProfiler_StatsEndpoint(t *testing.T) {
	p := New(&Config{
		Enabled:    true,
		ListenAddr: ":16062",
	})

	err := p.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer p.Stop()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:16062/debug/stats")
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestProfiler_GCEndpoint(t *testing.T) {
	p := New(&Config{
		Enabled:    true,
		ListenAddr: ":16063",
	})

	err := p.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer p.Stop()

	time.Sleep(100 * time.Millisecond)

	var beforeStats runtime.MemStats
	runtime.ReadMemStats(&beforeStats)
	before := beforeStats.NumGC

	resp, err := http.Get("http://localhost:16063/debug/gc")
	if err != nil {
		t.Fatalf("Failed to trigger GC: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var afterStats runtime.MemStats
	runtime.ReadMemStats(&afterStats)
	after := afterStats.NumGC
	if after <= before {
		t.Errorf("GC was not triggered: before=%d, after=%d", before, after)
	}
}

func TestProfiler_ProfilingRates(t *testing.T) {
	p := New(&Config{
		Enabled:               true,
		EnableBlockProfiling:  true,
		EnableMutexProfiling:  true,
		BlockRate:             10,
		MutexFraction:         5,
	})

	if p.config.BlockRate != 10 {
		t.Errorf("BlockRate = %d, want 10", p.config.BlockRate)
	}

	if p.config.MutexFraction != 5 {
		t.Errorf("MutexFraction = %d, want 5", p.config.MutexFraction)
	}
}
