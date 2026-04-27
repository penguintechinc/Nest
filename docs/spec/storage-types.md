# Nest Storage Types

## Overview

Nest supports six raw storage types backed by Ceph, providing flexible options for persistent data across Kubernetes workloads. Each type maps to a specific Ceph backend and access pattern, enabling use cases from high-performance databases to shared filesystems and object storage.

## Storage Types Summary

| Type | Backend | Access Mode | Use Case |
|------|---------|-------------|----------|
| `pvc/block` | Ceph RBD | ReadWriteOnce | Block device for databases, VMs, high-IOPS workloads |
| `pvc/file` | CephFS | ReadWriteOnce | Single-mount filesystem with strong consistency |
| `filesystem` | CephFS | ReadWriteMany | Shared filesystem, multi-pod access, NFS-like |
| `nfs` | NFS-Ganesha / CephFS | ReadWriteMany | Legacy NFS clients, non-Kubernetes consumers |
| `object` | Ceph RGW (S3) | S3 API | Object storage, ML datasets, unstructured data |
| `iscsi` | Ceph RBD / iSCSI | Block (iSCSI) | Non-Kubernetes iSCSI initiators, legacy systems |

---

## pvc/block — Block Storage (RBD)

**Backend:** Ceph RBD (RADOS Block Device)  
**Access Mode:** ReadWriteOnce  
**Throughput:** Up to 20K IOPS per volume  

Ideal for databases (PostgreSQL, MySQL), virtual machines, or any workload requiring block-level access. Single-mount only; cannot be shared across pods.

**Example DataResource:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: postgres-data
  namespace: myapp
spec:
  type: pvc/block
  size: 100Gi
  storageClass: nest-block
  reclaimPolicy: Retain
```

**Notes:**
- Uses `nest-block` StorageClass (Ceph RBD provisioner)
- Supports snapshots and cloning
- No filesystem overhead; raw block I/O

---

## pvc/file — File Storage (Single-Mount CephFS)

**Backend:** CephFS  
**Access Mode:** ReadWriteOnce  
**Throughput:** Up to 5K IOPS per mount  

Similar to block storage but filesystem-aware. Single pod can mount; mount-exclusive. Useful when filesystem semantics are required but sharing is not needed.

**Example DataResource:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: app-cache
  namespace: myapp
spec:
  type: pvc/file
  size: 50Gi
  storageClass: rook-cephfs
  reclaimPolicy: Delete
```

**Notes:**
- Uses `rook-cephfs` StorageClass
- Supports standard POSIX filesystem operations
- Lower latency than RWX shared mounts

---

## filesystem — Shared Filesystem (CephFS RWX)

**Backend:** CephFS  
**Access Mode:** ReadWriteMany  
**Throughput:** Up to 3K IOPS shared across consumers  

Shared filesystem accessible by multiple pods simultaneously. Similar to NFS but backed by Ceph, providing strong consistency guarantees within the cluster.

**Example DataResource:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: shared-data
  namespace: myapp
spec:
  type: filesystem
  size: 200Gi
  storageClass: rook-cephfs
  reclaimPolicy: Delete
```

**Notes:**
- Use for multi-pod shared scratch, logs, or distributed caches
- POSIX-compliant; standard file locks work
- Performance degrades with many concurrent writers

---

## nfs — NFS Export (NFS-Ganesha)

**Backend:** NFS-Ganesha / CephFS  
**Access Mode:** ReadWriteMany  
**Protocol:** NFSv3 / NFSv4.1  

Legacy NFS client support. Bridges non-Kubernetes systems (VMs, bare metal) with Nest storage. Backed by Ceph but exposed via standard NFS.

**Example DataResource:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: nfs-export
  namespace: myapp
spec:
  type: nfs
  size: 500Gi
  storageClass: nest-nfs
  reclaimPolicy: Retain
  nfs:
    server: nest-nfs-ganesha.rook-ceph.svc.cluster.local
    path: /exports/myapp
```

**Notes:**
- Requires NFS client on consumer (Linux, macOS, Windows CIFS/SMB bridge)
- Mount via standard `mount -t nfs` commands
- Performance slightly lower than direct Ceph RWX due to NFS translation layer

---

## object — Object Storage (S3/Ceph RGW)

**Backend:** Ceph RGW (RADOS Gateway)  
**Access Mode:** S3 API (HTTP/HTTPS)  
**Throughput:** Up to 10K req/sec per bucket  

S3-compatible object storage for unstructured data, backups, ML datasets, or archival. No filesystem semantics; bucket-based key-value store.

**Example DataResource:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: backup-bucket
  namespace: myapp
spec:
  type: object
  size: 1Ti
  storageClass: nest-object
  reclaimPolicy: Retain
  object:
    bucket: nest-myapp-backups
    region: us-east-1
```

**Notes:**
- Uses `nest-object` StorageClass (RGW bucket provisioner)
- Supports lifecycle policies for tiering (hot → cold)
- Bucket naming: `nest-{tenant}-{resource-name}`
- Access via S3 SDK or `aws s3` CLI

---

## iscsi — iSCSI Block Storage

**Backend:** Ceph RBD / iSCSI Gateway  
**Access Mode:** Block (iSCSI initiator)  
**Throughput:** Up to 15K IOPS per target  

Block storage accessible over iSCSI protocol. Bridges Kubernetes and non-Kubernetes environments requiring raw block access without direct Ceph client.

**Example DataResource:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: legacy-vm-disk
  namespace: infrastructure
spec:
  type: iscsi
  size: 500Gi
  storageClass: nest-iscsi
  reclaimPolicy: Retain
  iscsi:
    target: iqn.2026-04.nest.rook-ceph:legacy-vm-disk
    portal: nest-iscsi-gateway.rook-ceph.svc.cluster.local:3260
```

**Notes:**
- Consumer initiates iSCSI discovery and login
- Raw block device on consumer side; no filesystem
- Useful for VMs (KVM, Hyper-V) or legacy Unix systems

---

## StorageClasses Reference

| StorageClass | Provisioner | ReclaimPolicy | Access Mode |
|---|---|---|---|
| `nest-block` | rook-ceph.rbd.csi.ceph.com | Delete | RWO |
| `rook-cephfs` | rook-ceph.cephfs.csi.ceph.com | Delete | RWX |
| `nest-object` | rook-ceph.ceph.rook.io/bucket | Delete | S3 |
| `nest-nfs` | NFS (external provisioner) | Retain | RWX |
| `nest-iscsi` | rook-ceph.rbd.csi.ceph.com + iSCSI gateway | Retain | iSCSI |

---

## Choosing a Storage Type

- **Single-pod, high-performance:** `pvc/block`
- **Multi-pod, shared data:** `filesystem`
- **Legacy NFS clients:** `nfs`
- **Unstructured data, S3 API:** `object`
- **Non-Kubernetes iSCSI initiators:** `iscsi`
- **Single-pod, filesystem semantics:** `pvc/file`
