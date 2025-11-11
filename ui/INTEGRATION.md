# StreamBus Web Management UI - Broker Integration

This document describes the complete integration between the StreamBus Web Management UI and the broker's admin API.

## Architecture Overview

The Web Management UI consists of two layers:

1. **Frontend** (React + TypeScript + Vite) - Located in `ui/frontend/`
2. **Backend** (Go + Gin) - Located in `ui/backend/`

The backend acts as a proxy between the frontend and the StreamBus broker's admin API.

```
┌──────────────┐        ┌─────────────┐        ┌──────────────┐
│   Frontend   │  HTTP  │   Backend   │  HTTP  │    Broker    │
│  (React UI)  │ ────>  │  (Go/Gin)   │ ────>  │  (Admin API) │
│  Port 5173   │        │  Port 3000  │        │  Port 8081   │
└──────────────┘        └─────────────┘        └──────────────┘
```

## Broker Admin API Endpoints

The broker exposes the following RESTful admin API endpoints on port `:8081`:

### Cluster Endpoints

- **GET** `/api/v1/cluster` - Get cluster information
  ```json
  {
    "cluster_id": "streambus-cluster-1",
    "controller_id": 1,
    "version": "1.0.0",
    "total_brokers": 3,
    "active_brokers": 3,
    "total_topics": 10,
    "total_partitions": 50,
    "uptime": "2h15m30s"
  }
  ```

### Broker Endpoints

- **GET** `/api/v1/brokers` - List all brokers
- **GET** `/api/v1/brokers/:id` - Get broker details with resources

### Topic Endpoints

- **GET** `/api/v1/topics` - List all topics
- **POST** `/api/v1/topics` - Create a new topic
  ```json
  {
    "name": "my-topic",
    "num_partitions": 10,
    "replication_factor": 3,
    "config": {}
  }
  ```
- **GET** `/api/v1/topics/:name` - Get topic details
- **DELETE** `/api/v1/topics/:name` - Delete a topic
- **GET** `/api/v1/topics/:name/partitions` - Get partition details
- **GET** `/api/v1/topics/:name/messages` - Browse messages (query params: partition, offset, limit)

### Consumer Group Endpoints

- **GET** `/api/v1/consumer-groups` - List all consumer groups
- **GET** `/api/v1/consumer-groups/:id` - Get consumer group details
- **GET** `/api/v1/consumer-groups/:id/lag` - Get lag information by partition

### Health & Metrics Endpoints

- **GET** `/health` - Overall health status
- **GET** `/health/live` - Liveness probe
- **GET** `/health/ready` - Readiness probe
- **GET** `/metrics` - Prometheus metrics

## Backend Services

The backend has been updated to call the real broker API instead of returning mock data:

### 1. BrokerService (`ui/backend/services/broker_service.go`)

**Updated Methods:**
- `GetClusterInfo()` - Calls `GET /api/v1/cluster`
- `ListBrokers()` - Calls `GET /api/v1/brokers`
- `GetBroker(id)` - Calls `GET /api/v1/brokers/:id`
- `Ping(broker)` - Calls `GET /health/live`

### 2. TopicService (`ui/backend/services/topic_service.go`)

**Updated Methods:**
- `ListTopics()` - Calls `GET /api/v1/topics`
- `GetTopic(name)` - Calls `GET /api/v1/topics/:name`
- `CreateTopic(...)` - Calls `POST /api/v1/topics`
- `DeleteTopic(name)` - Calls `DELETE /api/v1/topics/:name`
- `GetPartitions(topicName)` - Calls `GET /api/v1/topics/:name/partitions`
- `GetMessages(...)` - Calls `GET /api/v1/topics/:name/messages`

### 3. ConsumerGroupService (`ui/backend/services/consumer_group_service.go`)

**Updated Methods:**
- `ListGroups()` - Calls `GET /api/v1/consumer-groups`
- `GetGroup(groupID)` - Calls `GET /api/v1/consumer-groups/:id`
- `GetGroupLag(groupID)` - Calls `GET /api/v1/consumer-groups/:id/lag`

### 4. MetricsService (`ui/backend/services/metrics_service.go`)

**Status:** Using mock data for now. In production, this should query Prometheus which scrapes the broker's `/metrics` endpoint.

## Running the Complete Stack

### 1. Start the StreamBus Broker

```bash
# Build the broker
go build -o bin/streambus-broker ./cmd/broker

# Run with configuration
./bin/streambus-broker --config config/broker.yaml
```

The broker will start with:
- **Protocol Port:** `:9092` (StreamBus protocol)
- **HTTP Port:** `:8081` (Admin API, Health, Metrics)

### 2. Start the UI Backend

```bash
cd ui/backend

# Build and run
go build -o bin/streambus-ui-backend ./main.go
./bin/streambus-ui-backend --brokers localhost:8081 --port 3000
```

The backend will start on port `:3000` and proxy requests to the broker at `localhost:8081`.

### 3. Start the UI Frontend

```bash
cd ui/frontend

# Install dependencies (first time only)
npm install

# Start development server
npm run dev
```

The frontend will start on port `:5173` and connect to the backend at `localhost:3000`.

### 4. Access the UI

Open your browser to: **http://localhost:5173**

## Configuration

### Backend Configuration

The backend accepts the following command-line flags:

```bash
./streambus-ui-backend \
  --brokers localhost:8081 \          # Broker HTTP address
  --port 3000 \                       # Backend server port
  --prometheus http://localhost:9090  # Prometheus URL (optional)
```

### Frontend Configuration

Frontend configuration is in `ui/frontend/vite.config.ts`:

```typescript
export default defineConfig({
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:3000',
        changeOrigin: true,
      },
    },
  },
})
```

## Docker Deployment

Build and run the entire UI stack using Docker:

```bash
# Build the UI image
docker build -t streambus-ui:latest -f ui/Dockerfile .

# Run the container
docker run -d \
  -p 3000:3000 \
  -e BROKERS=broker:8081 \
  -e PROMETHEUS_URL=http://prometheus:9090 \
  --name streambus-ui \
  streambus-ui:latest
```

## Features

The Web Management UI provides:

### Dashboard
- Cluster overview with broker count, topics, and partitions
- Real-time cluster health status
- Active broker list with status indicators

### Topics Management
- List all topics with partition and replication details
- Create new topics with configurable partitions and replication
- Delete topics
- View partition details (leader, replicas, ISR)
- Browse messages in topics

### Consumer Groups
- List all consumer groups with lag monitoring
- View group details and member information
- Monitor per-partition lag
- Track partition assignments

### Brokers
- List all brokers in the cluster
- View broker details and resource usage (CPU, memory, disk)
- Monitor broker health and status

### Metrics
- Real-time throughput charts (messages/sec, bytes/sec)
- Latency percentiles (p50, p95, p99)
- Resource usage visualization (CPU, memory)

## Data Refresh

The UI uses React Query for efficient data fetching and caching:

- **Dashboard:** Refreshes every 5 seconds
- **Broker List:** Refreshes every 5 seconds
- **Topic List:** Refreshes every 10 seconds
- **Consumer Groups:** Refreshes every 5 seconds
- **Metrics:** Refreshes every 30 seconds

## API Response Examples

### Cluster Info Response

```json
{
  "cluster_id": "streambus-cluster-1",
  "controller_id": 1,
  "version": "1.0.0",
  "total_brokers": 3,
  "active_brokers": 3,
  "total_topics": 5,
  "total_partitions": 25,
  "uptime": "4h32m18s"
}
```

### Topic Response

```json
{
  "name": "orders",
  "num_partitions": 10,
  "replication_factor": 3,
  "partitions": [
    {
      "id": 0,
      "leader": 1,
      "replicas": [1, 2, 3],
      "isr": [1, 2, 3],
      "beginning_offset": 0,
      "end_offset": 1000000,
      "message_count": 1000000
    }
  ]
}
```

### Consumer Group Response

```json
{
  "group_id": "order-processors",
  "state": "Stable",
  "protocol": "range",
  "members": [
    {
      "member_id": "consumer-1-uuid",
      "client_id": "order-processor-1",
      "client_host": "10.0.1.5",
      "partitions": [0, 1, 2, 3, 4],
      "joined_at": 1699564800
    }
  ],
  "coordinator": 1,
  "total_lag": 1500
}
```

## Security Considerations

### Production Deployment

For production deployments, consider:

1. **Enable TLS** for all HTTP endpoints
2. **Add authentication** to the admin API (e.g., JWT, OAuth)
3. **Implement rate limiting** on the backend
4. **Use CORS policies** to restrict frontend origins
5. **Enable audit logging** for all admin operations
6. **Set up monitoring** with Prometheus and Grafana

### Example TLS Configuration

```go
// In backend/main.go
tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS13,
    CertFile:   "certs/server.crt",
    KeyFile:    "certs/server.key",
}
```

## Troubleshooting

### Backend cannot connect to broker

**Error:** `failed to call broker API: dial tcp: connection refused`

**Solution:** Ensure the broker is running and the HTTP port (8081) is accessible:
```bash
curl http://localhost:8081/health/live
```

### Frontend shows "Network Error"

**Error:** Frontend displays network error when fetching data

**Solution:** Check that the backend is running on port 3000:
```bash
curl http://localhost:3000/api/cluster
```

### Empty data in UI

**Issue:** UI loads but shows no topics/brokers/groups

**Solution:**
1. Check broker logs for errors in the admin API
2. Verify topics/groups exist in the broker
3. Check browser developer console for API errors

## Future Enhancements

Planned improvements:

1. **Real-time Updates** - WebSocket support for live data streaming
2. **Message Publishing** - Ability to produce messages from the UI
3. **ACL Management** - Configure access control lists
4. **Schema Registry** - Manage Avro/Protobuf schemas
5. **Audit Logs** - View all admin operations
6. **Alerting** - Configure alerts for lag, errors, etc.
7. **Performance Tuning** - Adjust topic configurations from UI
8. **Multi-Cluster** - Support for managing multiple clusters

## Development

### Adding New Endpoints

To add a new endpoint:

1. **Add to Broker Admin API** (`pkg/broker/admin_api.go`)
2. **Update Backend Service** (`ui/backend/services/*.go`)
3. **Update Frontend API Client** (`ui/frontend/src/lib/api.ts`)
4. **Create/Update React Components** (`ui/frontend/src/pages/*.tsx`)

### Running Tests

```bash
# Test backend
cd ui/backend
go test ./...

# Test frontend
cd ui/frontend
npm test
```

## Contributing

When contributing to the Web Management UI:

1. Ensure all backend changes include proper error handling
2. Update type definitions for API responses
3. Add loading and error states in frontend components
4. Test with real broker before submitting PR
5. Update this documentation for new features

## Support

For issues or questions:
- **GitHub Issues:** https://github.com/shawntherrien/streambus/issues
- **Documentation:** https://docs.streambus.io
- **Community:** https://community.streambus.io
