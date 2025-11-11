package tracing

import (
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// Config holds tracing configuration
type Config struct {
	// Enable tracing
	Enabled bool

	// Service name for traces
	ServiceName string

	// Service version
	ServiceVersion string

	// Environment (dev, staging, prod)
	Environment string

	// Exporter configuration
	Exporter ExporterConfig

	// Sampling configuration
	Sampling SamplingConfig

	// Resource attributes
	ResourceAttributes map[string]string

	// Propagators (tracecontext, baggage, b3, jaeger, etc.)
	Propagators []string
}

// ExporterConfig holds exporter configuration
type ExporterConfig struct {
	// Exporter type (otlp, jaeger, zipkin, stdout)
	Type ExporterType

	// OTLP configuration
	OTLP OTLPConfig

	// Jaeger configuration
	Jaeger JaegerConfig

	// Zipkin configuration
	Zipkin ZipkinConfig
}

// ExporterType represents the type of trace exporter
type ExporterType string

const (
	// ExporterTypeOTLP exports to OTLP endpoint (OpenTelemetry Collector)
	ExporterTypeOTLP ExporterType = "otlp"

	// ExporterTypeJaeger exports to Jaeger
	ExporterTypeJaeger ExporterType = "jaeger"

	// ExporterTypeZipkin exports to Zipkin
	ExporterTypeZipkin ExporterType = "zipkin"

	// ExporterTypeStdout exports to stdout (for debugging)
	ExporterTypeStdout ExporterType = "stdout"

	// ExporterTypeNone disables exporting
	ExporterTypeNone ExporterType = "none"
)

// OTLPConfig holds OTLP exporter configuration
type OTLPConfig struct {
	// Endpoint (e.g., "localhost:4317")
	Endpoint string

	// Use insecure connection
	Insecure bool

	// Headers to send with requests
	Headers map[string]string

	// Timeout for exporting
	Timeout time.Duration

	// Compression (gzip, none)
	Compression string
}

// JaegerConfig holds Jaeger exporter configuration
type JaegerConfig struct {
	// Agent endpoint (e.g., "localhost:6831")
	AgentEndpoint string

	// Collector endpoint (e.g., "http://localhost:14268/api/traces")
	CollectorEndpoint string

	// Username for authentication
	Username string

	// Password for authentication
	Password string
}

// ZipkinConfig holds Zipkin exporter configuration
type ZipkinConfig struct {
	// Endpoint (e.g., "http://localhost:9411/api/v2/spans")
	Endpoint string

	// Timeout for exporting
	Timeout time.Duration
}

// SamplingConfig holds sampling configuration
type SamplingConfig struct {
	// Sampling rate (0.0 to 1.0)
	// 1.0 = sample all traces, 0.1 = sample 10% of traces
	SamplingRate float64

	// Parent-based sampling
	ParentBased bool
}

// DefaultConfig returns default tracing configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:        false,
		ServiceName:    "streambus",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Exporter: ExporterConfig{
			Type: ExporterTypeOTLP,
			OTLP: OTLPConfig{
				Endpoint:    "localhost:4317",
				Insecure:    true,
				Timeout:     10 * time.Second,
				Compression: "gzip",
			},
		},
		Sampling: SamplingConfig{
			SamplingRate: 1.0,
			ParentBased:  true,
		},
		Propagators: []string{"tracecontext", "baggage"},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.ServiceName == "" {
		return ErrInvalidServiceName
	}

	if c.Sampling.SamplingRate < 0.0 || c.Sampling.SamplingRate > 1.0 {
		return ErrInvalidSamplingRate
	}

	// Validate exporter configuration
	switch c.Exporter.Type {
	case ExporterTypeOTLP:
		if c.Exporter.OTLP.Endpoint == "" {
			return ErrInvalidOTLPEndpoint
		}
	case ExporterTypeJaeger:
		if c.Exporter.Jaeger.AgentEndpoint == "" && c.Exporter.Jaeger.CollectorEndpoint == "" {
			return ErrInvalidJaegerEndpoint
		}
	case ExporterTypeZipkin:
		if c.Exporter.Zipkin.Endpoint == "" {
			return ErrInvalidZipkinEndpoint
		}
	case ExporterTypeStdout, ExporterTypeNone:
		// No validation needed
	default:
		return ErrInvalidExporterType
	}

	return nil
}

// ToResourceAttributes converts resource attributes to OpenTelemetry attributes
func (c *Config) ToResourceAttributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("service.name", c.ServiceName),
		attribute.String("service.version", c.ServiceVersion),
		attribute.String("deployment.environment", c.Environment),
	}

	// Add custom resource attributes
	for key, value := range c.ResourceAttributes {
		attrs = append(attrs, attribute.String(key, value))
	}

	return attrs
}
