package health

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Status represents the health status of a component
type Status string

const (
	// StatusHealthy indicates the component is healthy
	StatusHealthy Status = "healthy"
	// StatusDegraded indicates the component is working but not optimally
	StatusDegraded Status = "degraded"
	// StatusUnhealthy indicates the component is not working
	StatusUnhealthy Status = "unhealthy"
	// StatusUnknown indicates the health status is unknown
	StatusUnknown Status = "unknown"
)

// Check represents a health check result for a component
type Check struct {
	Name      string                 `json:"name"`
	Status    Status                 `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  time.Duration          `json:"duration_ms"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// Response represents the aggregated health check response
type Response struct {
	Status    Status           `json:"status"`
	Timestamp time.Time        `json:"timestamp"`
	Checks    map[string]Check `json:"checks"`
}

// MarshalJSON customizes JSON marshaling for Response
func (r *Response) MarshalJSON() ([]byte, error) {
	type Alias Response
	return json.Marshal(&struct {
		Duration string `json:"total_duration_ms"`
		*Alias
	}{
		Duration: fmt.Sprintf("%.2f", r.calculateTotalDuration()),
		Alias:    (*Alias)(r),
	})
}

func (r *Response) calculateTotalDuration() float64 {
	var total time.Duration
	for _, check := range r.Checks {
		total += check.Duration
	}
	return float64(total.Milliseconds())
}

// Checker is the interface that health check providers must implement
type Checker interface {
	// Name returns the name of this health check
	Name() string
	// Check performs the health check and returns the result
	Check(ctx context.Context) Check
}

// Registry manages health checks
type Registry struct {
	mu       sync.RWMutex
	checkers map[string]Checker
	timeout  time.Duration
}

// NewRegistry creates a new health check registry
func NewRegistry() *Registry {
	return &Registry{
		checkers: make(map[string]Checker),
		timeout:  2 * time.Second,
	}
}

// SetTimeout sets the timeout for health checks
func (r *Registry) SetTimeout(timeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timeout = timeout
}

// Register adds a health checker to the registry
func (r *Registry) Register(checker Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers[checker.Name()] = checker
}

// Unregister removes a health checker from the registry
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.checkers, name)
}

// CheckAll runs all registered health checks and returns aggregated results
func (r *Registry) CheckAll(ctx context.Context) Response {
	r.mu.RLock()
	checkers := make([]Checker, 0, len(r.checkers))
	for _, checker := range r.checkers {
		checkers = append(checkers, checker)
	}
	timeout := r.timeout
	r.mu.RUnlock()

	response := Response{
		Timestamp: time.Now(),
		Checks:    make(map[string]Check),
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Run all checks in parallel
	type result struct {
		name  string
		check Check
	}

	results := make(chan result, len(checkers))
	var wg sync.WaitGroup

	for _, checker := range checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()
			check := c.Check(ctx)
			results <- result{name: c.Name(), check: check}
		}(checker)
	}

	// Close results channel when all checks complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for res := range results {
		response.Checks[res.name] = res.check
	}

	// Determine overall status
	response.Status = r.aggregateStatus(response.Checks)

	return response
}

// CheckOne runs a single health check by name
func (r *Registry) CheckOne(ctx context.Context, name string) (Check, error) {
	r.mu.RLock()
	checker, exists := r.checkers[name]
	timeout := r.timeout
	r.mu.RUnlock()

	if !exists {
		return Check{}, fmt.Errorf("health check not found: %s", name)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return checker.Check(ctx), nil
}

// aggregateStatus determines the overall health status from individual checks
func (r *Registry) aggregateStatus(checks map[string]Check) Status {
	if len(checks) == 0 {
		return StatusUnknown
	}

	hasUnhealthy := false
	hasDegraded := false

	for _, check := range checks {
		switch check.Status {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusDegraded:
			hasDegraded = true
		case StatusUnknown:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasDegraded {
		return StatusDegraded
	}
	return StatusHealthy
}

// LivenessCheck returns a simple check that always succeeds (process is alive)
func LivenessCheck() Check {
	return Check{
		Name:      "liveness",
		Status:    StatusHealthy,
		Message:   "process is alive",
		Timestamp: time.Now(),
		Duration:  0,
	}
}

// ReadinessCheck returns the aggregated status for readiness
// A system is ready if all critical components are healthy or degraded (not unhealthy)
func (r *Registry) ReadinessCheck(ctx context.Context) Check {
	start := time.Now()
	response := r.CheckAll(ctx)

	status := StatusHealthy
	if response.Status == StatusUnhealthy {
		status = StatusUnhealthy
	} else if response.Status == StatusDegraded {
		status = StatusDegraded
	}

	message := "ready to serve traffic"
	if status == StatusUnhealthy {
		message = "not ready to serve traffic"
	} else if status == StatusDegraded {
		message = "ready but degraded"
	}

	return Check{
		Name:      "readiness",
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Details: map[string]interface{}{
			"component_checks": len(response.Checks),
		},
	}
}

// SimpleChecker is a basic implementation of Checker using a function
type SimpleChecker struct {
	name string
	fn   func(ctx context.Context) Check
}

// NewSimpleChecker creates a new simple health checker
func NewSimpleChecker(name string, fn func(ctx context.Context) Check) *SimpleChecker {
	return &SimpleChecker{
		name: name,
		fn:   fn,
	}
}

// Name returns the checker name
func (sc *SimpleChecker) Name() string {
	return sc.name
}

// Check executes the health check function
func (sc *SimpleChecker) Check(ctx context.Context) Check {
	start := time.Now()
	check := sc.fn(ctx)
	check.Duration = time.Since(start)
	check.Timestamp = time.Now()
	if check.Name == "" {
		check.Name = sc.name
	}
	return check
}

// PeriodicChecker runs health checks periodically and caches results
type PeriodicChecker struct {
	mu       sync.RWMutex
	checker  Checker
	interval time.Duration
	last     Check
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewPeriodicChecker creates a health checker that runs periodically
func NewPeriodicChecker(checker Checker, interval time.Duration) *PeriodicChecker {
	return &PeriodicChecker{
		checker:  checker,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Name returns the wrapped checker name
func (pc *PeriodicChecker) Name() string {
	return pc.checker.Name()
}

// Start begins periodic health checking
func (pc *PeriodicChecker) Start() {
	go pc.run()
}

// Stop stops periodic health checking
func (pc *PeriodicChecker) Stop() {
	close(pc.stopCh)
	<-pc.doneCh
}

func (pc *PeriodicChecker) run() {
	defer close(pc.doneCh)

	// Run immediately
	ctx := context.Background()
	check := pc.checker.Check(ctx)
	pc.mu.Lock()
	pc.last = check
	pc.mu.Unlock()

	ticker := time.NewTicker(pc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			check := pc.checker.Check(ctx)
			pc.mu.Lock()
			pc.last = check
			pc.mu.Unlock()

		case <-pc.stopCh:
			return
		}
	}
}

// Check returns the last cached health check result
func (pc *PeriodicChecker) Check(ctx context.Context) Check {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.last.Name == "" {
		// No check has run yet
		return Check{
			Name:      pc.checker.Name(),
			Status:    StatusUnknown,
			Message:   "no health check has run yet",
			Timestamp: time.Now(),
		}
	}

	return pc.last
}
