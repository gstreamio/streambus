# Multi-stage Dockerfile for StreamBus Operator
# Stage 1: Builder
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files (from operator directory)
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the operator binary
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o manager \
    ./main.go

# Stage 2: Runtime
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy binary from builder
COPY --from=builder /build/manager /manager

# Use nonroot user (uid 65532)
USER 65532:65532

# Expose ports
EXPOSE 8080  # Metrics
EXPOSE 8081  # Health probes

# Health check (distroless doesn't support HEALTHCHECK)
# Health will be checked by Kubernetes probes

ENTRYPOINT ["/manager"]
