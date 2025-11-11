# StreamBus Docker Deployment

Docker deployment configurations for StreamBus.

## Quick Start

### Start a Local 3-Node Cluster

```bash
# From project root
docker-compose up -d

# View logs
docker-compose logs -f

# Check status
docker-compose ps
```

### Access Services

- **Broker 1**: `localhost:9092` (HTTP: `localhost:8081`)
- **Broker 2**: `localhost:9192` (HTTP: `localhost:8181`)
- **Broker 3**: `localhost:9292` (HTTP: `localhost:8281`)
- **Prometheus**: `http://localhost:9090`
- **Grafana**: `http://localhost:3000` (admin/admin)

## Configuration Files

### Broker Configurations

Located in `deploy/docker/configs/`:

- `broker1.yaml`: Configuration for broker 1
- `broker2.yaml`: Configuration for broker 2
- `broker3.yaml`: Configuration for broker 3

Each broker configuration includes:
- Server settings (ID, host, ports)
- Storage paths
- Cluster membership
- Raft configuration
- Logging and metrics

### Prometheus Configuration

`prometheus.yml`: Scrapes metrics from all brokers on port 8081

### Grafana Configuration

`grafana/provisioning/datasources/prometheus.yml`: Auto-configures Prometheus datasource

## Docker Compose Services

### Brokers

Three StreamBus broker nodes forming a Raft cluster:

```yaml
broker1:
  ports:
    - "9092:9092"  # Broker port
    - "8081:8081"  # HTTP/Metrics
    - "9093:9093"  # gRPC
  volumes:
    - broker1-data:/data
    - broker1-raft:/data/raft
```

### Monitoring

- **Prometheus**: Metrics collection and storage
- **Grafana**: Metrics visualization and dashboards

## Common Operations

### Start Cluster

```bash
docker-compose up -d
```

### Stop Cluster

```bash
docker-compose down
```

### Stop and Remove Volumes

```bash
docker-compose down -v
```

### Restart Cluster

```bash
docker-compose restart
```

### View Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f broker1

# Last 100 lines
docker-compose logs --tail=100 broker1
```

### Scale Cluster

To add more brokers, update `docker-compose.yml` and broker configurations.

## Environment Variables

Set in `.env` file:

```env
VERSION=v0.6.0
COMMIT=abc123
BUILD_TIME=2024-01-15T10:00:00Z
```

## Custom Configuration

### Override Broker Settings

Edit broker config files in `deploy/docker/configs/broker*.yaml`

### Change Ports

Modify port mappings in `docker-compose.yml`:

```yaml
ports:
  - "9092:9092"  # host:container
```

### Add Volumes

Add additional volumes in `docker-compose.yml`:

```yaml
volumes:
  - ./my-config:/config
  - ./my-data:/data
```

## Health Checks

Each broker includes health checks:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8081/health/live"]
  interval: 30s
  timeout: 5s
  retries: 3
  start_period: 10s
```

Check health status:

```bash
docker-compose ps
```

## Monitoring

### Prometheus

Access at `http://localhost:9090`

Query examples:
- `streambus_messages_total`: Total messages
- `streambus_topics_total`: Total topics
- `rate(streambus_messages_total[5m])`: Message rate

### Grafana

1. Access at `http://localhost:3000`
2. Login: `admin` / `admin`
3. Prometheus datasource is pre-configured
4. Import dashboards from `deploy/docker/configs/grafana/dashboards/`

## Troubleshooting

### Broker Won't Start

Check logs:
```bash
docker-compose logs broker1
```

Common issues:
- Port already in use
- Configuration file errors
- Volume permission issues

### Connection Refused

Ensure broker is healthy:
```bash
docker-compose ps
curl http://localhost:8081/health/live
```

### Raft Election Issues

Check all brokers are running:
```bash
docker-compose ps
```

View Raft logs:
```bash
docker-compose logs broker1 | grep -i raft
```

### Reset Cluster

Remove all data and restart:
```bash
docker-compose down -v
docker-compose up -d
```

## Production Considerations

### Don't Use in Production

This Docker Compose setup is for **development and testing only**.

For production:
- Use Kubernetes with the StreamBus Operator
- Configure persistent volumes with redundancy
- Enable TLS and authentication
- Set up proper monitoring and alerting
- Configure backups

See [Kubernetes deployment guide](../kubernetes/README.md).

## Building Images

### Build from Source

```bash
# Build broker image
docker build -t streambus/broker:dev \
  --build-arg VERSION=dev \
  -f Dockerfile .

# Build using Make
make docker-build
```

### Use Pre-built Images

```bash
# Pull from registry
docker pull ghcr.io/shawntherrien/streambus/broker:latest

# Update docker-compose.yml
image: ghcr.io/shawntherrien/streambus/broker:latest
```

## Advanced Usage

### Multi-Architecture Support

Build for multiple platforms:

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t streambus/broker:latest .
```

### Custom Network

Use external network:

```yaml
networks:
  streambus-network:
    external: true
    name: my-network
```

### Resource Limits

Set resource constraints:

```yaml
services:
  broker1:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 4G
        reservations:
          cpus: '1'
          memory: 2G
```

## See Also

- [Build Guide](../../BUILD.md)
- [Kubernetes Deployment](../kubernetes/README.md)
- [Operator Guide](../kubernetes/DEPLOYMENT.md)
