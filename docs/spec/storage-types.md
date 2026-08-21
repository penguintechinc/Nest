# Nest Storage Types Reference

Comprehensive specification for all DataResource storage types in Nest. DataResources represent persistent data abstractions backed by various storage engines and cloud providers.

**Last updated:** May 2026
**Nest version:** P1+

---

## Table of Contents

1. [Overview](#overview)
2. [Origination Modes](#origination-modes)
3. [Drive Preference & DarkDrives](#drive-preference--darkdrives)
4. [Block Storage Types](#block-storage-types)
5. [Object Storage Types](#object-storage-types)
6. [File/Network Storage Types](#filenetwork-storage-types)
7. [Database Types](#database-types)
8. [Analytics & Search Types](#analytics--search-types)
9. [Streaming Types](#streaming-types)
10. [Extended Types](#extended-types)
11. [Cloud Storage Types](#cloud-storage-types)
12. [Access Modes](#access-modes)
13. [DataResource Spec Reference](#dataresource-spec-reference)
14. [Eggs](#eggs)
15. [Decision Matrix](#decision-matrix)

---

## Overview

Nest provisions persistent data storage via DataResources—Kubernetes CRDs that abstract underlying storage engines. Each type maps to a specific backend and origination mode, supporting use cases from high-performance block I/O to multi-tenant analytics.

**Type naming convention:** `{category}/{subtype}` for namespaced types; plain `name` for broad categories.

### Supported Type Categories

| Category           | Types                                                | Backend                       | Origination                             |
| ------------------ | ---------------------------------------------------- | ----------------------------- | --------------------------------------- |
| **Block Storage**  | `pvc/block`, `pvc/file`                              | Ceph RBD, cloud block volumes | managed, imported, external             |
| **Object Storage** | `object`, `s3`, `gcs`, `azure-blob`                  | Ceph RGW, cloud buckets       | managed (self-hosted), external (cloud) |
| **File/Network**   | `filesystem`, `nfs`, `rockfs`                        | CephFS, NFS-Ganesha           | managed, imported, external             |
| **Databases**      | `postgres`, `keyvalue`                               | CNPG, Valkey, cloud-managed   | managed, imported, external             |
| **Analytics**      | `clickhouse`, `warehouse/trino`, `lakehouse/iceberg` | ClickHouse, Trino, Iceberg    | managed, imported, external             |
| **Search**         | `search`, `vector`                                   | OpenSearch (shared/dedicated) | managed, imported                       |
| **Streaming**      | `kafka`                                              | Kafka cluster                 | managed, imported, external             |
| **Extended**       | `rockfs`, `timeseries`                               | RockFS, VictoriaMetrics       | managed, imported, external             |

---

## Origination Modes

Every DataResource declares how it originates: **managed** (Nest provisions), **imported** (pre-existing), or **external** (cloud-provider).

### managed

Nest provisions and manages the resource end-to-end. Infrastructure lifecycle is under Nest control.

**Valid for:** `pvc/block`, `pvc/file`, `filesystem`, `nfs`, `rockfs`, `object`, `postgres`, `keyvalue`, `kafka`, `search`, `vector`, `clickhouse`, `warehouse/trino`, `lakehouse/iceberg`, `timeseries`

**Behavior:**

- Nest operator creates underlying infrastructure (PVC, databases, caches)
- Automatic backup, HA, and failover enabled by default
- Credential rotation and password resets managed by Nest
- Deprovisioning cascades (resource deletion removes underlying data unless `reclaimPolicy: Retain`)

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: app-db
  namespace: myapp
spec:
  type: postgres
  class: default
  tenant: acme
  origination: managed
  ha: true
  replicas:
    write:
      min: 3
      max: 5
      default: 3
```

---

### imported

Pre-existing resource outside Nest. Nest adopts and manages ongoing operations (credentials, monitoring, backups) but does not provision.

**Valid for:** `postgres`, `keyvalue`, `kafka`, `object`, `nfs`, `rockfs`, `search`, `vector`, `clickhouse`, `warehouse/trino`, `lakehouse/iceberg`, `timeseries`, `iscsi`

**Behavior:**

- Requires `spec.import` with connection string and credentials
- Nest reads and validates connection; credentials stored in SAL (Secrets Access Layer)
- Optional: `managedCredentials: true` allows Nest to rotate credentials on the engine
- Optional: `managedFailover: true` allows Nest to perform failover operations
- Resource isolation still enforced (tenant scoping in queries)

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: legacy-postgres
  namespace: myapp
spec:
  type: postgres
  class: default
  tenant: acme
  origination: imported
  import:
    connectionString: "postgresql://host.example.com:5432/legacy_db"
    tlsMode: verify-full
    credentialSecret: legacy-creds
    managedCredentials: true
    managedFailover: false
```

---

### external

Cloud-provider managed resource (AWS, GCP, Azure, or Tier 2 standard-protocol providers). Nest validates connectivity and provisions access credentials only.

**Valid for:** `ebs`, `azure-disk`, `gcp-disk`, `s3`, `gcs`, `azure-blob`, `postgres` (RDS/Cloud SQL/Azure Database), `keyvalue` (ElastiCache/Memorystore), `kafka` (MSK/Confluent), and any resource exposed via standard protocols on Tier 2 providers.

**Behavior:**

- Requires `spec.external` with provider, region, resource ID, and credentials
- Nest does NOT manage underlying resource lifecycle; customer controls via cloud provider console
- Nest manages access: generates temporary credentials, enforces tenant scoping, audits access
- Resource configuration changes (scaling, HA settings) require manual cloud provider action
- Nest integrates monitoring and billing via cloud provider APIs

**Example (AWS EBS):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: ebs-volume
  namespace: myapp
spec:
  type: ebs
  class: default
  tenant: acme
  origination: external
  external:
    provider: aws
    region: us-west-2
    resourceId: vol-0123456789abcdef0
    credentialSecret: aws-creds
    blockVolume:
      sizeGB: 500
      iops: 3000
      volumeType: gp3
      availabilityZone: us-west-2a
      encryptionKeyId: arn:aws:kms:...
```

**Example (Tier 2 standard-protocol Postgres):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: tier2-postgres
  namespace: myapp
spec:
  type: postgres
  class: default
  tenant: acme
  origination: external
  external:
    provider: custom-vultr-managed
    endpoint: pg.example.com:5432
    engineType: postgres
    credentialSecret: custom-db-creds
    extra:
      engine_version: "16"
```

---

## Drive Preference & DarkDrives

### Drive Selection Priority

Nest always prefers **DarkDrives** (unadopted, non-system block devices) over system or actively-used drives.

**Priority order:**

1. NVMe drives (nvme-hot class)
2. SSD drives (ssd-warm class)
3. SATA drives (sata-bulk class)
4. Legacy/cold drives (sata-cold class)

Within each class, Nest selects the drive with the best available capacity and health.

### System Drive Exclusion

Drives hosting `/`, `/boot`, or swap are automatically marked `System` state and **never** eligible for adoption. The node agent (`nest-node-agent` DaemonSet) performs SMART scan and partition signature detection on startup.

### DarkDrive Lifecycle

The **DarkDrive CRD** tracks discovered devices across the cluster.

```yaml
apiVersion: nest.penguintech.io/v1
kind: DarkDrive
metadata:
  name: node-a-nvme0
spec:
  node: worker-1
  device: /dev/nvme0n1
  size: 1.92TB
  class: nvme-hot
  serial: NVMESERIAL123ABC
  signature: blank # blank | nest-previous | foreign-fs:<type>
  hardwarePool: ssd-hot
  eraseConfirmed: false
status:
  state: Discovered # Discovered → AwaitingApproval → Approved → Adopted
  approvedBy: admin@acme
  approvedAt: "2026-05-01T10:00:00Z"
  conditions:
    - type: Healthy
      status: "True"
      reason: SMARTHealthy
      message: "SMART health: PASSED, wear: 2%, hours: 12000"
```

**Phases:**

- `Discovered`: Node agent found the device; awaiting manual approval
- `AwaitingApproval`: Pending operator approval via `spec.hardwarePool` assignment
- `Approved`: Operator approved; scheduler can assign to workloads
- `Adopted`: Currently in use by a workload; cannot be reassigned until deallocated
- `Rejected`: Operator rejected; device ignored in future scans

---

## Block Storage Types

### pvc/block

**Backend:** Ceph RBD (RADOS Block Device)
**Access Mode:** ReadWriteOnce
**Typical throughput:** Up to 20K IOPS per volume
**HA support:** Yes (via Ceph replication)

Raw block device for single-pod attachment. Ideal for databases, virtual machines, or any workload requiring direct block-level I/O. Cannot be shared across pods simultaneously.

**Required spec fields:**

- `type: "pvc/block"`
- `class`: Storage class (default: `nest-block`)
- `size.storage`: Capacity (e.g., `"100Gi"`)

**Status fields (when Ready):**

- `endpoints.native`: Block device path (e.g., `/dev/rbd0` on consumer pod)
- `health`: SMART-equivalent metrics from Ceph backend

**Example DataResource:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: postgres-data
  namespace: myapp
spec:
  type: pvc/block
  class: nest-block
  tenant: acme
  origination: managed
  size:
    storage: 100Gi
    iops: 5000
  ha: true
  reclaimPolicy: Retain
status:
  phase: Ready
  endpoints:
    native: "/dev/rbd-abc123"
  health:
    state: healthy
    message: "RBD healthy, replicas: 3/3"
```

**Storage class:**

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: nest-block
provisioner: rook-ceph.rbd.csi.ceph.com
parameters:
  ceph.com/pool: rbd
reclaimPolicy: Delete
allowVolumeExpansion: true
```

---

### pvc/file

**Backend:** CephFS
**Access Mode:** ReadWriteOnce
**Typical throughput:** Up to 5K IOPS per mount
**HA support:** Yes (via CephFS replicas)

Filesystem-aware storage for single-pod attachment. Similar to `pvc/block` but with POSIX filesystem semantics. Mount-exclusive; cannot be shared.

**Required spec fields:**

- `type: "pvc/file"`
- `class`: Storage class (default: `rook-cephfs`)
- `size.storage`: Capacity (e.g., `"50Gi"`)

**Status fields (when Ready):**

- `endpoints.native`: Mount path (e.g., `/mnt/mydata`)
- `health`: Filesystem health and available inode count

**Example DataResource:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: app-cache
  namespace: myapp
spec:
  type: pvc/file
  class: rook-cephfs
  tenant: acme
  origination: managed
  size:
    storage: 50Gi
  reclaimPolicy: Delete
status:
  phase: Ready
  endpoints:
    native: "/mnt/app-cache"
  health:
    state: healthy
    message: "CephFS mount healthy, available inodes: 1.2M"
```

---

## Object Storage Types

### object

**Backend:** Ceph RGW (RADOS Gateway)
**Access Mode:** S3 API (HTTP/HTTPS)
**Typical throughput:** Up to 10K req/sec per bucket
**HA support:** Yes (via Ceph replication)

S3-compatible object storage for unstructured data, backups, ML datasets, or archival. Bucket-based key-value store with no filesystem semantics.

**Required spec fields:**

- `type: "object"`
- `class`: Storage class (default: `nest-object`)
- `size.storage`: Capacity quota (e.g., `"1Ti"`)

**Status fields (when Ready):**

- `endpoints.rest`: S3 endpoint (e.g., `https://rook-ceph-rgw.rook-ceph.svc.cluster.local`)
- `health`: Bucket availability and replication status

**Example DataResource:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: backup-bucket
  namespace: myapp
spec:
  type: object
  class: nest-object
  tenant: acme
  origination: managed
  size:
    storage: 1Ti
  ha: true
  reclaimPolicy: Retain
status:
  phase: Ready
  endpoints:
    rest: "https://rook-ceph-rgw.rook-ceph.svc.cluster.local:443"
  health:
    state: healthy
    message: "Bucket replicas: 3/3, lifecycle policies active"
```

**Access pattern:**

```bash
# Use aws-cli with S3 API
aws s3 cp --endpoint-url https://rook-ceph-rgw.rook-ceph.svc.cluster.local \
  mydata.tar.gz s3://nest-acme-backup-bucket/
```

---

### s3

**Backend:** AWS S3 (cloud-managed)
**Access Mode:** S3 API
**Origination:** external only

Cloud-managed S3 bucket. Nest manages access credentials and enforces tenant isolation via IAM policies.

**Required spec fields:**

- `type: "s3"`
- `origination: "external"`
- `external.provider: "aws"`
- `external.region`: AWS region (e.g., `"us-west-2"`)
- `external.resourceId`: Bucket ARN or name

**Example DataResource:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: aws-s3-bucket
  namespace: myapp
spec:
  type: s3
  tenant: acme
  origination: external
  external:
    provider: aws
    region: us-west-2
    resourceId: my-app-bucket-prod
    credentialSecret: aws-iam-creds
    objectBucket:
      versioning: true
      encryptionType: AES256
      publicAccessBlock: true
      lifecycleDays: 90
status:
  phase: Ready
  endpoints:
    rest: "https://my-app-bucket-prod.s3.us-west-2.amazonaws.com"
  health:
    state: healthy
```

---

### gcs

**Backend:** Google Cloud Storage (cloud-managed)
**Access Mode:** S3 API / GCS API
**Origination:** external only

Google Cloud Storage bucket. Nest manages service account credentials.

**Required spec fields:**

- `type: "gcs"`
- `origination: "external"`
- `external.provider: "gcp"`
- `external.region`: GCP region (e.g., `"us-central1"`)
- `external.resourceId`: Bucket name

**Example DataResource:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: gcp-gcs-bucket
  namespace: myapp
spec:
  type: gcs
  tenant: acme
  origination: external
  external:
    provider: gcp
    region: us-central1
    resourceId: my-app-bucket-prod
    credentialSecret: gcp-sa-creds
    objectBucket:
      versioning: true
      encryptionType: "google-managed"
status:
  phase: Ready
  endpoints:
    rest: "https://storage.googleapis.com/my-app-bucket-prod"
  health:
    state: healthy
```

---

### azure-blob

**Backend:** Azure Blob Storage (cloud-managed)
**Access Mode:** Azure Blob API
**Origination:** external only

Azure Blob Storage container. Nest manages access keys and SAS tokens.

**Required spec fields:**

- `type: "azure-blob"`
- `origination: "external"`
- `external.provider: "azure"`
- `external.region`: Azure region (e.g., `"eastus"`)
- `external.resourceId`: Container name or resource URI

**Example DataResource:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: azure-blob-container
  namespace: myapp
spec:
  type: azure-blob
  tenant: acme
  origination: external
  external:
    provider: azure
    region: eastus
    resourceId: my-app-container
    credentialSecret: azure-storage-creds
    objectBucket:
      versioning: true
      encryptionType: "Microsoft.Storage/storageAccounts/encryptionServices/blob"
status:
  phase: Ready
  endpoints:
    rest: "https://myappaccount.blob.core.windows.net/my-app-container"
  health:
    state: healthy
```

---

## File/Network Storage Types

### filesystem

**Backend:** CephFS
**Access Mode:** ReadWriteMany
**Typical throughput:** Up to 3K IOPS shared across consumers
**HA support:** Yes (via CephFS subvolumes)

Shared filesystem accessible by multiple pods simultaneously. POSIX-compliant with strong consistency within the cluster. Similar to NFS but directly backed by Ceph.

**Required spec fields:**

- `type: "filesystem"`
- `class`: Storage class (default: `rook-cephfs`)
- `size.storage`: Capacity (e.g., `"200Gi"`)

**Status fields (when Ready):**

- `endpoints.native`: Mount path (e.g., `/mnt/shared`)
- `health`: Filesystem health, available inodes, concurrent client count

**Example DataResource:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: shared-data
  namespace: myapp
spec:
  type: filesystem
  class: rook-cephfs
  tenant: acme
  origination: managed
  size:
    storage: 200Gi
  ha: true
  reclaimPolicy: Delete
status:
  phase: Ready
  endpoints:
    native: "/mnt/shared-data"
  health:
    state: healthy
    message: "CephFS RWX healthy, 5 active clients, 2.1M available inodes"
```

**Multi-pod usage:**

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: worker-a
spec:
  containers:
    - name: app
      volumeMounts:
        - name: shared
          mountPath: /data
  volumes:
    - name: shared
      persistentVolumeClaim:
        claimName: shared-data
```

---

### nfs

**Backend:** NFS-Ganesha / CephFS
**Access Mode:** ReadWriteMany
**Protocol:** NFSv3 / NFSv4.1
**Typical throughput:** Up to 5K ops/sec shared
**HA support:** Yes (with NFS failover)

Legacy NFS client support. Bridges non-Kubernetes systems (VMs, bare metal, traditional Linux/Unix) with Nest storage. Backed by Ceph but exposed via standard NFS protocol.

**Required spec fields:**

- `type: "nfs"`
- `class`: Storage class (default: `nest-nfs`)
- `size.storage`: Capacity (e.g., `"500Gi"`)
- `annotations["nest.penguintech.io/nfs-allowed-clients"]`: CIDR permitted to mount the export
- `import.connectionString` (if imported): NFS server and path

**Access control:** the allowed-clients CIDR is the export's only access control — NFS-Ganesha admits any client within it. It has no default: a DataResource without the annotation goes to `Failed` rather than provisioning an export the whole cluster can mount. Scope it as narrowly as the consuming workload allows.

**Status fields (when Ready):**

- `endpoints.native`: NFS mount string (e.g., `nest-nfs-ganesha.rook-ceph.svc:/ exports/myapp`)
- `health`: NFS daemon status, export availability

**Example DataResource (managed):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: nfs-export
  namespace: myapp
spec:
  type: nfs
  class: nest-nfs
  tenant: acme
  origination: managed
  annotations:
    nest.penguintech.io/nfs-allowed-clients: "10.42.7.0/24"
  size:
    storage: 500Gi
  ha: true
  reclaimPolicy: Retain
status:
  phase: Ready
  endpoints:
    native: "nest-nfs-ganesha.rook-ceph.svc.cluster.local:/exports/acme-nfs-export"
  health:
    state: healthy
    message: "NFS Ganesha daemon healthy, failover enabled"
```

**Mount on legacy system:**

```bash
# Linux/macOS/Unix
mount -t nfs -o vers=4.1,proto=tcp \
  nest-nfs-ganesha.rook-ceph.svc.cluster.local:/exports/acme-nfs-export \
  /mnt/nest-data

# macOS (with vers option)
mount_nfs -o vers=4.1,proto=tcp \
  nest-nfs-ganesha.rook-ceph.svc.cluster.local:/exports/acme-nfs-export \
  /mnt/nest-data
```

---

### rockfs

**Backend:** RockFS (RocksDB + network protocol)
**Access Mode:** ReadWriteMany (streaming, eventual consistency)
**Typical throughput:** Up to 100K ops/sec per shard
**HA support:** Yes (via sharding + replication)

Distributed key-value filesystem optimized for streaming writes and sequential reads. Not POSIX; specialized for append-only workloads (logs, time-series, event streams).

**Required spec fields:**

- `type: "rockfs"`
- `class`: Storage class
- `size.storage`: Capacity (e.g., `"1Ti"`)

**Status fields (when Ready):**

- `endpoints.grpc`: gRPC endpoint
- `endpoints.rest`: REST endpoint (if enabled)
- `health`: Shard replication status, compaction progress

**Example DataResource:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: event-log
  namespace: analytics
spec:
  type: rockfs
  tenant: acme
  origination: managed
  size:
    storage: 1Ti
  protocols:
    - grpc
  ha: true
status:
  phase: Ready
  endpoints:
    grpc: "rockfs.analytics.svc.cluster.local:50051"
  health:
    state: healthy
    message: "Shards: 8/8 healthy, replication factor: 3"
```

---

### iscsi

**Backend:** Ceph RBD / iSCSI Gateway
**Access Mode:** Block (iSCSI initiator)
**Typical throughput:** Up to 15K IOPS per target
**HA support:** Yes (via target portal redundancy)

Block storage accessible over iSCSI protocol. Bridges Kubernetes and non-Kubernetes environments requiring raw block access without direct Ceph client or NFS.

**Required spec fields:**

- `type: "iscsi"`
- `class`: Storage class (default: `nest-iscsi`)
- `size.storage`: Capacity (e.g., `"500Gi"`)
- `annotations["nest.penguintech.io/iscsi-initiator-iqn"]`: IQN of the initiator permitted to attach the target

**Access control:** the initiator IQN is the target's ACL. It has no default: a DataResource without the annotation goes to `Failed` rather than provisioning a LUN attachable by anyone who can reach the portal.

**CHAP authentication:** provisioned automatically. On first reconcile the controller generates a 16-character CHAP secret (the range the Windows initiator accepts) and stores it in a Secret named `{tenant}-{name}-iscsi-chap` in the tenant namespace, owned by the DataResource so it is removed with it. Read the credentials with:

```bash
kubectl -n acme get secret acme-vm-disk-iscsi-chap \
  -o jsonpath='{.data.username}' | base64 -d
kubectl -n acme get secret acme-vm-disk-iscsi-chap \
  -o jsonpath='{.data.password}' | base64 -d
```

**Status fields (when Ready):**

- `endpoints.native`: iSCSI target IQN and portal (e.g., `iqn.2026-04.nest:target/acme-vm-disk`)
- `health`: iSCSI target status, portal availability

**Example DataResource:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: vm-disk
  namespace: infrastructure
spec:
  type: iscsi
  tenant: acme
  origination: managed
  annotations:
    nest.penguintech.io/iscsi-initiator-iqn: "iqn.1993-08.org.debian:01:9a8b7c6d5e4f"
  size:
    storage: 500Gi
  ha: true
  reclaimPolicy: Retain
status:
  phase: Ready
  endpoints:
    native: "iqn.2026-04.nest.rook-ceph:target/acme-vm-disk"
  health:
    state: healthy
    message: "iSCSI targets: 3/3 available, failover enabled"
```

**iSCSI discovery (Linux initiator):**

```bash
# Discover targets on iSCSI gateway
iscsiadm -m discovery -t st -p nest-iscsi-gateway.rook-ceph.svc.cluster.local:3260

# Login to target
iscsiadm -m node -T iqn.2026-04.nest.rook-ceph:target/acme-vm-disk \
  -p nest-iscsi-gateway.rook-ceph.svc.cluster.local:3260 \
  --login

# Verify block device appeared
lsblk
```

---

## Database Types

### postgres

**Backend:** CloudNativePG (CNPG) / PostgreSQL (managed or imported)
**Access Mode:** Client-server (TCP, port 5432 default)
**Typical throughput:** 5K-50K transactions/sec (depends on HA config)
**HA support:** Yes (via CNPG streaming replication)

Relational database management system. Nest can provision a managed cluster (multi-node with automatic failover) or import an existing PostgreSQL instance.

**Required spec fields:**

- `type: "postgres"`
- `class`: DataResourceClass reference (determines HA level, compute, storage)
- `tenant`: Tenant isolation via row-level security (RLS)

**Optional spec fields:**

- `ha: true`: Enable HA (default: true for managed)
- `replicas.write.count`: Number of write-capable replicas (default: 3)
- `replicas.read.count`: Number of read-only standby replicas
- `size.storage`: Volume per replica
- `size.iops`: IOPS target

**Status fields (when Ready):**

- `endpoints.native`: Connection string (e.g., `postgresql://postgres-0.postgres:5432/postgres`)
- `endpoints.rest`: REST endpoint (if enabled via Supabase-style proxy)
- `health`: Replication lag, active connections, transaction rate

**Example (managed):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: production-db
  namespace: myapp
spec:
  type: postgres
  class: production-3x-100gb
  tenant: acme
  origination: managed
  ha: true
  replicas:
    write:
      count: 3
    read:
      min: 1
      max: 5
  size:
    storage: 100Gi
    iops: 10000
  tls:
    mode: required
    minVersion: "1.3"
status:
  phase: Ready
  endpoints:
    native: "postgresql://acme-production-db-rw.myapp.svc.cluster.local:5432/acme_db"
  health:
    state: healthy
    message: "Replicas: 3/3 healthy, lag: 0.02s, connections: 45/200"
```

**Example (imported):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: legacy-postgres
  namespace: myapp
spec:
  type: postgres
  class: imported
  tenant: acme
  origination: imported
  import:
    connectionString: "postgresql://admin@db.example.com:5432/legacy_prod"
    tlsMode: verify-full
    credentialSecret: legacy-db-creds
    managedCredentials: true
    managedFailover: false
status:
  phase: Ready
  endpoints:
    native: "postgresql://admin@db.example.com:5432/legacy_prod"
  health:
    state: healthy
```

---

### keyvalue

**Backend:** Valkey (Redis fork) / Redis / Memcached
**Access Mode:** Client-server (TCP, port 6379 default)
**Typical throughput:** 100K-1M ops/sec (depends on setup)
**HA support:** Yes (via Valkey cluster or Sentinel)

In-memory key-value cache. Supports both managed (Nest-provisioned Valkey cluster) and imported (external Redis/Valkey).

**Required spec fields:**

- `type: "keyvalue"`
- `class`: DataResourceClass
- `tenant`: Tenant isolation via key-prefix ACLs

**Optional spec fields:**

- `ha: true`: Enable cluster mode (default: false for simple mode)
- `replicas.write.count`: Cluster replicas (for HA)
- `size.storage`: Memory quota

**Status fields (when Ready):**

- `endpoints.native`: Connection string (e.g., `redis://keyvalue-0.keyvalue:6379`)
- `health`: Memory usage, eviction policy, replication status

**Example (managed):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: session-cache
  namespace: myapp
spec:
  type: keyvalue
  class: valkey-cluster
  tenant: acme
  origination: managed
  ha: true
  replicas:
    write:
      count: 3
  size:
    storage: 64Gi
  tls:
    mode: required
status:
  phase: Ready
  endpoints:
    native: "redis://session-cache-0.session-cache.myapp.svc.cluster.local:6379"
  health:
    state: healthy
    message: "Nodes: 3/3 healthy, memory: 32GB/64GB, eviction: off"
```

**Example (imported):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: external-redis
  namespace: myapp
spec:
  type: keyvalue
  class: imported
  tenant: acme
  origination: imported
  import:
    connectionString: "redis://:password@redis.example.com:6379/0"
    tlsMode: disable
    credentialSecret: external-redis-creds
status:
  phase: Ready
  endpoints:
    native: "redis://:***@redis.example.com:6379/0"
  health:
    state: healthy
```

---

## Analytics & Search Types

### search

**Backend:** OpenSearch (managed or shared multi-tenant)
**Access Mode:** Client-server (HTTP/HTTPS, port 9200 default)
**Typical throughput:** 1K-100K queries/sec
**HA support:** Yes (via index replication)

Full-text search and analytics engine. Supports both dedicated (single-tenant) and shared (multi-tenant SearchPool) modes.

**Required spec fields:**

- `type: "search"`
- `class`: DataResourceClass
- `tenant`: Tenant ID

**Optional spec fields:**

- `ha: true`: Enable index replication (default: true)
- `size.storage`: Total index storage budget

**Status fields (when Ready):**

- `endpoints.rest`: OpenSearch REST endpoint (e.g., `https://search.myapp.svc:9200`)
- `health`: Shard count, unassigned shards, indexing rate

**Example (dedicated):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: logs-search
  namespace: myapp
spec:
  type: search
  class: dedicated-3node
  tenant: acme
  origination: managed
  ha: true
  size:
    storage: 500Gi
  tls:
    mode: required
status:
  phase: Ready
  endpoints:
    rest: "https://logs-search.myapp.svc.cluster.local:9200"
  health:
    state: healthy
    message: "Shards: 15/15 assigned, replicas: 1, docs: 2.3B"
```

**Example (shared SearchPool):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: SearchPool
metadata:
  name: shared-search
  namespace: infra
spec:
  nodes: 5
  totalStorage: 5Ti
  replication: 2
---
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: tenant-a-search
  namespace: myapp
spec:
  type: search
  class: shared
  tenant: acme
  origination: managed
  poolRef: shared-search
  size:
    storage: 100Gi
status:
  phase: Ready
  endpoints:
    rest: "https://shared-search.infra.svc.cluster.local:9200"
  health:
    state: healthy
    message: "Shared SearchPool mode, tenant quota: 100GB/500GB used"
```

---

### vector

**Backend:** Milvus / OpenSearch Vector (vector similarity search)
**Access Mode:** gRPC / REST
**Typical throughput:** 10K-100K vector similarity queries/sec
**HA support:** Yes (via replica shards)

Vector embedding database for semantic search and similarity operations. Complements `search` for AI/ML use cases.

**Required spec fields:**

- `type: "vector"`
- `class`: DataResourceClass
- `tenant`: Tenant ID

**Status fields (when Ready):**

- `endpoints.grpc`: gRPC endpoint
- `endpoints.rest`: REST endpoint
- `health`: Collection replication, vector indexing progress

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: embeddings
  namespace: myapp
spec:
  type: vector
  class: milvus-cluster
  tenant: acme
  origination: managed
  protocols:
    - grpc
    - rest
  ha: true
  size:
    storage: 200Gi
status:
  phase: Ready
  endpoints:
    grpc: "embeddings.myapp.svc.cluster.local:19530"
    rest: "http://embeddings.myapp.svc.cluster.local:9091"
  health:
    state: healthy
    message: "Collections: 5, indexed: 1.2B vectors, replicas: 2/2"
```

---

### clickhouse

**Backend:** ClickHouse (columnar OLAP database)
**Access Mode:** Client-server (TCP, HTTP)
**Typical throughput:** 1M+ rows/sec ingestion
**HA support:** Yes (via replication and distributed queries)

Columnar database optimized for analytical queries on large datasets. Fast aggregations and time-series analysis.

**Required spec fields:**

- `type: "clickhouse"`
- `class`: DataResourceClass
- `tenant`: Tenant isolation via database/user separation

**Status fields (when Ready):**

- `endpoints.native`: ClickHouse client endpoint (e.g., `clickhouse-0.clickhouse:9000`)
- `endpoints.rest`: HTTP endpoint for REST queries
- `health`: Replication lag, query throughput, table sizes

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: analytics-db
  namespace: analytics
spec:
  type: clickhouse
  class: production
  tenant: acme
  origination: managed
  ha: true
  replicas:
    write:
      count: 3
  size:
    storage: 2Ti
status:
  phase: Ready
  endpoints:
    native: "clickhouse-0.analytics-db.analytics.svc.cluster.local:9000"
    rest: "http://clickhouse-0.analytics-db.analytics.svc.cluster.local:8123"
  health:
    state: healthy
    message: "Replicas: 3/3 healthy, tables: 12, rows: 8.9B"
```

---

### warehouse/trino

**Backend:** Trino (distributed SQL query engine) + object storage (Iceberg, Delta, Hudi)
**Access Mode:** JDBC / CLI / REST
**Typical throughput:** Depends on underlying object storage
**HA support:** Yes (via coordinator redundancy)

Distributed SQL query engine for querying data across multiple sources (Iceberg, S3, HDFS, etc.). Often paired with lakehouse engines.

**Required spec fields:**

- `type: "warehouse/trino"`
- `class`: DataResourceClass
- `tenant`: Tenant isolation via schema/role separation

**Status fields (when Ready):**

- `endpoints.native`: JDBC connection string
- `health`: Worker node count, query queue depth, uptime

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: data-warehouse
  namespace: analytics
spec:
  type: warehouse/trino
  class: production
  tenant: acme
  origination: managed
  ha: true
  replicas:
    write:
      count: 1 # Single coordinator
    read:
      count: 8 # Worker nodes
  size:
    storage: 10Ti # For temporary query results
status:
  phase: Ready
  endpoints:
    native: "jdbc:trino://data-warehouse.analytics.svc.cluster.local:8080"
  health:
    state: healthy
    message: "Coordinator: healthy, workers: 8/8, queries/sec: 45"
```

---

### lakehouse/iceberg

**Backend:** Apache Iceberg (open table format) + object storage
**Access Mode:** SQL (via Trino/Spark) or direct file API
**Typical throughput:** 100K-1M rows/sec
**HA support:** Yes (object storage provides durability)

Open lakehouse format for data lakes. Provides ACID transactions, schema evolution, and time travel on object storage (S3, GCS, Azure Blob, HDFS).

**Required spec fields:**

- `type: "lakehouse/iceberg"`
- `class`: DataResourceClass
- `tenant`: Tenant isolation via namespace/warehouse separation

**Status fields (when Ready):**

- `endpoints.native`: Object storage URI (e.g., `s3a://nest-acme-iceberg/`)
- `health`: Metadata tree depth, snapshot count, compaction status

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: data-lake
  namespace: analytics
spec:
  type: lakehouse/iceberg
  class: production
  tenant: acme
  origination: managed
  size:
    storage: 50Ti
  protocols:
    - rest # Via Iceberg REST catalog
status:
  phase: Ready
  endpoints:
    native: "s3a://nest-acme-data-lake/"
    rest: "http://iceberg-catalog.analytics.svc.cluster.local:8080"
  health:
    state: healthy
    message: "Tables: 45, snapshots: 320, partitions: 8.2K"
```

---

## Streaming Types

### kafka

**Backend:** Apache Kafka / Confluent Kafka
**Access Mode:** Broker protocol (TCP, typically port 9092)
**Typical throughput:** 1M+ msgs/sec cluster-wide
**HA support:** Yes (via broker replication, min.insync.replicas)

Distributed event streaming platform. Supports both managed (Nest-provisioned cluster) and imported (external Kafka).

**Required spec fields:**

- `type: "kafka"`
- `class`: DataResourceClass
- `tenant`: Tenant isolation via topic ACLs and consumer group prefixes

**Optional spec fields:**

- `ha: true`: Enable broker replication
- `replicas.write.count`: Replication factor (default: 3)
- `size.storage`: Total storage across brokers

**Status fields (when Ready):**

- `endpoints.native`: Bootstrap servers (e.g., `kafka-0.kafka:9092,kafka-1.kafka:9092`)
- `health`: Broker count, topic count, under-replicated partition count

**Example (managed):**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: event-stream
  namespace: myapp
spec:
  type: kafka
  class: production
  tenant: acme
  origination: managed
  ha: true
  replicas:
    write:
      count: 5
  size:
    storage: 2Ti
  tls:
    mode: required
    clientAuth: required
status:
  phase: Ready
  endpoints:
    native: "kafka-0.event-stream.myapp.svc.cluster.local:9092,kafka-1.event-stream.myapp.svc.cluster.local:9092,kafka-2.event-stream.myapp.svc.cluster.local:9092,kafka-3.event-stream.myapp.svc.cluster.local:9092,kafka-4.event-stream.myapp.svc.cluster.local:9092"
  health:
    state: healthy
    message: "Brokers: 5/5, topics: 23, URPs: 0"
```

---

## Extended Types

### timeseries

**Backend:** VictoriaMetrics / Prometheus
**Access Mode:** HTTP/HTTPS (port 8428 default)
**Typical throughput:** 1M+ metrics/sec
**HA support:** Yes (via clustering)

Time-series database for metrics, monitoring, and observability. High-performance storage and querying.

**Required spec fields:**

- `type: "timeseries"`
- `class`: DataResourceClass
- `tenant`: Tenant isolation via relabeling and RBAC

**Status fields (when Ready):**

- `endpoints.rest`: VictoriaMetrics HTTP endpoint
- `health`: Ingestion rate, query latency, storage usage

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: metrics
  namespace: monitoring
spec:
  type: timeseries
  class: production
  tenant: acme
  origination: managed
  ha: true
  size:
    storage: 500Gi
status:
  phase: Ready
  endpoints:
    rest: "http://metrics.monitoring.svc.cluster.local:8428"
  health:
    state: healthy
    message: "Ingestion: 2.3M samples/sec, query latency: 85ms p99"
```

---

## Cloud Storage Types

Cloud-native storage types backed by AWS, GCP, Azure, or Tier 2 providers. Always require `origination: external`.

### ebs

**Backend:** AWS EBS (Elastic Block Store)
**Access Mode:** Block (via attachment to EC2)
**Origination:** external only

AWS Elastic Block Store volumes. Nest manages access and enforces tenant isolation via IAM policies.

**Required spec fields:**

- `type: "ebs"`
- `origination: "external"`
- `external.provider: "aws"`
- `external.region`: AWS region
- `external.resourceId`: Volume ID (e.g., `vol-0123456789abcdef0`)
- `external.blockVolume.sizeGB`: Volume size

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: ebs-volume
  namespace: infrastructure
spec:
  type: ebs
  tenant: acme
  origination: external
  external:
    provider: aws
    region: us-west-2
    resourceId: vol-0123456789abcdef0
    credentialSecret: aws-creds
    blockVolume:
      sizeGB: 500
      iops: 3000
      throughput: 125
      volumeType: gp3
      availabilityZone: us-west-2a
      encryptionKeyId: arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012
status:
  phase: Ready
  health:
    state: healthy
```

---

### azure-disk

**Backend:** Azure Managed Disk
**Access Mode:** Block (via attachment to VM)
**Origination:** external only

Azure Managed Disk volumes.

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: azure-disk
  namespace: infrastructure
spec:
  type: azure-disk
  tenant: acme
  origination: external
  external:
    provider: azure
    region: eastus
    resourceId: /subscriptions/SUB_ID/resourceGroups/RG/providers/Microsoft.Compute/disks/myDisk
    credentialSecret: azure-creds
    blockVolume:
      sizeGB: 500
      volumeType: Premium_LRS
status:
  phase: Ready
  health:
    state: healthy
```

---

### gcp-disk

**Backend:** Google Persistent Disk
**Access Mode:** Block (via attachment to Compute Engine VM)
**Origination:** external only

Google Cloud Persistent Disk volumes.

**Example:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: gcp-disk
  namespace: infrastructure
spec:
  type: gcp-disk
  tenant: acme
  origination: external
  external:
    provider: gcp
    region: us-central1
    resourceId: projects/PROJECT_ID/zones/us-central1-a/disks/myDisk
    credentialSecret: gcp-creds
    blockVolume:
      sizeGB: 500
      volumeType: pd-ssd
status:
  phase: Ready
  health:
    state: healthy
```

---

## Access Modes

### ReadWriteOnce (RWO)

- **Semantics:** Single pod can mount; exclusive access
- **Use cases:** Databases, VM disks, any single-consumer workload
- **Storage types:** `pvc/block`, `pvc/file`
- **Performance:** Highest (no contention)

### ReadWriteMany (RWX)

- **Semantics:** Multiple pods can mount simultaneously; shared access
- **Use cases:** Shared filesystems, log aggregation, distributed caches
- **Storage types:** `filesystem`, `nfs`, `rockfs`
- **Performance:** Lower due to coordination; degrade gracefully with many writers

### ReadOnlyMany (ROX)

- **Semantics:** Multiple pods can read; no writes
- **Use cases:** Shared config, reference data, container images
- **Storage types:** Derived from RWX types via read-only mounts
- **Performance:** Similar to RWX but no write coordination needed

### S3 API

- **Semantics:** HTTP/HTTPS bucket operations (GetObject, PutObject, DeleteObject, etc.)
- **Use cases:** Unstructured data, backups, ML datasets
- **Storage types:** `object`, `s3`, `gcs`, `azure-blob`
- **Performance:** Highly scalable; per-bucket throughput typically 10K-100K req/sec

### Client-Server (Database/Cache)

- **Semantics:** Native wire protocol (e.g., PostgreSQL, Redis, OpenSearch)
- **Use cases:** Database clients, cache clients, search queries
- **Storage types:** `postgres`, `keyvalue`, `search`, `vector`, `kafka`, `clickhouse`
- **Performance:** Depends on engine; typical 1K-100K ops/sec per node

---

## DataResource Spec Reference

### Spec Structure

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: resource-name
  namespace: namespace
spec:
  # Required
  type: string                          # Storage type constant (e.g., "postgres", "object")
  tenant: string                        # Tenant ID for isolation
  class: string                         # Reference to DataResourceClass

  # Common optional
  origination: managed | imported | external  # Default: managed
  protocols: [native | grpc | rest]    # Enabled access protocols
  size:
    storage: string                     # Capacity (e.g., "100Gi")
    iops: integer                       # Target IOPS (optional)

  # HA & replication
  ha: boolean                           # Enable high availability
  replicas:
    write:
      min: integer
      max: integer
      default: integer
      count: integer
    read:
      min: integer
      max: integer
      default: integer

  # Security & encryption
  tls:
    mode: required | preferred | disabled
    minVersion: string                  # TLS version (e.g., "1.3")
    clientAuth: none | optional | required
    certSource: nest-ca | cert-manager | byo
    atRestKmsId: string                 # KMS provider ID

  # Secrets management
  secretsBackend:
    kind: nest-envelope | vault | infisical | aws-sm | gcp-sm | azure-kv | bitwarden
    ref: string

  # Data protection
  dataProtectionPolicy: string          # Reference to DataProtectionPolicy

  # Import spec (when origination: imported)
  import:
    connectionString: string
    tlsMode: verify-full | verify-ca | require | disable
    credentialSecret: string            # K8s Secret name
    managedCredentials: boolean
    managedFailover: boolean

  # External spec (when origination: external)
  external:
    provider: string                    # aws | gcp | azure | vultr | cloudflare | custom-*
    region: string
    resourceId: string                  # ARN/ID/self-link
    credentialSecret: string
    costTagKey: string
    endpoint: string                    # For Tier 2 standard-protocol
    engineType: string                  # For Tier 2
    blockVolume:
      sizeGB: integer
      iops: integer
      throughput: integer
      volumeType: string
      availabilityZone: string
      encryptionKeyId: string
      multiAttach: boolean
    objectBucket:
      bucketName: string
      versioning: boolean
      encryptionType: string
      lifecycleDays: integer
      publicAccessBlock: boolean
      crossRegionReplication: boolean
      replicationTargetRegion: string
    extra: {}                           # Provider-specific config

  # Annotations
  annotations: {}
status:
  phase: Unknown | Pending | Provisioning | Ready | Degraded | Failed | Deleting
  conditions:
    - type: string
      status: "True" | "False" | "Unknown"
      reason: string
      message: string
      lastTransitionTime: timestamp
  endpoints:
    native: string                      # Native protocol endpoint
    grpc: string                        # gRPC endpoint (if enabled)
    rest: string                        # REST endpoint (if enabled)
  health:
    state: healthy | degraded | down
    message: string
  currentOperation: string              # In-flight operation (e.g., "backupInProgress")
  provisionedAt: timestamp
```

---

## Eggs

An **egg** is a deployable bundle of DataResources and/or data processors (transformations, ETL jobs) as a unit of composition. Eggs enable shipping multi-component workloads as a single package.

**Structure:**

```yaml
apiVersion: nest.penguintech.io/v1
kind: Egg
metadata:
  name: analytics-stack
  namespace: default
spec:
  description: "Complete analytics platform with data lake, warehouse, and search"
  version: "1.0.0"
  maintainer: "analytics-team"

  # Resources included in this egg
  resources:
    # Data resources
    - type: DataResource
      name: raw-data-lake
      spec:
        type: lakehouse/iceberg
        class: production
        size:
          storage: 50Ti

    - type: DataResource
      name: analytics-warehouse
      spec:
        type: warehouse/trino
        class: production
        replicas:
          read:
            count: 8

    - type: DataResource
      name: full-text-search
      spec:
        type: search
        class: dedicated-5node
        size:
          storage: 1Ti

    # Processors (ETL/transformations)
    - type: Processor
      name: raw-to-curated
      spec:
        source: raw-data-lake
        target: analytics-warehouse
        schedule: "0 2 * * *" # Daily at 2am

    - type: Processor
      name: index-generator
      spec:
        source: analytics-warehouse
        target: full-text-search
        schedule: "0 3 * * *" # Daily at 3am

  # Optional: input parameters
  parameters:
    - name: tenant
      description: "Tenant ID"
      required: true
    - name: region
      description: "Deployment region"
      default: "us-west-2"

  # Optional: output references
  outputs:
    datalakeUri: "${resources.raw-data-lake.endpoints.native}"
    warehouseJdbc: "${resources.analytics-warehouse.endpoints.native}"
    searchEndpoint: "${resources.full-text-search.endpoints.rest}"
```

**Usage:**

```bash
# Install egg into cluster
nest egg install analytics-stack --tenant acme --region us-west-2

# Check egg status
nest egg status analytics-stack -n myapp

# Upgrade egg
nest egg upgrade analytics-stack --version 1.1.0

# Uninstall egg (cascades to resources)
nest egg delete analytics-stack
```

---

## Decision Matrix

Choose the right storage type for your workload:

| Requirement                            | Recommended Type                        | Alternative                             | Avoid                    |
| -------------------------------------- | --------------------------------------- | --------------------------------------- | ------------------------ |
| **Single-pod database**                | `postgres` (managed)                    | Imported PostgreSQL                     | `object`, `nfs`          |
| **High-concurrency shared filesystem** | `filesystem`                            | `nfs` (for legacy)                      | `pvc/block`, `pvc/file`  |
| **Object storage (unstructured)**      | `object` (managed)                      | `s3`, `gcs`, `azure-blob` (cloud)       | `postgres`, `filesystem` |
| **Full-text search (single tenant)**   | `search` (dedicated)                    | `vector` (if semantic)                  | `object`, `pvc/block`    |
| **Multi-tenant search**                | `search` (shared SearchPool)            | None                                    | Dedicated per tenant     |
| **Vector similarity**                  | `vector`                                | None                                    | `search` alone           |
| **Analytics (SQL)**                    | `warehouse/trino` + `lakehouse/iceberg` | `clickhouse` (OLAP only)                | `postgres`               |
| **Time-series metrics**                | `timeseries`                            | None                                    | `postgres`               |
| **Distributed streaming**              | `kafka` (managed)                       | Imported Kafka                          | `object`, `filesystem`   |
| **High-IOPS block I/O**                | `pvc/block`                             | `ebs`, `azure-disk`, `gcp-disk` (cloud) | `filesystem`, `nfs`      |
| **VM disk**                            | `iscsi` (K8s) or `ebs` (cloud)          | `pvc/block` (K8s)                       | `object`                 |
| **Cache**                              | `keyvalue` (managed Valkey)             | Imported Redis                          | `postgres`, `filesystem` |
| **Legacy NFS client access**           | `nfs`                                   | None                                    | `filesystem` alone       |
| **Append-only event logs**             | `rockfs`                                | `kafka` (if streaming)                  | `postgres`               |

---

## Status Phases

All DataResources follow a consistent lifecycle:

| Phase          | Meaning                                                          | Operator Action                          |
| -------------- | ---------------------------------------------------------------- | ---------------------------------------- |
| `Unknown`      | Initial state; controller hasn't reconciled                      | Wait                                     |
| `Pending`      | Prerequisites not met (e.g., hardware not available)             | Check DarkDrives, resource limits        |
| `Provisioning` | Infrastructure being created; may take minutes                   | Monitor `status.currentOperation`        |
| `Ready`        | Operational; endpoints available                                 | Use the resource                         |
| `Degraded`     | Operational but degraded (e.g., 1 replica down in 3-replica set) | Investigate `status.conditions`          |
| `Failed`       | Cannot provision or has failed unhealthily                       | Check `status.conditions` for root cause |
| `Deleting`     | Deletion in progress; cascading to resources                     | Wait for cleanup                         |

---

## Condition Types

Common conditions on DataResources:

| Type                 | Meaning                                   | Example                                          |
| -------------------- | ----------------------------------------- | ------------------------------------------------ |
| `Ready`              | Resource is operational                   | Status: True (ready) or False (not ready)        |
| `Healthy`            | Health check passed                       | Status: True (healthy) or False (degraded/down)  |
| `ReplicationHealthy` | HA replication is healthy                 | Status: True (all replicas) or False (some down) |
| `CredentialValid`    | Credentials (imported/external) are valid | Status: True or False (expired/invalid)          |
| `BackupScheduled`    | Data protection backup is scheduled       | Status: True or False                            |

---

## Version & Changelog

- **v1.0.0** (May 2026): Initial comprehensive reference
- **Supported Nest versions:** P1+
- **Last updated:** 2026-05-01
