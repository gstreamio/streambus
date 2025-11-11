package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// Tracer wraps OpenTelemetry tracer
type Tracer struct {
	config   *Config
	tracer   trace.Tracer
	provider *sdktrace.TracerProvider
}

// New creates a new tracer with the given configuration
func New(cfg *Config) (*Tracer, error) {
	if !cfg.Enabled {
		return &Tracer{
			config: cfg,
			tracer: otel.Tracer(cfg.ServiceName),
		}, nil
	}

	// Create resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Add custom resource attributes
	if len(cfg.ResourceAttributes) > 0 {
		attrs := make([]attribute.KeyValue, 0, len(cfg.ResourceAttributes))
		for k, v := range cfg.ResourceAttributes {
			attrs = append(attrs, attribute.String(k, v))
		}
		res, err = resource.Merge(res, resource.NewWithAttributes(semconv.SchemaURL, attrs...))
		if err != nil {
			return nil, fmt.Errorf("failed to merge custom attributes: %w", err)
		}
	}

	// Create exporter
	var exporter sdktrace.SpanExporter
	switch cfg.Exporter.Type {
	case ExporterTypeJaeger:
		if cfg.Exporter.Jaeger.CollectorEndpoint != "" {
			exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(cfg.Exporter.Jaeger.CollectorEndpoint)))
		} else if cfg.Exporter.Jaeger.AgentEndpoint != "" {
			exporter, err = jaeger.New(jaeger.WithAgentEndpoint(jaeger.WithAgentHost(cfg.Exporter.Jaeger.AgentEndpoint)))
		} else {
			return nil, fmt.Errorf("jaeger exporter requires either collector endpoint or agent endpoint")
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create jaeger exporter: %w", err)
		}
	case ExporterTypeOTLP:
		ctx := context.Background()
		clientOpts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.Exporter.OTLP.Endpoint),
		}
		if cfg.Exporter.OTLP.Insecure {
			clientOpts = append(clientOpts, otlptracegrpc.WithInsecure())
		}
		if len(cfg.Exporter.OTLP.Headers) > 0 {
			clientOpts = append(clientOpts, otlptracegrpc.WithHeaders(cfg.Exporter.OTLP.Headers))
		}
		client := otlptracegrpc.NewClient(clientOpts...)
		exporter, err = otlptrace.New(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("failed to create otlp exporter: %w", err)
		}
	case ExporterTypeNone, ExporterTypeStdout:
		// No exporter or stdout exporter
		return &Tracer{
			config: cfg,
			tracer: otel.Tracer(cfg.ServiceName),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", cfg.Exporter.Type)
	}

	// Create tracer provider
	sampler := sdktrace.TraceIDRatioBased(cfg.Sampling.SamplingRate)
	if cfg.Sampling.ParentBased {
		sampler = sdktrace.ParentBased(sampler)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator for context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracer{
		config:   cfg,
		tracer:   tp.Tracer(cfg.ServiceName),
		provider: tp,
	}, nil
}

// Start starts a new span
func (t *Tracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !t.config.Enabled {
		return ctx, trace.SpanFromContext(ctx)
	}
	return t.tracer.Start(ctx, spanName, opts...)
}

// StartWithAttributes starts a new span with attributes
func (t *Tracer) StartWithAttributes(ctx context.Context, spanName string, attrs map[string]interface{}) (context.Context, trace.Span) {
	spanOpts := []trace.SpanStartOption{
		trace.WithAttributes(convertAttributes(attrs)...),
	}
	return t.Start(ctx, spanName, spanOpts...)
}

// AddEvent adds an event to the current span
func (t *Tracer) AddEvent(ctx context.Context, name string, attrs map[string]interface{}) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(convertAttributes(attrs)...))
	}
}

// RecordError records an error in the current span
func (t *Tracer) RecordError(ctx context.Context, err error, attrs map[string]interface{}) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.RecordError(err, trace.WithAttributes(convertAttributes(attrs)...))
	}
}

// SetAttributes sets attributes on the current span
func (t *Tracer) SetAttributes(ctx context.Context, attrs map[string]interface{}) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(convertAttributes(attrs)...)
	}
}

// Shutdown shuts down the tracer provider
func (t *Tracer) Shutdown(ctx context.Context) error {
	if t.provider != nil {
		return t.provider.Shutdown(ctx)
	}
	return nil
}

// convertAttributes converts map to OpenTelemetry attributes
func convertAttributes(attrs map[string]interface{}) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		switch val := v.(type) {
		case string:
			result = append(result, attribute.String(k, val))
		case int:
			result = append(result, attribute.Int(k, val))
		case int64:
			result = append(result, attribute.Int64(k, val))
		case float64:
			result = append(result, attribute.Float64(k, val))
		case bool:
			result = append(result, attribute.Bool(k, val))
		case []string:
			result = append(result, attribute.StringSlice(k, val))
		default:
			result = append(result, attribute.String(k, fmt.Sprintf("%v", val)))
		}
	}
	return result
}

// Extract extracts the trace context from a carrier
func (t *Tracer) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if !t.config.Enabled {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// Inject injects the trace context into a carrier
func (t *Tracer) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	if t.config.Enabled {
		otel.GetTextMapPropagator().Inject(ctx, carrier)
	}
}

// IsEnabled returns whether tracing is enabled
func (t *Tracer) IsEnabled() bool {
	return t.config.Enabled
}
