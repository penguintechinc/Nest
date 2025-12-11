# Ceph Storage Cluster for LXD

```
   ____            _       ____ _           _
  / ___|___ _ __ | |__   / ___| |_   _ ___| |_ ___ _ __
 | |   / _ \ '_ \| '_ \ | |   | | | | / __| __/ _ \ '__|
 | |__|  __/ |_) | | | || |___| | |_| \__ \ ||  __/ |
  \____\___| .__/|_| |_| \____|_|\__,_|___/\__\___|_|
           |_|

    🚀 Enterprise Storage • LXD Deployment • Ubuntu 24.04
```

**Version:** 1.0.0
**Maintained by:** Penguin Tech Inc
**License:** Limited AGPL3

## 📋 Overview

Production-ready Ceph storage cluster deployment for LXD containers on Ubuntu 24.04 LTS. This solution provides a complete, automated deployment supporting all major Ceph storage types in privileged LXD containers.

### 🎯 Features

- ✅ **CephFS** - Distributed POSIX-compliant filesystem
- ✅ **RBD** - RADOS Block Device for VMs and containers
- ✅ **iSCSI Gateway** - Enterprise iSCSI target support
- ✅ **RGW** - S3/Swift-compatible object storage
- ✅ **Automated Deployment** - Cloud-init based deployment
- ✅ **Multi-Node Support** - Scale from 1 to N nodes
- ✅ **Production Ready** - Security, monitoring, and HA built-in

### 📦 What's Included

This deployment package includes:

- **Cloud-Init Configuration** - Automated cluster bootstrap
- **LXD Profile** - Pre-configured container profile
- **Deployment Scripts** - Automated deployment and validation
- **Comprehensive Documentation** - Architecture, deployment, troubleshooting
- **Management Tools** - Pool configuration and cluster validation

## 🚀 Quick Start

### Prerequisites

- Ubuntu 24.04 LTS host
- LXD installed and initialized
- Minimum 8GB RAM, 4 CPU cores
- 50GB+ available storage

### Deploy in 3 Steps

```bash
# 1. Navigate to project root
cd /path/to/Nest

# 2. Deploy single-node cluster
./scripts/infrastructure/deploy-ceph-lxd.sh

# 3. Validate deployment
./scripts/infrastructure/validate-ceph-cluster.sh
```

**That's it!** Your Ceph cluster is ready.

### Access Your Cluster

```bash
# Ceph Dashboard
URL: https://<container-ip>:8443
Username: admin
Password: admin

# S3 Endpoint
URL: http://<container-ip>:8080
Access Key: ACCESSKEY123
Secret Key: SECRETKEY123

# Container Shell
lxc exec ceph-node-01 -- bash
```

## 📂 File Structure

```
infrastructure/lxd/ceph/
├── cloud-init.yaml           # Main cloud-init configuration
├── ceph-profile.yaml         # LXD container profile
├── ceph-config.yaml          # Configuration templates
└── README.md                 # This file

scripts/infrastructure/
├── deploy-ceph-lxd.sh        # Deployment automation
├── validate-ceph-cluster.sh  # Cluster validation
└── configure-ceph-pools.sh   # Pool management

docs/infrastructure/
├── ceph-deployment.md        # Deployment guide
├── ceph-architecture.md      # Architecture documentation
└── ceph-troubleshooting.md   # Troubleshooting guide
```

## 🔧 Deployment Options

### Single-Node (Development/Testing)

Perfect for development and testing:

```bash
./scripts/infrastructure/deploy-ceph-lxd.sh
```

### Multi-Node (Production)

For production deployments:

```bash
# Deploy 3-node cluster
./scripts/infrastructure/deploy-ceph-lxd.sh -n 3

# Deploy 5-node cluster with custom settings
./scripts/infrastructure/deploy-ceph-lxd.sh \
    -n 5 \
    -p ceph-prod \
    -b lxdbr0 \
    -s fast-storage
```

### Custom Deployment

For advanced control:

```bash
# 1. Create profile
lxc profile create ceph-cluster
cat ceph-profile.yaml | lxc profile edit ceph-cluster

# 2. Launch container
lxc launch ubuntu:24.04 ceph-node-01 \
    --profile ceph-cluster \
    --config=user.user-data="$(cat cloud-init.yaml)"

# 3. Monitor deployment
lxc exec ceph-node-01 -- cloud-init status --wait
lxc exec ceph-node-01 -- tail -f /var/log/ceph-bootstrap.log
```

## 🛠️ Management Commands

### Cluster Validation

```bash
# Basic validation
./scripts/infrastructure/validate-ceph-cluster.sh

# Detailed validation
./scripts/infrastructure/validate-ceph-cluster.sh -d

# JSON output
./scripts/infrastructure/validate-ceph-cluster.sh -j
```

### Pool Management

```bash
# Create all pool types
./scripts/infrastructure/configure-ceph-pools.sh

# Create specific pool type
./scripts/infrastructure/configure-ceph-pools.sh -t rbd

# List pools
./scripts/infrastructure/configure-ceph-pools.sh -l

# Custom pool settings
./scripts/infrastructure/configure-ceph-pools.sh \
    -t cephfs \
    -s 2 \
    -p 64
```

### Cluster Operations

```bash
# Check cluster health
lxc exec ceph-node-01 -- ceph health detail

# View cluster status
lxc exec ceph-node-01 -- ceph -s

# Watch cluster (real-time)
lxc exec ceph-node-01 -- ceph -w

# Check disk usage
lxc exec ceph-node-01 -- ceph df
```

## 📊 Storage Types

### 1. CephFS (Filesystem)

Distributed POSIX filesystem for shared storage:

```bash
# Access CephFS
lxc exec ceph-node-01 -- ceph fs ls

# Mount CephFS (from client)
mount -t ceph mon-ip:6789:/ /mnt/cephfs \
    -o name=admin,secret=<key>
```

**Use Cases:** Home directories, shared application data, container volumes

### 2. RBD (Block Storage)

Block storage for VMs and databases:

```bash
# Create RBD image
lxc exec ceph-node-01 -- rbd create rbd/myimage --size 100G

# Map RBD image (from client)
rbd map rbd/myimage
mkfs.ext4 /dev/rbd0
mount /dev/rbd0 /mnt/rbd
```

**Use Cases:** VM disks, database storage, Kubernetes PVs

### 3. RGW (S3 Object Storage)

S3-compatible object storage:

```bash
# Configure AWS CLI
aws configure set aws_access_key_id ACCESSKEY123
aws configure set aws_secret_access_key SECRETKEY123

# Use S3 API
aws s3 --endpoint-url http://<ip>:8080 ls
aws s3 --endpoint-url http://<ip>:8080 mb s3://mybucket
```

**Use Cases:** Backups, application object storage, data lakes

### 4. iSCSI Gateway

Enterprise iSCSI target for legacy systems:

```bash
# Configure iSCSI (via gwcli)
lxc exec ceph-node-01 -- gwcli

# Or via API
curl -u admin:admin http://<ip>:5001/api
```

**Use Cases:** VMware datastores, enterprise SAN replacement, boot from SAN

## 🔒 Security

### Change Default Passwords

**Important:** Change default passwords before production use!

```bash
# Dashboard password
lxc exec ceph-node-01 -- \
    ceph dashboard set-login-credentials admin <new-password>

# S3 credentials
lxc exec ceph-node-01 -- radosgw-admin user modify \
    --uid=s3user \
    --access-key=<new-key> \
    --secret-key=<new-secret>
```

### Enable TLS

```bash
# Generate certificate
lxc exec ceph-node-01 -- bash << 'EOF'
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout /etc/ceph/rgw-key.pem \
    -out /etc/ceph/rgw-cert.pem

# Configure RGW with TLS
ceph config set client.rgw rgw_frontends \
    "beast port=8443 ssl_certificate=/etc/ceph/rgw-cert.pem"
EOF
```

### Network Isolation

Separate public and cluster networks for production:

```bash
# Create cluster network
lxc network create ceph-cluster-net

# Update containers
lxc config device add ceph-node-01 eth1 \
    nic nictype=bridged parent=ceph-cluster-net
```

## 📈 Scaling

### Add Nodes

```bash
# Deploy additional nodes
./scripts/infrastructure/deploy-ceph-lxd.sh \
    -c ceph-node-04

# Or manually
lxc launch ubuntu:24.04 ceph-node-04 \
    --profile ceph-cluster \
    --config=user.user-data="$(cat cloud-init.yaml)"

# Add to cluster
lxc exec ceph-node-01 -- ceph orch host add ceph-node-04
```

### Add Storage

```bash
# Add disk to node
lxc config device add ceph-node-01 osd-disk1 disk \
    source=/dev/disk/by-id/YOUR-DISK \
    path=/dev/sdb

# Create OSD
lxc exec ceph-node-01 -- \
    ceph orch daemon add osd ceph-node-01:/dev/sdb
```

### Scale Services

```bash
# Scale RGW for S3
lxc exec ceph-node-01 -- \
    ceph orch apply rgw default --placement="count:3"

# Scale MDS for CephFS
lxc exec ceph-node-01 -- \
    ceph orch apply mds cephfs --placement="count:3"
```

## 📚 Documentation

### Comprehensive Guides

- **[Deployment Guide](../../../docs/infrastructure/ceph-deployment.md)** - Complete deployment instructions
- **[Architecture](../../../docs/infrastructure/ceph-architecture.md)** - System architecture and design
- **[Troubleshooting](../../../docs/infrastructure/ceph-troubleshooting.md)** - Problem diagnosis and solutions

### Quick Reference

#### Deployment Commands
```bash
# Single node
./scripts/infrastructure/deploy-ceph-lxd.sh

# Multi-node
./scripts/infrastructure/deploy-ceph-lxd.sh -n 3

# Validate
./scripts/infrastructure/validate-ceph-cluster.sh
```

#### Health Commands
```bash
ceph health
ceph -s
ceph df
ceph osd tree
```

#### Service Commands
```bash
ceph orch ps                    # List services
ceph orch restart rgw.default   # Restart RGW
ceph mgr module ls              # List MGR modules
```

## 🔍 Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| OSDs won't start | Check loop devices: `losetup -a` |
| Dashboard not accessible | Enable module: `ceph mgr module enable dashboard` |
| PGs stuck | Force create: `ceph pg force-create-pg <pg-id>` |
| Clock skew warning | Sync time: `chronyc -a makestep` |

### Debug Commands

```bash
# Check logs
lxc exec ceph-node-01 -- journalctl -f -u ceph\*

# View specific component logs
lxc exec ceph-node-01 -- cat /var/log/ceph-bootstrap.log
lxc exec ceph-node-01 -- cat /var/log/ceph-validation.log

# Get detailed status
lxc exec ceph-node-01 -- ceph health detail
```

### Getting Help

1. Check [Troubleshooting Guide](../../../docs/infrastructure/ceph-troubleshooting.md)
2. Review [Ceph Documentation](https://docs.ceph.com)
3. Contact [Enterprise Support](mailto:support@penguintech.group)
4. Open [GitHub Issue](https://github.com/PenguinCloud/Nest/issues)

## 🎯 Use Cases

### Development/Testing
- Local development storage
- CI/CD artifact storage
- Testing distributed systems

### Production Deployment
- VM storage backend (Proxmox, OpenStack)
- Kubernetes persistent volumes
- Application object storage
- Backup and archive storage

### Enterprise Storage
- SAN replacement
- Unified storage platform
- Multi-tenant cloud storage
- Disaster recovery solution

## 📋 Requirements

### Minimum (Testing)
- 1 LXD container
- 4 CPU cores
- 8GB RAM
- 50GB storage

### Recommended (Production)
- 3+ LXD containers
- 8+ CPU cores per container
- 32GB+ RAM per container
- Dedicated SSDs for OSDs
- 10Gbps+ network

### Network
- Reliable connectivity between nodes
- Optional: Separate cluster network
- NTP synchronization

## 🔄 Updates and Maintenance

### Regular Tasks

**Daily:**
```bash
./scripts/infrastructure/validate-ceph-cluster.sh
```

**Weekly:**
```bash
lxc exec ceph-node-01 -- ceph df
lxc exec ceph-node-01 -- ceph health detail
```

**Monthly:**
```bash
# Software updates
lxc exec ceph-node-01 -- apt update && apt upgrade -y

# Deep scrub
lxc exec ceph-node-01 -- ceph pg deep-scrub --deep
```

### Backup Procedures

```bash
# CephFS snapshot
lxc exec ceph-node-01 -- \
    ceph fs snapshot create cephfs snap-$(date +%Y%m%d)

# RBD snapshot
lxc exec ceph-node-01 -- \
    rbd snap create rbd/myimage@snap-$(date +%Y%m%d)

# Configuration backup
lxc exec ceph-node-01 -- \
    ceph config dump > ceph-config-backup.txt
```

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](../../../CONTRIBUTING.md) for guidelines.

## 📝 License

This project is licensed under the Limited AGPL3 with preamble for fair use - see [LICENSE.md](../../../LICENSE.md) for details.

## 🙏 Acknowledgments

- [Ceph Community](https://ceph.io/community/)
- [Canonical LXD Team](https://ubuntu.com/lxd)
- Penguin Tech Inc Development Team

## 📞 Support

- **Documentation:** [docs/infrastructure/](../../../docs/infrastructure/)
- **Enterprise Support:** support@penguintech.group
- **Community:** [GitHub Issues](https://github.com/PenguinCloud/Nest/issues)
- **Company:** [www.penguintech.io](https://www.penguintech.io)

---

**Last Updated:** 2024-10-07
**Version:** 1.0.0
**Maintained by:** Penguin Tech Inc

---

## 🚦 Next Steps

1. ✅ Deploy cluster: `./scripts/infrastructure/deploy-ceph-lxd.sh`
2. ✅ Validate deployment: `./scripts/infrastructure/validate-ceph-cluster.sh`
3. ✅ Configure pools: `./scripts/infrastructure/configure-ceph-pools.sh`
4. ✅ Access dashboard: `https://<container-ip>:8443`
5. ✅ Change default passwords
6. ✅ Configure monitoring
7. ✅ Test your storage types
8. ✅ Read the [deployment guide](../../../docs/infrastructure/ceph-deployment.md)

**Happy Storage! 🎉**
