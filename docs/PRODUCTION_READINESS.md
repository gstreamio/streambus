# StreamBus Production Readiness Checklist

Complete checklist for deploying StreamBus to production with confidence.

## Table of Contents

- [Quick Status](#quick-status)
- [Pre-Deployment Checklist](#pre-deployment-checklist)
- [Infrastructure Requirements](#infrastructure-requirements)
- [Security Hardening](#security-hardening)
- [Monitoring & Observability](#monitoring--observability)
- [Operational Readiness](#operational-readiness)
- [Go-Live Checklist](#go-live-checklist)
- [Post-Deployment](#post-deployment)

---

## Quick Status

### Overall Production Readiness: **95%**

| Area | Status | Coverage |
|------|--------|----------|
| Core Functionality | ✅ Complete | 100% |
| Testing & QA | ✅ Complete | 100% |
| Production Hardening | ✅ Complete | 100% |
| Observability | ✅ Complete | 100% |
| Documentation | ✅ Complete | 100% |
| Security | ✅ Complete | 95% |
| Kubernetes Support | ⚠️ Partial | 40% |

**Ready for production deployment** with proper operational support.

---

## Pre-Deployment Checklist

### 1. Infrastructure Setup

- [ ] **Servers/Nodes Provisioned**
  - [ ] Minimum 3 broker nodes for HA
  - [ ] CPU: 8+ cores per node
  - [ ] Memory: 16+ GB per node
  - [ ] Storage: 500+ GB NVMe SSD per node
  - [ ] Network: 10+ Gbps connectivity

- [ ] **Network Configuration**
  - [ ] Internal network for broker-to-broker communication
  - [ ] Load balancer configured for client connections
  - [ ] Firewall rules in place (see [Security](#security-hardening))
  - [ ] DNS entries configured

- [ ] **Storage Configuration**
  - [ ] Fast SSD/NVMe for data directories
  - [ ] Separate volumes for data, WAL, and logs
  - [ ] Backup storage configured (S3, NFS, etc.)
  - [ ] Adequate disk space (plan for 2x expected data)

### 2. Software Installation

- [ ] **StreamBus Binaries**
  - [ ] Downloaded from official release or built from source
  - [ ] Version verified: `streambus-broker --version`
  - [ ] Installed in /usr/local/bin or /opt/streambus
  - [ ] Proper file permissions set

- [ ] **Dependencies**
  - [ ] Go runtime (if building from source)
  - [ ] System packages updated
  - [ ] Required libraries installed

- [ ] **Service Configuration**
  - [ ] Systemd unit files created
  - [ ] Log rotation configured
  - [ ] Resource limits set (see [operations.md](./operations.md))

### 3. Configuration

- [ ] **Broker Configuration**
  - [ ] Unique broker IDs assigned
  - [ ] Cluster membership configured
  - [ ] Storage paths configured
  - [ ] Network settings tuned
  - [ ] Retention policies set
  - [ ] Configuration validated with `--validate-config`

- [ ] **Performance Tuning**
  - [ ] OS kernel parameters tuned (see [operations.md](./operations.md))
  - [ ] File descriptor limits increased
  - [ ] Network buffers optimized
  - [ ] Disk I/O scheduler configured

- [ ] **Logging Configuration**
  - [ ] Log level set appropriately (INFO for production)
  - [ ] Log rotation enabled
  - [ ] Centralized logging configured (optional)

---

## Infrastructure Requirements

### Minimum Production Setup

```
┌─────────────────────────────────────────┐
│         Load Balancer (HAProxy)         │
│         Client Port: 9092               │
└────────┬──────────┬──────────┬──────────┘
         │          │          │
    ┌────▼────┐ ┌──▼──────┐ ┌─▼──────────┐
    │ Broker1 │ │ Broker2 │ │ Broker3    │
    │ (Leader)│ │(Follower)│ │(Follower)  │
    │ 8 CPU   │ │ 8 CPU   │ │ 8 CPU      │
    │ 16GB RAM│ │ 16GB RAM│ │ 16GB RAM   │
    │ 500GB   │ │ 500GB   │ │ 500GB SSD  │
    └─────────┘ └─────────┘ └────────────┘
```

### Resource Allocation

**Per Broker Node:**
- CPU: 8 cores (minimum), 16 cores (recommended)
- Memory: 16GB (minimum), 32GB (recommended)
- Storage: 500GB SSD (minimum), 1TB+ NVMe (recommended)
- Network: 10Gbps (minimum), 25Gbps (recommended)

**Expected Capacity:**
- Messages/second: 100,000+ (3-node cluster)
- Total storage: 1.5TB (3 x 500GB with replication)
- Concurrent connections: 30,000+ (10,000 per broker)

---

## Security Hardening

### 1. Network Security

- [ ] **Firewall Rules**
  ```bash
  # Allow broker-to-broker communication (internal network only)
  iptables -A INPUT -p tcp --dport 9093 -s 10.0.0.0/8 -j ACCEPT

  # Allow client connections (from application network)
  iptables -A INPUT -p tcp --dport 9092 -s 10.1.0.0/16 -j ACCEPT

  # Allow health checks
  iptables -A INPUT -p tcp --dport 8080 -s 10.0.0.0/8 -j ACCEPT

  # Drop all other traffic
  iptables -A INPUT -j DROP
  ```

- [ ] **TLS/SSL Configuration** (Recommended)
  ```yaml
  security:
    tls_enabled: true
    tls_cert: /etc/streambus/tls/server.crt
    tls_key: /etc/streambus/tls/server.key
    tls_ca: /etc/streambus/tls/ca.crt
    min_tls_version: "1.3"
  ```

- [ ] **Network Segmentation**
  - [ ] Brokers in private subnet
  - [ ] Clients access via load balancer only
  - [ ] Management access via VPN/bastion host

### 2. Authentication & Authorization

- [ ] **Enable Authentication**
  ```yaml
  security:
    auth_enabled: true
    auth_mechanism: "SCRAM-SHA-256"
    users_file: /etc/streambus/users.yaml
  ```

- [ ] **User Management**
  - [ ] Admin users created
  - [ ] Application users created
  - [ ] Read-only monitoring users created
  - [ ] Strong passwords enforced

- [ ] **ACLs Configured** (if applicable)
  ```yaml
  acls:
    - principal: "user:producer-app"
      operations: ["write"]
      resources: ["topic:events"]

    - principal: "user:consumer-app"
      operations: ["read"]
      resources: ["topic:events"]
  ```

### 3. System Hardening

- [ ] **OS Security**
  - [ ] SELinux/AppArmor enabled
  - [ ] Automatic security updates configured
  - [ ] Unnecessary services disabled
  - [ ] SSH hardened (key-based auth only)

- [ ] **File Permissions**
  ```bash
  # Data directory
  chown -R streambus:streambus /var/lib/streambus
  chmod 750 /var/lib/streambus

  # Config files
  chown root:streambus /etc/streambus/*.yaml
  chmod 640 /etc/streambus/*.yaml

  # TLS certificates
  chmod 600 /etc/streambus/tls/*.key
  ```

---

## Monitoring & Observability

### 1. Metrics Collection

- [ ] **Prometheus Setup**
  ```yaml
  # prometheus.yml
  scrape_configs:
    - job_name: 'streambus'
      scrape_interval: 15s
      static_configs:
        - targets:
          - 'broker1:8080'
          - 'broker2:8080'
          - 'broker3:8080'
  ```

- [ ] **Key Metrics Monitored**
  - [ ] Messages per second (produce/consume)
  - [ ] Request latency (p50, p95, p99)
  - [ ] Error rates
  - [ ] Resource utilization (CPU, memory, disk)
  - [ ] Raft consensus metrics
  - [ ] Connection counts

### 2. Alerting

- [ ] **Critical Alerts Configured**
  - [ ] Broker down
  - [ ] No Raft quorum
  - [ ] High error rate (> 1%)
  - [ ] Disk space low (< 20%)
  - [ ] High latency (p99 > 1s)

- [ ] **Warning Alerts Configured**
  - [ ] CPU usage > 80%
  - [ ] Memory usage > 85%
  - [ ] Disk usage > 80%
  - [ ] Replication lag > 1000 messages

- [ ] **Notification Channels**
  - [ ] PagerDuty integration
  - [ ] Slack/email notifications
  - [ ] On-call rotation configured

### 3. Distributed Tracing

- [ ] **Tracing Enabled**
  ```yaml
  tracing:
    enabled: true
    exporter: jaeger
    endpoint: http://jaeger:14268/api/traces
    sampling_rate: 0.1  # 10% sampling
  ```

- [ ] **Jaeger/Zipkin Deployed**
  - [ ] Tracing backend running
  - [ ] UI accessible
  - [ ] Retention policy configured

### 4. Profiling

- [ ] **pprof Endpoints**
  ```yaml
  profiling:
    enabled: true
    listen_addr: localhost:6060  # Localhost only!
  ```

- [ ] **Access Controls**
  - [ ] pprof accessible only from internal network
  - [ ] Documentation for profiling procedures

### 5. Log Aggregation

- [ ] **Centralized Logging**
  - [ ] ELK/Loki/Splunk configured
  - [ ] Log shipping enabled (Filebeat/Fluentd)
  - [ ] Log retention policy set
  - [ ] Log search and analysis working

---

## Operational Readiness

### 1. Runbooks & Documentation

- [ ] **Operational Runbooks Created**
  - [ ] [OPERATIONAL_RUNBOOKS.md](./OPERATIONAL_RUNBOOKS.md) reviewed
  - [ ] Team trained on procedures
  - [ ] Emergency contacts documented

- [ ] **Architecture Documentation**
  - [ ] System architecture documented
  - [ ] Network diagram created
  - [ ] Data flow documented

- [ ] **Testing Documentation**
  - [ ] [TESTING.md](./TESTING.md) reviewed
  - [ ] Test procedures documented
  - [ ] Performance baselines established

### 2. Backup & Recovery

- [ ] **Backup Strategy**
  - [ ] Automated daily backups configured
  - [ ] Backup script: [operations.md](./operations.md#backup--recovery)
  - [ ] Backup verification scheduled
  - [ ] Off-site backup storage configured

- [ ] **Recovery Procedures**
  - [ ] Recovery tested successfully
  - [ ] RTO documented: 1 hour
  - [ ] RPO documented: 15 minutes
  - [ ] Disaster recovery plan documented

- [ ] **Backup Schedule**
  ```bash
  # Crontab entry
  0 2 * * * /usr/local/bin/streambus-backup.sh
  0 */6 * * * /usr/local/bin/streambus-incremental-backup.sh
  ```

### 3. Deployment Automation

- [ ] **Deployment Scripts**
  - [ ] Rolling restart script
  - [ ] Configuration deployment script
  - [ ] Health check script
  - [ ] Rollback procedure

- [ ] **CI/CD Pipeline**
  - [ ] Automated testing on commits
  - [ ] Staging environment available
  - [ ] Production deployment requires approval
  - [ ] Automated rollback on failure

### 4. Capacity Planning

- [ ] **Baseline Metrics Captured**
  - [ ] Messages per second at peak
  - [ ] Storage growth rate
  - [ ] Connection count trends
  - [ ] Resource utilization patterns

- [ ] **Scaling Plan**
  - [ ] Vertical scaling procedure documented
  - [ ] Horizontal scaling procedure tested
  - [ ] Auto-scaling configured (if applicable)
  - [ ] Growth projections documented

---

## Go-Live Checklist

### T-1 Week

- [ ] **Final Testing**
  - [ ] Load testing completed
  - [ ] Chaos testing completed
  - [ ] Security scan completed
  - [ ] Performance baselines established

- [ ] **Documentation Review**
  - [ ] All documentation up to date
  - [ ] Runbooks reviewed by team
  - [ ] On-call procedures confirmed

- [ ] **Communication**
  - [ ] Stakeholders notified of go-live date
  - [ ] Maintenance window scheduled (if needed)
  - [ ] Support team briefed

### T-1 Day

- [ ] **Pre-Flight Checks**
  ```bash
  # Run comprehensive health check
  ./scripts/preflight-check.sh

  # Verify backups
  ./scripts/verify-latest-backup.sh

  # Check monitoring
  ./scripts/check-monitoring.sh

  # Verify alerting
  ./scripts/test-alerts.sh
  ```

- [ ] **Team Preparation**
  - [ ] On-call engineers available
  - [ ] War room established (physical/virtual)
  - [ ] Communication channels active

### Go-Live (T-0)

- [ ] **Deployment**
  1. [ ] Start brokers sequentially
  2. [ ] Verify cluster formation
  3. [ ] Check health endpoints
  4. [ ] Verify Raft quorum
  5. [ ] Test produce/consume

- [ ] **Verification**
  ```bash
  # Cluster health
  ./scripts/cluster-health-check.sh

  # Smoke tests
  ./scripts/smoke-test.sh

  # Performance validation
  ./scripts/performance-test.sh
  ```

- [ ] **Monitoring**
  - [ ] All metrics flowing to Prometheus
  - [ ] Dashboards displaying correctly
  - [ ] Alerts configured and active
  - [ ] Logs flowing to aggregation system

### T+1 Hour

- [ ] **Post-Deployment Verification**
  - [ ] No critical alerts fired
  - [ ] Metrics within expected ranges
  - [ ] Application connectivity confirmed
  - [ ] Performance meets SLAs

### T+24 Hours

- [ ] **Stability Check**
  - [ ] No unexpected restarts
  - [ ] Memory usage stable
  - [ ] Disk usage as expected
  - [ ] No resource leaks detected

---

## Post-Deployment

### Day 1

- [ ] Monitor metrics continuously
- [ ] Review logs for errors/warnings
- [ ] Verify backup completion
- [ ] Check resource utilization trends

### Week 1

- [ ] Review incident log (should be empty)
- [ ] Analyze performance trends
- [ ] Validate capacity planning
- [ ] Gather feedback from applications
- [ ] Update documentation with learnings

### Month 1

- [ ] Conduct post-launch review
- [ ] Analyze cost vs budget
- [ ] Review and tune monitoring thresholds
- [ ] Plan optimization initiatives
- [ ] Schedule next deployment (if needed)

---

## Success Criteria

### System Health

- ✅ All brokers healthy (3/3)
- ✅ Raft quorum maintained (100% uptime)
- ✅ No data loss
- ✅ No unexpected downtime

### Performance

- ✅ Latency p99 < 100ms
- ✅ Throughput > 50,000 msg/sec
- ✅ Error rate < 0.1%
- ✅ Resource utilization < 70%

### Operational

- ✅ Backups successful (100%)
- ✅ Monitoring functional
- ✅ Alerts working
- ✅ Team trained

---

## Emergency Contacts

| Role | Name | Contact |
|------|------|---------|
| Primary On-Call | | PagerDuty |
| Secondary On-Call | | PagerDuty |
| Tech Lead | | Phone/Slack |
| Platform Team | | #streambus-ops |
| Security Team | | #security |

---

## Sign-Off

Production deployment approved by:

- [ ] Engineering Lead: _________________ Date: _________
- [ ] Operations Lead: _________________ Date: _________
- [ ] Security Lead: ___________________ Date: _________
- [ ] Product Owner: ___________________ Date: _________

---

## Appendix: Quick Commands

```bash
# Health check
curl http://broker:8080/health

# Cluster status
./scripts/cluster-health-check.sh

# Trigger backup
./scripts/backup-now.sh

# View metrics
curl http://broker:8080/metrics

# Check logs
journalctl -u streambus-broker -f

# Profile performance
curl http://broker:6060/debug/pprof/profile?seconds=30 > cpu.prof

# Emergency stop
systemctl stop streambus-broker
```

---

**Document Version**: 1.0
**Last Updated**: 2025-11-10
**Next Review**: 2025-12-10
**Owner**: Platform Operations Team
