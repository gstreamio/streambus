# Multi-stage Dockerfile for StreamBus UI
# Stage 1: Build frontend
FROM node:18-alpine AS frontend-builder

WORKDIR /app/frontend

# Copy frontend package files
COPY ui/frontend/package*.json ./
RUN npm ci

# Copy frontend source
COPY ui/frontend ./

# Build frontend
RUN npm run build

# Stage 2: Build backend
FROM golang:1.21-alpine AS backend-builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make ca-certificates tzdata

# Copy go mod files
COPY ui/backend/go.* ./
RUN go mod download

# Copy backend source
COPY ui/backend ./

# Copy built frontend
COPY --from=frontend-builder /app/frontend/dist ./static

# Build backend
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o streambus-ui \
    .

# Stage 3: Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata curl

WORKDIR /app

# Copy binary and static files
COPY --from=backend-builder /app/streambus-ui /app/streambus-ui
COPY --from=backend-builder /app/static /app/static

# Create non-root user
RUN addgroup -g 1000 streambus && \
    adduser -D -u 1000 -G streambus streambus && \
    chown -R streambus:streambus /app

USER streambus

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/streambus-ui"]
