# Nest Workflows

Complete reference for all major workflows in Nest: provisioning, policies, deployment, and recovery.

## DataResource Provisioning Lifecycle

A `DataResource` progresses through states from creation to ready-to-use:

```
┌─────────────┐
│   Pending   │  User creates DataResource YAML
└──────┬──────┘
       ↓
┌─────────────────┐
│  Provisioning   │  Controller reconciles, reserves capacity, provisions backend
└──────┬──────────┘
       ↓
┌─────────────┐
│    Ready    │  Resource available for workloads
└─────────────┘
       ↓
┌─────────────┐
│  Deallocated│  (Optional) Resource released after Egg deletion
└─────────────┘
```

### Detailed Workflow

1. **Create DataResource YAML**
   ```yaml
   apiVersion: nest.penguintech.io/v1
   kind: DataResource
   metadata:
     name: prod-db-vol-01
     namespace: default
   spec:
     type: block          # Type: block, filesystem, object, etc.
     size: "100Gi"
     provisioner: rook    # Backend provisioner
     snapshotable: true
   ```

2. **Controller Detects Creation** (`k8s-controller`)
   - Watches for new DataResource objects
   - Validates spec against CRD schema
   - Marks status as `Provisioning`

3. **Capacity Reservation**
   - Queries available DarkDrives (unallocated drives)
   - Prefers dark drives over system drives
   - Reserves capacity based on size + type

4. **Backend Provisioning**
   - For block storage: Creates RBD image in Ceph
   - For filesystem: Formats partition, mounts NFS export
   - For object: Creates S3 bucket
   - Stores connection details in Secret

5. **Status Update**
   - Updates DataResource.status.phase = `Ready`
   - Records provisioning time, backend ID, connection endpoint
   - Emits event for audit logging

6. **Workload Binding**
   - Pod claims DataResource via PVC (block) or volume mount (filesystem)
   - CSI driver mounts volume
   - Application accesses resource

### State Transitions

| From | To | Trigger | Controller Action |
|------|----|---------|--------------------|
| Pending | Provisioning | Create event | Validate + reserve capacity |
| Provisioning | Ready | Backend ready | Update status, emit event |
| Ready | Deleting | Delete DataResource | Cleanup backend, deallocate |
| Any | Failed | Error | Log event, mark status |

### Example: Creating a Block Volume

```bash
# Define DataResource
cat <<EOF | kubectl apply -f -
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: my-database
spec:
  type: block
  size: 500Gi
  provisioner: rook
  replication: 3
EOF

# Watch provisioning
kubectl get dataresource my-database -w

# Once Ready, use in Pod
kubectl create pvc my-database-pvc --selector dataresource=my-database

# Pod mounts it
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: db-pod
spec:
  containers:
  - name: postgres
    image: postgres:16
    volumeMounts:
    - name: data
      mountPath: /var/lib/postgresql
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: my-database-pvc
EOF
```

## DataProtectionPolicy Workflow

Policies define automated snapshots, backups, PITR, and cross-region replication:

```
┌──────────────────────┐
│  DataProtectionPolicy │  User defines protection rules
└──────────┬───────────┘
           ↓
  ┌────────────────────────────┐
  │ Snapshot Scheduler         │  Automated snapshot schedule
  │ (e.g., hourly, daily)      │  → Creates point-in-time copy
  └─────────────┬──────────────┘
                ↓
  ┌────────────────────────────┐
  │ Backup Schedule            │  Backup to off-cluster storage
  │ (e.g., daily to S3/GCS)    │  → Velero or custom backup job
  └─────────────┬──────────────┘
                ↓
  ┌────────────────────────────┐
  │ PITR (Point-in-Time Restore)│ Continuous WAL archival
  │ (optional)                 │  → Restore to any second
  └─────────────┬──────────────┘
                ↓
  ┌────────────────────────────┐
  │ Replication Schedule       │  Cross-region/cross-cluster copy
  │ (optional)                 │  → Async replication job
  └────────────────────────────┘
```

### Define a Protection Policy

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataProtectionPolicy
metadata:
  name: prod-db-policy
spec:
  dataResourceSelector:
    matchLabels:
      tier: production
  
  # Snapshots
  snapshots:
    enabled: true
    schedule: "0 * * * *"        # Every hour
    retention: 72h               # Keep 72 hours worth
    maxSnapshots: 100
  
  # Backups
  backups:
    enabled: true
    schedule: "0 2 * * *"        # Daily at 2 AM
    destination: s3://backup-bucket/
    retention: 90d
    encryption: aes256
  
  # PITR
  pitr:
    enabled: true
    retention: 30d               # 30 days of WAL history
  
  # Replication
  replication:
    enabled: true
    destinations:
    - cluster: secondary-cluster
      region: us-west-2
      asyncInterval: 300s
```

### Workflow Steps

1. **User creates DataProtectionPolicy**
2. **Controller reconciles policy**
   - Selects matching DataResources (via matchLabels)
   - Validates schedule syntax
   - Creates snapshot/backup jobs

3. **Snapshot Scheduler (hourly)**
   - Creates snapshot of selected DataResources
   - Tags snapshot with timestamp + policy name
   - Cleans up old snapshots per retention

4. **Backup Job (daily)**
   - Snapshots data → uploads to S3/GCS
   - Stores backup manifest (size, checksum, encryption keys)
   - Verifies integrity via checksums

5. **PITR Archiver (continuous)**
   - Streams transaction logs (WAL) to S3
   - Indexes by timestamp for fast restore lookup

6. **Replication Worker (async)**
   - Syncs snapshots/backups to secondary cluster
   - Updates replication status + lag metrics

### Example: Restore from Snapshot

```bash
# List available snapshots
kubectl get snapshots -l dataresource=my-database

# Create a DataResource from snapshot
kubectl apply -f - <<EOF
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: restored-db
spec:
  type: block
  restoreFrom:
    snapshot: my-database-snap-2025-05-01-10-00
    timestamp: 2025-05-01T10:00:00Z
EOF

# Or restore to point-in-time (if PITR enabled)
kubectl apply -f - <<EOF
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: restored-db-pit
spec:
  type: block
  restoreFrom:
    dataResource: my-database
    timestamp: 2025-05-01T09:30:00Z  # Exact second
EOF
```

## DarkDrive Adoption Workflow

Nest discovers and allocates unallocated (dark) storage devices:

```
┌─────────────────────────┐
│  Node Startup           │
│  (node-agent DaemonSet) │
└────────────┬────────────┘
             ↓
┌─────────────────────────────┐
│  Hardware Inventory Scan    │  Scan /dev, check partition table
│  (node-agent inventory.go)  │  Skip system drives (/dev/sda*, /dev/nvme0n1p*)
└────────────┬────────────────┘
             ↓
┌─────────────────────────────┐
│  Create HardwareInventory CR│  One CR per node
│  (node-local authority)     │  Contains: device list, NUMA topology, state
└────────────┬────────────────┘
             ↓
┌─────────────────────────────┐
│  Scheduler Placement        │  Controllers read HardwareInventory
│  (k8s-controller)           │  Matches DataResources to best node
└────────────┬────────────────┘
             ↓
┌─────────────────────────────┐
│  Allocate Dark Drive        │  Mark device as allocated
│  (controller updates status)│  Lock prevents double allocation
└─────────────────────────────┘
```

### Detailed Workflow

1. **Node Agent Starts**
   - Runs as DaemonSet on every Nest-enabled node
   - Listens on `:9090` (/health endpoint)

2. **Hardware Inventory Scan** (`inventory.go`)
   ```go
   // Scan block devices
   lsblk --json
   
   // Skip system drives (mounted /, /boot, swap)
   df -P | grep "^/dev"
   
   // Detect NUMA topology
   numactl --hardware
   
   // Build HardwareInventory CR
   ```

3. **Discover Dark Drives** (unallocated devices)
   - Device has no filesystem
   - Device not mounted
   - Device not in /etc/fstab
   - Device not a swap partition

4. **Create HardwareInventory CR**
   ```yaml
   apiVersion: nest.penguintech.io/v1
   kind: HardwareInventory
   metadata:
     name: worker-node-01
     namespace: nest
   spec:
     nodeName: worker-node-01
     devices:
     - name: sdb
       type: disk
       size: 2TB
       state: Dark
       numaNode: 0
       health: Healthy
     - name: sdc
       type: disk
       size: 2TB
       state: Dark
       numaNode: 1
       health: Healthy
     - name: sda
       type: disk
       size: 512GB
       state: System          # Skip this
       numaNode: 0
       health: Healthy
   ```

5. **Scheduler Placement** (`k8s-controller`)
   - Reads pending DataResources
   - Queries HardwareInventory for candidates
   - Matches on: size, NUMA affinity, device type
   - Selects best-fit node
   - Allocates DarkDrive

6. **Mark Allocated**
   - Updates DataResource.status.allocatedNode
   - Updates DataResource.status.allocatedDevice
   - Updates HardwareInventory device.state = `Allocated`

### Example: Monitor DarkDrive Usage

```bash
# View all hardware inventory
kubectl get hardwareinventory -A

# View devices on specific node
kubectl get hardwareinventory worker-node-01 -o yaml

# See allocated vs available
kubectl get hardwareinventory -o jsonpath='{.items[*].spec.devices[?(@.state=="Dark")]}'

# Watch allocation in real-time
kubectl get dataresource -w --output wide
```

## Egg Deployment Workflow

An Egg is a versioned bundle of DataResources and processors. Deploy as one atomic unit:

```
┌──────────────────┐
│  Define Egg YAML │  Bundle multiple DataResources
└────────┬─────────┘
         ↓
┌──────────────────────────────────┐
│  kubectl apply egg.yaml          │  Submit to API
└────────┬─────────────────────────┘
         ↓
┌──────────────────────────────────┐
│  Controller Validates            │  Check syntax, schema, resource availability
└────────┬─────────────────────────┘
         ↓
┌──────────────────────────────────┐
│  Atomic Provisioning             │  All DataResources or none
│  (transaction-like semantics)    │
└────────┬─────────────────────────┘
         ↓
┌──────────────────────────────────┐
│  All DataResources Ready         │  Egg.status.phase = Ready
└──────────────────────────────────┘
```

### Define an Egg

```yaml
apiVersion: nest.penguintech.io/v1
kind: Egg
metadata:
  name: production-webapp
  namespace: default
spec:
  version: "1.2.0"
  description: "Production web application storage bundle"
  
  # DataResources included in this egg
  dataResources:
  - name: webapp-db
    spec:
      type: block
      size: 500Gi
      provisioner: rook
      replication: 3
      snapshotable: true
  
  - name: webapp-cache
    spec:
      type: block
      size: 100Gi
      provisioner: rook
      replication: 2
  
  - name: webapp-logs
    spec:
      type: filesystem
      size: 50Gi
      exportProtocol: nfs
  
  - name: webapp-objects
    spec:
      type: object
      size: 1Ti
      provisioner: s3
      bucket: webapp-artifacts
  
  # Processors / data pipelines
  processors:
  - name: log-aggregator
    image: filebeat:latest
    config:
      inputs:
      - type: log
        paths: ["/logs/*"]
```

### Deploy Workflow

1. **User creates Egg YAML** and applies:
   ```bash
   kubectl apply -f production-webapp.yaml
   ```

2. **API validates**
   - Syntax check
   - Schema validation
   - Resource count limits (soft limits per tenant)

3. **Controller reconciles Egg**
   - Marks status = `Provisioning`
   - Creates all DataResources atomically
   - If any fails: rolls back all (transaction semantics)

4. **All DataResources provision** (in parallel)
   - Scheduler places each on best-fit node
   - Capacity reservation + provisioning
   - Status updates flow through

5. **Egg Ready**
   - Marks status.phase = `Ready`
   - Records provisioning time
   - Emits audit event

### Example: Share Egg Across Tenants

```bash
# Define shared egg (in tenant-a namespace)
kubectl apply -f shared-database.yaml -n tenant-a

# Expose to tenant-b via ClusterRole
kubectl create clusterrolebinding egg-share-to-tenant-b \
  --clusterrole=egg-viewer \
  --serviceaccount=tenant-b:default

# Tenant-b can now reference it
cat <<EOF | kubectl apply -f - -n tenant-b
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: shared-db-reference
spec:
  type: block
  linkedResource: tenant-a/shared-database
  readOnly: true
EOF
```

## Tenant Onboarding Workflow

Add a new tenant to Nest:

```
┌──────────────────┐
│  Create Namespace│
└────────┬─────────┘
         ↓
┌──────────────────────────────────┐
│  Create TenantCR                 │
│  (assigns quotas, policies)      │
└────────┬─────────────────────────┘
         ↓
┌──────────────────────────────────┐
│  Create RBAC Bindings            │
│  (service accounts, roles)       │
└────────┬─────────────────────────┘
         ↓
┌──────────────────────────────────┐
│  Configure Storage Quotas        │
│  (ResourceQuota + tenant limits) │
└────────┬─────────────────────────┘
         ↓
┌──────────────────────────────────┐
│  Apply Default Policies          │
│  (retention, replication)        │
└──────────────────────────────────┘
```

### Steps

1. **Create namespace**
   ```bash
   kubectl create namespace acme-corp
   ```

2. **Create Tenant CR**
   ```yaml
   apiVersion: nest.penguintech.io/v1
   kind: Tenant
   metadata:
     name: acme-corp
     namespace: acme-corp
   spec:
     displayName: "ACME Corporation"
     storageQuotaBytes: 100Ti       # Hard limit
     maxDataResources: 1000
     maxSnapshots: 10000
     defaultReplication: 3
     defaultEncryption: aes256
     billingContactEmail: billing@acme.corp
   ```

3. **Create service account + RBAC**
   ```bash
   kubectl create serviceaccount app-user -n acme-corp
   
   kubectl create rolebinding app-reader \
     --clusterrole=dataresource-reader \
     --serviceaccount=acme-corp:app-user \
     -n acme-corp
   ```

4. **Set storage quota**
   ```yaml
   apiVersion: v1
   kind: ResourceQuota
   metadata:
     name: storage-quota
     namespace: acme-corp
   spec:
     hard:
       requests.storage: 100Ti
       nest.penguintech.io/dataresources: "1000"
   ```

5. **Apply default policies**
   ```yaml
   apiVersion: nest.penguintech.io/v1
   kind: DataProtectionPolicy
   metadata:
     name: tenant-default
     namespace: acme-corp
   spec:
     dataResourceSelector:
       matchLabels: {}   # All DataResources
     snapshots:
       enabled: true
       schedule: "0 * * * *"
       retention: 7d
   ```

## Longhorn Migration Workflow

Migrate existing Longhorn volumes to Nest:

See comprehensive guide: `/docs/migration/longhorn-to-nest.md`

```
┌────────────────────────────────┐
│  Export Longhorn Snapshots     │  Export to S3 or backup store
└────────┬───────────────────────┘
         ↓
┌────────────────────────────────┐
│  Create Nest DataResources     │  With backup source reference
└────────┬───────────────────────┘
         ↓
┌────────────────────────────────┐
│  Restore from Backup           │  Import historical data
└────────┬───────────────────────┘
         ↓
┌────────────────────────────────┐
│  Workload Cutover              │  Switch pods to Nest volumes
└────────┬───────────────────────┘
         ↓
┌────────────────────────────────┐
│  Decommission Longhorn         │  Delete old PVCs
└────────────────────────────────┘
```

### Quick Start

```bash
# 1. Export Longhorn volume
kubectl get volume longhorn-vol-01 -o yaml > /tmp/export.yaml

# 2. Create migration job
kubectl apply -f - <<EOF
apiVersion: nest.penguintech.io/v1
kind: MigrationJob
metadata:
  name: lh-to-nest-001
spec:
  sourceType: longhorn
  sourcePVC: longhorn-vol-01
  targetNamespace: default
  targetDataResourceName: migrated-vol-01
  verification: checksummed
EOF

# 3. Monitor progress
kubectl get migrationjob lh-to-nest-001 -w

# 4. Verify in Nest
kubectl get dataresource migrated-vol-01 -o yaml
```

## Restore Workflow (Snapshot vs Backup)

Choose restore path based on RPO/RTO needs:

### Restore from Snapshot (Fast)

**Use when:** RTO < 1 hour, local recovery
**RPO:** Minutes (frequency of snapshots)
**RTO:** Seconds to minutes

```bash
# 1. List snapshots
kubectl get snapshots -l dataresource=my-database

# 2. Create DataResource from snapshot
kubectl apply -f - <<EOF
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: restored-from-snap
spec:
  type: block
  restoreFrom:
    snapshot: my-database-snap-2025-05-01-14-30
EOF

# 3. Verify Ready
kubectl get dataresource restored-from-snap -w
```

### Restore from Backup (Remote)

**Use when:** Original data lost, cross-region recovery
**RPO:** Hours (backup frequency)
**RTO:** Hours (download + import time)

```bash
# 1. List available backups
kubectl get backups --sort-by=.metadata.creationTimestamp

# 2. Create DataResource from backup
kubectl apply -f - <<EOF
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: restored-from-backup
spec:
  type: block
  size: 100Gi
  restoreFrom:
    backup: s3://backup-bucket/my-database/2025-04-30T02-00-00Z.backup
    verifyChecksum: true
EOF

# 3. Monitor restoration (may take time)
kubectl get dataresource restored-from-backup -w
```

### Point-in-Time Restore (PITR)

**Use when:** Granular recovery needed (e.g., undo accidental delete)
**RPO:** Seconds (continuous WAL archival)
**RTO:** Hours (WAL replay time)

```bash
# 1. Determine target timestamp
# (exact second you want to restore to)
TARGET_TIME="2025-05-01T12:34:56Z"

# 2. Create DataResource at specific point-in-time
kubectl apply -f - <<EOF
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: restored-pit
spec:
  type: block
  restoreFrom:
    dataResource: my-database
    timestamp: $TARGET_TIME
EOF

# 3. Verify ready
kubectl get dataresource restored-pit -w
```

## OpenSearch Shared Pool Provisioning Workflow

Deploy OpenSearch once, share across multiple DataResources:

```
┌──────────────────────────┐
│  Define SearchPool CR    │  Single OpenSearch cluster
│  (node count, replicas)  │
└────────┬─────────────────┘
         ↓
┌──────────────────────────────────┐
│  Controller Deploys SearchPool   │  StatefulSet + service
└────────┬─────────────────────────┘
         ↓
┌──────────────────────────────────┐
│  Create Search DataResources     │  mode: shared
│  (logical indices in pool)       │
└────────┬─────────────────────────┘
         ↓
┌──────────────────────────────────┐
│  Multiple Apps Query Pool        │  Isolated indices per app
└──────────────────────────────────┘
```

### Deploy Shared OpenSearch Pool

1. **Define SearchPool** (once per environment)
   ```yaml
   apiVersion: nest.penguintech.io/v1
   kind: SearchPool
   metadata:
     name: prod-opensearch
     namespace: nest
   spec:
     elasticsearch:
       nodeCount: 3
       replicas: 2
       resources:
         requests:
           memory: 8Gi
           cpu: 2
         limits:
           memory: 16Gi
           cpu: 4
     storage:
       dataSize: 500Gi
   ```

2. **Create search DataResource with mode: shared**
   ```yaml
   apiVersion: nest.penguintech.io/v1
   kind: DataResource
   metadata:
     name: app-search-index
   spec:
     type: search
     mode: shared              # Share pooled OpenSearch
     poolRef: prod-opensearch  # Link to SearchPool
     indexConfig:
       name: myapp-events
       shards: 3
       replicas: 1
   ```

3. **Provision isolated indices** (one per app)
   ```yaml
   ---
   apiVersion: nest.penguintech.io/v1
   kind: DataResource
   metadata:
     name: web-logs
   spec:
     type: search
     mode: shared
     poolRef: prod-opensearch
     indexConfig:
       name: web-app-logs
       shards: 2
       replicas: 1
   ---
   apiVersion: nest.penguintech.io/v1
   kind: DataResource
   metadata:
     name: audit-logs
   spec:
     type: search
     mode: shared
     poolRef: prod-opensearch
     indexConfig:
       name: audit-logs
       shards: 1
       replicas: 1
   ```

4. **Query pool from applications**
   ```go
   // Each app accesses its own index in shared pool
   client := opensearch.NewClient(
       opensearch.Config{
           Addresses: []string{"opensearch.nest.svc:9200"},
           Index: "web-app-logs",  // App-specific index
       },
   )
   ```

### Monitor Pool Usage

```bash
# View pool status
kubectl get searchpool prod-opensearch -o yaml

# Check indices in pool
curl http://opensearch.nest.svc:9200/_cat/indices

# View resource usage
kubectl top pod -n nest -l pool=prod-opensearch

# Scale pool if needed
kubectl patch searchpool prod-opensearch -p '{"spec":{"elasticsearch":{"nodeCount":5}}}'
```

## Summary Table

| Workflow | Duration | Automation | Trigger |
|----------|----------|-----------|---------|
| DataResource Provisioning | Minutes | Full | Create DataResource |
| Snapshot | Seconds | Scheduled hourly | DataProtectionPolicy |
| Backup | Minutes | Scheduled daily | DataProtectionPolicy |
| PITR | Continuous | Automatic | WAL archival job |
| Replication | Async (300s intervals) | Scheduled | DataProtectionPolicy |
| DarkDrive Discovery | Seconds | Automatic per node | Node startup |
| Egg Deployment | Minutes | Atomic transaction | kubectl apply |
| Tenant Onboarding | Manual | Interactive | Admin command |
| Longhorn Migration | Hours | Guided | MigrationJob CR |
| OpenSearch Pool Deploy | Minutes | Full | SearchPool CR |
| Snapshot Restore | Seconds | On-demand | Manual kubectl |
| Backup Restore | Hours | On-demand | Manual kubectl |
| PITR Restore | Hours | On-demand | Manual kubectl |

## Troubleshooting Workflows

**DataResource stuck in Provisioning:** Check controller logs, verify Rook-Ceph availability
**Snapshots not running:** Check scheduler job status, verify DataProtectionPolicy
**DarkDrive not detected:** Check node-agent health on target node, verify device not mounted
**Egg atomic failure:** Check resource limits, quota available, error events in Egg status
**Restore failing:** Verify backup exists, source snapshot healthy, target capacity available

See `/docs/ops/` for detailed troubleshooting guides.
