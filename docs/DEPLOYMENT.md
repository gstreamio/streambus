# StreamBus Deployment Guide

Complete guide for deploying StreamBus in various environments, from local development to production Kubernetes clusters.

## Table of Contents

- [Deployment Overview](#deployment-overview)
- [Prerequisites](#prerequisites)
- [Deployment Methods](#deployment-methods)
  - [1. Binary Deployment](#1-binary-deployment)
  - [2. Docker Deployment](#2-docker-deployment)
  - [3. Docker Compose Deployment](#3-docker-compose-deployment)
  - [4. Kubernetes Deployment](#4-kubernetes-deployment)
  - [5. Helm Chart Deployment](#5-helm-chart-deployment)
- [Configuration](#configuration)
- [Security Setup](#security-setup)
- [Monitoring Setup](#monitoring-setup)
- [High Availability](#high-availability)
- [Scaling](#scaling)
- [Troubleshooting](#troubleshooting)

---

## Deployment Overview

StreamBus can be deployed using multiple methods, each suited for different environments:

| Method | Use Case | Complexity | HA Support |
|--------|----------|------------|------------|
| Binary | Development, testing | Low | No |
| Docker | Development, simple deployments | Low | No |
| Docker Compose | Multi-broker testing | Medium | Yes |
| Kubernetes | Production | High | Yes |
| Helm | Production (recommended) | Medium | Yes |

**Recommended Approach:**
- **Development**: Docker or Binary
- **Staging**: Docker Compose or Kubernetes
- **Production**: Helm Chart on Kubernetes

---

## Prerequisites

### All Deployments
- StreamBus binary or Docker image
- Configuration file (`broker.yaml`)
- Network connectivity on required ports

### Kubernetes/Helm Deployments
- Kubernetes cluster 1.21+
- kubectl configured
- Helm 3.8+ (for Helm deployments)
- PersistentVolume provisioner
- (Optional) Prometheus Operator

---

## Deployment Methods

### 1. Binary Deployment

Best for: Development, testing, single-node deployments

#### Step 1: Download Binary

```bash
# Download from releases (when published)
curl -L https://github.com/shawntherrien/streambus/releases/download/v1.0.0/streambus-broker-linux-amd64 -o streambus-broker
chmod +x streambus-broker

# Or build from source
git clone https://github.com/shawntherrien/streambus.git
cd streambus
make build
```

#### Step 2: Create Configuration

```yaml
# broker.yaml
server:
  listen_addr: "0.0.0.0:9092"
  data_dir: "/var/lib/streambus"

logging:
  level: "info"
  format: "json"

health:
  enabled: true
  listen_addr: ":8080"

metrics:
  enabled: true
  listen_addr: ":8080"
  path: "/metrics"
```

#### Step 3: Run Broker

```bash
# Create data directory
mkdir -p /var/lib/streambus

# Run broker
./streambus-broker --config broker.yaml

# Or as systemd service
sudo cp streambus-broker /usr/local/bin/
sudo cp deploy/systemd/streambus-broker.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable streambus-broker
sudo systemctl start streambus-broker
```

#### Systemd Service File

```ini
# /etc/systemd/system/streambus-broker.service
[Unit]
Description=StreamBus Message Broker
After=network.target

[Service]
Type=simple
User=streambus
Group=streambus
ExecStart=/usr/local/bin/streambus-broker --config /etc/streambus/broker.yaml
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

---

### 2. Docker Deployment

Best for: Development, containerized testing

#### Step 1: Pull Image

```bash
# Pull from Docker Hub (when published)
docker pull streambus/broker:1.0.0

# Or build locally
cd streambus
docker build -t streambus/broker:1.0.0 -f deploy/docker/Dockerfile .
```

#### Step 2: Run Container

```bash
# Simple single-node deployment
docker run -d \
  --name streambus-broker \
  -p 9092:9092 \
  -p 8080:8080 \
  -v /var/lib/streambus:/data \
  streambus/broker:1.0.0

# With custom configuration
docker run -d \
  --name streambus-broker \
  -p 9092:9092 \
  -p 8080:8080 \
  -v $(pwd)/config/broker.yaml:/etc/streambus/broker.yaml:ro \
  -v /var/lib/streambus:/data \
  streambus/broker:1.0.0 \
  --config /etc/streambus/broker.yaml
```

#### Step 3: Verify

```bash
# Check logs
docker logs streambus-broker

# Check health
curl http://localhost:8080/health/live

# Check metrics
curl http://localhost:8080/metrics
```

---

### 3. Docker Compose Deployment

Best for: Multi-broker testing, development clusters

#### Step 1: Create Docker Compose File

```yaml
# deploy/docker-compose-cluster.yml
version: '3.8'

services:
  broker-1:
    image: streambus/broker:1.0.0
    container_name: streambus-broker-1
    hostname: broker-1
    ports:
      - "9092:9092"
      - "8080:8080"
    environment:
      - STREAMBUS_NODE_ID=broker-1
      - STREAMBUS_ADVERTISE_ADDR=broker-1:9092
      - STREAMBUS_CLUSTER_BOOTSTRAP=broker-1:9092,broker-2:9092,broker-3:9092
      - STREAMBUS_LOG_LEVEL=info
    volumes:
      - broker-1-data:/data
    networks:
      - streambus

  broker-2:
    image: streambus/broker:1.0.0
    container_name: streambus-broker-2
    hostname: broker-2
    ports:
      - "9093:9092"
      - "8081:8080"
    environment:
      - STREAMBUS_NODE_ID=broker-2
      - STREAMBUS_ADVERTISE_ADDR=broker-2:9092
      - STREAMBUS_CLUSTER_BOOTSTRAP=broker-1:9092,broker-2:9092,broker-3:9092
      - STREAMBUS_LOG_LEVEL=info
    volumes:
      - broker-2-data:/data
    networks:
      - streambus

  broker-3:
    image: streambus/broker:1.0.0
    container_name: streambus-broker-3
    hostname: broker-3
    ports:
      - "9094:9092"
      - "8082:8080"
    environment:
      - STREAMBUS_NODE_ID=broker-3
      - STREAMBUS_ADVERTISE_ADDR=broker-3:9092
      - STREAMBUS_CLUSTER_BOOTSTRAP=broker-1:9092,broker-2:9092,broker-3:9092
      - STREAMBUS_LOG_LEVEL=info
    volumes:
      - broker-3-data:/data
    networks:
      - streambus

volumes:
  broker-1-data:
  broker-2-data:
  broker-3-data:

networks:
  streambus:
    driver: bridge
```

#### Step 2: Deploy Cluster

```bash
# Start cluster
docker-compose -f deploy/docker-compose-cluster.yml up -d

# Check status
docker-compose -f deploy/docker-compose-cluster.yml ps

# View logs
docker-compose -f deploy/docker-compose-cluster.yml logs -f

# Stop cluster
docker-compose -f deploy/docker-compose-cluster.yml down
```

#### Step 3: Test Cluster

```bash
# Create topic
streambus-cli topic create test-topic --brokers localhost:9092

# Produce messages
streambus-cli produce test-topic --brokers localhost:9092

# Consume messages
streambus-cli consume test-topic --brokers localhost:9092
```

---

### 4. Kubernetes Deployment

Best for: Production deployments without Helm

#### Step 1: Create Namespace

```bash
kubectl create namespace streambus-production
```

#### Step 2: Deploy Using Kustomize

```bash
# Base deployment (3 replicas)
kubectl apply -k deploy/kubernetes/base

# Production overlay (5 replicas, higher resources)
kubectl apply -k deploy/kubernetes/overlays/production
```

#### Step 3: Verify Deployment

```bash
# Check pods
kubectl get pods -n streambus-production

# Check StatefulSet
kubectl get statefulset -n streambus-production

# Check services
kubectl get svc -n streambus-production

# View logs
kubectl logs -n streambus-production streambus-broker-0
```

#### Step 4: Access Cluster

```bash
# Get LoadBalancer IP
export SERVICE_IP=$(kubectl get svc streambus-client -n streambus-production -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "StreamBus available at: $SERVICE_IP:9092"

# Or use port-forward for testing
kubectl port-forward -n streambus-production svc/streambus-client 9092:9092
```

---

### 5. Helm Chart Deployment

Best for: Production deployments (recommended)

#### Step 1: Install Helm Chart

```bash
# Basic installation
helm install streambus deploy/kubernetes/helm/streambus \
  --namespace streambus-production \
  --create-namespace

# Production installation with custom values
helm install streambus deploy/kubernetes/helm/streambus \
  -f deploy/kubernetes/helm/streambus/examples/values-production.yaml \
  --namespace streambus-production \
  --create-namespace
```

#### Step 2: Customize Configuration

```yaml
# custom-values.yaml
replicaCount: 5

image:
  repository: streambus/broker
  tag: "1.0.0"

resources:
  requests:
    cpu: 2000m
    memory: 4Gi
  limits:
    cpu: 8000m
    memory: 16Gi

persistence:
  enabled: true
  size: 500Gi
  storageClass: fast-ssd

monitoring:
  serviceMonitor:
    enabled: true
  prometheusRule:
    enabled: true

config:
  tracing:
    enabled: true
    exporter: jaeger
    endpoint: "http://jaeger-collector:14268/api/traces"
```

```bash
# Install with custom values
helm install streambus deploy/kubernetes/helm/streambus \
  -f custom-values.yaml \
  --namespace streambus-production \
  --create-namespace
```

#### Step 3: Verify Installation

```bash
# Check release status
helm status streambus -n streambus-production

# Check pods
kubectl get pods -n streambus-production -l app.kubernetes.io/name=streambus

# View logs
kubectl logs -n streambus-production -l app.kubernetes.io/name=streambus --tail=100
```

#### Step 4: Upgrade Deployment

```bash
# Upgrade with new values
helm upgrade streambus deploy/kubernetes/helm/streambus \
  -f custom-values.yaml \
  --namespace streambus-production

# Upgrade to new version
helm upgrade streambus deploy/kubernetes/helm/streambus \
  --set image.tag=1.1.0 \
  --namespace streambus-production
```

#### Step 5: Rollback if Needed

```bash
# View history
helm history streambus -n streambus-production

# Rollback to previous version
helm rollback streambus -n streambus-production
```

---

## Configuration

### Basic Configuration

```yaml
# Minimal configuration
server:
  listen_addr: "0.0.0.0:9092"
  data_dir: "/data"

logging:
  level: "info"
```

### Production Configuration

```yaml
# Production-ready configuration
server:
  listen_addr: "0.0.0.0:9092"
  data_dir: "/data"

cluster:
  bootstrap_peers:
    - broker-1:9092
    - broker-2:9092
    - broker-3:9092

raft:
  heartbeat_timeout: 1s
  election_timeout: 3s
  snapshot_interval: 5m

storage:
  segment_size: 1073741824  # 1GB
  retention_bytes: 107374182400  # 100GB
  retention_duration: 168h  # 7 days
  flush_interval: 500ms

replication:
  min_insync_replicas: 2
  replication_factor: 3

performance:
  max_batch_size: 2097152  # 2MB
  batch_timeout: 5ms
  max_concurrent_requests: 2000

logging:
  level: "info"
  format: "json"

health:
  enabled: true
  listen_addr: ":8080"

metrics:
  enabled: true
  listen_addr: ":8080"
  path: "/metrics"

tracing:
  enabled: true
  exporter: "otlp"
  endpoint: "otel-collector:4317"
  sampling_rate: 0.1
```

---

## Security Setup

### TLS Configuration

```yaml
# broker.yaml with TLS
server:
  tls:
    enabled: true
    cert_file: "/etc/certs/server.crt"
    key_file: "/etc/certs/server.key"
    ca_file: "/etc/certs/ca.crt"
```

### SASL Authentication

```yaml
# broker.yaml with SASL
security:
  authentication:
    enabled: true
    mechanism: "SCRAM-SHA-256"

  authorization:
    enabled: true
    acl_backend: "etcd"
```

### Kubernetes Secrets

```bash
# Create TLS secret
kubectl create secret tls streambus-tls \
  --cert=server.crt \
  --key=server.key \
  -n streambus-production

# Mount in Helm values
```yaml
extraVolumes:
  - name: tls-certs
    secret:
      secretName: streambus-tls

extraVolumeMounts:
  - name: tls-certs
    mountPath: /etc/certs
    readOnly: true
```
```

---

## Monitoring Setup

### Prometheus Integration

```yaml
# Helm values for monitoring
monitoring:
  serviceMonitor:
    enabled: true
    interval: 30s
    labels:
      prometheus: kube-prometheus

  prometheusRule:
    enabled: true
    labels:
      prometheus: kube-prometheus
```

### Grafana Dashboard

```bash
# Import Grafana dashboard
kubectl create configmap streambus-dashboard \
  --from-file=dashboards/streambus-overview.json \
  -n monitoring
```

### Alert Configuration

Alerts are automatically configured when using Helm with `monitoring.prometheusRule.enabled: true`.

---

## High Availability

### Minimum HA Setup

- **Replicas**: 3 (minimum for quorum)
- **Anti-affinity**: Spread across nodes/zones
- **Replication Factor**: 3
- **Min ISR**: 2

### Kubernetes HA Configuration

```yaml
# Helm values for HA
replicaCount: 5

podDisruptionBudget:
  enabled: true
  minAvailable: 3

affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
            - key: app.kubernetes.io/name
              operator: In
              values:
                - streambus
        topologyKey: kubernetes.io/hostname

config:
  replication:
    min_insync_replicas: 2
    replication_factor: 3
```

---

## Scaling

### Horizontal Scaling

```bash
# Scale using kubectl
kubectl scale statefulset streambus-broker --replicas=7 -n streambus-production

# Scale using Helm
helm upgrade streambus deploy/kubernetes/helm/streambus \
  --set replicaCount=7 \
  -n streambus-production
```

### Vertical Scaling

```bash
# Increase resources using Helm
helm upgrade streambus deploy/kubernetes/helm/streambus \
  --set resources.requests.cpu=4000m \
  --set resources.requests.memory=8Gi \
  -n streambus-production
```

### Storage Scaling

```bash
# Expand PVC (if storage class supports it)
kubectl patch pvc data-streambus-broker-0 -n streambus-production \
  -p '{"spec":{"resources":{"requests":{"storage":"1Ti"}}}}'
```

---

## Troubleshooting

### Common Issues

#### Pods Not Starting

```bash
# Check pod events
kubectl describe pod streambus-broker-0 -n streambus-production

# Check logs
kubectl logs streambus-broker-0 -n streambus-production

# Common causes:
# - Insufficient resources
# - PVC provisioning failure
# - Image pull errors
# - Configuration errors
```

#### Cluster Not Forming

```bash
# Check if pods can reach each other
kubectl exec -it streambus-broker-0 -n streambus-production -- \
  nc -zv streambus-broker-1.streambus-broker 9092

# Check Raft status
kubectl logs streambus-broker-0 -n streambus-production | grep -i raft

# Common causes:
# - Network policies blocking traffic
# - Incorrect bootstrap peers
# - DNS resolution issues
```

#### High Latency

```bash
# Check metrics
kubectl port-forward svc/streambus-metrics 8080:8080 -n streambus-production
curl http://localhost:8080/metrics | grep latency

# Check resource usage
kubectl top pods -n streambus-production

# Common causes:
# - Resource constraints
# - Disk I/O bottleneck
# - Network congestion
```

### Debug Mode

```bash
# Enable debug logging
helm upgrade streambus deploy/kubernetes/helm/streambus \
  --set config.logging.level=debug \
  -n streambus-production

# View detailed logs
kubectl logs -f streambus-broker-0 -n streambus-production
```

### Health Checks

```bash
# Liveness check
kubectl exec streambus-broker-0 -n streambus-production -- \
  curl -f http://localhost:8080/health/live

# Readiness check
kubectl exec streambus-broker-0 -n streambus-production -- \
  curl -f http://localhost:8080/health/ready
```

---

## Best Practices

### Development
1. Use Docker or binary deployment
2. Single replica is sufficient
3. Enable debug logging
4. Use small retention settings

### Staging
1. Use Docker Compose or Kubernetes
2. 3 replicas minimum
3. Test failover scenarios
4. Monitor resource usage

### Production
1. **Always use Helm** for easy management
2. **Minimum 3 replicas**, recommend 5
3. **Enable monitoring** (Prometheus, Grafana)
4. **Enable distributed tracing** for debugging
5. **Use PodDisruptionBudget** for HA
6. **Configure resource limits** appropriately
7. **Use fast storage** (SSD/NVMe)
8. **Enable network policies** for security
9. **Regular backups** of configuration and data
10. **Test disaster recovery** procedures

---

## Next Steps

- [Getting Started Guide](GETTING_STARTED.md)
- [Production Readiness Checklist](PRODUCTION_READINESS.md)
- [Operational Runbooks](OPERATIONAL_RUNBOOKS.md)
- [Monitoring Guide](MONITORING.md)
- [Security Guide](SECURITY.md)

---

**Need Help?**
- Documentation: `docs/README.md`
- Issues: GitHub Issues
- Discussions: GitHub Discussions
