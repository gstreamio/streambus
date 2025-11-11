package profiling

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"runtime"
	"runtime/debug"
	"time"
)

// Config holds profiling configuration
type Config struct {
	// Enabled enables/disables profiling endpoints
	Enabled bool

	// ListenAddr is the address to listen on (default: :6060)
	ListenAddr string

	// EnableCPUProfiling enables CPU profiling
	EnableCPUProfiling bool

	// EnableMemoryProfiling enables memory profiling
	EnableMemoryProfiling bool

	// EnableBlockProfiling enables block profiling
	EnableBlockProfiling bool

	// EnableMutexProfiling enables mutex profiling
	EnableMutexProfiling bool

	// BlockRate sets the block profiling rate (default: 1)
	BlockRate int

	// MutexFraction sets the mutex profiling fraction (default: 1)
	MutexFraction int
}

// Profiler manages profiling endpoints
type Profiler struct {
	config *Config
	server *http.Server
}

// New creates a new profiler with the given configuration
func New(cfg *Config) *Profiler {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":6060"
	}
	if cfg.BlockRate == 0 {
		cfg.BlockRate = 1
	}
	if cfg.MutexFraction == 0 {
		cfg.MutexFraction = 1
	}

	return &Profiler{
		config: cfg,
	}
}

// Start starts the profiling HTTP server
func (p *Profiler) Start() error {
	if !p.config.Enabled {
		return nil
	}

	// Configure profiling rates
	if p.config.EnableBlockProfiling {
		runtime.SetBlockProfileRate(p.config.BlockRate)
	}
	if p.config.EnableMutexProfiling {
		runtime.SetMutexProfileFraction(p.config.MutexFraction)
	}

	// Create HTTP mux
	mux := http.NewServeMux()

	// Register pprof handlers
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Register additional handlers
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))

	// Add custom endpoints
	mux.HandleFunc("/debug/stats", p.statsHandler)
	mux.HandleFunc("/debug/gc", p.gcHandler)
	mux.HandleFunc("/debug/freeOSMemory", p.freeOSMemoryHandler)

	// Create server
	p.server = &http.Server{
		Addr:         p.config.ListenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 10 * time.Minute,
	}

	// Start server in background
	go func() {
		if err := p.server.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Printf("Profiling server error: %v\n", err)
		}
	}()

	fmt.Printf("Profiling server started on %s\n", p.config.ListenAddr)
	return nil
}

// Stop stops the profiling HTTP server
func (p *Profiler) Stop() error {
	if p.server != nil {
		return p.server.Close()
	}
	return nil
}

// statsHandler returns runtime statistics
func (p *Profiler) statsHandler(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Fprintf(w, "# Runtime Statistics\n\n")
	fmt.Fprintf(w, "## Memory\n")
	fmt.Fprintf(w, "Alloc: %d MB\n", m.Alloc/1024/1024)
	fmt.Fprintf(w, "TotalAlloc: %d MB\n", m.TotalAlloc/1024/1024)
	fmt.Fprintf(w, "Sys: %d MB\n", m.Sys/1024/1024)
	fmt.Fprintf(w, "NumGC: %d\n", m.NumGC)
	fmt.Fprintf(w, "GCCPUFraction: %.6f\n\n", m.GCCPUFraction)

	fmt.Fprintf(w, "## Goroutines\n")
	fmt.Fprintf(w, "NumGoroutine: %d\n\n", runtime.NumGoroutine())

	fmt.Fprintf(w, "## CPU\n")
	fmt.Fprintf(w, "NumCPU: %d\n", runtime.NumCPU())
	fmt.Fprintf(w, "GOMAXPROCS: %d\n\n", runtime.GOMAXPROCS(0))

	fmt.Fprintf(w, "## GC Stats\n")
	var gcStats debug.GCStats
	debug.ReadGCStats(&gcStats)
	fmt.Fprintf(w, "LastGC: %v\n", gcStats.LastGC)
	fmt.Fprintf(w, "NumGC: %d\n", gcStats.NumGC)
	fmt.Fprintf(w, "PauseTotal: %v\n", gcStats.PauseTotal)
	if len(gcStats.Pause) > 0 {
		fmt.Fprintf(w, "LastPause: %v\n", gcStats.Pause[0])
	}
}

// gcHandler triggers a garbage collection
func (p *Profiler) gcHandler(w http.ResponseWriter, r *http.Request) {
	runtime.GC()
	fmt.Fprintf(w, "GC triggered\n")
}

// freeOSMemoryHandler returns memory to the OS
func (p *Profiler) freeOSMemoryHandler(w http.ResponseWriter, r *http.Request) {
	debug.FreeOSMemory()
	fmt.Fprintf(w, "Memory returned to OS\n")
}

// GetMemStats returns current memory statistics
func GetMemStats() *runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return &m
}

// GetGoroutineCount returns the number of goroutines
func GetGoroutineCount() int {
	return runtime.NumGoroutine()
}

// TriggerGC triggers a garbage collection
func TriggerGC() {
	runtime.GC()
}

// FreeOSMemory returns memory to the operating system
func FreeOSMemory() {
	debug.FreeOSMemory()
}
