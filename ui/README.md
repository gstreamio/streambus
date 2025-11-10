# StreamBus Web UI

Modern web-based management console for StreamBus.

## Overview

The StreamBus Web UI provides a user-friendly interface for:
- Cluster monitoring and health
- Topic management
- Consumer group monitoring
- Message browsing
- Configuration management
- User/tenant management
- Metrics visualization

## Architecture

```
┌─────────────────┐
│   Frontend      │  React + TypeScript
│   (Port 3000)   │  TailwindCSS + shadcn/ui
└────────┬────────┘
         │ REST API
┌────────▼────────┐
│   Backend       │  Go + Gin
│   (Port 8080)   │
└────────┬────────┘
         │
┌────────▼────────┐
│  StreamBus      │  Broker API
│  Cluster        │  Prometheus
└─────────────────┘
```

## Tech Stack

### Frontend
- **React 18** with TypeScript
- **Vite** for build tooling
- **TailwindCSS** for styling
- **shadcn/ui** for component library
- **React Query** for data fetching
- **React Router** for navigation
- **Recharts** for data visualization
- **Zustand** for state management

### Backend
- **Go** with Gin web framework
- **REST API** for frontend communication
- **WebSocket** for real-time updates
- **Prometheus client** for metrics
- **JWT** for authentication

## Quick Start

### Development

```bash
# Start backend
cd ui/backend
go run main.go

# Start frontend (in another terminal)
cd ui/frontend
npm install
npm run dev
```

Visit http://localhost:3000

### Production

```bash
# Build frontend
cd ui/frontend
npm run build

# Build backend with embedded frontend
cd ui/backend
go build -o streambus-ui

# Run
./streambus-ui --brokers localhost:9092
```

## Features

### 1. Dashboard

- Cluster overview
- Broker health status
- Real-time metrics
- Recent events
- System alerts

### 2. Topics

- List all topics
- Create/delete topics
- View topic details
- Partition information
- Replication status
- Configuration management

### 3. Consumer Groups

- List consumer groups
- Group details
- Consumer lag monitoring
- Partition assignment
- Offset management

### 4. Messages

- Browse messages
- Search messages
- Filter by timestamp
- View message details
- Export messages

### 5. Metrics

- Throughput graphs
- Latency histograms
- Resource usage
- Custom dashboards
- Prometheus integration

### 6. Administration

- User management
- Tenant management
- ACL management
- Configuration
- Audit logs

## Screenshots

[Coming soon]

## API Documentation

See [API.md](./API.md) for REST API documentation.

## Development

### Prerequisites

- Node.js 18+
- Go 1.21+
- StreamBus cluster running

### Setup

```bash
# Clone repository
git clone https://github.com/shawntherrien/streambus
cd streambus/ui

# Install frontend dependencies
cd frontend
npm install

# Install backend dependencies
cd ../backend
go mod download
```

### Running Tests

```bash
# Frontend tests
cd frontend
npm test

# Backend tests
cd backend
go test ./...
```

### Building

```bash
# Build frontend
cd frontend
npm run build

# Build backend
cd backend
go build -o streambus-ui
```

## Docker

```dockerfile
# Dockerfile
FROM node:18-alpine AS frontend-builder
WORKDIR /app/frontend
COPY ui/frontend/package*.json ./
RUN npm ci
COPY ui/frontend ./
RUN npm run build

FROM golang:1.21-alpine AS backend-builder
WORKDIR /app
COPY ui/backend/go.* ./
RUN go mod download
COPY ui/backend ./
COPY --from=frontend-builder /app/frontend/dist ./static
RUN go build -o streambus-ui

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=backend-builder /app/streambus-ui /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["streambus-ui"]
```

## Configuration

### Backend Configuration

```yaml
# config.yaml
server:
  port: 8080
  cors:
    enabled: true
    origins:
      - http://localhost:3000

brokers:
  - localhost:9092
  - localhost:9192
  - localhost:9292

auth:
  enabled: true
  jwt:
    secret: your-secret-key
    expiration: 24h

prometheus:
  url: http://localhost:9090

websocket:
  enabled: true
  pingInterval: 30s
```

### Frontend Configuration

```typescript
// src/config.ts
export const config = {
  apiUrl: import.meta.env.VITE_API_URL || 'http://localhost:8080/api',
  wsUrl: import.meta.env.VITE_WS_URL || 'ws://localhost:8080/ws',
  refreshInterval: 5000, // 5 seconds
}
```

## Environment Variables

### Backend

```bash
STREAMBUS_BROKERS=localhost:9092
STREAMBUS_PORT=8080
STREAMBUS_JWT_SECRET=your-secret
STREAMBUS_PROMETHEUS_URL=http://localhost:9090
```

### Frontend

```bash
VITE_API_URL=http://localhost:8080/api
VITE_WS_URL=ws://localhost:8080/ws
```

## Deployment

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: streambus-ui
spec:
  replicas: 2
  selector:
    matchLabels:
      app: streambus-ui
  template:
    metadata:
      labels:
        app: streambus-ui
    spec:
      containers:
      - name: streambus-ui
        image: streambus/ui:latest
        ports:
        - containerPort: 8080
        env:
        - name: STREAMBUS_BROKERS
          value: "streambus-broker:9092"
        - name: STREAMBUS_PROMETHEUS_URL
          value: "http://prometheus:9090"
---
apiVersion: v1
kind: Service
metadata:
  name: streambus-ui
spec:
  type: LoadBalancer
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app: streambus-ui
```

### Docker Compose

```yaml
version: '3.8'

services:
  streambus-ui:
    image: streambus/ui:latest
    ports:
      - "8080:8080"
    environment:
      - STREAMBUS_BROKERS=broker1:9092,broker2:9092,broker3:9092
      - STREAMBUS_PROMETHEUS_URL=http://prometheus:9090
    depends_on:
      - broker1
      - broker2
      - broker3
```

## Security

### Authentication

The UI supports multiple authentication methods:

- **JWT tokens**: Default method
- **OAuth2**: Integration with external providers
- **LDAP**: Enterprise directory integration

### Authorization

Role-based access control (RBAC):
- **Admin**: Full access
- **Operator**: Manage topics, view metrics
- **Developer**: View-only access
- **Tenant User**: Limited to tenant resources

### TLS

Enable TLS for production:

```yaml
server:
  tls:
    enabled: true
    certFile: /etc/certs/server.crt
    keyFile: /etc/certs/server.key
```

## Contributing

See [CONTRIBUTING.md](../../CONTRIBUTING.md)

## License

See [LICENSE](../../LICENSE)

## Support

- Documentation: https://docs.streambus.io
- GitHub Issues: https://github.com/shawntherrien/streambus/issues
- Slack: https://streambus.slack.com
