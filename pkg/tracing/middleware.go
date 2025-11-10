package tracing

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
)

// HTTPMiddleware returns an HTTP middleware that traces requests
func (t *Tracer) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !t.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Extract trace context from headers
		ctx := t.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Start span
		ctx, span := t.StartWithAttributes(ctx, r.URL.Path, map[string]interface{}{
			"http.method":      r.Method,
			"http.url":         r.URL.String(),
			"http.host":        r.Host,
			"http.scheme":      r.URL.Scheme,
			"http.user_agent":  r.UserAgent(),
			"http.remote_addr": r.RemoteAddr,
		})
		defer span.End()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Serve request
		next.ServeHTTP(wrapped, r.WithContext(ctx))

		// Record response status
		span.SetAttributes(convertAttributes(map[string]interface{}{
			"http.status_code": wrapped.statusCode,
		})...)

		if wrapped.statusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(wrapped.statusCode))
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// HeaderCarrierFromMap creates a propagation carrier from a map
func HeaderCarrierFromMap(m map[string]string) propagation.TextMapCarrier {
	return propagation.MapCarrier(m)
}

// InjectTraceContext injects trace context into a map (for message headers)
func (t *Tracer) InjectTraceContext(ctx context.Context, headers map[string]string) {
	if t.config.Enabled {
		t.Inject(ctx, HeaderCarrierFromMap(headers))
	}
}

// ExtractTraceContext extracts trace context from a map (from message headers)
func (t *Tracer) ExtractTraceContext(ctx context.Context, headers map[string]string) context.Context {
	if !t.config.Enabled {
		return ctx
	}
	return t.Extract(ctx, HeaderCarrierFromMap(headers))
}
