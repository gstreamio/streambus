# StreamBus Operational Runbooks

Quick-reference operational procedures for production incidents and maintenance.

## Emergency Contact

- **On-Call**: PagerDuty rotation
- **Team Channel**: #streambus-ops
- **Escalation**: See [Escalation Matrix](#escalation-matrix)

## Quick Health Check

```bash
# Single-command cluster health
./scripts/cluster-health-check.sh

# Expected output:
# ✓ broker-1: healthy (leader)
# ✓ broker-2: healthy
# ✓ broker-3: healthy
# ✓ Raft: quorum OK (3/3)
# ✓ Partitions: balanced
# ✓ Disk: 45% used
```

---

## Incident Response Matrix

| Severity | Description | Response Time | Actions |
|----------|-------------|---------------|---------|
| **SEV1** | Complete outage, data loss risk | Immediate | Page on-call, all hands |
| **SEV2** | Degraded performance, partial outage | 15 minutes | Notify team, investigate |
| **SEV3** | Non-critical issue, no user impact | 1 hour | Create ticket, monitor |

---

## SEV1: Complete Cluster Outage

**Symptoms**: All brokers down, no leader, complete unavailability

### Immediate Actions (0-5 minutes)

```bash
# 1. Check all brokers
for broker in broker1 broker2 broker3; do
  echo "=== $broker ==="
  ssh $broker "systemctl status streambus-broker"
  curl -s http://$broker:8080/health || echo "DOWN"
done

# 2. Check recent deployments
kubectl rollout history deployment/streambus-broker

# 3. Check system resources
for broker in broker1 broker2 broker3; do
  ssh $broker "df -h && free -h"
done
```

### Recovery Procedure

**Option A: Restart brokers sequentially**
```bash
# Start with former leader (check logs for last known leader)
ssh broker1 "sudo systemctl start streambus-broker"
sleep 10

# Wait for leader election
curl http://broker1:8080/metrics | grep raft_state

# Start followers
for broker in broker2 broker3; do
  ssh $broker "sudo systemctl start streambus-broker"
  sleep 5
done

# Verify quorum
./scripts/check-quorum.sh
```

**Option B: Force new election (if Option A fails)**
```bash
# Stop all brokers
parallel ssh {} "sudo systemctl stop streambus-broker" ::: broker1 broker2 broker3

# Clear Raft state (CAUTION: only if necessary)
ssh broker1 "sudo rm -rf /var/lib/streambus/raft-state"

# Start in specific order
ssh broker1 "sudo systemctl start streambus-broker"
sleep 15
ssh broker2 "sudo systemctl start streambus-broker"
sleep 10
ssh broker3 "sudo systemctl start streambus-broker"
```

**Option C: Restore from backup (last resort)**
```bash
./scripts/restore-latest-backup.sh
```

### Verification
```bash
# Check health
./scripts/cluster-health-check.sh

# Verify data
./scripts/verify-data-integrity.sh

# Monitor for 30 minutes
watch -n 10 './scripts/cluster-health-check.sh'
```

---

## SEV1: Split Brain

**Symptoms**: Multiple leaders, inconsistent data, conflict errors

### Detection
```bash
# Check leader on all brokers
for broker in broker1 broker2 broker3; do
  leader=$(curl -s http://$broker:8080/metrics | grep 'raft_state.*leader' | wc -l)
  term=$(curl -s http://$broker:8080/admin/raft/status | jq .term)
  echo "$broker: leader=$leader, term=$term"
done

# WARNING: If more than one broker reports leader=1
```

### Recovery Procedure
```bash
# 1. Identify legitimate leader (highest term)
# From output above, find broker with highest term number

# 2. Stop all non-legitimate leaders
ssh broker2 "sudo systemctl stop streambus-broker"
ssh broker3 "sudo systemctl stop streambus-broker"

# 3. Let legitimate leader stabilize
sleep 30

# 4. Restart followers (they will sync from leader)
ssh broker2 "sudo systemctl start streambus-broker"
sleep 10
ssh broker3 "sudo systemctl start streambus-broker"

# 5. Verify single leader
./scripts/check-quorum.sh
```

---

## SEV1: Data Loss

**Symptoms**: Missing messages, offset gaps, corruption errors

### Assessment
```bash
# Check data integrity
./scripts/check-data-integrity.sh

# Identify affected partitions
curl http://broker1:8080/admin/partitions/health | jq '.unhealthy'

# Check backup availability
aws s3 ls s3://streambus-backups/ | tail -10
```

### Recovery Options

**Option 1: Restore partition from replica**
```bash
# Stop affected broker
ssh broker2 "sudo systemctl stop streambus-broker"

# Delete corrupted partition
ssh broker2 "sudo rm -rf /var/lib/streambus/data/topic-123/partition-0"

# Restart (will resync from leader)
ssh broker2 "sudo systemctl start streambus-broker"

# Monitor sync progress
watch 'curl -s http://broker2:8080/metrics | grep sync_progress'
```

**Option 2: Restore from backup**
```bash
# Identify backup point
./scripts/list-backups.sh

# Restore specific partition
./scripts/restore-partition.sh \
  --backup-id 20251110-120000 \
  --topic events \
  --partition 0 \
  --broker broker2
```

### Communication Template
```
Subject: [SEV1] Data Loss Detected in topic: {TOPIC}

Status: Under Investigation
Impact: Messages between offset {START} and {END} may be missing
Action: Restoring from backup
ETA: {TIME}
Updates: Every 15 minutes in #incidents
```

---

## SEV2: High Latency

**Symptoms**: P99 > 1s, slow produces/consumes, client timeouts

### Diagnosis Script
```bash
#!/bin/bash
# diagnose-latency.sh

echo "=== Broker Metrics ==="
curl -s http://broker1:8080/metrics | grep -E 'latency|duration'

echo "=== System Resources ==="
for broker in broker1 broker2 broker3; do
  echo "--- $broker ---"
  ssh $broker "top -b -n 1 | head -20"
  ssh $broker "iostat -x 1 3"
done

echo "=== Network Latency ==="
for broker in broker1 broker2 broker3; do
  echo "$broker: $(ping -c 5 $broker | grep avg | awk -F'/' '{print $5}') ms"
done

echo "=== Recent Slow Queries ==="
curl -s http://broker1:8080/admin/slow-queries | jq '.[0:10]'
```

### Common Fixes

**High CPU**
```bash
# Check hotspots
curl http://broker1:8080/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof -top cpu.prof

# Temporary: Scale up
kubectl scale deployment streambus-broker --replicas=5

# Permanent: Vertical scaling
kubectl set resources deployment streambus-broker \
  --limits=cpu=8,memory=16Gi
```

**Disk I/O Bottleneck**
```bash
# Check I/O wait
iostat -x 1

# Trigger compaction
curl -X POST http://broker1:8080/admin/storage/compact

# Adjust flush interval
# In config: storage.flush_interval: "5s"
```

**Network Saturation**
```bash
# Check bandwidth
iftop -i eth0

# Enable compression
# In config: network.compression: true

# Rate limit producers
curl -X POST http://broker1:8080/admin/quotas \
  -d '{"default": {"produce_rate": 10000}}'
```

---

## SEV2: Disk Space Critical

**Symptoms**: Disk > 90%, write failures, broker crashes

### Immediate Actions
```bash
# 1. Check usage
df -h /var/lib/streambus

# 2. Emergency: Enable read-only mode
curl -X POST http://broker1:8080/admin/readonly

# 3. Free up space immediately
# Delete old log files
find /var/log/streambus -name "*.log.*" -mtime +7 -delete

# Clear temporary files
rm -rf /var/lib/streambus/tmp/*

# 4. Trigger aggressive compaction
curl -X POST http://broker1:8080/admin/storage/compact?aggressive=true
```

### Data Archival
```bash
#!/bin/bash
# archive-old-segments.sh

ARCHIVE_DATE=$(date -d "30 days ago" +%s)

# Find old segments
for segment in /var/lib/streambus/data/*/segments/*.seg; do
  seg_time=$(stat -c %Y "$segment")
  if [ $seg_time -lt $ARCHIVE_DATE ]; then
    # Archive to S3
    aws s3 cp "$segment" s3://streambus-archive/$(basename $segment)
    rm "$segment"
  fi
done

# Update indices
curl -X POST http://broker1:8080/admin/storage/rebuild-index
```

### Prevention
```yaml
# Add to config
storage:
  retention:
    max_bytes: 100GB  # Per topic
    max_age: 7d

  compaction:
    enabled: true
    interval: 1h
    target_size: 50GB
```

---

## SEV3: Memory Leak

**Symptoms**: Gradual memory increase, eventual OOM

### Investigation
```bash
# 1. Monitor memory over time
watch -n 60 'ps aux | grep streambus-broker'

# 2. Heap dump
curl http://broker1:8080/debug/pprof/heap > heap-$(date +%s).prof

# 3. Analyze with pprof
go tool pprof -http=:8081 heap-$(date +%s).prof
# Open browser to localhost:8081

# 4. Check goroutine leaks
curl http://broker1:8080/debug/pprof/goroutine?debug=2 > goroutines.txt
grep -c "goroutine " goroutines.txt
```

### Temporary Mitigation
```bash
# Rolling restart to clear memory
./scripts/rolling-restart.sh

# Set memory limits
kubectl set resources deployment streambus-broker \
  --limits=memory=8Gi
```

### Permanent Fix
```bash
# Enable aggressive GC
# Add to config or env:
GOGC=50 ./bin/streambus-broker

# Set max memory limit in code
# server:
#   max_memory: 8GB
```

---

## Routine Maintenance

### Weekly Health Check
```bash
#!/bin/bash
# weekly-health-check.sh

echo "=== Cluster Status ==="
./scripts/cluster-health-check.sh

echo "=== Disk Usage ==="
for broker in broker1 broker2 broker3; do
  ssh $broker "df -h /var/lib/streambus"
done

echo "=== Backup Status ==="
aws s3 ls s3://streambus-backups/ | tail -5

echo "=== Slow Queries ==="
curl -s http://broker1:8080/admin/slow-queries | jq '.[0:5]'

echo "=== Error Rates ==="
curl -s http://broker1:8080/metrics | grep error_total

echo "=== Partition Balance ==="
curl -s http://broker1:8080/admin/partitions | \
  jq '.partitions | group_by(.leader) | map({broker: .[0].leader, count: length})'
```

### Monthly Maintenance
```bash
#!/bin/bash
# monthly-maintenance.sh

# 1. Update packages
sudo apt update && sudo apt upgrade

# 2. Rotate logs
sudo logrotate /etc/logrotate.d/streambus

# 3. Cleanup old data
./scripts/cleanup-old-segments.sh --older-than 30d

# 4. Compact storage
curl -X POST http://broker1:8080/admin/storage/compact

# 5. Verify backups
./scripts/test-restore.sh --backup latest

# 6. Security scan
./scripts/security-audit.sh

# 7. Review metrics
./scripts/generate-monthly-report.sh
```

---

## Scaling Operations

### Add Broker
```bash
# 1. Provision new server
terraform apply -target=aws_instance.streambus-broker-4

# 2. Configure broker
scp config/broker-template.yaml broker4:/etc/streambus/broker.yaml
ssh broker4 "sed -i 's/BROKER_ID/4/' /etc/streambus/broker.yaml"

# 3. Start broker
ssh broker4 "sudo systemctl start streambus-broker"

# 4. Verify cluster membership
curl http://broker1:8080/admin/cluster/members | jq .

# 5. Trigger rebalancing
curl -X POST http://broker1:8080/admin/partitions/rebalance

# 6. Monitor rebalancing
watch 'curl -s http://broker1:8080/admin/partitions/rebalance-status'
```

### Remove Broker
```bash
# 1. Drain partitions
curl -X POST http://broker1:8080/admin/brokers/4/drain

# 2. Wait for empty
until curl -s http://broker1:8080/admin/brokers/4/partitions | jq '.count' | grep -q '^0$'; do
  echo "Waiting for drain..."
  sleep 10
done

# 3. Decommission
curl -X POST http://broker1:8080/admin/cluster/leave \
  -d '{"broker_id": 4}'

# 4. Stop broker
ssh broker4 "sudo systemctl stop streambus-broker"

# 5. Remove from config
# Update cluster config files

# 6. Deprovision
terraform destroy -target=aws_instance.streambus-broker-4
```

---

## Backup & Recovery

### Manual Backup
```bash
#!/bin/bash
# manual-backup.sh

BACKUP_DIR="/backup/manual-$(date +%Y%m%d-%H%M%S)"

# 1. Trigger snapshots
for broker in broker1 broker2 broker3; do
  curl -X POST http://$broker:8080/admin/raft/snapshot
done

sleep 5

# 2. Backup data
rsync -av --exclude='*.tmp' \
  broker1:/var/lib/streambus/data/ \
  $BACKUP_DIR/data/

# 3. Backup metadata
curl http://broker1:8080/admin/metadata/export > $BACKUP_DIR/metadata.json

# 4. Backup configs
for broker in broker1 broker2 broker3; do
  scp $broker:/etc/streambus/broker.yaml $BACKUP_DIR/config-$broker.yaml
done

# 5. Compress and upload
tar -czf $BACKUP_DIR.tar.gz $BACKUP_DIR
aws s3 cp $BACKUP_DIR.tar.gz s3://streambus-backups/
rm -rf $BACKUP_DIR $BACKUP_DIR.tar.gz
```

### Restore Procedure
```bash
#!/bin/bash
# restore.sh <backup-file>

BACKUP=$1

# 1. Download backup
aws s3 cp s3://streambus-backups/$BACKUP /tmp/

# 2. Stop cluster
./scripts/stop-cluster.sh

# 3. Extract
tar -xzf /tmp/$BACKUP -C /tmp/

# 4. Restore data
for broker in broker1 broker2 broker3; do
  ssh $broker "sudo rm -rf /var/lib/streambus/data/*"
  rsync -av /tmp/backup/data/ $broker:/var/lib/streambus/data/
done

# 5. Start cluster
./scripts/start-cluster.sh

# 6. Verify
./scripts/cluster-health-check.sh
```

---

## Monitoring Alerts

### Critical Alerts

**Broker Down**
```yaml
alert: BrokerDown
expr: up{job="streambus"} == 0
for: 1m
action: Page on-call immediately
```

**No Quorum**
```yaml
alert: NoQuorum
expr: sum(raft_state{state="leader"}) != 1
for: 1m
action: Page on-call immediately
```

**Data Loss Risk**
```yaml
alert: ReplicationBehind
expr: raft_commit_index - raft_applied_index > 10000
for: 5m
action: Notify team, investigate
```

### Warning Alerts

**High Latency**
```yaml
alert: HighLatency
expr: histogram_quantile(0.99, request_duration_seconds_bucket) > 1
for: 5m
action: Investigate, may need scaling
```

**Disk Space Low**
```yaml
alert: DiskSpaceLow
expr: (disk_free / disk_total) < 0.2
for: 5m
action: Clean up or add storage
```

**High Error Rate**
```yaml
alert: HighErrorRate
expr: rate(errors_total[5m]) > 10
for: 5m
action: Check logs, investigate
```

---

## Troubleshooting Checklist

### Broker Won't Start
- [ ] Check logs: `journalctl -u streambus-broker -n 100`
- [ ] Verify config: `./bin/streambus-broker --validate-config`
- [ ] Check ports: `lsof -i :9092`
- [ ] Verify permissions: `ls -la /var/lib/streambus`
- [ ] Check disk space: `df -h`
- [ ] Review system logs: `dmesg | tail -50`

### Network Issues
- [ ] Ping brokers: `ping broker1`
- [ ] Check DNS: `nslookup broker1`
- [ ] Verify firewall: `iptables -L`
- [ ] Test connectivity: `telnet broker1 9092`
- [ ] Check network metrics: `netstat -s`

### Performance Issues
- [ ] Check metrics: `curl http://broker1:8080/metrics`
- [ ] Review slow queries: `curl http://broker1:8080/admin/slow-queries`
- [ ] Check CPU: `top`
- [ ] Check disk I/O: `iostat -x 1`
- [ ] Network throughput: `iftop`
- [ ] Profile CPU: `curl http://broker1:8080/debug/pprof/profile`

---

## Escalation Matrix

| Level | When | Who | How |
|-------|------|-----|-----|
| L1 | Simple issues, monitoring | Ops team | Slack/email |
| L2 | Complex issues, SEV2/3 | On-call engineer | PagerDuty |
| L3 | SEV1, architecture questions | Tech lead | Phone + PagerDuty |
| L4 | Critical outages, major incidents | CTO/VP Eng | Emergency hotline |

---

## Useful Commands Reference

```bash
# Health checks
curl http://broker:8080/health
curl http://broker:8080/health/ready
curl http://broker:8080/health/live

# Metrics
curl http://broker:8080/metrics
curl http://broker:8080/metrics | grep raft

# Admin APIs
curl http://broker:8080/admin/cluster/members
curl http://broker:8080/admin/partitions
curl http://broker:8080/admin/topics

# Debugging
curl http://broker:8080/debug/pprof/goroutine?debug=2
curl http://broker:8080/debug/pprof/heap > heap.prof
curl http://broker:8080/debug/pprof/profile?seconds=30 > cpu.prof

# System
systemctl status streambus-broker
journalctl -u streambus-broker -f
ps aux | grep streambus
lsof -i :9092
netstat -an | grep 9092
```

---

**Document Owner**: Platform Operations Team
**Last Updated**: 2025-11-10
**Review Schedule**: Monthly
**Feedback**: #streambus-ops
