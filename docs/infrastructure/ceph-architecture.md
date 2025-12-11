# Ceph Storage Architecture

**Version:** 1.0.0
**Maintained by:** Penguin Tech Inc
**License:** Limited AGPL3

## Table of Contents

1. [Overview](#overview)
2. [Core Components](#core-components)
3. [Storage Types](#storage-types)
4. [Data Flow](#data-flow)
5. [CRUSH Algorithm](#crush-algorithm)
6. [Deployment Architecture](#deployment-architecture)
7. [Performance Considerations](#performance-considerations)
8. [Scalability](#scalability)
9. [High Availability](#high-availability)

## Overview

Ceph is a unified, distributed storage system designed for excellent performance, reliability, and scalability. This document describes the architecture of Ceph as deployed in LXD containers on Ubuntu 24.04 LTS.

### Key Architectural Principles

- **Unified Storage**: Single cluster provides block, file, and object storage
- **Distributed**: Data distributed across cluster nodes using CRUSH algorithm
- **Self-Healing**: Automatic data replication and recovery
- **No Single Point of Failure**: All components can be redundant
- **Scalable**: Linear scaling to thousands of nodes and exabytes of storage

## Core Components

### 1. Monitor (MON)

**Purpose:** Maintains cluster membership and state

```
┌─────────────────────────────┐
│      Monitor (MON)          │
│  ┌───────────────────────┐  │
│  │   Cluster Map         │  │
│  │   - Monitor Map       │  │
│  │   - OSD Map           │  │
│  │   - PG Map            │  │
│  │   - CRUSH Map         │  │
│  │   - MDS Map           │  │
│  └───────────────────────┘  │
│  ┌───────────────────────┐  │
│  │   Paxos Consensus     │  │
│  └───────────────────────┘  │
└─────────────────────────────┘
```

**Characteristics:**
- Runs Paxos consensus algorithm
- Maintains authoritative cluster map
- Requires odd number of monitors (3 or 5 recommended)
- Low disk I/O and CPU requirements
- Critical for cluster operation

**Resource Requirements:**
- CPU: 1-2 cores
- RAM: 2-4GB
- Disk: 10GB+ (SSD preferred)
- Network: 1Gbps+

### 2. Manager (MGR)

**Purpose:** Cluster management and monitoring

```
┌─────────────────────────────┐
│      Manager (MGR)          │
│  ┌───────────────────────┐  │
│  │   Dashboard           │  │
│  │   Prometheus          │  │
│  │   RESTful API         │  │
│  │   Orchestrator        │  │
│  │   PG Autoscaler       │  │
│  │   Balancer            │  │
│  └───────────────────────┘  │
└─────────────────────────────┘
```

**Characteristics:**
- Active/standby configuration
- Hosts management modules
- Provides metrics and monitoring
- Orchestrates service deployment

**Resource Requirements:**
- CPU: 2-4 cores
- RAM: 4-8GB
- Disk: 20GB+
- Network: 1Gbps+

### 3. Object Storage Daemon (OSD)

**Purpose:** Data storage and replication

```
┌─────────────────────────────┐
│     OSD Daemon              │
│  ┌───────────────────────┐  │
│  │   BlueStore Backend   │  │
│  │   ┌─────────────────┐ │  │
│  │   │  RocksDB (meta) │ │  │
│  │   ├─────────────────┤ │  │
│  │   │  Block Device   │ │  │
│  │   │  (data)         │ │  │
│  │   └─────────────────┘ │  │
│  └───────────────────────┘  │
│  ┌───────────────────────┐  │
│  │   Replication Engine  │  │
│  └───────────────────────┘  │
└─────────────────────────────┘
```

**Characteristics:**
- One OSD per physical disk (best practice)
- Handles data replication and recovery
- Performs scrubbing for data integrity
- Communicates peer-to-peer

**Resource Requirements (per OSD):**
- CPU: 1-2 cores
- RAM: 4-8GB (BlueStore)
- Disk: Dedicated physical disk
- Network: 10Gbps+ recommended

### 4. Metadata Server (MDS)

**Purpose:** CephFS metadata management

```
┌─────────────────────────────┐
│   Metadata Server (MDS)     │
│  ┌───────────────────────┐  │
│  │   Metadata Cache      │  │
│  │   - Inodes            │  │
│  │   - Directory entries │  │
│  │   - Capabilities      │  │
│  └───────────────────────┘  │
│  ┌───────────────────────┐  │
│  │   Journal             │  │
│  └───────────────────────┘  │
└─────────────────────────────┘
```

**Characteristics:**
- Only required for CephFS
- Active/standby configuration
- Highly CPU and RAM intensive
- Caches metadata in RAM

**Resource Requirements:**
- CPU: 4-8 cores
- RAM: 8-32GB (4GB minimum)
- Disk: SSD for metadata pool
- Network: 10Gbps+

### 5. RADOS Gateway (RGW)

**Purpose:** S3/Swift object storage API

```
┌─────────────────────────────┐
│    RADOS Gateway (RGW)      │
│  ┌───────────────────────┐  │
│  │   HTTP Server (Beast) │  │
│  │   ┌─────────────────┐ │  │
│  │   │  S3 API         │ │  │
│  │   ├─────────────────┤ │  │
│  │   │  Swift API      │ │  │
│  │   └─────────────────┘ │  │
│  └───────────────────────┘  │
│  ┌───────────────────────┐  │
│  │   librados            │  │
│  └───────────────────────┘  │
└─────────────────────────────┘
```

**Characteristics:**
- Stateless (can be load balanced)
- Multi-tenant support
- Bucket lifecycle policies
- S3 and Swift compatible

**Resource Requirements:**
- CPU: 4-8 cores
- RAM: 8-16GB
- Disk: Minimal (OS only)
- Network: 10Gbps+ (external facing)

### 6. iSCSI Gateway

**Purpose:** Block storage via iSCSI protocol

```
┌─────────────────────────────┐
│    iSCSI Gateway            │
│  ┌───────────────────────┐  │
│  │   LIO Target          │  │
│  │   ┌─────────────────┐ │  │
│  │   │  iSCSI Target   │ │  │
│  │   ├─────────────────┤ │  │
│  │   │  RBD Images     │ │  │
│  │   └─────────────────┘ │  │
│  └───────────────────────┘  │
│  ┌───────────────────────┐  │
│  │   API (rbd-target-api)│  │
│  └───────────────────────┘  │
└─────────────────────────────┘
```

**Characteristics:**
- HA pair configuration recommended
- Maps RBD images as iSCSI LUNs
- ALUA support for multipathing
- CHAP authentication

**Resource Requirements:**
- CPU: 4-8 cores
- RAM: 8-16GB
- Disk: Minimal
- Network: 10Gbps+ (dedicated iSCSI network)

## Storage Types

### 1. CephFS (Filesystem)

**Architecture:**

```
Client → MDS (metadata) → Metadata Pool
      → OSD (data)      → Data Pool
```

**Use Cases:**
- Shared filesystem for containers/VMs
- Home directories
- Application data storage
- Big data analytics

**Characteristics:**
- POSIX compliant
- Kernel and FUSE clients
- Snapshots and quotas
- Multi-active MDS for scale

### 2. RBD (Block Storage)

**Architecture:**

```
Client → librbd → OSD → Block Pool
         ↓
       Kernel Module
         ↓
       Block Device
```

**Use Cases:**
- VM disks (OpenStack, Proxmox)
- Container persistent volumes (Kubernetes)
- Database storage
- High-performance applications

**Characteristics:**
- Thin provisioning
- Snapshots and clones
- Incremental backups
- Live migration support

### 3. RGW (Object Storage)

**Architecture:**

```
S3/Swift Client → RGW → Index Pool
                      → Data Pool
                      → Metadata Pool
```

**Use Cases:**
- Application object storage
- Backup and archive
- Static website hosting
- Data lake storage

**Characteristics:**
- Multi-tenant buckets
- Lifecycle policies
- Versioning
- Server-side encryption

### 4. iSCSI

**Architecture:**

```
iSCSI Initiator → iSCSI Target → RBD → OSD → Pool
```

**Use Cases:**
- Legacy application storage
- VMware datastores
- Enterprise SAN replacement
- Boot from SAN

**Characteristics:**
- Multipath I/O support
- CHAP authentication
- High availability
- Performance comparable to local storage

## Data Flow

### Write Operation

```
1. Client writes data
   ↓
2. Primary OSD receives write
   ↓
3. Primary OSD writes to disk
   ↓
4. Primary OSD replicates to secondary OSDs
   ↓
5. Secondary OSDs acknowledge
   ↓
6. Primary OSD acknowledges to client
```

### Read Operation

```
1. Client requests data
   ↓
2. Client calculates object location (CRUSH)
   ↓
3. Client reads from primary OSD
   ↓
4. OSD returns data to client
```

## CRUSH Algorithm

**Controlled Replication Under Scalable Hashing**

### How CRUSH Works

```
Object → Hash → PG ID → CRUSH → OSD Set
```

**Steps:**

1. **Object Hashing**: Object name hashed to PG ID
2. **CRUSH Calculation**: PG mapped to OSD set using CRUSH rules
3. **Replica Placement**: OSDs selected based on failure domains

**Example CRUSH Map:**

```
root default {
    id -1
    alg straw2

    datacenter dc1 {
        id -2
        alg straw2

        rack rack1 {
            id -3
            alg straw2

            host ceph-node-01 {
                id -4
                alg straw2
                osd.0 {weight 1.0}
                osd.1 {weight 1.0}
            }

            host ceph-node-02 {
                id -5
                alg straw2
                osd.2 {weight 1.0}
                osd.3 {weight 1.0}
            }
        }
    }
}
```

### Benefits

- **No Central Metadata**: Clients calculate placement
- **Pseudo-random Distribution**: Even data distribution
- **Failure Domain Awareness**: Replicas across failure boundaries
- **Efficient Rebalancing**: Minimal data movement on changes

## Deployment Architecture

### Single-Node Architecture (Development/Testing)

```
┌─────────────────────────────────────┐
│         LXD Container               │
│  ┌──────────────────────────────┐   │
│  │  MON + MGR + MDS             │   │
│  ├──────────────────────────────┤   │
│  │  OSD (loop device)           │   │
│  ├──────────────────────────────┤   │
│  │  RGW (port 8080)             │   │
│  ├──────────────────────────────┤   │
│  │  iSCSI Gateway               │   │
│  ├──────────────────────────────┤   │
│  │  Dashboard (port 8443)       │   │
│  └──────────────────────────────┘   │
└─────────────────────────────────────┘
```

### Multi-Node Architecture (Production)

```
┌─────────────────────────────────────────────────────────┐
│                    Public Network                       │
└────────┬──────────────────┬──────────────────┬─────────┘
         │                  │                  │
    ┌────▼────┐        ┌────▼────┐        ┌────▼────┐
    │ Node 1  │        │ Node 2  │        │ Node 3  │
    │         │        │         │        │         │
    │ MON+MGR │◄──────►│ MON+MGR │◄──────►│ MON     │
    │ MDS     │        │ MDS     │        │         │
    │ RGW     │        │ RGW     │        │         │
    │ iSCSI   │        │ iSCSI   │        │         │
    │ OSD×3   │        │ OSD×3   │        │ OSD×3   │
    └────┬────┘        └────┬────┘        └────┬────┘
         │                  │                  │
┌────────▼──────────────────▼──────────────────▼─────────┐
│                 Cluster Network (10Gbps+)              │
└─────────────────────────────────────────────────────────┘
```

## Performance Considerations

### Network Architecture

**Recommended Setup:**

```
┌──────────────────────────────────────┐
│         Public Network               │
│  (Client traffic: 1-10 Gbps)        │
└──────────────────────────────────────┘
              │
         ┌────▼────┐
         │  Nodes  │
         └────┬────┘
              │
┌──────────────▼───────────────────────┐
│       Cluster Network                │
│  (Replication traffic: 10-100 Gbps) │
└──────────────────────────────────────┘
```

### Storage Hierarchy

**Performance Tiers:**

1. **Hot Tier**: NVMe SSDs for metadata and high-IOPS workloads
2. **Warm Tier**: SATA SSDs for general-purpose storage
3. **Cold Tier**: HDDs for archive and backup

### CRUSH Rule Optimization

```bash
# Fast pool on SSDs
ceph osd crush rule create-replicated fast-rule \
    default host ssd

# Archive pool on HDDs
ceph osd crush rule create-replicated archive-rule \
    default host hdd
```

## Scalability

### Horizontal Scaling

**Adding Capacity:**

```
Initial: 3 nodes × 3 OSDs = 9 OSDs (27TB usable with 3x replication)
   ↓
Scale: 6 nodes × 6 OSDs = 36 OSDs (108TB usable with 3x replication)
   ↓
Scale: 12 nodes × 8 OSDs = 96 OSDs (256TB usable with 3x replication)
```

**Performance Scaling:**

- **Linear IOPS scaling** with additional OSDs
- **Bandwidth scaling** with network capacity
- **Parallel processing** across all OSDs

### Capacity Planning

**Formula:**

```
Raw Capacity = (OSD Count × OSD Size)
Usable Capacity = Raw Capacity / Replication Factor
Effective Capacity = Usable Capacity × 0.8 (recommended max utilization)
```

**Example:**

```
12 nodes × 8 OSDs × 4TB = 384TB raw
384TB / 3 replicas = 128TB usable
128TB × 0.8 = 102TB effective capacity
```

## High Availability

### Component Redundancy

| Component | Redundancy | Failure Tolerance |
|-----------|------------|-------------------|
| MON | 3 or 5 | (n-1)/2 |
| MGR | 2+ | n-1 |
| OSD | 3+ replicas | replica_size - min_size |
| MDS | 2+ (active/standby) | n-1 |
| RGW | 2+ (load balanced) | n-1 |
| iSCSI | 2 (HA pair) | 1 |

### Failure Scenarios

**OSD Failure:**
```
1. OSD marked down
2. PGs marked degraded
3. Recovery begins after 600s (default)
4. Data rebalanced to healthy OSDs
5. Cluster returns to HEALTH_OK
```

**Node Failure:**
```
1. All OSDs on node marked down
2. MON quorum maintained (if ≥2 MONs remain)
3. Recovery initiated
4. Services redeployed on healthy nodes (cephadm)
5. Data rebalanced
```

**Network Partition:**
```
1. Split-brain prevention via Paxos
2. Majority partition continues operation
3. Minority partition blocks I/O
4. Automatic recovery on network restoration
```

## Best Practices

### Design Recommendations

1. **Separate Public and Cluster Networks** for optimal performance
2. **Use Odd Number of MONs** (3 or 5) for quorum
3. **Dedicated Disks for OSDs** - one OSD per physical disk
4. **SSD for Metadata Pools** (CephFS, RGW index)
5. **NVMe for RocksDB/WAL** on OSDs when possible

### Operational Guidelines

1. **Monitor Cluster Health** daily
2. **Keep Software Updated** for security and performance
3. **Test Disaster Recovery** procedures regularly
4. **Plan for 20% Growth** annually
5. **Document All Changes** to cluster configuration

## References

- [Ceph Architecture Documentation](https://docs.ceph.com/en/latest/architecture/)
- [CRUSH Map Documentation](https://docs.ceph.com/en/latest/rados/operations/crush-map/)
- [Performance Tuning Guide](https://docs.ceph.com/en/latest/rados/configuration/bluestore-config-ref/)

---

**Last Updated:** 2024-10-07
**Document Version:** 1.0.0
**Maintained by:** Penguin Tech Inc
