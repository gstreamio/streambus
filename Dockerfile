# Multi-stage Dockerfile for StreamBus Broker
# Stage 1: Builder
FROM golang:1.21-alpine AS builder

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
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o streambus-broker \
    ./cmd/broker

# Stage 2: Runtime
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl

# Create non-root user
RUN addgroup -g 1000 streambus && \
    adduser -D -u 1000 -G streambus streambus

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/streambus-broker /app/streambus-broker

# Create directories
RUN mkdir -p /data /data/raft /config && \
    chown -R streambus:streambus /data /config /app

# Switch to non-root user
USER streambus

# Expose ports
EXPOSE 9092  # Broker port
EXPOSE 8081  # HTTP/Metrics port
EXPOSE 9093  # gRPC port

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8081/health/live || exit 1

# Volume for persistent data
VOLUME ["/data", "/config"]

# Environment variables
ENV STREAMBUS_DATA_DIR=/data \
    STREAMBUS_PORT=9092 \
    STREAMBUS_HTTP_PORT=8081 \
    STREAMBUS_GRPC_PORT=9093

# Default command
ENTRYPOINT ["/app/streambus-broker"]
CMD ["--config=/config/broker.yaml"]
