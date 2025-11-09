# StreamBus Kubernetes Deployment Guide

This guide covers deployment patterns and best practices for running StreamBus on Kubernetes.

## Table of Contents

- [Pre-Deployment Checklist](#pre-deployment-checklist)
- [Deployment Patterns](#deployment-patterns)
- [Sizing and Capacity Planning](#sizing-and-capacity-planning)
- [Storage Configuration](#storage-configuration)
- [Network Configuration](#network-configuration)
- [Security Best Practices](#security-best-practices)
- [Monitoring Setup](#monitoring-setup)
- [Backup and Disaster Recovery](#backup-and-disaster-recovery)

## Pre-Deployment Checklist

Before deploying StreamBus, ensure:

- [ ] Kubernetes cluster version 1.19+
- [ ] Sufficient node resources for your workload
- [ ] StorageClass configured with appropriate performance characteristics
- [ ] Network policies configured (if using)
- [ ] TLS certificates generated (for production)
- [ ] Monitoring stack deployed (Prometheus, Grafana)
- [ ] Backup solution configured
- [ ] Resource quotas and limits defined

## Deployment Patterns

### Pattern 1: Development/Testing

**Use Case**: Local development, feature testing, CI/CD pipelines

**Configuration**:
- 1 replica (single broker)
- Minimal resources (250m CPU, 256Mi memory)
- Standard storage
- No TLS
- Debug logging

```yaml
spec:
  replicas: 1
  resources:
    requests:
      cpu: 250m
      memory: 256Mi
  storage:
    class: standard
    size: 5Gi
  config:
    logLevel: debug
  security:
    tls:
      enabled: false
```

**Deployment**:
```bash
kubectl apply -f deploy/kubernetes/examples/dev-cluster.yaml
```

### Pattern 2: Staging

**Use Case**: Pre-production testing, integration testing

**Configuration**:
- 3 replicas (minimal quorum)
- Moderate resources (500m CPU, 1Gi memory)
- Fast storage
- TLS enabled
- Info logging

```yaml
spec:
  replicas: 3
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
  storage:
    class: fast-ssd
    size: 20Gi
  config:
    logLevel: info
  security:
    tls:
      enabled: true
```

### Pattern 3: Production

**Use Case**: Production workloads requiring high availability

**Configuration**:
- 5+ replicas (for zone redundancy)
- High resources (2+ CPU, 4Gi+ memory)
- Premium SSD storage
- TLS with mTLS authentication
- Structured JSON logging
- Full observability stack
- Anti-affinity rules

```yaml
spec:
  replicas: 5
  resources:
    requests:
      cpu: 2000m
      memory: 4Gi
  storage:
    class: premium-ssd
    size: 100Gi
  security:
    tls:
      enabled: true
      clientAuthRequired: true
    authentication:
      enabled: true
      method: mtls
  observability:
    metrics:
      enabled: true
      serviceMonitor: true
    tracing:
      enabled: true
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - streambus-broker
          topologyKey: topology.kubernetes.io/zone
```

**Deployment**:
```bash
kubectl apply -f deploy/kubernetes/examples/production-cluster.yaml
```

### Pattern 4: Multi-Tenant SaaS

**Use Case**: Multi-tenant platforms with tenant isolation

**Configuration**:
- 5+ replicas
- High resources
- Premium storage
- Full security (TLS, auth, authz)
- Tenant quotas and isolation
- Per-tenant metrics
- Audit logging

```yaml
spec:
  replicas: 5
  multiTenancy:
    enabled: true
    isolation:
      networkPolicies: true
      resourceQuotas: true
      rateLimiting: true
    defaultQuotas:
      maxTopics: 100
      maxConnections: 1000
    tenantOverrides:
      - tenantId: "premium-tenant"
        quotas:
          maxTopics: 500
  observability:
    metrics:
      perTenantMetrics: true
    logging:
      includeTenantContext: true
      auditLog:
        enabled: true
```

**Deployment**:
```bash
kubectl apply -f deploy/kubernetes/examples/multi-tenant-cluster.yaml
```

## Sizing and Capacity Planning

### Broker Resource Estimation

**CPU Requirements**:
- Base: 250m per broker
- Per 1000 connections: +100m
- Per 1000 messages/sec: +200m
- With TLS: +50% overhead

**Memory Requirements**:
- Base: 256Mi per broker
- Per connection: +100KB
- Per topic partition: +10MB (buffer cache)
- Per active consumer: +5MB

**Storage Requirements**:
- Data volume: Expected message retention size
- Raft volume: ~20% of data volume
- Recommended: Separate fast SSD for Raft logs

### Example Sizing

**Small Workload** (1000 msgs/sec, 100 connections):
```yaml
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 1000m
    memory: 1Gi
storage:
  size: 10Gi
  raftSize: 5Gi
```

**Medium Workload** (10,000 msgs/sec, 1000 connections):
```yaml
resources:
  requests:
    cpu: 2000m
    memory: 4Gi
  limits:
    cpu: 4000m
    memory: 8Gi
storage:
  size: 100Gi
  raftSize: 20Gi
```

**Large Workload** (100,000 msgs/sec, 10,000 connections):
```yaml
resources:
  requests:
    cpu: 8000m
    memory: 16Gi
  limits:
    cpu: 16000m
    memory: 32Gi
storage:
  size: 500Gi
  raftSize: 100Gi
```

## Storage Configuration

### Storage Class Selection

**Development/Testing**:
- Standard HDD storage
- No replication required
- Cost-effective

```yaml
storage:
  class: standard
  size: 10Gi
  raftSize: 5Gi
```

**Production**:
- SSD storage (AWS gp3, Azure Premium SSD, GCP pd-ssd)
- Replication for durability
- High IOPS

```yaml
storage:
  class: fast-ssd
  size: 100Gi
  raftSize: 50Gi
```

**High Performance**:
- NVMe SSDs (AWS io2, Azure Ultra Disk, GCP pd-extreme)
- Maximum IOPS and throughput
- For latency-sensitive workloads

```yaml
storage:
  class: ultra-ssd
  size: 500Gi
  raftSize: 100Gi
```

### Cloud Provider Storage Classes

**AWS EKS**:
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: streambus-storage
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  iops: "3000"
  throughput: "125"
  encrypted: "true"
allowVolumeExpansion: true
```

**GCP GKE**:
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: streambus-storage
provisioner: pd.csi.storage.gke.io
parameters:
  type: pd-ssd
  replication-type: regional-pd
allowVolumeExpansion: true
```

**Azure AKS**:
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: streambus-storage
provisioner: disk.csi.azure.com
parameters:
  skuName: Premium_LRS
  kind: Managed
allowVolumeExpansion: true
```

## Network Configuration

### Service Types

**ClusterIP** (default):
- Internal cluster access only
- Recommended for most deployments
- Use Ingress or LoadBalancer for external access

```yaml
# Created automatically by operator
apiVersion: v1
kind: Service
metadata:
  name: streambus-minimal
spec:
  type: ClusterIP
  ports:
    - name: broker
      port: 9092
      targetPort: 9092
```

**LoadBalancer**:
For external client access:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: streambus-external
spec:
  type: LoadBalancer
  selector:
    app.kubernetes.io/name: streambus-broker
    app.kubernetes.io/instance: streambus-minimal
  ports:
    - name: broker
      port: 9092
      targetPort: 9092
```

**Ingress**:
For HTTP/gRPC access:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: streambus-ingress
  annotations:
    nginx.ingress.kubernetes.io/backend-protocol: "GRPC"
spec:
  rules:
    - host: streambus.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: streambus-minimal
                port:
                  number: 9093
```

### Network Policies

Restrict traffic to StreamBus pods:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: streambus-network-policy
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: streambus-broker
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow from client namespace
    - from:
        - namespaceSelector:
            matchLabels:
              name: client-apps
      ports:
        - protocol: TCP
          port: 9092
    # Allow inter-broker communication
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: streambus-broker
      ports:
        - protocol: TCP
          port: 9093
  egress:
    # Allow DNS
    - to:
        - namespaceSelector:
            matchLabels:
              name: kube-system
      ports:
        - protocol: UDP
          port: 53
    # Allow inter-broker
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: streambus-broker
```

## Security Best Practices

### 1. Enable TLS

Always use TLS in production:

```bash
# Generate certificates
openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt \
  -days 365 -nodes -subj "/CN=*.streambus.svc.cluster.local"

# Create secret
kubectl create secret tls streambus-tls-certs \
  --cert=tls.crt \
  --key=tls.key
```

### 2. Enable Authentication

Use mTLS or JWT for authentication:

```yaml
spec:
  security:
    tls:
      enabled: true
      certSecretName: streambus-tls-certs
      clientAuthRequired: true
    authentication:
      enabled: true
      method: mtls
```

### 3. Enable Authorization

Use role-based access control:

```yaml
spec:
  security:
    authorization:
      enabled: true
      defaultPolicy: deny
```

### 4. Pod Security

Apply pod security standards:

```yaml
spec:
  podSecurityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000
    seccompProfile:
      type: RuntimeDefault

  securityContext:
    allowPrivilegeEscalation: false
    capabilities:
      drop:
        - ALL
    readOnlyRootFilesystem: true
```

### 5. Network Isolation

Use NetworkPolicies to restrict traffic:

```yaml
spec:
  multiTenancy:
    enabled: true
    isolation:
      networkPolicies: true
```

## Monitoring Setup

### Prometheus

Install Prometheus Operator:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack
```

Enable ServiceMonitor in StreamBus:

```yaml
spec:
  observability:
    metrics:
      enabled: true
      serviceMonitor: true
```

### Grafana Dashboards

Import StreamBus dashboards:

```bash
kubectl apply -f deploy/kubernetes/monitoring/dashboards/
```

### Distributed Tracing

Deploy Jaeger:

```bash
kubectl apply -f https://github.com/jaegertracing/jaeger-operator/releases/download/v1.50.0/jaeger-operator.yaml
```

Configure StreamBus:

```yaml
spec:
  observability:
    tracing:
      enabled: true
      endpoint: "jaeger-collector.observability.svc:4317"
      samplingRate: 0.1
```

## Backup and Disaster Recovery

### Volume Snapshots

Create VolumeSnapshot:

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: streambus-backup-20240115
spec:
  volumeSnapshotClassName: csi-snapclass
  source:
    persistentVolumeClaimName: data-streambus-minimal-0
```

### Automated Backups with Velero

Install Velero:

```bash
velero install \
  --provider aws \
  --bucket streambus-backups \
  --backup-location-config region=us-east-1 \
  --snapshot-location-config region=us-east-1
```

Create backup schedule:

```bash
velero schedule create streambus-daily \
  --schedule="0 2 * * *" \
  --include-namespaces default \
  --selector app.kubernetes.io/name=streambus-broker
```

### Disaster Recovery Procedure

1. **Stop client traffic**:
```bash
kubectl scale deployment client-app --replicas=0
```

2. **Create backup**:
```bash
velero backup create streambus-disaster-backup \
  --include-namespaces default \
  --selector app.kubernetes.io/name=streambus-broker \
  --wait
```

3. **Restore in new cluster**:
```bash
# In new cluster
velero restore create --from-backup streambus-disaster-backup --wait
```

4. **Verify cluster**:
```bash
kubectl get streambuscluster
kubectl get pods -l app.kubernetes.io/name=streambus-broker
```

5. **Resume traffic**:
```bash
kubectl scale deployment client-app --replicas=3
```

## Upgrade Procedures

### Operator Upgrade

```bash
# Using Helm
helm upgrade streambus-operator streambus/streambus-operator \
  --version 0.2.0

# Verify
kubectl get pods -n streambus-system
```

### Cluster Upgrade (Rolling Update)

1. Update image in StreamBusCluster:
```yaml
spec:
  image:
    tag: v0.7.0
```

2. Apply changes:
```bash
kubectl apply -f my-cluster.yaml
```

3. Monitor rollout:
```bash
kubectl rollout status statefulset/streambus-minimal
```

### Cluster Upgrade (Blue-Green)

1. Deploy new cluster with new version:
```yaml
apiVersion: streambus.io/v1alpha1
kind: StreamBusCluster
metadata:
  name: streambus-v2
spec:
  image:
    tag: v0.7.0
  # ... rest of config
```

2. Migrate clients to new cluster

3. Delete old cluster:
```bash
kubectl delete streambuscluster streambus-v1
```

## Performance Tuning

### Pod Resources

Set appropriate requests and limits:

```yaml
resources:
  requests:
    cpu: 2000m
    memory: 4Gi
  limits:
    cpu: 4000m
    memory: 8Gi
```

### Storage Performance

Use fast storage classes:

```yaml
storage:
  class: premium-ssd
  size: 100Gi
  raftSize: 50Gi
```

### Network Optimization

Enable host networking for better performance (if security allows):

```yaml
spec:
  hostNetwork: true
```

### Broker Configuration

Tune broker settings:

```yaml
config:
  maxConnections: 10000
  maxMessageSize: 10485760  # 10MB
  compressionEnabled: true
  batchSize: 1000
  batchTimeout: 10ms
```

## Troubleshooting

See [README.md](README.md#troubleshooting) for detailed troubleshooting guide.

## Additional Resources

- [Kubernetes Best Practices](https://kubernetes.io/docs/concepts/configuration/overview/)
- [StatefulSet Documentation](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
- [Storage Classes](https://kubernetes.io/docs/concepts/storage/storage-classes/)
- [Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
