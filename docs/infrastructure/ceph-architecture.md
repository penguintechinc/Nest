# Nest Storage Architecture: Rook-Ceph Integration

**Version:** 2.0.0  
**Maintained by:** Penguin Tech Inc  
**License:** Limited AGPL3

## Table of Contents

1. [Overview](#overview)
2. [Storage Backends](#storage-backends)
3. [Rook-Ceph Architecture](#rook-ceph-architecture)
4. [StorageClass Aliases](#storageclass-aliases)
5. [CSI Driver Architecture](#csi-driver-architecture)
6. [DarkDrive Discovery](#darkdrive-discovery)
7. [Pool Scheduling](#pool-scheduling)
8. [Snapshot Architecture](#snapshot-architecture)
9. [Backup Architecture](#backup-architecture)
10. [Encryption](#encryption)

## Overview

Nest uses **Rook-Ceph** as its distributed storage backend. Nest manages the **data plane** (DataResource allocation, replication policies, compliance scheduling); **Rook-Ceph manages the Ceph cluster** (OSD orchestration, monitor quorum, rebalancing, health monitoring).

### Architecture Principles

- **Unified Storage**: Single Ceph cluster provides block (RBD), filesystem (CephFS), and object (RGW) storage
- **Kubernetes Native**: Ceph deployed via Rook operator; volumes provisioned via CSI drivers
- **Data Plane Managed by Nest**: DataResource CRs define storage intent; Nest scheduler makes placement decisions
- **Control Plane by Rook**: Ceph cluster state, OSD lifecycle, replication handled by Rook
- **Zero-Copy I/O**: Nest CSI driver proxies to Rook-Ceph CSI sockets; minimal overhead
- **Hardware-Aware Scheduling**: DarkDrive discovery feeds pool scheduling; prefer pools with spare capacity

## Storage Backends

### 1. RBD (RADOS Block Device)

**Type:** Block storage  
**Access Mode:** RWO (ReadWriteOnce)  
**Provisioner:** `rook-ceph.rbd.csi.ceph.com`  
**Typical Use:** VM disks, database storage, container volumes

**Characteristics:**
- Thin-provisioned block devices
- Snapshots and clones
- Incremental backups
- Live migration support
- Encryption-ready (KMS integration via Skauswatch)

**Pool Config:**
```yaml
pool: nest-rbd-pool
replicas: 3
min_size: 2
pg_autoscale_mode: on
application: rbd
```

### 2. CephFS (Filesystem)

**Type:** POSIX-compliant distributed filesystem  
**Access Mode:** RWX (ReadWriteMany)  
**Provisioner:** `rook-ceph.cephfs.csi.ceph.com`  
**Typical Use:** Shared storage, container volumes, home directories

**Characteristics:**
- POSIX semantics
- Kernel and FUSE mount options
- Metadata management via MDS (Metadata Server)
- Snapshots and quotas
- Multi-active MDS for horizontal scaling

**Pool Config:**
```yaml
fsName: nest-cephfs
metadata_pool: nest-cephfs-metadata
data_pools:
  - nest-cephfs-data0
mds_count: 2  # Active/standby for HA
```

### 3. RGW (RADOS Gateway)

**Type:** S3-compatible object storage  
**Access Mode:** HTTP API  
**Typical Use:** Backup and archive, data lakes, static website hosting

**Characteristics:**
- Multi-tenant bucket support
- S3 and Swift API compatibility
- Lifecycle policies and versioning
- Server-side encryption
- Stateless gateway (can be load-balanced)

**Deployment:**
```bash
ceph orch apply rgw default --placement="count:2"
```

## Rook-Ceph Architecture

### Control Flow

```
Kubernetes API
    ↓
Rook Operator
    ├─→ CephCluster CR (defines cluster spec)
    ├─→ StorageClass (RBD, CephFS)
    └─→ CSI Drivers (rook-ceph-rbd, rook-ceph-cephfs)
         ↓
    Ceph Cluster (MON, MGR, OSD, MDS, RGW, etc.)
    ↓
PVC Created
    ↓
CSI Controller (provision volume)
    ↓
PVC Bound
    ↓
Pod Scheduled
    ↓
CSI Node Plugin (attach/mount)
    ↓
Workload Access
```

### Component Responsibilities

**Rook Operator:**
- Reconciles CephCluster CR with actual Ceph cluster state
- Deploys Ceph daemons (MON, MGR, OSD, MDS, RGW) in Kubernetes
- Manages OSD creation, replacement, and removal
- Health monitoring and alerting

**Ceph Cluster:**
- Data storage and replication (OSDs)
- Metadata management (Monitors)
- API servers (RGW for S3, MDS for CephFS)
- Fault tolerance and self-healing

**Nest (above Rook):**
- DataResource CRs define storage intent (capacity, replication, compliance)
- Scheduler selects pools based on DarkDrive inventory
- DataProtectionPolicy creates snapshots on schedule
- License-gated backup to RGW

## StorageClass Aliases

Nest provides **branded StorageClass aliases** for better UX. These are rewritten at admission time by the injector webhook.

### StorageClass Mapping

| Branded | Rewrites To | Type | Access Mode | Use Case |
|---------|------------|------|-------------|----------|
| `nest-block` | `rook-ceph-block` | RBD | RWO | Block storage (default for DataResources) |
| `nest-filesystem` | `rook-cephfs` | CephFS | RWX | Shared filesystem (RWX) |
| `nest-file` | `rook-cephfs-rwo` | CephFS | RWO | Single-node filesystem |

### Rewriting Mechanism

**Admissions Webhook** (injector):
1. Intercepts PVC creation in nest namespace
2. Checks `storageClassName` against branded aliases
3. Rewrites to canonical Rook StorageClass
4. Allows PVC to proceed

**Fallback:** If webhook is bypassed, branded StorageClasses exist as real resources so PVC creation still succeeds.

### StorageClass Details

**`rook-ceph-block` (RBD)**
```yaml
provisioner: rook-ceph.rbd.csi.ceph.com
parameters:
  clusterID: rook-ceph
  pool: nest-rbd-pool
  encrypted: "true"
  encryptionKMSID: skauswatch
reclaimPolicy: Delete
allowVolumeExpansion: true
```

**`rook-cephfs` (CephFS RWX)**
```yaml
provisioner: rook-ceph.cephfs.csi.ceph.com
parameters:
  clusterID: rook-ceph
  fsName: nest-cephfs
  pool: nest-cephfs-data0
reclaimPolicy: Delete
allowVolumeExpansion: true
mountOptions:
  - discard
```

## CSI Driver Architecture

### Nest CSI Driver as Thin Shim

Nest CSI driver is a **thin proxy** that forwards gRPC calls to Rook-Ceph CSI sockets. It does not implement storage logic itself.

```
Client Pod (kubelet)
    ↓
Nest CSI Node Plugin (/csi/csi.sock)
    ↓
gRPC Proxy
    ├─→ RBD: /var/lib/kubelet/plugins/rook-ceph.rbd.csi.ceph.com/csi.sock
    └─→ CephFS: /var/lib/kubelet/plugins/rook-ceph.cephfs.csi.ceph.com/csi.sock
    ↓
Rook-Ceph CSI Drivers
    ↓
Kernel/FUSE
    ↓
Block Device / Mount
```

### Socket Paths

**Configuration in `nest-csi` Helm values:**
```yaml
rook:
  rbdSocket: "unix:///var/lib/kubelet/plugins/rook-ceph.rbd.csi.ceph.com/csi.sock"
  cephfsSocket: "unix:///var/lib/kubelet/plugins/rook-ceph.cephfs.csi.ceph.com/csi.sock"
```

### Try-RBD-First, CephFS-Fallback Pattern

When provisioning a volume from a DataResource:

1. **Attempt RBD** (block storage preferred for performance)
   - Check `nest-block` StorageClass availability
   - Try to create RBD image in `nest-rbd-pool`
   - If successful, return PV

2. **Fallback to CephFS** (if RBD fails or pool full)
   - Check `nest-filesystem` StorageClass availability
   - Create CephFS subvolume in `nest-cephfs-data0`
   - Mount as RWX

**Decision Logic:**
- RBD preferred for single-node workloads (performance)
- CephFS fallback for multi-node workloads (RWX requirement) or when RBD pool capacity exhausted

## DarkDrive Discovery

### Overview

**DarkDrive:** A raw disk (SSD or HDD) with no mountpoint and not hosting OS/boot/swap. Eligible for Ceph OSD allocation.

### Discovery Process

1. **Node Agent (runs on every node):**
   - Scans local block devices via `lsblk`
   - Filters: no mountpoint, no active partitions, not in use by kubelet
   - Publishes `HardwareInventory` CR per node

2. **Inventory CR Structure:**
   ```yaml
   apiVersion: nest.io/v1alpha1
   kind: HardwareInventory
   metadata:
     name: node-01
   spec:
     node: node-01
     devices:
       - name: sdb
         size: 1099511627776  # 1TB bytes
         type: ssd
         mounted: false
         status: available
   ```

3. **Scheduling Uses Inventory:**
   - Pool status includes `DarkDriveCount` per node
   - Scheduler prefers pools with DarkDriveCount > 0
   - If all pools full, falls back to "most free bytes available"

### Monitoring

```bash
# Check HardwareInventory on a node
kubectl get hardwareinventory -n nest

# Inspect DarkDrives on node-01
kubectl get hardwareinventory node-01 -o yaml
```

## Pool Scheduling

### Scheduling Algorithm

**Primary:** Prefer pools with spare DarkDrives
**Secondary:** Prefer pools with most free bytes

**Implementation:**
1. Sort eligible pools by DarkDriveCount descending
2. If tied, sort by available capacity descending
3. Select highest-ranked pool
4. Allocate DataResource volume from selected pool

### Pool Types and Capacity

| Pool | Type | Purpose | Typical Capacity |
|------|------|---------|-----------------|
| `nest-rbd-pool` | RBD | Block storage (default) | 80% of cluster |
| `nest-cephfs-data0` | CephFS | Shared filesystem | 15% of cluster |
| `nest-cephfs-metadata` | CephFS metadata | FS metadata | 5% of cluster |

## Snapshot Architecture

### VolumeSnapshot CRDs

**Automatic snapshots** created by DataProtectionPolicy controller.

**StorageClass Mapping:**
```yaml
# For RBD snapshots
VolumeSnapshotClass: nest-rbd-snapshot
provisioner: rook-ceph.rbd.csi.ceph.com

# For CephFS snapshots
VolumeSnapshotClass: nest-cephfs-snapshot
provisioner: rook-ceph.cephfs.csi.ceph.com
```

### DataProtectionPolicy Schedule

```yaml
apiVersion: nest.io/v1alpha1
kind: DataProtectionPolicy
metadata:
  name: daily-backups
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  retention: 7d
  storageClasses:
    - nest-block
  destination: nest-backup  # RGW bucket
```

### Snapshot Lifecycle

1. **Trigger:** Schedule matches (e.g., daily 2 AM)
2. **Create:** VolumeSnapshot CR created
3. **Provision:** CSI controller creates snapshot on backend
4. **Backup:** (Optional) Copy snapshot to RGW via Velero
5. **Cleanup:** Old snapshots deleted per retention policy

## Backup Architecture

### Velero Integration

Nest uses **Velero** for cluster-wide backup with **BackupStorageLocation** pointing to Ceph RGW.

**Configuration:**
```yaml
apiVersion: velero.io/v1
kind: BackupStorageLocation
metadata:
  name: nest-default
spec:
  provider: aws
  objectStorage:
    bucket: nest-backups
  config:
    s3Url: http://ceph-rgw.rook-ceph:8080
    region: us-east-1
```

### Backup Flow

```
DataProtectionPolicy Schedule
    ↓
VolumeSnapshot Created
    ↓
Velero Backup Job
    ├─→ Snapshot snapshot (if RBD)
    ├─→ Archive metadata
    └─→ Copy to RGW (nest-backups bucket)
    ↓
Backup Complete (phase=Completed)
```

### Restore Procedure

```bash
# List available backups
velero backup get

# Restore from backup
velero restore create --from-backup <backup-name>

# Monitor restore
velero restore logs <restore-name>
```

## Encryption

### KMS Integration

**Encryption Provider:** Skauswatch  
**Encryption Key Manager:** Configured in StorageClass parameters

**StorageClass Configuration:**
```yaml
parameters:
  encrypted: "true"
  encryptionKMSID: skauswatch
```

### Encryption at Rest

- **RBD volumes:** Encrypted by Ceph with keys from Skauswatch
- **CephFS:** Encryption optional (data-at-rest, not metadata)
- **RGW objects:** Server-side encryption via KMS

### Key Rotation

Keys rotated by Skauswatch on policy schedule. Ceph re-encrypts data transparently.

### In-Transit Encryption

- **Kubernetes API → CSI Driver:** TLS (kubelet plugin socket)
- **CSI Driver → Ceph:** Ceph cluster network (internal, authenticated)

## Performance Characteristics

### Latency

| Operation | RBD | CephFS | Notes |
|-----------|-----|--------|-------|
| Create | 100-500ms | 200-800ms | Includes provisioning |
| Attach | 50-200ms | 100-400ms | Mount time varies by FS |
| Read | 1-5ms (avg) | 2-10ms (avg) | Depends on OSD backend |
| Write | 2-10ms (avg) | 3-15ms (avg) | Replication latency |

### Throughput

- **RBD:** Up to 1GB/s per volume (depends on OSD count)
- **CephFS:** Up to 500MB/s aggregated (limited by MDS)
- **RGW:** Up to 10Gbps aggregate bandwidth

### Scaling

- **Horizontal:** Add OSDs to increase capacity and throughput linearly
- **Vertical:** Larger OSD hardware improves per-disk latency
- **Metadata (CephFS):** MDS count determines metadata concurrency

## High Availability

### Component Redundancy

| Component | Minimum | Recommended | Failure Impact |
|-----------|---------|-------------|-----------------|
| MON (Monitor) | 1 | 3 or 5 | Quorum loss → cluster halt |
| MGR (Manager) | 1 | 2 | No data loss; management API down |
| OSD (Storage) | 1 | 3+ replicas | Data loss if replicas < min_size |
| MDS (Metadata) | 1 | 2+ (active/standby) | CephFS metadata unavailable |
| RGW (S3) | 1 | 2+ (load-balanced) | S3 API down |

### Failure Scenarios

**Single OSD Failure:**
1. OSD marked down by MON
2. PGs rebalance to healthy OSDs
3. Recovery begins (background, configurable bandwidth)
4. Cluster returns to HEALTH_OK when rebalancing complete

**Monitor Quorum Loss:**
1. Cluster enters read-only mode
2. No writes accepted
3. Recovery: restore quorum on healthy MONs or rebuild from backups

**Node Failure:**
1. All components on node marked down
2. Ceph triggers recovery if not isolated
3. Rook operator redeploys services on healthy nodes
4. Data rebalances

## Next Steps

- **Deployment:** See [ceph-deployment.md](ceph-deployment.md)
- **Troubleshooting:** See [ceph-troubleshooting.md](ceph-troubleshooting.md)
- **Rook Docs:** https://rook.io/docs/rook/latest/
- **Ceph Docs:** https://docs.ceph.com

---

**Last Updated:** 2025-05-01  
**Document Version:** 2.0.0  
**Maintained by:** Penguin Tech Inc
