# Ceph Storage Cluster Deployment Guide

**Version:** 1.0.0
**Maintained by:** Penguin Tech Inc
**License:** Limited AGPL3

## Table of Contents

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Architecture](#architecture)
4. [Deployment Methods](#deployment-methods)
5. [Quick Start](#quick-start)
6. [Advanced Configuration](#advanced-configuration)
7. [Scaling](#scaling)
8. [Security](#security)
9. [Monitoring](#monitoring)
10. [Maintenance](#maintenance)

## Overview

This guide provides comprehensive instructions for deploying a Ceph storage cluster using LXD privileged containers on Ubuntu 24.04 LTS. The deployment supports all major Ceph storage types:

- **CephFS** - Distributed POSIX-compliant filesystem
- **RBD (RADOS Block Device)** - Block storage for VMs and containers
- **iSCSI Gateway** - iSCSI target for enterprise storage
- **RGW (RADOS Gateway)** - S3/Swift-compatible object storage

### Key Features

- **Automated deployment** via cloud-init and helper scripts
- **Single-node or multi-node** cluster support
- **Production-ready** configuration with security best practices
- **Comprehensive monitoring** with Ceph dashboard and Prometheus
- **Flexible storage pools** for different workload requirements

## Prerequisites

### System Requirements

#### Hardware (Minimum)

- **CPU:** 4 cores per node
- **RAM:** 8GB per node (16GB recommended)
- **Storage:** 50GB root disk + additional storage for OSDs
- **Network:** 1Gbps (10Gbps recommended for production)

#### Hardware (Recommended Production)

- **CPU:** 8+ cores per node
- **RAM:** 32GB+ per node
- **Storage:**
  - Dedicated SSD for OS (100GB+)
  - Dedicated SSDs/NVMe for OSD data
  - Dedicated SSD for RocksDB/WAL
- **Network:** 10Gbps+ with separate public and cluster networks

#### Software

- **OS:** Ubuntu 24.04 LTS (host and containers)
- **LXD:** Latest stable version
- **Kernel:** 5.15+ (6.x recommended)
- **Python:** 3.12+

### Network Requirements

- Reliable network connectivity between nodes
- Optional: Separate networks for public and cluster traffic
- DNS resolution or hosts file configuration
- NTP synchronization across all nodes

## Architecture

### Deployment Architecture

```
┌─────────────────────────────────────────────────────┐
│                    LXD Host                         │
│  ┌───────────────────────────────────────────────┐  │
│  │              LXD Bridge (lxdbr0)              │  │
│  └───────────────────────────────────────────────┘  │
│     │              │              │                  │
│  ┌──▼───┐       ┌──▼───┐       ┌──▼───┐             │
│  │ MON  │       │ MON  │       │ MON  │             │
│  │ MGR  │       │ MGR  │       │      │             │
│  │ OSD  │       │ OSD  │       │ OSD  │             │
│  │ MDS  │       │ MDS  │       │      │             │
│  │ RGW  │       │ RGW  │       │      │             │
│  │iSCSI │       │      │       │      │             │
│  └──────┘       └──────┘       └──────┘             │
│  Node 1          Node 2         Node 3               │
└─────────────────────────────────────────────────────┘
```

### Component Distribution

| Component | Purpose | Minimum | Recommended |
|-----------|---------|---------|-------------|
| MON (Monitor) | Cluster state and consensus | 1 | 3 or 5 (odd number) |
| MGR (Manager) | Cluster management and metrics | 1 | 2+ (active/standby) |
| OSD (Object Storage Daemon) | Data storage | 1 | 3+ per node |
| MDS (Metadata Server) | CephFS metadata | 1 | 2+ (active/standby) |
| RGW (RADOS Gateway) | S3/Swift API | 0 | 2+ (load balanced) |
| iSCSI Gateway | iSCSI target | 0 | 2+ (HA pair) |

## Deployment Methods

### Method 1: Automated Deployment (Recommended)

Use the provided deployment script for quick and consistent deployments.

```bash
# Navigate to project root
cd /path/to/Nest

# Deploy single-node cluster
./scripts/infrastructure/deploy-ceph-lxd.sh

# Deploy 3-node cluster
./scripts/infrastructure/deploy-ceph-lxd.sh -n 3

# Deploy with custom settings
./scripts/infrastructure/deploy-ceph-lxd.sh \
    -n 3 \
    -p ceph-prod \
    -b lxdbr0 \
    -s fast-storage
```

### Method 2: Manual Deployment

For more control over the deployment process:

#### Step 1: Create LXD Profile

```bash
# Create profile from template
lxc profile create ceph-cluster

# Apply configuration
cat infrastructure/lxd/ceph/ceph-profile.yaml | lxc profile edit ceph-cluster
```

#### Step 2: Launch Container

```bash
# Launch with cloud-init
lxc launch ubuntu:24.04 ceph-node-01 \
    --profile ceph-cluster \
    --config=user.user-data="$(cat infrastructure/lxd/ceph/cloud-init.yaml)"
```

#### Step 3: Monitor Deployment

```bash
# Watch cloud-init progress
lxc exec ceph-node-01 -- cloud-init status --wait

# Check logs
lxc exec ceph-node-01 -- tail -f /var/log/ceph-bootstrap.log
```

#### Step 4: Validate Deployment

```bash
# Run validation
./scripts/infrastructure/validate-ceph-cluster.sh -c ceph-node-01
```

## Quick Start

### Single-Node Test Cluster

Perfect for development and testing:

```bash
# 1. Deploy cluster
./scripts/infrastructure/deploy-ceph-lxd.sh

# 2. Wait for completion (5-10 minutes)
# Monitor with: lxc exec ceph-node-01 -- tail -f /var/log/ceph-bootstrap.log

# 3. Validate
./scripts/infrastructure/validate-ceph-cluster.sh

# 4. Access dashboard
# URL: https://<container-ip>:8443
# Username: admin
# Password: admin
```

### Multi-Node Production Cluster

For production deployments:

```bash
# 1. Deploy 3-node cluster
./scripts/infrastructure/deploy-ceph-lxd.sh -n 3

# 2. Validate all nodes
for i in {01..03}; do
    ./scripts/infrastructure/validate-ceph-cluster.sh -c ceph-node-$i
done

# 3. Configure pools
./scripts/infrastructure/configure-ceph-pools.sh -c ceph-node-01

# 4. Configure monitoring
lxc exec ceph-node-01 -- ceph mgr module enable prometheus
lxc exec ceph-node-01 -- ceph mgr module enable dashboard
```

## Advanced Configuration

### Custom Pool Configuration

Create pools optimized for specific workloads:

```bash
# High-performance VM storage
lxc exec ceph-node-01 -- bash << 'EOF'
ceph osd pool create vm-fast 128 128
ceph osd pool set vm-fast size 2
ceph osd pool set vm-fast min_size 1
ceph osd pool application enable vm-fast rbd
rbd pool init vm-fast
EOF

# Archive storage with erasure coding
lxc exec ceph-node-01 -- bash << 'EOF'
ceph osd erasure-code-profile set ec-archive k=4 m=2
ceph osd pool create archive 64 64 erasure ec-archive
ceph osd pool application enable archive rgw
EOF
```

### CephFS Configuration

#### Create Additional Filesystems

```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Create pools
ceph osd pool create cephfs2_metadata 32
ceph osd pool create cephfs2_data 128

# Create filesystem
ceph fs new cephfs2 cephfs2_metadata cephfs2_data

# Deploy MDS
ceph orch apply mds cephfs2 --placement="count:2"
EOF
```

#### Mount CephFS

```bash
# Inside a client container
apt-get install ceph-common

# Mount with kernel driver
mount -t ceph mon-ip:6789:/ /mnt/cephfs \
    -o name=admin,secret=<admin-key>

# Mount with FUSE
ceph-fuse -m mon-ip:6789 /mnt/cephfs
```

### RBD Configuration

#### Create and Map RBD Images

```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Create image
rbd create rbd/myimage --size 100G --image-feature layering

# Enable RBD mirroring
rbd mirror pool enable rbd image
rbd mirror image enable rbd/myimage snapshot
EOF
```

#### Map RBD on Client

```bash
# Map image
rbd map rbd/myimage

# Format and mount
mkfs.ext4 /dev/rbd0
mount /dev/rbd0 /mnt/rbd
```

### iSCSI Gateway Configuration

```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Create iSCSI target with gwcli
gwcli

# Inside gwcli
/iscsi-targets> create iqn.2024-01.io.penguintech:storage
/iscsi-targets> cd iqn.2024-01.io.penguintech:storage/gateways
/gateways> create ceph-node-01 10.0.0.10
/gateways> cd ../disks
/disks> create pool=iscsi image=lun-01 size=100G
EOF
```

### S3/RGW Configuration

#### Create S3 Users

```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Create user
radosgw-admin user create \
    --uid=production-user \
    --display-name="Production S3 User" \
    --email=admin@example.com

# Create subuser with Swift access
radosgw-admin subuser create \
    --uid=production-user \
    --subuser=production-user:swift \
    --access=full
EOF
```

#### Configure S3 Client

```bash
# Install AWS CLI
pip3 install awscli

# Configure
aws configure set aws_access_key_id ACCESSKEY
aws configure set aws_secret_access_key SECRETKEY
aws configure set default.region us-east-1

# Test
aws s3 --endpoint-url http://<rgw-ip>:8080 ls
```

## Scaling

### Add More Nodes

```bash
# Deploy additional node
lxc launch ubuntu:24.04 ceph-node-04 \
    --profile ceph-cluster \
    --config=user.user-data="$(cat infrastructure/lxd/ceph/cloud-init.yaml)"

# Add to cluster
lxc exec ceph-node-01 -- ceph orch host add ceph-node-04
```

### Add More OSDs

```bash
# Add disk to existing node
lxc config device add ceph-node-01 osd-disk1 disk \
    source=/dev/disk/by-id/YOUR-DISK \
    path=/dev/sdb

# Create OSD
lxc exec ceph-node-01 -- ceph orch daemon add osd ceph-node-01:/dev/sdb
```

### Scale Services

```bash
# Scale RGW
lxc exec ceph-node-01 -- ceph orch apply rgw default --placement="count:3"

# Scale MDS
lxc exec ceph-node-01 -- ceph orch apply mds cephfs --placement="count:3"

# Scale MGR
lxc exec ceph-node-01 -- ceph orch apply mgr --placement="count:2"
```

## Security

### Change Default Passwords

```bash
# Dashboard password
lxc exec ceph-node-01 -- ceph dashboard set-login-credentials admin <new-password>

# RGW user password
lxc exec ceph-node-01 -- radosgw-admin user modify \
    --uid=s3user \
    --access-key=<new-access-key> \
    --secret-key=<new-secret-key>
```

### Enable TLS for RGW

```bash
lxc exec ceph-node-01 -- bash << 'EOF'
# Generate certificate
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout /etc/ceph/rgw-key.pem \
    -out /etc/ceph/rgw-cert.pem

# Configure RGW
ceph config set client.rgw rgw_frontends \
    "beast port=8443 ssl_certificate=/etc/ceph/rgw-cert.pem ssl_private_key=/etc/ceph/rgw-key.pem"

# Restart RGW
ceph orch restart rgw.default
EOF
```

### Network Isolation

```bash
# Create separate cluster network
lxc network create ceph-cluster-net

# Update profile
lxc profile device set ceph-cluster eth1 \
    nictype=bridged \
    parent=ceph-cluster-net

# Configure Ceph
lxc exec ceph-node-01 -- ceph config set global cluster_network 10.1.0.0/24
```

## Monitoring

### Ceph Dashboard

Access the built-in dashboard:

```
URL: https://<container-ip>:8443
Username: admin
Password: admin (change this!)
```

Features:
- Cluster health monitoring
- Performance metrics
- Pool management
- OSD management
- User management

### Prometheus Integration

```bash
# Enable Prometheus module
lxc exec ceph-node-01 -- ceph mgr module enable prometheus

# Get metrics
curl http://<container-ip>:9283/metrics
```

### Custom Monitoring

```bash
# Watch cluster status
watch 'lxc exec ceph-node-01 -- ceph -s'

# Monitor OSD performance
lxc exec ceph-node-01 -- ceph osd perf

# Watch scrub progress
lxc exec ceph-node-01 -- ceph pg dump | grep scrubbing
```

## Maintenance

### Regular Tasks

#### Daily

```bash
# Check cluster health
./scripts/infrastructure/validate-ceph-cluster.sh

# Review alerts
lxc exec ceph-node-01 -- ceph health detail
```

#### Weekly

```bash
# Check disk usage
lxc exec ceph-node-01 -- ceph df

# Review slow operations
lxc exec ceph-node-01 -- ceph daemon osd.0 dump_historic_slow_ops
```

#### Monthly

```bash
# Deep scrub all PGs
lxc exec ceph-node-01 -- ceph pg deep-scrub --deep

# Update software
lxc exec ceph-node-01 -- apt update && apt upgrade -y
```

### Backup Procedures

#### CephFS Snapshots

```bash
# Create snapshot
lxc exec ceph-node-01 -- ceph fs snapshot create cephfs snap-$(date +%Y%m%d)

# List snapshots
lxc exec ceph-node-01 -- ceph fs snapshot ls cephfs
```

#### RBD Snapshots

```bash
# Create snapshot
lxc exec ceph-node-01 -- rbd snap create rbd/myimage@snap-$(date +%Y%m%d)

# Clone snapshot
lxc exec ceph-node-01 -- rbd clone rbd/myimage@snap-20240101 rbd/myimage-clone
```

### Disaster Recovery

#### Backup Cluster Configuration

```bash
# Backup configuration
lxc exec ceph-node-01 -- ceph config dump > ceph-config-backup.txt

# Backup keyring
lxc exec ceph-node-01 -- cat /etc/ceph/ceph.client.admin.keyring > admin-keyring-backup.txt
```

#### Restore Procedures

See [ceph-troubleshooting.md](ceph-troubleshooting.md) for detailed recovery procedures.

## Next Steps

- Review [Architecture Documentation](ceph-architecture.md)
- Check [Troubleshooting Guide](ceph-troubleshooting.md)
- Configure [Production Security](#security)
- Set up [Monitoring](#monitoring)
- Plan [Capacity and Scaling](#scaling)

## Support

- **Documentation:** [Ceph Documentation](https://docs.ceph.com)
- **Community:** [Ceph Mailing Lists](https://ceph.io/community/)
- **Enterprise Support:** support@penguintech.group
- **Issues:** [GitHub Issues](https://github.com/PenguinCloud/Nest/issues)

---

**Last Updated:** 2024-10-07
**Document Version:** 1.0.0
**Maintained by:** Penguin Tech Inc
