[[_TOC_]]

# Nest — Unified Storage & Data Platform

Nest is a Kubernetes-native unified storage and data platform. It provides a comprehensive data infrastructure abstraction layer via DataResource custom resources (CRDs), backed by Rook-Ceph for storage primitives, CloudNativePG for PostgreSQL, Strimzi for Kafka, and integrations for managed databases and cloud-native storage services.

**Module:** `github.com/penguintechinc/nest`
**Default namespace:** `nest`
**API base:** `http://nest-api.nest.svc:8080/api/v1`

---

## 1. Core Concepts & Terminology

| Term | Definition |
|------|-----------|
| **DataResource** | A single unit of managed storage, database, or compute resource — a block volume, PostgreSQL instance, Kafka cluster, object bucket, or any supported storage/database type. DataResources are Kubernetes Custom Resources (CRDs) that declare intent; the controller translates them into upstream operator resources. |
| **Egg** | A named, versioned package (bundle) of one or more DataResource objects and/or data processors. Eggs are the unit of composition in Nest: group related resources together, deploy them as one, share them across tenants, and manage them as a single deployable unit. The name is intentional — eggs live in a Nest. |
| **Tenant** | An isolated, namespace-level grouping; all DataResources are scoped to a single tenant. Isolation is enforced at the API layer via JWT claims and at the data layer for shared resources (e.g., shared search pools). Each tenant has its own namespace and isolated view of resources. |
| **DarkDrive** | A storage device (state: `Dark`) on a node detected by the node-agent that is not yet allocated to any workload. Nest's drive preference policy always prefers dark drives (unallocated, non-OS drives) over other drives when provisioning new storage pools. Dark drives are the primary storage resource for Ceph cluster expansion. |
| **System Drive** | A block device hosting the OS root (`/`), boot, or swap partition on a node. System drives are marked with state `System` and are never adopted by Nest — they are exclusively managed by the operating system. Nest always prefers dark (unallocated) drives and expects dedicated, non-system storage to be present on each node for storage provisioning. |
| **DataProtectionPolicy** | A Kubernetes Custom Resource that configures data protection strategies for DataResources: snapshots (local VolumeSnapshots via Ceph), backups (remote Velero backups to S3/RGW), point-in-time recovery (PITR) windows, cross-region replication, and restore verification (periodic test restores to scratch namespaces). |
| **SearchPool** | A shared OpenSearch cluster used by multiple DataResource instances for cost-efficient multi-tenant search. Index-prefix isolation via OpenSearch security roles ensures tenant isolation within a shared cluster. Can also deploy dedicated search clusters when isolation or performance requirements demand. |
| **Origination Mode** | How a DataResource is provisioned: `managed` (Nest provisions and owns the resource), `imported` (existing external resource; Nest adopts and manages it), `external` (cloud-managed resource like AWS EBS/S3; Nest acts as a control plane interface). |
| **Access Protocol** | How a DataResource is accessed: `native` (wire-protocol like `postgres://`, `redis://`), `grpc` (gRPC service interface), `rest` (HTTP REST API via gateway service). Multiple protocols can be enabled on a single DataResource. |

---

## 2. Prerequisites

| Requirement | Notes |
|---|---|
| Kubernetes 1.28+ | MicroK8s for alpha, remote cluster for beta/prod |
| Rook-Ceph 1.12+ | Must be deployed before Nest provisioning works; provides RBD (block) and CephFS (file) pools, RGW (object) |
| kubectl | Configured with appropriate cluster context and credentials |
| Helm 3.13+ | For beta and production deployments |
| Go 1.24.2+ | For building from source only |

### Cluster Contexts

| Environment | Context | Registry | Deployment Tool |
|---|---|---|---|
| Alpha (local) | `local-alpha` | `localhost:32000` (MicroK8s) | Kustomize |
| Beta | `dal2-beta` | `ghcr.io/penguintechinc/nest` | Helm 3 |
| Production | `{product}-prod` | `ghcr.io/penguintechinc/nest` | Helm 3 |

### Rook-Ceph Requirement

Nest's storage layer depends on Rook-Ceph:
- **RBD pools** back `pvc/block` DataResources (raw block volumes, RWO)
- **CephFS** backs `pvc/file` and `filesystem` DataResources (shared filesystems, RWX)
- **RGW** backs `object` DataResources (S3-compatible bucket storage)
- **Snapshots** via Ceph VolumeSnapshot integration for data protection

Deploy Rook-Ceph before provisioning any DataResources. The node-agent detects available drives and reports them to the scheduler for Ceph OSD allocation.

---

## 3. Installation

### Prerequisites Checklist

```bash
# Verify Kubernetes
kubectl version --short
# Expected: Server version 1.28+

# Verify Rook-Ceph deployed
kubectl get pod -n rook-ceph
# Expected: rook-ceph-operator, OSD pods on nodes with dark drives

# Verify Ceph storage pool exists
kubectl get CephBlockPool -n rook-ceph
# Expected: replicapool, device-health-metrics, or custom pool
```

### Alpha (Local / MicroK8s)

Alpha deploys via Kustomize against the `local-alpha` context.

```bash
# 1. Enable local registry if not already
microk8s enable registry

# 2. Build and push images to local registry
docker build -t localhost:32000/nest-api:latest -f apps/api/Dockerfile .
docker push localhost:32000/nest-api:latest

docker build -t localhost:32000/nest-controller:latest -f services/k8s-controller/Dockerfile .
docker push localhost:32000/nest-controller:latest

docker build -t localhost:32000/nest-node-agent:latest -f services/node-agent/Dockerfile .
docker push localhost:32000/nest-node-agent:latest

# 3. Deploy Nest via Kustomize (includes namespace creation, RBAC, all components)
kubectl kustomize k8s/kustomize/overlays/alpha | kubectl --context local-alpha apply -f -

# 4. Verify pods running
kubectl --context local-alpha get pods -n nest
# Expected: nest-api-*, nest-controller-*, nest-node-agent-*

# 5. Check CSI driver registration
kubectl --context local-alpha get csidrivers | grep nest
# Expected: csi.nest.penguintech.io

# 6. Check StorageClasses
kubectl --context local-alpha get storageclasses | grep nest
# Expected: nest-block, nest-filesystem, nest-file
```

### Beta

Beta deploys via Helm from CI-built images on `ghcr.io`. **Never build and push images manually for beta** — CI builds and pushes images on release branch builds.

```bash
# 1. Add Nest Helm repo (if not already)
helm repo add nest https://charts.penguintech.io
helm repo update

# 2. Update Helm dependencies
helm dependency update k8s/helm/nest

# 3. Deploy via Helm
helm upgrade --install nest k8s/helm/nest \
  --kube-context dal2-beta \
  -n nest \
  --create-namespace \
  --reset-values \
  -f k8s/helm/nest/values.yaml \
  -f k8s/helm/nest/values-beta.yaml

# 4. Verify deployment
kubectl --context dal2-beta get pods -n nest
kubectl --context dal2-beta get svc -n nest
kubectl --context dal2-beta logs -n nest -l app=nest-api --tail=50
```

### Production

```bash
# 1. Update Helm dependencies
helm dependency update k8s/helm/nest

# 2. Deploy via Helm with prod values
helm upgrade --install nest k8s/helm/nest \
  --kube-context {product}-prod \
  -n nest \
  --create-namespace \
  --reset-values \
  -f k8s/helm/nest/values.yaml \
  -f k8s/helm/nest/values-prod.yaml

# 3. Verify deployment
kubectl --context {product}-prod get pods -n nest
kubectl --context {product}-prod rollout status deployment/nest-api -n nest
kubectl --context {product}-prod get svc -n nest
```

### Verify Deployment

```bash
# Check all Nest components
kubectl --context <context> get pods -n nest

# Check CSI driver registration
kubectl --context <context> get csidrivers | grep nest

# Check StorageClasses
kubectl --context <context> get storageclasses | grep nest

# Tail API logs (should see health/readiness checks)
kubectl --context <context> logs -n nest -l app=nest-api --tail=50

# Check API is healthy
kubectl --context <context> port-forward svc/nest-api 8080:8080 -n nest &
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

---

## 4. StorageClasses

Nest provides branded StorageClasses that are rewritten at admission time to upstream Rook-Ceph equivalents:

| StorageClass | Access Mode | Backing | Use Case |
|---|---|---|---|
| `nest-block` | RWO | Ceph RBD | Primary block storage for databases, stateful services |
| `nest-filesystem` | RWX | CephFS | Shared filesystem for multi-reader/writer workloads |
| `nest-file` | RWX | CephFS | Shared file storage for application data |

**Note:** `nest-block`, `nest-filesystem`, and `nest-file` are Nest-branded provisioners. The injector webhook rewrites DataResource `storageClass` references to Rook-Ceph equivalents at admission time. These StorageClasses also exist as real K8s objects as a failsafe.

### Creating a PVC using Nest Block Storage

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-data
  namespace: my-app
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: nest-block
  resources:
    requests:
      storage: 10Gi
```

### Creating a PVC using Nest Shared Filesystem

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-shared-data
  namespace: my-app
spec:
  accessModes:
    - ReadWriteMany  # RWX requires nest-filesystem
  storageClassName: nest-filesystem
  resources:
    requests:
      storage: 50Gi
```

---

## 5. DataResource Types

### 5.1 Storage Types (Managed by Nest)

#### `pvc/block` — RWO Block Volume (Ceph RBD)

Raw block storage optimized for databases and stateful services. Backed by Ceph RBD.

**Access mode:** `ReadWriteOnce` (RWO)

**Provisioning:** Single node attachment via CSI driver

**Use case:** Primary storage for PostgreSQL, MySQL, Redis, Kafka, ClickHouse

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: my-block-volume
  namespace: my-app
spec:
  type: pvc/block
  tenant: tenant-1
  size:
    storage: 100Gi
    iops: 1000
  ha: true
```

**Status fields:**
```json
{
  "endpoints": {
    "native": "/dev/nest-123-abc"
  },
  "health": {
    "state": "healthy",
    "message": "RBD volume provisioned and healthy"
  },
  "phase": "Ready"
}
```

---

#### `pvc/file` — RWX File Storage (CephFS)

Shared file storage for multi-reader/writer access. Backed by CephFS.

**Access mode:** `ReadWriteMany` (RWX)

**Provisioning:** Shared access across multiple nodes/pods

**Use case:** Shared data lakes, multi-tenant file storage, NAS replacement

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: my-shared-fs
  namespace: data-team
spec:
  type: pvc/file
  tenant: tenant-1
  size:
    storage: 500Gi
```

**Status fields:**
```json
{
  "endpoints": {
    "native": "cephfs://ceph-1.ceph-mon.nest.svc:6789/mysharedfs"
  },
  "health": {
    "state": "healthy"
  },
  "phase": "Ready"
}
```

---

#### `filesystem` — RWX CephFS

Alternative to `pvc/file` — direct CephFS export via Ceph CephFS. Same underlying backing as `pvc/file` but may have different provisioning/networking semantics.

**Access mode:** `ReadWriteMany` (RWX)

**Provisioning:** Shared filesystem mount across cluster

**Use case:** Data lakes, analytics, shared compute storage

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: my-cephfs
  namespace: analytics
spec:
  type: filesystem
  tenant: tenant-1
  size:
    storage: 1Ti
  protocols:
    - native
    - rest  # via NFS gateway
```

---

#### `object` — S3-compatible Object Storage (Ceph RGW)

S3-compatible bucket storage backed by Ceph RGW. Supports versioning, lifecycle policies, encryption.

**Access modes:** n/a (HTTP-based)

**Protocols:** S3 API, REST, native

**Use case:** Data lakes, backups, media asset storage, data archives

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: my-bucket
  namespace: my-app
spec:
  type: object
  tenant: tenant-1
  size:
    storage: 500Gi
  annotations:
    bucket-name: "my-app-assets"
    versioning: "true"
    lifecycle-days: "90"
```

**Status fields:**
```json
{
  "endpoints": {
    "native": "s3://my-bucket.ceph-rgw.nest.svc:7480",
    "rest": "https://my-bucket.nest.ceph-rgw.penguintech.cloud"
  },
  "health": {
    "state": "healthy"
  },
  "phase": "Ready"
}
```

---

#### `nfs` — NFS Export (via nest-nfs-gateway)

NFS v4 export of storage resources via `nest-nfs-gateway` service.

**Access mode:** RWX

**Protocols:** NFS v4.1

**Use case:** Legacy NFS clients, on-premises integration, edge nodes

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: nfs-export
  namespace: legacy-app
spec:
  type: nfs
  tenant: tenant-1
  size:
    storage: 250Gi
```

**Status fields:**
```json
{
  "endpoints": {
    "native": "nfs://nest-nfs-gateway.nest.svc:/mnt/nfs-export"
  }
}
```

---

#### `iscsi` — iSCSI Target (via nest-iscsi-gateway)

iSCSI block storage export via `nest-iscsi-gateway` service.

**Access mode:** RWO (block-level)

**Protocols:** iSCSI (TCP/IP SCSI)

**Use case:** Hypervisor integration (VMware, Hyper-V), on-premises servers, edge compute

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: iscsi-target
  namespace: hypervisor
spec:
  type: iscsi
  tenant: tenant-1
  size:
    storage: 1Ti
```

**Status fields:**
```json
{
  "endpoints": {
    "native": "iscsi://nest-iscsi-gateway.nest.svc:3260/iqn.2026-04.nest/iscsi-target"
  }
}
```

---

### 5.2 Database Types (Managed by Nest via Upstream Operators)

#### `postgres` — PostgreSQL Cluster (CloudNativePG)

Managed PostgreSQL instances via CloudNativePG operator.

**Protocols:** `native` (postgres://), `rest`, `grpc`

**Features:** High availability, streaming replication, point-in-time recovery, automated backups

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: app-db
  namespace: my-app
spec:
  type: postgres
  tenant: tenant-1
  size:
    storage: 100Gi
  ha: true
  replicas:
    write:
      min: 1
      max: 3
      default: 1
    read:
      min: 0
      max: 5
      default: 2
  annotations:
    postgres-version: "16"
    backup-enabled: "true"
    pitr-window-days: "7"
```

**Status fields:**
```json
{
  "endpoints": {
    "native": "postgres://app-db.my-app.svc:5432/postgres",
    "rest": "https://app-db-api.nest.svc:8443/api/v1",
    "grpc": "app-db.my-app.svc:50051"
  },
  "health": {
    "state": "healthy",
    "message": "3 replicas in sync"
  },
  "phase": "Ready"
}
```

---

#### `keyvalue` — Redis/Valkey Cluster

Managed Redis/Valkey instances.

**Protocols:** `native` (redis://), `rest`, `grpc`

**Features:** In-memory key-value store, persistence (RDB/AOF), replication, cluster mode optional

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: session-cache
  namespace: my-app
spec:
  type: keyvalue
  tenant: tenant-1
  annotations:
    engine: "valkey"
    version: "7"
    max-memory: "4Gi"
    persistence: "true"
    persistence-storage: "10Gi"
```

**Status fields:**
```json
{
  "endpoints": {
    "native": "redis://session-cache.my-app.svc:6379"
  },
  "health": {
    "state": "healthy"
  },
  "phase": "Ready"
}
```

---

#### `kafka` — Kafka Cluster (Strimzi)

Managed Apache Kafka clusters via Strimzi operator.

**Protocols:** `native` (Kafka wire), `grpc`, `rest`

**Features:** Brokers, ZooKeeper, topic management, replication, schema registry integration

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: streaming-platform
  namespace: data-pipeline
spec:
  type: kafka
  tenant: tenant-1
  size:
    storage: 500Gi
  replicas:
    write:
      count: 3
  annotations:
    broker-count: "3"
    replication-factor: "3"
    partition-default: "12"
```

**Status fields:**
```json
{
  "endpoints": {
    "native": "kafka://streaming-platform-kafka-bootstrap.data-pipeline.svc:9092",
    "rest": "https://kafka-rest-proxy.nest.svc:8443"
  },
  "health": {
    "state": "healthy",
    "message": "3 brokers healthy, 12 partitions"
  },
  "phase": "Ready"
}
```

---

#### `search` — OpenSearch Cluster

Managed OpenSearch instances for full-text search and analytics.

**Access mode:** Shared via `SearchPool` or dedicated cluster

**Protocols:** `native` (OpenSearch HTTP), `rest`, `grpc`

**Features:** Full-text search, analytics, time-series, log aggregation, index-based tenant isolation

**Dedicated example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: search-cluster
  namespace: search-team
spec:
  type: search
  tenant: tenant-1
  size:
    storage: 200Gi
  replicas:
    write:
      count: 3
    read:
      count: 2
  annotations:
    engine: "opensearch"
    version: "2.11"
    shards: "10"
    replicas: "2"
```

**Shared SearchPool example (cost-efficient):**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: shared-search
  namespace: analytics
spec:
  type: search
  tenant: tenant-1
  annotations:
    search-pool: "analytics-shared-pool"
    index-prefix: "tenant-1-"
```

**Status fields:**
```json
{
  "endpoints": {
    "native": "opensearch://search-cluster.search-team.svc:9200",
    "rest": "https://search-cluster-api.nest.svc:8443"
  },
  "health": {
    "state": "healthy",
    "message": "green status, 100 shards"
  },
  "phase": "Ready"
}
```

---

#### `vector` — Vector Database

Managed vector database for embeddings and semantic search (e.g., Weaviate, Milvus, Pinecone).

**Protocols:** `native`, `rest`, `grpc`

**Use case:** LLM embeddings, semantic search, recommendation systems, AI/ML workloads

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: embeddings-db
  namespace: ai-pipeline
spec:
  type: vector
  tenant: tenant-1
  size:
    storage: 100Gi
  annotations:
    engine: "weaviate"
    version: "1.7"
    vector-dimension: "1536"
```

---

#### `clickhouse` — ClickHouse OLAP

Managed ClickHouse data warehouse for analytics.

**Protocols:** `native` (TCP), `http`, `grpc`

**Use case:** Real-time analytics, time-series data, log analytics, columnar OLAP

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: analytics-warehouse
  namespace: analytics
spec:
  type: clickhouse
  tenant: tenant-1
  size:
    storage: 1Ti
  replicas:
    write:
      count: 3
  annotations:
    version: "24.1"
    shard-count: "3"
    replication-factor: "2"
```

---

#### `warehouse/trino` — Trino Query Engine

Managed Trino/Presto distributed query engine for federated queries across data sources.

**Protocols:** `native`, `rest`, `grpc`

**Use case:** Federated analytics, multi-source queries, data virtualization

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: federated-query
  namespace: analytics
spec:
  type: warehouse/trino
  tenant: tenant-1
  replicas:
    write:
      count: 1
    read:
      count: 4
  annotations:
    version: "426"
```

---

#### `lakehouse/iceberg` — Apache Iceberg Catalog

Managed Apache Iceberg metadata catalog and lakehouse platform.

**Protocols:** `native`, `rest`, `grpc`

**Use case:** Data lakes, table format standardization, schema evolution, ACID semantics

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: data-lakehouse
  namespace: data-lake
spec:
  type: lakehouse/iceberg
  tenant: tenant-1
  size:
    storage: 2Ti
  annotations:
    catalog-type: "nessie"  # or "glue", "jdbc"
```

---

#### `rockfs` — RockFS / FerretDB

Managed document database (MongoDB-compatible via FerretDB) backed by RockFS distributed filesystem.

**Protocols:** `native` (Mongo wire), `rest`, `grpc`

**Use case:** Document storage, MongoDB migration, schema flexibility

**Example:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: document-db
  namespace: app
spec:
  type: rockfs
  tenant: tenant-1
  size:
    storage: 200Gi
  ha: true
```

---

### 5.3 Cloud-Native External Types (Origination: External)

These types provision cloud-managed resources via cloud provider APIs. **Require `origination: external` and cloud provider credentials.**

#### `ebs` — AWS Elastic Block Store

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: aws-volume
  namespace: my-app
spec:
  type: ebs
  tenant: tenant-1
  origination: external
  external:
    provider: aws
    region: us-east-1
    credentialSecret: aws-credentials
    blockVolume:
      sizeGB: 100
      iops: 3000
      throughput: 125
      volumeType: gp3
      availabilityZone: us-east-1a
      encryptionKeyId: arn:aws:kms:...
```

---

#### `azure-disk` — Azure Managed Disk

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: azure-volume
spec:
  type: azure-disk
  tenant: tenant-1
  origination: external
  external:
    provider: azure
    region: eastus
    credentialSecret: azure-credentials
    blockVolume:
      sizeGB: 200
      volumeType: Premium_LRS
```

---

#### `gcp-disk` — GCP Persistent Disk

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: gcp-volume
spec:
  type: gcp-disk
  tenant: tenant-1
  origination: external
  external:
    provider: gcp
    region: us-central1
    credentialSecret: gcp-credentials
    blockVolume:
      sizeGB: 150
```

---

#### `s3` — AWS S3 Bucket

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: s3-bucket
spec:
  type: s3
  tenant: tenant-1
  origination: external
  external:
    provider: aws
    region: us-west-2
    credentialSecret: aws-credentials
    objectBucket:
      bucketName: my-data-lake-2026
      versioning: true
      encryptionType: aws:kms
      lifecycleDays: 90
      publicAccessBlock: true
      crossRegionReplication: true
      replicationTargetRegion: us-east-1
```

---

#### `gcs` — Google Cloud Storage Bucket

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: gcs-bucket
spec:
  type: gcs
  tenant: tenant-1
  origination: external
  external:
    provider: gcp
    credentialSecret: gcp-credentials
    objectBucket:
      bucketName: my-gcs-bucket
      versioning: true
      lifecycleDays: 180
```

---

#### `azure-blob` — Azure Blob Storage

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: azure-blob
spec:
  type: azure-blob
  tenant: tenant-1
  origination: external
  external:
    provider: azure
    credentialSecret: azure-credentials
    objectBucket:
      bucketName: mycontainer
      lifecycleDays: 365
```

---

## 6. Origination Modes

### Origination: `managed` (Default)

Nest provisions and owns the resource. Lifecycle is fully managed by Nest: creation, configuration, scaling, deletion.

```yaml
spec:
  origination: managed  # or omit (default)
  # Controller provisions the resource via upstream operators
  # Nest manages the complete lifecycle
```

**When to use:** New workloads, internal services, standard requirements

---

### Origination: `imported`

Nest adopts a pre-existing external resource (e.g., an on-premises PostgreSQL server or external Redis cluster). Connection details are provided, and Nest manages credentials, monitoring, and failover.

```yaml
spec:
  origination: imported
  import:
    connectionString: "postgres://postgres.external.corp:5432/mydb"
    tlsMode: verify-full
    credentialSecret: imported-db-creds
    managedCredentials: true       # Allow Nest to rotate credentials
    managedFailover: true          # Allow Nest to manage failover
```

**When to use:** Legacy databases, on-premises systems, gradual cloud migration

**Credential handling:** Credentials are extracted from `credentialSecret` and stored securely via SAL (Secrets Abstraction Layer).

---

### Origination: `external`

Nest acts as a control plane interface for cloud-managed resources (EBS, S3, GCP Persistent Disk, etc.). The cloud provider owns the resource lifecycle; Nest provides visibility and management APIs.

```yaml
spec:
  origination: external
  external:
    provider: aws          # aws, gcp, azure, vultr, cloudflare
    region: us-east-1
    resourceId: vol-123456789abcdef01
    credentialSecret: cloud-provider-creds
    costTagKey: "cost-center"
    blockVolume:
      sizeGB: 100
      iops: 3000
      volumeType: gp3
```

**When to use:** Cloud-native workloads, cost tracking via tags, provider-specific features

**Credential handling:** Cloud provider credentials (AWS access key/secret, GCP service account, Azure credentials) stored in Kubernetes Secret referenced by `credentialSecret`.

---

## 7. Data Protection Policies

DataProtectionPolicy CRs configure snapshots, backups, PITR, and restore verification for DataResources.

### DataProtectionPolicy Structure

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataProtectionPolicy
metadata:
  name: production-backup-policy
  namespace: nest
spec:
  # Local snapshots (VolumeSnapshot via Ceph)
  snapshots:
    schedule: "@daily"
    pvcName: my-postgres-pvc
    retention:
      hourly: 24
      daily: 7
      weekly: 4
      monthly: 12
      yearly: 3

  # Remote backups (Velero to S3/RGW)
  backups:
    schedule: "@daily"
    destination:
      kind: object
      resource: backup-bucket
      region: us-east-1
    crossRegionCopy:
      enabled: true
      destination: us-west-2
      lagBudgetSeconds: 3600
    retention:
      daily: 14
      weekly: 8
      monthly: 24
    encryption: kms-key-prod

  # Point-in-time recovery
  pitr:
    enabled: true
    windowDays: 15

  # Restore verification (test restores to scratch namespace)
  verify:
    restoreTest:
      schedule: "@weekly"
      target: scratch-namespace
```

### Snapshots

**VolumeSnapshot** via Ceph integration. Local, fast snapshots for immediate recovery.

- **Schedule:** Cron expressions (`@hourly`, `@daily`, `@weekly`, `@monthly`, or `@every Xh/Xm`)
- **Retention:** Hourly, daily, weekly, monthly, yearly counts
- **Use case:** Immediate rollback, corruption recovery, clone for testing

---

### Backups

**Remote backups** via Velero to S3 or Ceph RGW.

- **Schedule:** Same cron syntax
- **Destination:** Object DataResource (S3, RGW, GCS, Azure Blob)
- **Cross-region replication:** Automatic backup copy to secondary region with configurable lag budget
- **Encryption:** KMS provider for encryption at rest
- **Retention:** Granular retention per frequency

---

### PITR (Point-in-Time Recovery)

Continuous backup of transaction logs (database-specific) with configurable window. Enables recovery to any point within the window.

- **WindowDays:** How far back PITR is available (typically 7–30 days)
- **Requires:** Database support (PostgreSQL, MySQL with binary log enabled)

---

### Verify (Restore Testing)

Periodic restore tests to a scratch namespace to validate backup integrity and recovery procedures.

- **Schedule:** Test frequency (typically weekly)
- **Target:** Scratch namespace where test restore runs
- **Use case:** Catch backup corruption early, validate recovery procedures

---

### Linking a DataProtectionPolicy to a DataResource

Reference the policy in the DataResource:

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: production-postgres
spec:
  type: postgres
  dataProtectionPolicy: production-backup-policy
  # ... rest of spec
```

---

## 8. Eggs (Composition & Bundling)

An **Egg** is a named, versioned package of DataResources and/or processors — the unit of composition in Nest.

### Why Eggs?

- **Bundle related resources:** Group databases, caches, search clusters, and processors into logical units
- **Version and release together:** Single egg version = all contained resources versioned together
- **Deploy as one:** `apply egg my-data-pipeline:v2.1.0` deploys all resources
- **Share across tenants:** Create once, reference from multiple tenants
- **Resource discovery:** List resources by egg: `kubectl get dataresources -l egg=my-data-pipeline`

### Egg Manifest Structure

```yaml
apiVersion: nest.penguintech.io/v1
kind: Egg
metadata:
  name: my-data-pipeline
  namespace: eggs
spec:
  version: "2.1.0"
  description: "Data processing pipeline: postgres + kafka + search"
  
  # DataResources contained in this egg
  dataResources:
    - name: pipeline-db
      type: postgres
      spec:
        size: 100Gi
        replicas:
          write:
            count: 1
          read:
            count: 2
    
    - name: events-stream
      type: kafka
      spec:
        replicas:
          count: 3
        size: 200Gi
    
    - name: search-index
      type: search
      spec:
        size: 50Gi
  
  # Processors (optional)
  processors:
    - name: event-transformer
      type: kafka-processor
      spec:
        image: my-org/event-transformer:v2.1.0
        input: events-stream
        output: search-index
```

### Creating an Egg

```bash
kubectl apply -f my-data-pipeline-egg.yaml -n eggs

# Verify
kubectl get eggs -n eggs
kubectl describe egg my-data-pipeline -n eggs
```

### Deploying an Egg to a Tenant

```bash
# Create tenant-specific instance of the egg
kubectl apply -f - <<EOF
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: tenant-1-pipeline
  namespace: nest
  labels:
    egg: my-data-pipeline
spec:
  type: egg-instance
  tenant: tenant-1
  eggRef: my-data-pipeline:2.1.0
EOF
```

---

## 9. Tenant Isolation

Each DataResource is scoped to a single tenant via the `spec.tenant` field. Isolation is enforced at multiple layers:

### API-Layer Isolation

The `nest-api` validates the JWT `tenant` claim and enforces that all requests operate on resources belonging to that tenant.

```bash
# Token must contain tenant claim
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/tenant-1/data-resources
# $TOKEN.tenant == "tenant-1"
```

### Data-Layer Isolation

For shared resources (SearchPool), index-prefix isolation via OpenSearch security roles ensures cross-tenant data protection.

```yaml
# SearchPool with tenant isolation
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: shared-search
spec:
  type: search
  tenant: tenant-1
  annotations:
    search-pool: "shared-pool"
    index-prefix: "tenant-1-"  # All indexes are prefixed
  # OpenSearch security role restricts access to tenant-1-* indexes
```

### Database User Isolation (Per-Tenant Accounts)

For shared databases, create per-tenant database accounts:

```sql
-- PostgreSQL per-tenant isolation
CREATE USER tenant-1 WITH PASSWORD '...';
CREATE SCHEMA tenant-1;
ALTER DEFAULT PRIVILEGES IN SCHEMA tenant-1 GRANT ALL ON TABLES TO tenant-1;
GRANT USAGE ON SCHEMA tenant-1 TO tenant-1;

-- RLS (Row-Level Security) for row-based isolation if needed
CREATE POLICY tenant_rls ON data_table
  USING (tenant_id = current_setting('app.current_tenant')::int);
```

### Cross-Tenant API Enforcement

```go
// In API middleware
func TenantMiddleware(c *gin.Context) {
    token := c.GetHeader("Authorization")
    claims := ValidateJWT(token)
    tenantID := claims["tenant"]
    
    // All subsequent queries scoped to tenantID
    c.Set("tenant_id", tenantID)
}
```

---

## 10. Drive Preference Policy (Node-Agent)

Nest's **drive preference policy** ensures optimal storage allocation:

1. **System drives (marked `System` state) are NEVER adopted** — OS root, boot, swap partitions remain under OS control
2. **DarkDrives (marked `Dark` state) are ALWAYS preferred** — Unallocated, non-OS drives are the primary storage source
3. **Node-agent discovers available drives** — Periodically scans node block devices and reports state
4. **Scheduler allocates OSD slots** — Rook-Ceph scheduler consumes drive inventory for OSD placement

### Drive States

| State | Meaning | Adoptable? |
|-------|---------|-----------|
| `Dark` | Unallocated, non-OS drive | ✅ Yes (preferred) |
| `System` | OS root/boot/swap partition | ❌ No (never) |
| `Used` | Already allocated to OSD/workload | ❌ No |
| `Reserved` | Manually reserved | ❌ No (unless released) |

### Node-Agent Discovery Workflow

```
1. Node-agent starts → scan /dev for block devices
2. Identify OS root device via /proc/mounts
3. Mark OS root + dependents (boot, swap) as System
4. Mark remaining unused drives as Dark
5. Report to scheduler via API: [ { "device": "sda", "state": "Dark", "capacity": "1T", "node": "worker-1" } ]
6. Scheduler consumes Dark drives for Ceph OSD placement
7. Rook-Ceph provisioning → OSD containers start on allocated drives
8. Drive state transitions: Dark → Used
```

### Preferred Node Pool Configuration

```yaml
# Helm values: prefer dark drives for new storage pools
nest:
  drivePreference:
    preferDark: true        # Always prefer Dark state drives
    reserveSystem: true     # Never touch System drives
    minFreePercent: 10      # Keep 10% of drives unallocated
    cycleReplace: true      # Replace aged drives without touching OS disk
```

### Expanding Cluster Storage (Drive Replacement)

```bash
# 1. Identify dark drives on new nodes
kubectl get nodes -o wide
# Expected: new nodes with available block devices

# 2. Node-agent discovers drives automatically
# Check node-agent logs for drive discovery
kubectl logs -n nest -l app=nest-node-agent --tail=100 | grep -i "dark"

# 3. Scheduler allocates Dark drives to Ceph
# Watch Ceph cluster size increase
kubectl get CephCluster -n rook-ceph

# 4. Replace aged/failed drives
kubectl exec -n nest <node-agent-pod> -- \
  nest-cli drive replace --node=<node> --old-device=/dev/sdb --force
# Does not touch system disk, only rotates the replaceable drive
```

---

## 11. Status & Health

### DataResource Phases

| Phase | Meaning |
|-------|---------|
| `Unknown` | Initial state, no reconciliation yet |
| `Pending` | Waiting for scheduler/prerequisites |
| `Provisioning` | Controller actively provisioning resource |
| `Ready` | Resource fully provisioned and healthy |
| `Degraded` | Resource operational but in degraded state (e.g., replica down) |
| `Failed` | Provisioning failed or resource unhealthy |
| `Deleting` | Resource deletion in progress |

### Health States

| State | Meaning |
|-------|---------|
| `healthy` | All replicas/shards up, no errors |
| `degraded` | Functional but with issues (e.g., 2/3 replicas down) |
| `down` | Not responding, unable to serve requests |

### Reading Status

```bash
# Get status of a DataResource
kubectl get dataresource my-postgres -o jsonpath='{.status.phase}'
# Output: Ready

# Check health
kubectl get dataresource my-postgres -o jsonpath='{.status.health}'
# Output: {"state":"healthy","message":"3 replicas in sync"}

# Full status
kubectl describe dataresource my-postgres
# Shows Phase, Health, Endpoints, Conditions, CurrentOperation, ProvisionedAt
```

### Status Conditions

```json
{
  "status": {
    "phase": "Ready",
    "conditions": [
      {
        "type": "Ready",
        "status": "True",
        "observedGeneration": 1,
        "lastTransitionTime": "2026-04-22T10:15:30Z",
        "reason": "Ready",
        "message": "Resource fully provisioned"
      }
    ],
    "endpoints": {
      "native": "postgres://my-postgres.my-app.svc:5432",
      "grpc": "my-postgres.my-app.svc:50051",
      "rest": "https://my-postgres-api.nest.svc:8443"
    },
    "health": {
      "state": "healthy",
      "message": "Primary + 2 standby replicas healthy"
    },
    "provisionedAt": "2026-04-22T10:15:00Z"
  }
}
```

---

## 12. API Reference

All API calls require a Bearer token with a valid `tenant` claim. For local development, any JWT with `{ "tenant": "tenant-1" }` is accepted.

**Base URL (in-cluster):** `http://nest-api.nest.svc:8080`

### Health & Status

```bash
# Liveness probe
curl http://nest-api.nest.svc:8080/health
# Expected: 200 OK

# Readiness probe
curl http://nest-api.nest.svc:8080/ready
# Expected: 200 OK

# Prometheus metrics
curl http://nest-api.nest.svc:8080/metrics
# Expected: Prometheus format

# Available resource types
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/catalog
# Returns: list of supported DataResource types

# API versions
curl http://nest-api.nest.svc:8080/api/v1/versions
# Returns: supported API versions
```

### DataResource CRUD

All DataResource operations are scoped to a tenant.

```bash
# List DataResources for tenant
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/tenant-1/data-resources
# Returns: paginated list of DataResources

# Get a specific DataResource
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/tenant-1/data-resources/my-postgres

# Create a DataResource (returns 202 + LRO operation)
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-postgres",
    "type": "postgres",
    "tenant": "tenant-1",
    "size": {"storage": "100Gi"},
    "ha": true
  }' \
  http://nest-api.nest.svc:8080/api/v1/tenants/tenant-1/data-resources
# Returns: {"status":"success","data":{"operationId":"op-abc123"}}

# Update a DataResource
curl -X PATCH \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"size": {"storage": "200Gi"}}' \
  http://nest-api.nest.svc:8080/api/v1/tenants/tenant-1/data-resources/my-postgres

# Delete a DataResource
curl -X DELETE \
  -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/tenant-1/data-resources/my-postgres
# Returns: 202 with operation ID
```

### Long-Running Operations (LRO)

Create and delete operations are asynchronous and return HTTP 202 with an operation ID.

```bash
# Poll LRO status
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/tenant-1/operations/op-abc123

# Response example:
{
  "status": "success",
  "data": {
    "operationId": "op-abc123",
    "state": "RUNNING",
    "progress": 65,
    "resourceName": "my-postgres",
    "startedAt": "2026-04-22T10:00:00Z",
    "estimatedCompletionTime": "2026-04-22T10:05:00Z"
  },
  "meta": {
    "version": 1,
    "timestamp": "2026-04-22T10:02:00Z"
  }
}
```

**LRO states:** `PENDING`, `RUNNING`, `SUCCEEDED`, `FAILED`

---

## 13. Environment Variables

These variables configure the `nest-api` and related containers.

| Variable | Default | Required | Description |
|---|---|---|---|
| `PORT` | `8080` | No | HTTP listen port |
| `VERSION` | (injected at build) | No | Application version string |
| `LICENSE_KEY` | — | No | PenguinTech license key for enterprise features |
| `GIN_MODE` | `debug` | Yes (prod) | Set to `release` in production for performance |
| `DATABASE_URL` | — | Yes | PostgreSQL connection string for Nest metadata (e.g., `postgres://user:pass@localhost/nest`) |
| `REDIS_URL` | — | Yes | Redis/Valkey URL for caching and session state (e.g., `redis://localhost:6379`) |
| `NEST_NAMESPACE` | `nest` | No | Kubernetes namespace where Nest components run |
| `KUBECONFIG` | — | No | Path to kubeconfig for out-of-cluster API access; omit for in-cluster API access |
| `ROOK_NAMESPACE` | `rook-ceph` | No | Kubernetes namespace where Rook-Ceph is deployed |
| `LOG_LEVEL` | `info` | No | Log level: `debug`, `info`, `warn`, `error` |
| `ENABLE_METRICS` | `true` | No | Enable Prometheus metrics endpoint |
| `TLS_CERT_FILE` | — | No | Path to TLS certificate file for HTTPS |
| `TLS_KEY_FILE` | — | No | Path to TLS key file for HTTPS |

---

## 14. Building from Source

All builds run inside Docker. Do not rely on host Go toolchain for production builds.

```bash
# Build all services (containerized)
make build

# Run all tests
make test

# Run linters
make lint

# Build only the API image (alpha / local dev)
docker build -t localhost:32000/nest-api:latest -f apps/api/Dockerfile .
docker push localhost:32000/nest-api:latest

# Build with version injection
docker build \
  --build-arg VERSION=$(cat .version) \
  -t localhost:32000/nest-api:latest \
  -f apps/api/Dockerfile .

# Build all images
docker build -t localhost:32000/nest-controller:latest -f services/k8s-controller/Dockerfile .
docker build -t localhost:32000/nest-node-agent:latest -f services/node-agent/Dockerfile .

# Build and push to alpha registry
make docker-push-alpha
```

**Go version:** 1.24.2 minimum. All builds use `golang:1.24-bookworm`.

**Module:** `github.com/penguintechinc/nest`

---

## 15. Observability & Monitoring

### Prometheus Metrics

Nest exposes Prometheus metrics at `/metrics` on port 8080.

```bash
# Scrape metrics
curl http://nest-api.nest.svc:8080/metrics
```

Key metric prefixes:

| Prefix | Description |
|---|---|
| `nest_api_request_duration_seconds` | API request latency histogram |
| `nest_api_requests_total` | Total API requests by method/route/status |
| `nest_dataresource_operations_total` | DataResource create/delete/get counts |
| `nest_lro_duration_seconds` | Long-running operation duration histogram |
| `nest_csi_volume_provision_total` | CSI volume provision counts |
| `nest_csi_volume_attach_duration_seconds` | Volume attach latency histogram |
| `nest_dataprotection_backup_duration_seconds` | Backup duration |
| `nest_dataprotection_restore_duration_seconds` | Restore duration |

### Kubernetes ServiceMonitor

If Prometheus Operator is deployed, create a `ServiceMonitor` to scrape Nest metrics:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: nest-api
  namespace: nest
spec:
  selector:
    matchLabels:
      app: nest-api
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
```

### Logging

All Nest components log to stdout/stderr. Configure log level via `LOG_LEVEL` environment variable.

```bash
# Tail API logs
kubectl logs -n nest -l app=nest-api --tail=100 -f

# Tail controller logs
kubectl logs -n nest -l app=nest-controller --tail=100 -f

# Tail node-agent logs
kubectl logs -n nest -l app=nest-node-agent --tail=100 -f
```

### Health Check Endpoints

| Endpoint | Purpose |
|---|---|
| `/health` | Liveness: is the service running? |
| `/ready` | Readiness: is the service ready to serve traffic? |
| `/metrics` | Prometheus metrics |

---

## 16. Common Operations

### Creating a Production PostgreSQL Cluster

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: prod-postgres
  namespace: nest
spec:
  type: postgres
  tenant: tenant-prod
  size:
    storage: 500Gi
    iops: 5000
  ha: true
  replicas:
    write:
      min: 1
      max: 1
      count: 1
    read:
      min: 2
      max: 5
      count: 2
  dataProtectionPolicy: prod-backup-policy
  annotations:
    postgres-version: "16"
    max-connections: "200"
    shared-buffers: "2GB"
---
apiVersion: nest.penguintech.io/v1
kind: DataProtectionPolicy
metadata:
  name: prod-backup-policy
  namespace: nest
spec:
  snapshots:
    schedule: "@hourly"
    pvcName: prod-postgres-pvc
    retention:
      hourly: 24
      daily: 7
      weekly: 4
  backups:
    schedule: "@daily"
    destination:
      kind: object
      resource: backup-bucket
  pitr:
    enabled: true
    windowDays: 14
```

### Creating a Shared Search Pool for Multiple Tenants

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: shared-search-pool
  namespace: nest
spec:
  type: search
  tenant: platform-admin
  size:
    storage: 500Gi
  replicas:
    write:
      count: 3
    read:
      count: 2
  annotations:
    engine: opensearch
    version: "2.11"
    node-count: "5"
    search-pool: "shared"
```

Then reference in tenant DataResources:

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: tenant-1-search
  namespace: nest
spec:
  type: search
  tenant: tenant-1
  annotations:
    search-pool: shared
    index-prefix: "tenant-1-"
  # No size: uses shared pool capacity
```

### Scaling a DataResource

```bash
# Patch to increase storage
kubectl patch dataresource my-postgres -p '{"spec":{"size":{"storage":"200Gi"}}}' --type=merge

# Patch to increase replicas
kubectl patch dataresource my-postgres -p '{"spec":{"replicas":{"read":{"count":5}}}}' --type=merge

# Watch rollout
kubectl get dataresource my-postgres -w
```

### Importing an External PostgreSQL Server

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: external-postgres-creds
  namespace: nest
type: Opaque
stringData:
  password: "mypassword123"
---
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: legacy-postgres
  namespace: nest
spec:
  type: postgres
  tenant: tenant-1
  origination: imported
  import:
    connectionString: "postgres://postgres@postgres.corp.internal:5432/legacy_db"
    tlsMode: verify-full
    credentialSecret: external-postgres-creds
    managedCredentials: true
    managedFailover: false
```

---

## 17. Troubleshooting

### DataResource Stuck in Provisioning

```bash
# Check controller logs
kubectl logs -n nest -l app=nest-controller --tail=200 | grep -i error

# Check DataResource status
kubectl describe dataresource <name> -n nest

# Check upstream operator resource (e.g., CloudNativePG Cluster)
kubectl get cluster <name> -o yaml

# Check LRO status
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/tenant-1/operations/<opId>
```

### API Health Issues

```bash
# Check API pod logs
kubectl logs -n nest -l app=nest-api --tail=200

# Check liveness/readiness probes
kubectl describe pod -n nest -l app=nest-api

# Check API service
kubectl get svc -n nest nest-api

# Test API connectivity
kubectl exec -n nest <api-pod> -- curl http://localhost:8080/health
```

### Drive Discovery Issues

```bash
# Check node-agent logs
kubectl logs -n nest -l app=nest-node-agent --tail=200 | grep -i drive

# Check node inventory
kubectl get nodes -o wide

# Manually trigger drive scan
kubectl exec -n nest <node-agent-pod> -- nest-cli drive scan --node=<node-name>
```

### Ceph Cluster Issues

```bash
# Check Ceph status
kubectl exec -n rook-ceph <mon-pod> -- ceph status

# Check OSD health
kubectl exec -n rook-ceph <mon-pod> -- ceph osd tree

# Check pool status
kubectl exec -n rook-ceph <mon-pod> -- ceph osd pool ls detail
```

---

## 18. Performance Tuning

### Database Performance

```yaml
spec:
  annotations:
    shared-buffers: "4GB"
    effective-cache-size: "16GB"
    work-mem: "32MB"
    maintenance-work-mem: "1GB"
    random-page-cost: "1.1"  # For SSD (RBD)
    max-connections: "500"
```

### Search Cluster Tuning

```yaml
spec:
  annotations:
    shard-count: "20"
    replica-count: "2"
    refresh-interval: "5s"  # Reduce for real-time updates
    segment-memory-limit: "512mb"
```

### CSI Performance

```yaml
spec:
  size:
    iops: 5000  # Request specific IOPS for RBD volumes
```

---

## 19. Security Best Practices

### TLS Configuration

```yaml
spec:
  tls:
    mode: required
    minVersion: "1.3"
    clientAuth: required
    certSource: cert-manager
    atRestKmsId: skauswatch
```

### Secrets Backend

```yaml
spec:
  secretsBackend:
    kind: vault
    ref: https://vault.corp.internal
    # Nest stores connection credentials in Vault instead of K8s Secrets
```

### RBAC

The controller requires these RBAC permissions:

```yaml
rules:
- apiGroups: ["nest.penguintech.io"]
  resources: ["dataresources", "dataresources/status", "dataresources/finalizers"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["nest.penguintech.io"]
  resources: ["dataprotectionpolicies"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["namespaces", "secrets", "persistentvolumes", "persistentvolumeclaims"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["postgresql.cnpg.io"]
  resources: ["clusters"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

---

## 20. Version & Support

**Nest Version:** Check `.version` file in repository root.

**Supported Kubernetes:** 1.28+

**Supported Rook-Ceph:** 1.12+

**Go Version:** 1.24.2+

For issues, questions, or contributions: open a GitHub issue or pull request in the Nest repository.
