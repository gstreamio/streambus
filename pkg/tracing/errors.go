package tracing

import "errors"

var (
	// ErrTracerNotInitialized is returned when tracer is not initialized
	ErrTracerNotInitialized = errors.New("tracer not initialized")

	// ErrInvalidServiceName is returned when service name is invalid
	ErrInvalidServiceName = errors.New("invalid service name")

	// ErrInvalidSamplingRate is returned when sampling rate is invalid
	ErrInvalidSamplingRate = errors.New("invalid sampling rate: must be between 0.0 and 1.0")

	// ErrInvalidExporterType is returned when exporter type is invalid
	ErrInvalidExporterType = errors.New("invalid exporter type")

	// ErrInvalidOTLPEndpoint is returned when OTLP endpoint is invalid
	ErrInvalidOTLPEndpoint = errors.New("invalid OTLP endpoint")

	// ErrInvalidJaegerEndpoint is returned when Jaeger endpoint is invalid
	ErrInvalidJaegerEndpoint = errors.New("invalid Jaeger endpoint: must specify agent or collector endpoint")

	// ErrInvalidZipkinEndpoint is returned when Zipkin endpoint is invalid
	ErrInvalidZipkinEndpoint = errors.New("invalid Zipkin endpoint")

	// ErrExporterShutdownFailed is returned when exporter shutdown fails
	ErrExporterShutdownFailed = errors.New("exporter shutdown failed")

	// ErrTracerProviderShutdownFailed is returned when tracer provider shutdown fails
	ErrTracerProviderShutdownFailed = errors.New("tracer provider shutdown failed")
)
