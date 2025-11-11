package tenancy

import (
	"fmt"
	"sync"
	"time"
)

// QuotaTracker tracks resource usage for a tenant and enforces quotas
type QuotaTracker struct {
	tenantID TenantID
	quotas   *Quotas

	mu sync.RWMutex

	// Current usage counters
	currentConnections    int
	currentProducers      int
	currentConsumers      int
	currentTopics         int
	currentPartitions     int
	currentConsumerGroups int
	currentStorageBytes   int64

	// Rate tracking (windowed counters for throughput)
	bytesWindow    *RateWindow
	messagesWindow *RateWindow
	requestsWindow *RateWindow
}

// RateWindow tracks usage over a sliding time window for rate limiting
type RateWindow struct {
	mu          sync.Mutex
	windowSize  time.Duration
	bucketSize  time.Duration
	buckets     []int64
	bucketTimes []time.Time
	currentIdx  int
}

// NewRateWindow creates a new rate tracking window
func NewRateWindow(windowSize, bucketSize time.Duration) *RateWindow {
	numBuckets := int(windowSize / bucketSize)
	if numBuckets < 1 {
		numBuckets = 1
	}

	return &RateWindow{
		windowSize:  windowSize,
		bucketSize:  bucketSize,
		buckets:     make([]int64, numBuckets),
		bucketTimes: make([]time.Time, numBuckets),
		currentIdx:  0,
	}
}

// Add adds a value to the current bucket
func (w *RateWindow) Add(value int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()

	// Check if we need to rotate to a new bucket
	if now.Sub(w.bucketTimes[w.currentIdx]) >= w.bucketSize {
		// Move to next bucket
		w.currentIdx = (w.currentIdx + 1) % len(w.buckets)
		w.buckets[w.currentIdx] = 0
		w.bucketTimes[w.currentIdx] = now
	}

	w.buckets[w.currentIdx] += value
}

// Rate returns the current rate (total in window / window duration in seconds)
func (w *RateWindow) Rate() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	var total int64

	// Sum all buckets within the window
	for i, bucketTime := range w.bucketTimes {
		if now.Sub(bucketTime) <= w.windowSize {
			total += w.buckets[i]
		}
	}

	// Return rate per second
	return float64(total) / w.windowSize.Seconds()
}

// NewQuotaTracker creates a new quota tracker for a tenant
func NewQuotaTracker(tenantID TenantID, quotas *Quotas) *QuotaTracker {
	return &QuotaTracker{
		tenantID:       tenantID,
		quotas:         quotas,
		bytesWindow:    NewRateWindow(1*time.Second, 100*time.Millisecond),
		messagesWindow: NewRateWindow(1*time.Second, 100*time.Millisecond),
		requestsWindow: NewRateWindow(1*time.Second, 100*time.Millisecond),
	}
}

// QuotaError represents a quota exceeded error
type QuotaError struct {
	TenantID     TenantID
	QuotaType    string
	Current      int64
	Limit        int64
	Message      string
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("quota exceeded for tenant %s: %s (current: %d, limit: %d)",
		e.TenantID, e.Message, e.Current, e.Limit)
}

// CheckThroughput checks if the tenant can send/receive the specified bytes and messages
func (qt *QuotaTracker) CheckThroughput(bytes, messages int64) error {
	if qt.quotas.MaxBytesPerSecond >= 0 {
		currentRate := int64(qt.bytesWindow.Rate())
		if currentRate+bytes > qt.quotas.MaxBytesPerSecond {
			return &QuotaError{
				TenantID:  qt.tenantID,
				QuotaType: "bytes_per_second",
				Current:   currentRate,
				Limit:     qt.quotas.MaxBytesPerSecond,
				Message:   "bytes per second quota exceeded",
			}
		}
	}

	if qt.quotas.MaxMessagesPerSecond >= 0 {
		currentRate := int64(qt.messagesWindow.Rate())
		if currentRate+messages > qt.quotas.MaxMessagesPerSecond {
			return &QuotaError{
				TenantID:  qt.tenantID,
				QuotaType: "messages_per_second",
				Current:   currentRate,
				Limit:     qt.quotas.MaxMessagesPerSecond,
				Message:   "messages per second quota exceeded",
			}
		}
	}

	return nil
}

// RecordThroughput records bytes and messages used
func (qt *QuotaTracker) RecordThroughput(bytes, messages int64) {
	qt.bytesWindow.Add(bytes)
	qt.messagesWindow.Add(messages)
}

// CheckConnection checks if a new connection can be added
func (qt *QuotaTracker) CheckConnection() error {
	qt.mu.RLock()
	defer qt.mu.RUnlock()

	if qt.quotas.MaxConnections >= 0 && qt.currentConnections >= qt.quotas.MaxConnections {
		return &QuotaError{
			TenantID:  qt.tenantID,
			QuotaType: "connections",
			Current:   int64(qt.currentConnections),
			Limit:     int64(qt.quotas.MaxConnections),
			Message:   "max connections quota exceeded",
		}
	}

	return nil
}

// AddConnection increments the connection count
func (qt *QuotaTracker) AddConnection() error {
	if err := qt.CheckConnection(); err != nil {
		return err
	}

	qt.mu.Lock()
	defer qt.mu.Unlock()

	qt.currentConnections++
	return nil
}

// RemoveConnection decrements the connection count
func (qt *QuotaTracker) RemoveConnection() {
	qt.mu.Lock()
	defer qt.mu.Unlock()

	if qt.currentConnections > 0 {
		qt.currentConnections--
	}
}

// CheckProducer checks if a new producer can be added
func (qt *QuotaTracker) CheckProducer() error {
	qt.mu.RLock()
	defer qt.mu.RUnlock()

	if qt.quotas.MaxProducers >= 0 && qt.currentProducers >= qt.quotas.MaxProducers {
		return &QuotaError{
			TenantID:  qt.tenantID,
			QuotaType: "producers",
			Current:   int64(qt.currentProducers),
			Limit:     int64(qt.quotas.MaxProducers),
			Message:   "max producers quota exceeded",
		}
	}

	return nil
}

// AddProducer increments the producer count
func (qt *QuotaTracker) AddProducer() error {
	if err := qt.CheckProducer(); err != nil {
		return err
	}

	qt.mu.Lock()
	defer qt.mu.Unlock()

	qt.currentProducers++
	return nil
}

// RemoveProducer decrements the producer count
func (qt *QuotaTracker) RemoveProducer() {
	qt.mu.Lock()
	defer qt.mu.Unlock()

	if qt.currentProducers > 0 {
		qt.currentProducers--
	}
}

// CheckStorage checks if storage quota allows the specified bytes
func (qt *QuotaTracker) CheckStorage(additionalBytes int64) error {
	qt.mu.RLock()
	defer qt.mu.RUnlock()

	if qt.quotas.MaxStorageBytes >= 0 {
		if qt.currentStorageBytes+additionalBytes > qt.quotas.MaxStorageBytes {
			return &QuotaError{
				TenantID:  qt.tenantID,
				QuotaType: "storage",
				Current:   qt.currentStorageBytes,
				Limit:     qt.quotas.MaxStorageBytes,
				Message:   "max storage quota exceeded",
			}
		}
	}

	return nil
}

// UpdateStorage updates the current storage usage
func (qt *QuotaTracker) UpdateStorage(bytes int64) {
	qt.mu.Lock()
	defer qt.mu.Unlock()

	qt.currentStorageBytes = bytes
	if qt.currentStorageBytes < 0 {
		qt.currentStorageBytes = 0
	}
}

// CheckTopic checks if a new topic can be created
func (qt *QuotaTracker) CheckTopic() error {
	qt.mu.RLock()
	defer qt.mu.RUnlock()

	if qt.quotas.MaxTopics >= 0 && qt.currentTopics >= qt.quotas.MaxTopics {
		return &QuotaError{
			TenantID:  qt.tenantID,
			QuotaType: "topics",
			Current:   int64(qt.currentTopics),
			Limit:     int64(qt.quotas.MaxTopics),
			Message:   "max topics quota exceeded",
		}
	}

	return nil
}

// AddTopic increments the topic count
func (qt *QuotaTracker) AddTopic(partitions int) error {
	if err := qt.CheckTopic(); err != nil {
		return err
	}

	// Also check partition quota
	qt.mu.RLock()
	if qt.quotas.MaxPartitions >= 0 && qt.currentPartitions+partitions > qt.quotas.MaxPartitions {
		qt.mu.RUnlock()
		return &QuotaError{
			TenantID:  qt.tenantID,
			QuotaType: "partitions",
			Current:   int64(qt.currentPartitions),
			Limit:     int64(qt.quotas.MaxPartitions),
			Message:   "max partitions quota exceeded",
		}
	}
	qt.mu.RUnlock()

	qt.mu.Lock()
	defer qt.mu.Unlock()

	qt.currentTopics++
	qt.currentPartitions += partitions
	return nil
}

// RemoveTopic decrements the topic count
func (qt *QuotaTracker) RemoveTopic(partitions int) {
	qt.mu.Lock()
	defer qt.mu.Unlock()

	if qt.currentTopics > 0 {
		qt.currentTopics--
	}

	qt.currentPartitions -= partitions
	if qt.currentPartitions < 0 {
		qt.currentPartitions = 0
	}
}

// GetUsage returns current usage statistics
func (qt *QuotaTracker) GetUsage() *Usage {
	qt.mu.RLock()
	defer qt.mu.RUnlock()

	return &Usage{
		TenantID:             qt.tenantID,
		Connections:          qt.currentConnections,
		Producers:            qt.currentProducers,
		Consumers:            qt.currentConsumers,
		Topics:               qt.currentTopics,
		Partitions:           qt.currentPartitions,
		ConsumerGroups:       qt.currentConsumerGroups,
		StorageBytes:         qt.currentStorageBytes,
		BytesPerSecond:       int64(qt.bytesWindow.Rate()),
		MessagesPerSecond:    int64(qt.messagesWindow.Rate()),
		RequestsPerSecond:    int64(qt.requestsWindow.Rate()),
	}
}

// Usage represents current resource usage for a tenant
type Usage struct {
	TenantID           TenantID
	Connections        int
	Producers          int
	Consumers          int
	Topics             int
	Partitions         int
	ConsumerGroups     int
	StorageBytes       int64
	BytesPerSecond     int64
	MessagesPerSecond  int64
	RequestsPerSecond  int64
}

// UtilizationPercent calculates quota utilization percentage
func (qt *QuotaTracker) UtilizationPercent(quotaType string) float64 {
	qt.mu.RLock()
	defer qt.mu.RUnlock()

	switch quotaType {
	case "connections":
		if qt.quotas.MaxConnections <= 0 {
			return 0
		}
		return float64(qt.currentConnections) / float64(qt.quotas.MaxConnections) * 100.0

	case "storage":
		if qt.quotas.MaxStorageBytes <= 0 {
			return 0
		}
		return float64(qt.currentStorageBytes) / float64(qt.quotas.MaxStorageBytes) * 100.0

	case "topics":
		if qt.quotas.MaxTopics <= 0 {
			return 0
		}
		return float64(qt.currentTopics) / float64(qt.quotas.MaxTopics) * 100.0

	default:
		return 0
	}
}
