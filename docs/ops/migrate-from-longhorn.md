# Migrating from Longhorn to Nest

## Overview

Longhorn is deprecated in favor of Nest (Ceph-backed storage). This guide walks through discovering Longhorn volumes, generating equivalent Nest DataResource manifests, and migrating workloads to the new storage backend.

**When to migrate:**
- Longhorn cluster is experiencing performance bottlenecks
- Consolidating to Ceph for HA and multi-node support
- Updating to latest Nest storage infrastructure

**What `nestctl` does:**
- Discovers Longhorn PersistentVolumes and PersistentVolumeClaims
- Maps Longhorn volume properties to Nest storage types
- Generates DataResource manifests ready for deployment
- Does NOT copy data automatically (requires Velero or manual transfer)

---

## Prerequisites

- `kubectl` access to both Longhorn and Nest clusters (or same cluster if co-existing)
- `nestctl` installed: `curl https://releases.nest.penguintech.io/nestctl | bash`
- Nest cluster running with Ceph RBD, CephFS, and RGW provisioners
- Backup of Longhorn data (recommended via Velero or snapshots)
- No active workloads on source Longhorn volumes during migration (optional, depends on migration strategy)

---

## Migration Steps

### Step 1: Discover Longhorn Volumes

Scan the cluster for Longhorn-backed PersistentVolumes:

```bash
nestctl migrate longhorn --tenant mycompany --dry-run
```

**Output example:**
```
Discovered 5 Longhorn volumes:
  • data-db (100Gi, RWO) → pvc/block
  • shared-logs (50Gi, RWX) → filesystem
  • cache (20Gi, RWO) → pvc/file
  • archive (500Gi, RWO) → pvc/block
  • uploads (1Ti, RWX) → filesystem
```

The `--dry-run` flag scans without making changes. Inspect output to ensure all volumes are recognized.

### Step 2: Review the Migration Plan

Check that storage type assignments are correct. Common mappings:
- Longhorn RWO → `pvc/block` (Nest RBD)
- Longhorn RWX → `filesystem` (Nest CephFS RWX)

If a volume should use a different type (e.g., object storage instead of block), you can override in the next step.

### Step 3: Generate DataResource Manifests

Create manifest files without applying them:

```bash
nestctl migrate longhorn --tenant mycompany --output ./migration
```

**Output structure:**
```
migration/
├── dataresources/
│   ├── data-db.yaml
│   ├── shared-logs.yaml
│   ├── cache.yaml
│   ├── archive.yaml
│   └── uploads.yaml
├── pvc-mapping.yaml
└── migration-report.txt
```

**Review generated DataResources:**
```bash
cat migration/dataresources/data-db.yaml
# Edit if needed, e.g., change storageClass or reclaimPolicy
```

### Step 4: Apply Nest DataResource Manifests

Deploy DataResources to the Nest cluster:

```bash
kubectl apply -f ./migration/dataresources/
```

**Verify provisioning:**
```bash
kubectl get dataresources -n nest
NAME        STATUS    SIZE    AGE
data-db     Bound     100Gi   2m
shared-logs Bound     50Gi    2m
cache       Bound     20Gi    2m
archive     Bound     500Gi   2m
uploads     Bound     1Ti     2m
```

Wait for all DataResources to reach `Bound` status before proceeding.

### Step 5: Re-Point Workload PVCs

Update workload manifests (Deployments, StatefulSets, etc.) to reference Nest storage instead of Longhorn:

**Before (Longhorn):**
```yaml
spec:
  persistentVolumeClaim:
    claimName: data-db-longhorn  # Old Longhorn PVC
    storageClass: longhorn
```

**After (Nest):**
```yaml
spec:
  persistentVolumeClaim:
    claimName: data-db  # New Nest DataResource
    storageClass: nest-block
```

**Update and deploy:**
```bash
kubectl apply -f ./updated-workloads/
```

**Verify workloads:**
```bash
kubectl rollout status deployment/myapp -n myapp
```

### Step 6: Verify Data Access

Test that workloads can read/write to new Nest volumes:

```bash
# Check pod logs for errors
kubectl logs -f deployment/myapp -n myapp

# Test data integrity (app-specific validation)
kubectl exec -it deployment/myapp -- /bin/bash
  $ cat /data/test-file.txt  # Verify data is readable
```

---

## Rollback

If issues arise, rollback is possible since **Longhorn volumes are NOT deleted automatically**:

1. **Revert workload manifests** to point back to Longhorn PVCs:
   ```bash
   kubectl apply -f ./original-workloads/
   kubectl rollout status deployment/myapp -n myapp
   ```

2. **Verify Longhorn data is intact:**
   ```bash
   kubectl get persistentvolume | grep longhorn
   ```

3. **Clean up Nest DataResources** (optional):
   ```bash
   kubectl delete dataresource data-db -n nest
   ```

**Keep Longhorn volumes in place** until migration is fully validated and stable (at least 7 days).

---

## Data Migration Options

### Option A: Copy via Snapshot (Zero-Downtime)

Use Velero to backup Longhorn volumes and restore to Nest:

```bash
velero backup create longhorn-backup --include-namespaces myapp
velero restore create longhorn-restore --from-backup longhorn-backup \
  --namespace-mappings myapp:myapp
```

### Option B: Direct Block Copy (Requires Downtime)

For single RBD volumes, use `rbd` tools to copy snapshots:

```bash
# On Ceph admin pod
rbd create nest-archive --size 500G --pool rbd
rbd export longhorn-archive@snapshot1 - | rbd import - nest-archive
```

### Option C: Application-Level Sync

For databases, use native replication (PostgreSQL logical replication, MySQL binary logs):

```bash
# PostgreSQL example: replicate from Longhorn → Nest
psql -h longhorn-db -c "CREATE PUBLICATION repl FOR ALL TABLES;"
# Configure standby on Nest-backed replica
```

---

## Troubleshooting

**DataResource stuck in "Pending" state:**
```bash
kubectl describe dataresource data-db -n nest
# Check events for provisioner errors
# Verify storage pool has free space: kubectl top nodes
```

**Workload cannot mount Nest volume:**
```bash
kubectl logs -p <pod-name> -n myapp
# Check for RBAC issues or mount errors
kubectl get events -n myapp --sort-by='.lastTimestamp'
```

**Longhorn volume still in use:**
```bash
# Find which pod is holding the lock
kubectl get persistentvolumeclaim -A | grep longhorn-volume
kubectl describe pvc <name> -n <namespace>
```

---

## Next Steps

- Monitor Nest volume performance for 2 weeks post-migration
- Decommission Longhorn storage pools once stable
- Update documentation and runbooks to reference Nest storage
- Archive migration logs for compliance/audit trails
