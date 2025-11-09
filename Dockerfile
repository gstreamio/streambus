# Multi-stage Dockerfile for StreamBus
# Stage 1: Builder
FROM golang:alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 for static binary
# -ldflags="-w -s" to reduce binary size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty) -X main.commit=$(git rev-parse --short HEAD) -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o streambus-broker \
    ./cmd/broker

# Stage 2: Runtime
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 streambus && \
    adduser -D -u 1000 -G streambus streambus

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/streambus-broker /app/streambus-broker

# Create data directory
RUN mkdir -p /data && \
    chown -R streambus:streambus /data /app

# Switch to non-root user
USER streambus

# Expose ports
# 9000 - Broker communication (default port)
EXPOSE 9000

# Health check (will be implemented in Phase 3)
# HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
#     CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health/live || exit 1

# Volume for persistent data
VOLUME ["/data"]

# Default command
ENTRYPOINT ["/app/streambus-broker"]
CMD ["--data-dir=/data"]
