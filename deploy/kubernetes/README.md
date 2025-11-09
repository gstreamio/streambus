# StreamBus Kubernetes Operator

The StreamBus Kubernetes Operator enables declarative deployment and management of StreamBus clusters on Kubernetes.

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Examples](#examples)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)
- [Development](#development)

## Features

- **Declarative Management**: Define StreamBus clusters using Kubernetes custom resources
- **Automated Lifecycle**: Handles cluster creation, updates, scaling, and deletion
- **High Availability**: Built-in support for multi-replica clusters with anti-affinity
- **Persistent Storage**: Automatic PVC creation and management for data and Raft logs
- **Security**: TLS, mutual TLS authentication, and authorization support
- **Multi-Tenancy**: Tenant isolation with resource quotas and rate limiting
- **Observability**: Prometheus metrics, distributed tracing, and structured logging
- **Rolling Updates**: Zero-downtime updates with StatefulSet rolling update strategy
- **Resource Management**: CPU, memory, and storage limits with pod resource requests
- **Cloud Native**: Runs on any Kubernetes cluster (EKS, GKE, AKS, OpenShift, etc.)

## Architecture

The operator follows the Kubernetes operator pattern with these components:

### Custom Resource Definition (CRD)

The `StreamBusCluster` CRD defines the desired state of a StreamBus cluster:

```yaml
apiVersion: streambus.io/v1alpha1
kind: StreamBusCluster
metadata:
  name: my-cluster
spec:
  replicas: 3
  image:
    repository: streambus/broker
    tag: v0.6.0
  storage:
    class: standard
    size: 10Gi
```

### Controller

The operator controller reconciles `StreamBusCluster` resources by:

1. **ConfigMap**: Creates broker configuration
2. **Services**: Creates headless service (for StatefulSet) and client service
3. **StatefulSet**: Deploys broker pods with persistent storage
4. **Status**: Updates cluster status with phase, conditions, and endpoints

### Resources Created

For each `StreamBusCluster`, the operator creates:

- `ConfigMap`: Broker configuration file
- `Service` (headless): Internal pod-to-pod communication
- `Service` (ClusterIP): External client access
- `StatefulSet`: Broker pods with stable identities
- `PersistentVolumeClaim` (per pod): Data storage
- `PersistentVolumeClaim` (per pod): Raft log storage

## Installation

### Prerequisites

- Kubernetes 1.19+
- kubectl configured to access your cluster
- Helm 3.0+ (for Helm installation method)

### Method 1: Helm Installation (Recommended)

```bash
# Add the StreamBus Helm repository
helm repo add streambus https://charts.streambus.io
helm repo update

# Install the operator
helm install streambus-operator streambus/streambus-operator \
  --namespace streambus-system \
  --create-namespace

# Verify installation
kubectl get pods -n streambus-system
```

### Method 2: Manual Installation

```bash
# Create namespace
kubectl create namespace streambus-system

# Install CRD
kubectl apply -f deploy/kubernetes/operator/config/crd/streambus.io_streambusclusters.yaml

# Install RBAC
kubectl apply -f deploy/kubernetes/operator/config/rbac/rbac.yaml

# Install operator deployment
kubectl apply -f deploy/kubernetes/operator/config/manager/deployment.yaml

# Verify installation
kubectl get pods -n streambus-system
```

### Method 3: Local Development

```bash
# Build operator binary
cd deploy/kubernetes/operator
go build -o bin/manager main.go

# Run locally (requires kubeconfig)
./bin/manager --leader-elect=false
```

## Quick Start

### Deploy a Minimal Cluster

Create a minimal 3-broker cluster:

```bash
kubectl apply -f deploy/kubernetes/examples/minimal-cluster.yaml
```

Wait for the cluster to be ready:

```bash
kubectl get streambuscluster streambus-minimal
kubectl get pods -l app.kubernetes.io/name=streambus-broker
```

### Connect to the Cluster

Get the service endpoint:

```bash
kubectl get service streambus-minimal

# Port forward for local access
kubectl port-forward svc/streambus-minimal 9092:9092
```

Test connectivity:

```bash
# Using StreamBus CLI
streambus-cli --broker localhost:9092 ping

# Using StreamBus client library
# See pkg/client/README.md for examples
```

## Configuration

### StreamBusCluster Specification

#### Basic Configuration

```yaml
apiVersion: streambus.io/v1alpha1
kind: StreamBusCluster
metadata:
  name: my-cluster
  namespace: default
spec:
  # Number of broker replicas (required)
  replicas: 3

  # Container image configuration
  image:
    repository: streambus/broker
    tag: v0.6.0
    pullPolicy: IfNotPresent

  # Resource requirements
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 2000m
      memory: 2Gi

  # Persistent storage
  storage:
    class: standard        # StorageClass name
    size: 10Gi            # Data volume size
    raftSize: 5Gi         # Raft log volume size

  # Broker configuration
  config:
    logLevel: info
    port: 9092
    httpPort: 8081
    grpcPort: 9093
```

#### Security Configuration

```yaml
spec:
  security:
    tls:
      enabled: true
      certSecretName: streambus-tls-certs
      caSecretName: streambus-ca-cert
      clientAuthRequired: true

    authentication:
      enabled: true
      method: mtls  # Options: mtls, jwt
      allowedMethods:
        - mtls
        - jwt

    authorization:
      enabled: true
      defaultPolicy: deny
```

To create TLS certificates:

```bash
# Create self-signed certificates (for testing)
openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt \
  -days 365 -nodes -subj "/CN=streambus-minimal"

kubectl create secret tls streambus-tls-certs \
  --cert=tls.crt \
  --key=tls.key
```

#### Multi-Tenancy Configuration

```yaml
spec:
  multiTenancy:
    enabled: true
    isolation:
      networkPolicies: true
      resourceQuotas: true
      rateLimiting: true

    defaultQuotas:
      maxTopics: 100
      maxPartitionsPerTopic: 10
      maxConnections: 1000
      maxBandwidthMBps: 100
      maxStorageGB: 50
      maxMessagesPerSecond: 10000

    tenantOverrides:
      - tenantId: "premium-tenant"
        quotas:
          maxTopics: 500
          maxConnections: 5000
```

#### Observability Configuration

```yaml
spec:
  observability:
    metrics:
      enabled: true
      serviceMonitor: true  # For Prometheus Operator
      interval: 30s
      perTenantMetrics: true

    tracing:
      enabled: true
      endpoint: "jaeger-collector.observability.svc:4317"
      samplingRate: 0.1
      includeTenantId: true

    logging:
      level: info
      format: json
      includeTenantContext: true
      auditLog:
        enabled: true
        level: detailed
```

#### High Availability Configuration

```yaml
spec:
  # Pod anti-affinity for spreading across nodes
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - streambus-broker
          topologyKey: kubernetes.io/hostname

  # Node selection
  nodeSelector:
    workload: streaming

  # Tolerations for tainted nodes
  tolerations:
    - key: dedicated
      operator: Equal
      value: streambus
      effect: NoSchedule

  # Pod annotations and labels
  podAnnotations:
    prometheus.io/scrape: "true"
  podLabels:
    environment: production
```

### Status Fields

The operator maintains status information:

```yaml
status:
  phase: Running  # Pending, Creating, Running, Updating, Degraded, Failed
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2024-01-15T10:30:00Z"
  replicas: 3
  readyReplicas: 3
  observedGeneration: 1
  endpoints:
    - broker-0.streambus-minimal-headless.default.svc:9092
    - broker-1.streambus-minimal-headless.default.svc:9092
    - broker-2.streambus-minimal-headless.default.svc:9092
```

Check cluster status:

```bash
kubectl get streambuscluster my-cluster -o yaml
kubectl describe streambuscluster my-cluster
```

## Examples

### Development Cluster

Single-node cluster for development:

```bash
kubectl apply -f deploy/kubernetes/examples/dev-cluster.yaml
```

### Production Cluster

High-availability 5-node cluster:

```bash
kubectl apply -f deploy/kubernetes/examples/production-cluster.yaml
```

### Secure Cluster

Cluster with TLS and authentication:

```bash
# Create TLS secrets first
kubectl create secret tls streambus-tls-certs --cert=tls.crt --key=tls.key

# Deploy cluster
kubectl apply -f deploy/kubernetes/examples/secure-cluster.yaml
```

### Multi-Tenant Cluster

Cluster with tenant isolation and quotas:

```bash
kubectl apply -f deploy/kubernetes/examples/multi-tenant-cluster.yaml
```

## Monitoring

### Prometheus Metrics

The operator exposes metrics at `:8080/metrics`:

```yaml
# Operator metrics
streambus_operator_reconcile_total
streambus_operator_reconcile_errors_total
streambus_operator_reconcile_duration_seconds
```

Broker metrics are exposed by each broker pod at `:8081/metrics`.

### ServiceMonitor

If using Prometheus Operator, enable ServiceMonitor:

```yaml
spec:
  observability:
    metrics:
      serviceMonitor: true
```

### Grafana Dashboards

Import the StreamBus dashboards from `deploy/kubernetes/monitoring/dashboards/`:

- Operator Dashboard
- Broker Cluster Dashboard
- Multi-Tenancy Dashboard

## Troubleshooting

### Check Operator Logs

```bash
kubectl logs -n streambus-system deployment/streambus-operator
```

### Check Cluster Status

```bash
# Get cluster overview
kubectl get streambuscluster

# Get detailed status
kubectl describe streambuscluster my-cluster

# Check events
kubectl get events --sort-by='.lastTimestamp' | grep StreamBus
```

### Check Broker Pods

```bash
# Get pods
kubectl get pods -l app.kubernetes.io/name=streambus-broker

# Check pod logs
kubectl logs my-cluster-0

# Debug pod
kubectl exec -it my-cluster-0 -- /bin/sh
```

### Common Issues

#### Pods Not Starting

Check storage class availability:
```bash
kubectl get storageclass
```

Check resource quotas:
```bash
kubectl describe resourcequota -n default
```

#### Cluster Stuck in Creating Phase

Check StatefulSet status:
```bash
kubectl get statefulset
kubectl describe statefulset my-cluster
```

Check PVC status:
```bash
kubectl get pvc
kubectl describe pvc data-my-cluster-0
```

#### Connection Issues

Verify service endpoints:
```bash
kubectl get endpoints my-cluster
```

Test DNS resolution:
```bash
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nslookup my-cluster.default.svc.cluster.local
```

## Development

### Building the Operator

```bash
cd deploy/kubernetes/operator

# Build binary
go build -o bin/manager main.go

# Build Docker image
docker build -t streambus/operator:dev .
```

### Running Tests

```bash
# Unit tests
go test ./...

# Integration tests (requires kind or minikube)
make test-integration
```

### Code Generation

After modifying CRD types:

```bash
# Generate deepcopy methods
controller-gen object paths=./api/v1alpha1/...

# Generate CRD manifests
controller-gen crd paths=./api/v1alpha1/... output:crd:dir=config/crd

# Generate RBAC
controller-gen rbac:roleName=streambus-operator-role paths=./controllers/...
```

### Local Testing with Kind

```bash
# Create kind cluster
kind create cluster --name streambus-test

# Load operator image
kind load docker-image streambus/operator:dev --name streambus-test

# Deploy operator
kubectl apply -f deploy/kubernetes/operator/config/crd/
kubectl apply -f deploy/kubernetes/operator/config/rbac/
kubectl apply -f deploy/kubernetes/operator/config/manager/

# Deploy test cluster
kubectl apply -f deploy/kubernetes/examples/minimal-cluster.yaml
```

## Upgrading

### Upgrading the Operator

```bash
# Using Helm
helm upgrade streambus-operator streambus/streambus-operator \
  --namespace streambus-system

# Manual
kubectl apply -f deploy/kubernetes/operator/config/manager/deployment.yaml
```

### Upgrading StreamBus Clusters

Update the image tag in your StreamBusCluster resource:

```yaml
spec:
  image:
    tag: v0.7.0  # New version
```

Apply the change:

```bash
kubectl apply -f my-cluster.yaml
```

The operator will perform a rolling update of the StatefulSet.

## Uninstalling

### Delete Clusters

```bash
# Delete all StreamBus clusters
kubectl delete streambuscluster --all

# Or delete specific cluster
kubectl delete streambuscluster my-cluster
```

### Uninstall Operator

```bash
# Using Helm
helm uninstall streambus-operator --namespace streambus-system

# Manual
kubectl delete -f deploy/kubernetes/operator/config/manager/
kubectl delete -f deploy/kubernetes/operator/config/rbac/
kubectl delete -f deploy/kubernetes/operator/config/crd/
kubectl delete namespace streambus-system
```

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](../../CONTRIBUTING.md) for guidelines.

## License

See [LICENSE](../../LICENSE) for details.

## Support

- Documentation: https://docs.streambus.io
- GitHub Issues: https://github.com/shawntherrien/streambus/issues
- Community Slack: https://streambus.slack.com
