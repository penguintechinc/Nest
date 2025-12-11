# Ceph Troubleshooting Guide

**Version:** 1.0.0
**Maintained by:** Penguin Tech Inc
**License:** Limited AGPL3

## Table of Contents

1. [Quick Diagnostics](#quick-diagnostics)
2. [Common Issues](#common-issues)
3. [Health Warnings](#health-warnings)
4. [Component Issues](#component-issues)
5. [Performance Problems](#performance-problems)
6. [Recovery Procedures](#recovery-procedures)
7. [Debugging Tools](#debugging-tools)
8. [Support Resources](#support-resources)

## Quick Diagnostics

### Health Check Commands

```bash
# Basic health status
lxc exec ceph-node-01 -- ceph health
lxc exec ceph-node-01 -- ceph health detail

# Full cluster status
lxc exec ceph-node-01 -- ceph -s

# Detailed status
lxc exec ceph-node-01 -- ceph status

# Watch status (real-time)
lxc exec ceph-node-01 -- ceph -w
```

### Validation Script

```bash
# Run comprehensive validation
./scripts/infrastructure/validate-ceph-cluster.sh -d

# JSON output for automation
./scripts/infrastructure/validate-ceph-cluster.sh -j
```

## Common Issues

### 1. Cloud-Init Failed to Complete

**Symptoms:**
- Container deployed but Ceph not running
- Services not started
- Missing configuration files

**Diagnosis:**
```bash
# Check cloud-init status
lxc exec ceph-node-01 -- cloud-init status

# View cloud-init logs
lxc exec ceph-node-01 -- cat /var/log/cloud-init.log
lxc exec ceph-node-01 -- cat /var/log/cloud-init-output.log
```

**Solution:**
```bash
# Re-run cloud-init
lxc exec ceph-node-01 -- cloud-init clean
lxc restart ceph-node-01

# Or manually run bootstrap
lxc exec ceph-node-01 -- /usr/local/bin/bootstrap-ceph.sh
```

### 2. OSDs Won't Start

**Symptoms:**
- `ceph osd tree` shows OSDs down
- HEALTH_WARN: X osds down

**Diagnosis:**
```bash
# Check OSD status
lxc exec ceph-node-01 -- ceph osd tree
lxc exec ceph-node-01 -- ceph osd stat

# Check OSD logs
lxc exec ceph-node-01 -- journalctl -u ceph-osd@0 -n 100

# Check disk status
lxc exec ceph-node-01 -- ceph-volume lvm list
```

**Solutions:**

**A. Loop device not mounted:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Check loop devices
losetup -a

# Recreate if missing
losetup /dev/loop0 /var/lib/ceph/osd0.img

# Restart OSD
ceph orch daemon restart osd.0
EOF
```

**B. Permissions issue:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Fix ownership
chown -R ceph:ceph /var/lib/ceph/osd/

# Restart OSD
systemctl restart ceph-osd@0
EOF
```

**C. Corrupted OSD:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Mark OSD out and down
ceph osd out 0
ceph osd down 0

# Remove OSD
ceph osd purge 0 --yes-i-really-mean-it

# Recreate OSD
ceph orch daemon add osd ceph-node-01:/dev/loop0
EOF
```

### 3. MON Quorum Lost

**Symptoms:**
- Cluster unresponsive
- "unable to get monitor info from DNS" errors
- Authentication failures

**Diagnosis:**
```bash
# Check MON status
lxc exec ceph-node-01 -- ceph mon stat
lxc exec ceph-node-01 -- ceph quorum_status --format json-pretty

# Check MON logs
lxc exec ceph-node-01 -- journalctl -u ceph-mon@$(hostname) -n 100
```

**Solutions:**

**A. Single MON recovery:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Extract monmap
ceph-mon -i $(hostname) --extract-monmap /tmp/monmap

# Rebuild MON store
ceph-mon -i $(hostname) --mkfs --monmap /tmp/monmap

# Restart MON
systemctl restart ceph-mon@$(hostname)
EOF
```

**B. Recreate MON quorum:**
```bash
# On surviving node
lxc exec ceph-node-01 -- bash << 'EOF'
# Create new monmap with single mon
monmaptool --create --add $(hostname) $(hostname -i) --fsid $(ceph fsid) /tmp/monmap

# Inject monmap
ceph-mon -i $(hostname) --inject-monmap /tmp/monmap

# Restart
systemctl restart ceph-mon@$(hostname)
EOF
```

### 4. Dashboard Not Accessible

**Symptoms:**
- Can't access https://<ip>:8443
- Connection refused or timeout

**Diagnosis:**
```bash
# Check dashboard module
lxc exec ceph-node-01 -- ceph mgr module ls | grep dashboard

# Check dashboard status
lxc exec ceph-node-01 -- ceph mgr services

# Check if port is listening
lxc exec ceph-node-01 -- ss -tlnp | grep 8443
```

**Solutions:**

**A. Enable dashboard:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Enable module
ceph mgr module enable dashboard

# Create self-signed certificate
ceph dashboard create-self-signed-cert

# Set credentials
ceph dashboard set-login-credentials admin admin

# Restart MGR
ceph mgr fail $(ceph mgr dump | grep -oP 'active_name": "\K[^"]+')
EOF
```

**B. Fix SSL certificate:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Generate new certificate
openssl req -new -nodes -x509 \
  -subj "/O=IT/CN=ceph-mgr-dashboard" \
  -days 3650 \
  -keyout dashboard.key \
  -out dashboard.crt

# Set certificate
ceph dashboard set-ssl-certificate -i dashboard.crt
ceph dashboard set-ssl-certificate-key -i dashboard.key

# Restart dashboard
ceph mgr module disable dashboard
ceph mgr module enable dashboard
EOF
```

### 5. PGs Stuck

**Symptoms:**
- HEALTH_WARN: X pgs stuck inactive/unclean/stale

**Diagnosis:**
```bash
# List stuck PGs
lxc exec ceph-node-01 -- ceph pg dump_stuck inactive
lxc exec ceph-node-01 -- ceph pg dump_stuck unclean

# Check PG status
lxc exec ceph-node-01 -- ceph pg stat
lxc exec ceph-node-01 -- ceph pg dump
```

**Solutions:**

**A. Force PG creation:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Query PG
ceph pg <pg-id> query

# Force create if inactive
ceph pg force-create-pg <pg-id>
EOF
```

**B. Unfound objects:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Mark unfound as lost (DATA LOSS!)
# Only as last resort
ceph pg <pg-id> mark_unfound_lost revert
EOF
```

## Health Warnings

### HEALTH_WARN: clock skew detected

**Cause:** Time synchronization issue between nodes

**Solution:**
```bash
# Check time on all nodes
for i in {01..03}; do
    echo "Node $i:"
    lxc exec ceph-node-$i -- date
done

# Fix with chrony
lxc exec ceph-node-01 -- bash << 'EOF'
systemctl restart chrony
chronyc -a makestep
EOF
```

### HEALTH_WARN: too few PGs per OSD

**Cause:** PG count too low for number of OSDs

**Solution:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Increase PG count
ceph osd pool set <pool-name> pg_num 128
ceph osd pool set <pool-name> pgp_num 128

# Enable autoscaler
ceph osd pool set <pool-name> pg_autoscale_mode on
EOF
```

### HEALTH_WARN: pool has too many PGs

**Cause:** PG count too high for number of OSDs

**Solution:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Reduce PG count (can take time)
ceph osd pool set <pool-name> pg_num 64

# Wait for rebalancing
watch ceph -s
EOF
```

### HEALTH_WARN: osds are near full

**Cause:** OSD utilization > 85% (default warning threshold)

**Solution:**
```bash
# Check OSD usage
lxc exec ceph-node-01 -- ceph osd df tree

# Add more OSDs or delete data
# Temporary: Increase threshold
lxc exec ceph-node-01 -- bash << 'EOF'
ceph osd set-nearfull-ratio 0.90
ceph osd set-full-ratio 0.95
EOF
```

## Component Issues

### MGR Module Issues

**Problem:** Module won't enable/disable

```bash
# List modules
lxc exec ceph-node-01 -- ceph mgr module ls

# Force enable
lxc exec ceph-node-01 -- bash << 'EOF'
ceph mgr module enable <module> --force

# Check MGR log
journalctl -u ceph-mgr@$(hostname) -f
EOF
```

### MDS Issues

**Problem:** MDS in damaged state

```bash
# Check MDS status
lxc exec ceph-node-01 -- ceph mds stat
lxc exec ceph-node-01 -- ceph fs status

# Repair filesystem
lxc exec ceph-node-01 -- bash << 'EOF'
# Take filesystem offline
ceph fs set cephfs cluster_down true

# Run repair
ceph mds repaired cephfs:0

# Bring back online
ceph fs set cephfs cluster_down false
EOF
```

### RGW Issues

**Problem:** RGW returns 500 errors

```bash
# Check RGW status
lxc exec ceph-node-01 -- ceph orch ps --daemon_type rgw

# Check RGW logs
lxc exec ceph-node-01 -- journalctl -u ceph-radosgw@* -f

# Restart RGW
lxc exec ceph-node-01 -- ceph orch restart rgw.default
```

### iSCSI Gateway Issues

**Problem:** iSCSI targets not accessible

```bash
# Check service
lxc exec ceph-node-01 -- systemctl status rbd-target-api

# Check configuration
lxc exec ceph-node-01 -- cat /etc/ceph/iscsi-gateway.cfg

# Restart service
lxc exec ceph-node-01 -- systemctl restart rbd-target-api
```

## Performance Problems

### Slow Requests

**Diagnosis:**
```bash
# Check for slow ops
lxc exec ceph-node-01 -- ceph health detail | grep slow

# List slow ops
lxc exec ceph-node-01 -- ceph daemon osd.0 dump_historic_slow_ops

# Check OSD perf
lxc exec ceph-node-01 -- ceph osd perf
```

**Solutions:**

**A. Scrubbing interference:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Limit scrubbing impact
ceph config set osd osd_scrub_sleep 0.1
ceph config set osd osd_scrub_begin_hour 1
ceph config set osd osd_scrub_end_hour 5
EOF
```

**B. Recovery impact:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Limit recovery bandwidth
ceph config set osd osd_recovery_max_active 1
ceph config set osd osd_recovery_sleep_hdd 0.1
EOF
```

### High CPU/Memory Usage

**Diagnosis:**
```bash
# Check resource usage
lxc exec ceph-node-01 -- top
lxc exec ceph-node-01 -- htop

# Check per-OSD usage
lxc exec ceph-node-01 -- bash << 'EOF'
for osd in $(ceph osd ls); do
    echo "OSD $osd:"
    ps aux | grep "ceph-osd.*id $osd"
done
EOF
```

**Solutions:**

**A. Reduce OSD cache:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Reduce BlueStore cache
ceph config set osd osd_memory_target 2147483648  # 2GB
ceph config set osd bluestore_cache_size 1073741824  # 1GB
EOF
```

**B. Reduce MDS cache:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Reduce MDS cache
ceph config set mds mds_cache_memory_limit 2147483648  # 2GB
EOF
```

## Recovery Procedures

### Full Cluster Recovery

**Scenario:** Complete cluster failure

**Steps:**

1. **Restore MON quorum:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Stop all Ceph services
systemctl stop ceph.target

# Restore MON monmap
monmaptool --create --add $(hostname) $(hostname -i) \
    --fsid $(cat /etc/ceph/ceph.conf | grep fsid | awk '{print $3}') \
    /tmp/monmap

# Inject monmap and start
ceph-mon -i $(hostname) --inject-monmap /tmp/monmap
systemctl start ceph-mon@$(hostname)
EOF
```

2. **Restart OSDs:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Start OSDs
for osd in $(ceph osd ls); do
    systemctl start ceph-osd@$osd
done
EOF
```

3. **Restore services:**
```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Start MGR
systemctl start ceph-mgr@$(hostname)

# Redeploy services
ceph orch apply mds cephfs --placement="count:1"
ceph orch apply rgw default --placement="count:1"
EOF
```

### Data Recovery

**Scenario:** Accidental deletion

**RBD Image Recovery:**
```bash
# From snapshot
lxc exec ceph-node-01 -- bash << 'EOF'
# List snapshots
rbd snap ls rbd/myimage

# Rollback to snapshot
rbd snap rollback rbd/myimage@snapshot-name

# Or clone snapshot
rbd clone rbd/myimage@snapshot-name rbd/myimage-recovered
EOF
```

**CephFS Recovery:**
```bash
# From snapshot
lxc exec ceph-node-01 -- bash << 'EOF'
# List snapshots
ceph fs snapshot ls cephfs

# Access snapshot (client-side)
# Snapshots available at: /mnt/cephfs/.snap/snapshot-name/
EOF
```

## Debugging Tools

### Log Analysis

```bash
# Real-time log monitoring
lxc exec ceph-node-01 -- bash << 'EOF'
# All Ceph logs
journalctl -f -u ceph\*

# Specific component
journalctl -f -u ceph-osd@0
journalctl -f -u ceph-mon@$(hostname)
journalctl -f -u ceph-mgr@$(hostname)
EOF
```

### Debug Logging

```bash
# Enable debug logging
lxc exec ceph-node-01 -- bash << 'EOF'
# For OSD
ceph daemon osd.0 config set debug_osd 20

# For MON
ceph daemon mon.$(hostname) config set debug_mon 20

# Reset to default (0)
ceph daemon osd.0 config set debug_osd 0
EOF
```

### Network Debugging

```bash
# Check connectivity
lxc exec ceph-node-01 -- bash << 'EOF'
# Test MON connectivity
ceph mon dump

# Test OSD connectivity
ceph osd dump

# Network latency
ceph osd perf
EOF
```

### Performance Profiling

```bash
# Enable perf counters
lxc exec ceph-node-01 -- bash << 'EOF'
# Dump performance counters
ceph daemon osd.0 perf dump

# Monitor specific counters
ceph daemon osd.0 perf reset
# ... run workload ...
ceph daemon osd.0 perf dump
EOF
```

## Support Resources

### Log Files

- Bootstrap: `/var/log/ceph-bootstrap.log`
- CephFS: `/var/log/ceph-cephfs.log`
- RBD: `/var/log/ceph-rbd.log`
- iSCSI: `/var/log/ceph-iscsi.log`
- RGW: `/var/log/ceph-rgw.log`
- Validation: `/var/log/ceph-validation.log`
- Cloud-init: `/var/log/cloud-init*.log`

### Diagnostic Commands

```bash
# Generate diagnostic bundle
lxc exec ceph-node-01 -- bash << 'EOF'
mkdir -p /tmp/ceph-diag

# Collect information
ceph report > /tmp/ceph-diag/report.json
ceph health detail > /tmp/ceph-diag/health.txt
ceph -s > /tmp/ceph-diag/status.txt
ceph osd tree > /tmp/ceph-diag/osd-tree.txt
ceph df > /tmp/ceph-diag/df.txt

# Collect logs
journalctl -u ceph\* --since "1 hour ago" > /tmp/ceph-diag/logs.txt

# Create tarball
tar -czf /tmp/ceph-diagnostic-$(date +%Y%m%d-%H%M%S).tar.gz -C /tmp ceph-diag/
EOF

# Copy to host
lxc file pull ceph-node-01/tmp/ceph-diagnostic-*.tar.gz ./
```

### Getting Help

1. **Official Documentation:** https://docs.ceph.com
2. **Mailing Lists:** https://ceph.io/community/
3. **IRC:** #ceph on OFTC
4. **Enterprise Support:** support@penguintech.group
5. **GitHub Issues:** https://github.com/PenguinCloud/Nest/issues

### Useful Commands Reference

```bash
# Health and status
ceph health
ceph status
ceph -w

# Component status
ceph mon stat
ceph osd stat
ceph mds stat
ceph fs status

# Performance
ceph osd perf
ceph osd df tree

# Debugging
ceph daemon osd.0 help
ceph daemon mon.$(hostname) help

# Configuration
ceph config dump
ceph config show osd.0
ceph config set osd.0 <option> <value>
```

---

**Last Updated:** 2024-10-07
**Document Version:** 1.0.0
**Maintained by:** Penguin Tech Inc
