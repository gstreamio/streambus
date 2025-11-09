package tracing

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Tracer manages OpenTelemetry tracing
type Tracer struct {
	config   *Config
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	exporter sdktrace.SpanExporter
	mu       sync.RWMutex
	shutdown bool
}

// NewTracer creates a new tracer
func NewTracer(config *Config) (*Tracer, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// If tracing is disabled, return a no-op tracer
	if !config.Enabled {
		return &Tracer{
			config:   config,
			provider: nil,
			tracer:   trace.NewNoopTracerProvider().Tracer(config.ServiceName),
		}, nil
	}

	t := &Tracer{
		config: config,
	}

	// Initialize exporter
	exporter, err := t.createExporter()
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}
	t.exporter = exporter

	// Create resource
	res, err := t.createResource()
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler
	sampler := t.createSampler()

	// Create tracer provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	t.provider = provider

	// Set global tracer provider
	otel.SetTracerProvider(provider)

	// Set global propagator
	t.setPropagators()

	// Create tracer
	t.tracer = provider.Tracer(
		config.ServiceName,
		trace.WithInstrumentationVersion(config.ServiceVersion),
	)

	return t, nil
}

// createExporter creates the appropriate span exporter based on configuration
func (t *Tracer) createExporter() (sdktrace.SpanExporter, error) {
	switch t.config.Exporter.Type {
	case ExporterTypeOTLP:
		return t.createOTLPExporter()
	case ExporterTypeStdout:
		return t.createStdoutExporter()
	case ExporterTypeNone:
		return &noopExporter{}, nil
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", t.config.Exporter.Type)
	}
}

// createOTLPExporter creates an OTLP exporter
func (t *Tracer) createOTLPExporter() (sdktrace.SpanExporter, error) {
	ctx := context.Background()
	config := t.config.Exporter.OTLP

	// Create gRPC options
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.Endpoint),
		otlptracegrpc.WithTimeout(config.Timeout),
	}

	// Add insecure option if specified
	if config.Insecure {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()))
	}

	// Add headers
	if len(config.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(config.Headers))
	}

	// Add compression
	if config.Compression == "gzip" {
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithDefaultCallOptions(grpc.UseCompressor("gzip"))))
	}

	// Create exporter
	client := otlptracegrpc.NewClient(opts...)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	return exporter, nil
}

// createStdoutExporter creates a stdout exporter for debugging
func (t *Tracer) createStdoutExporter() (sdktrace.SpanExporter, error) {
	exporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
	}
	return exporter, nil
}

// createResource creates the resource for the tracer
func (t *Tracer) createResource() (*resource.Resource, error) {
	return resource.New(
		context.Background(),
		resource.WithAttributes(t.config.ToResourceAttributes()...),
	)
}

// createSampler creates the sampler based on configuration
func (t *Tracer) createSampler() sdktrace.Sampler {
	samplingRate := t.config.Sampling.SamplingRate

	var baseSampler sdktrace.Sampler
	if samplingRate >= 1.0 {
		baseSampler = sdktrace.AlwaysSample()
	} else if samplingRate <= 0.0 {
		baseSampler = sdktrace.NeverSample()
	} else {
		baseSampler = sdktrace.TraceIDRatioBased(samplingRate)
	}

	// Use parent-based sampling if configured
	if t.config.Sampling.ParentBased {
		return sdktrace.ParentBased(baseSampler)
	}

	return baseSampler
}

// setPropagators sets the global propagators
func (t *Tracer) setPropagators() {
	propagators := []propagation.TextMapPropagator{}

	for _, p := range t.config.Propagators {
		switch p {
		case "tracecontext":
			propagators = append(propagators, propagation.TraceContext{})
		case "baggage":
			propagators = append(propagators, propagation.Baggage{})
		}
	}

	if len(propagators) > 0 {
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagators...))
	}
}

// Tracer returns the OpenTelemetry tracer
func (t *Tracer) Tracer() trace.Tracer {
	return t.tracer
}

// Provider returns the tracer provider
func (t *Tracer) Provider() *sdktrace.TracerProvider {
	return t.provider
}

// Shutdown shuts down the tracer and flushes any remaining spans
func (t *Tracer) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.shutdown {
		return nil
	}

	var errs []error

	// Shutdown provider
	if t.provider != nil {
		if err := t.provider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("provider shutdown: %w", err))
		}
	}

	t.shutdown = true

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	return nil
}

// IsEnabled returns whether tracing is enabled
func (t *Tracer) IsEnabled() bool {
	return t.config.Enabled
}

// noopExporter is a no-op span exporter
type noopExporter struct{}

func (e *noopExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}
