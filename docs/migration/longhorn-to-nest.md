# Longhorn to Nest Migration Guide

Migrate your Kubernetes persistent volumes from Longhorn block storage to Nest, a multi-tenant data infrastructure platform backed by Ceph. This guide covers the conceptual rationale, migration paths, and step-by-step procedures for moving workloads to Nest's native storage layer.

## Why Migrate from Longhorn to Nest?

Longhorn provides lightweight block storage for single-tenant Kubernetes clusters. Nest replaces that capability with a more comprehensive, enterprise-grade data platform:

### Nest Advantages

| Feature | Longhorn | Nest |
|---------|----------|------|
| **Storage Backend** | Distributed block replicas (not Ceph) | Ceph RBD, CephFS, RGW (S3), iSCSI |
| **Multi-Tenancy** | Single tenant per cluster | First-class tenant isolation (DataResource CRD) |
| **Data Protection** | Manual snapshots only | Automated snapshots, Velero backups, PITR, cross-region replication (DataProtectionPolicy) |
| **Object Storage** | Not included | S3-compatible RGW built-in |
| **Shared Filesystems** | Not supported | CephFS RWX volumes (RWMany support) |
| **Redundancy** | 2–3 replicas | Erasure coding, higher availability |
| **Hardware Integration** | Limited | Dark drive adoption, NUMA-aware scheduling |

### Key Nest Concepts

**DataResource CRD:** A Kubernetes-native abstraction for data infrastructure (databases, object stores, block/file volumes). Each DataResource includes:
- **Type:** postgres, mysql, keyvalue, pvc/block, pvc/file, object, etc.
- **Tenant:** Mandatory tenant ID for isolation
- **Protocols:** native (wire protocol), gRPC, REST access methods
- **Data Protection:** Optional DataProtectionPolicy reference for automated backups and disaster recovery
- **Storage Class:** References a DataResourceClass that binds to Ceph pools

**StorageClasses:** Nest provides Ceph-backed storage classes (`nest-block`, `nest-filesystem`, `nest-file`) that abstract the underlying Ceph cluster from applications.

**Dark Drives:** After Longhorn is decommissioned, its replica disks become unprovisioned block devices. The Nest node-agent detects these and creates DarkDrive events for hardware pool adoption, returning them to the infrastructure pool.

## Migration Paths

### RWO (ReadWriteOnce) Block Volumes → `pvc/block`

Use Nest's `nest-block` StorageClass, which provisions Ceph RBD images. This is equivalent to Longhorn's RWO semantics: single node at a time, block-level access, snapshots via DataProtectionPolicy.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: orders-data
  namespace: my-app
spec:
  storageClassName: nest-block
  accessModes: ["ReadWriteOnce"]
  resources:
    requests:
      storage: 100Gi
```

### RWX (ReadWriteMany) Shared Volumes → `pvc/filesystem`

Use Nest's `nest-filesystem` StorageClass, which provisions CephFS. Multiple pods can mount simultaneously with read/write access—ideal for shared application state, home directories, or build artifact caches.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: shared-cache
  namespace: my-app
spec:
  storageClassName: nest-filesystem
  accessModes: ["ReadWriteMany"]
  resources:
    requests:
      storage: 500Gi
```

### Adding Data Protection (Snapshots, Backups, PITR)

Once migrated, augment PVCs with a DataProtectionPolicy to enable:
- Automated point-in-time snapshots (hourly, daily, weekly)
- Velero backup integration for disaster recovery
- Point-in-time recovery (PITR) for databases
- Cross-region replication for high availability

**Example:** Automated daily snapshots with 7-day retention and Velero backups:

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataProtectionPolicy
metadata:
  name: orders-protection
  namespace: my-app
spec:
  resourceSelector:
    matchLabels:
      pv: orders-data
  snapshots:
    schedule: "0 2 * * *"  # Daily at 2 AM UTC
    retention: 7           # Keep 7 snapshots
  veleroBackup:
    enabled: true
    schedule: "0 4 * * 0"  # Weekly Sunday at 4 AM UTC
    retention: 4 weeks
```

## Compatibility & Coexistence

### StorageClass Mappings

| Use Case | StorageClass | Access | Reclaim | Equivalent |
|----------|--------------|--------|---------|------------|
| New RWO workloads | `nest-block` | RWO | Retain/Delete | Longhorn RWO |
| Migration target (RWO) | `nest-longhorn-compat` | RWO | Retain | Longhorn RWO (namespaced) |
| New RWX workloads | `nest-filesystem` | RWX | Retain/Delete | CephFS |
| Throwaway volumes (tests) | `nest-file` | RWO | Delete | Ephemeral storage |

### Webhook FailurePolicy

Nest's CSI injector webhook uses `failurePolicy: Ignore` — existing workloads continue running if the webhook is unavailable. No downtime from Nest component failures.

### Longhorn Volumes in Parallel

Longhorn PVCs coexist with Nest PVCs during migration. Scale down the workload, migrate data, update the manifest, and test before removing the old PVC. No forced cutover.

## Step-by-Step Migration

### Prerequisite Checks

Verify your environment is ready for migration:

```bash
# 1. Confirm Nest CSI driver is registered
kubectl get csidriver csi.nest.penguintech.io
# Output: csi.nest.penguintech.io   <running>

# 2. Check Nest CSI node DaemonSet is ready
kubectl -n nest get daemonset nest-csi-node
# Output: nest-csi-node   3   3   3   3   3   ...   (ready on all nodes)

# 3. Verify Rook-Ceph cluster is healthy
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph status
# Output: HEALTH_OK (or HEALTH_WARN with brief warnings only)

# 4. Inventory all Longhorn PVCs
kubectl get pvc --all-namespaces -o jsonpath='{range .items[?(@.spec.storageClassName=="longhorn")]}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.spec.resources.requests.storage}{"\n"}{end}'
```

### Phase 1: Migrate RWO Block Volumes

#### Step 1.1: Pre-Flight Check

Run the migration preflight script to validate your cluster state:

```bash
./scripts/migration/longhorn-preflight.sh local-alpha

# Or limit to a specific namespace
./scripts/migration/longhorn-preflight.sh local-alpha my-app-namespace
```

This script:
- Validates CSIDriver registration
- Confirms `nest-csi-node` DaemonSet ready
- Lists all Longhorn PVCs with size and status
- Checks Rook-Ceph cluster health

#### Step 1.2: Select Longhorn PVCs to Migrate

Identify the PVCs from preflight output. Start with non-critical or test workloads to validate the migration process before moving production data.

```bash
# Example output from preflight:
# Namespace: default
#   PVC: postgres-data (50Gi)
#   PVC: cache-vol (200Gi)
#
# Namespace: my-app
#   PVC: orders-data (100Gi)
```

#### Step 1.3: Prepare the Workload

Scale down the application to release the old PVC:

```bash
# Scale to zero replicas
kubectl -n my-app scale deployment my-app --replicas=0

# Wait for pods to terminate
kubectl -n my-app rollout status deployment/my-app

# Verify the old PVC is no longer mounted
kubectl -n my-app get pvc orders-data
# Output: orders-data   Bound   pv-xxx   100Gi   RWO   longhorn   ...
```

#### Step 1.4: Create Target PVC

Create a new PVC backed by Nest's Ceph storage:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: orders-data-nest
  namespace: my-app
spec:
  storageClassName: nest-longhorn-compat
  accessModes: ["ReadWriteOnce"]
  resources:
    requests:
      storage: 100Gi
EOF

# Wait for the PVC to bind (may take 30–60 seconds)
kubectl -n my-app get pvc orders-data-nest --watch
# Output: orders-data-nest   Pending → Bound
```

#### Step 1.5: Migrate Data

Launch a migration pod that syncs data from old to new PVC using `rsync`:

```bash
./scripts/migration/longhorn-migrate-pvc.sh local-alpha my-app orders-data
```

This script:
1. Creates a migration pod with both PVCs mounted
2. Rsyncs data from `orders-data` (old) → `orders-data-nest` (new)
3. Verifies byte counts match
4. Reports success or detailed error logs

**Alternative: Use `pv-migrate` for live migration (if downtime unacceptable):**

```bash
# Install pv-migrate (if not present)
brew install pv-migrate

# Live migration without scaling down
pv-migrate migrate \
  --context local-alpha \
  --source-namespace my-app \
  --dest-namespace my-app \
  --dest-storage-class nest-longhorn-compat \
  orders-data orders-data-nest
```

#### Step 1.6: Update Application Manifest

Update your Deployment to reference the new PVC:

```yaml
# Before
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    spec:
      volumes:
      - name: orders-data
        persistentVolumeClaim:
          claimName: orders-data

# After
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  template:
    spec:
      volumes:
      - name: orders-data
        persistentVolumeClaim:
          claimName: orders-data-nest
```

Apply the updated manifest:

```bash
kubectl apply -f deployment.yaml -n my-app
```

#### Step 1.7: Validate New Volume

Restart the workload and monitor its health:

```bash
# Scale back up
kubectl -n my-app scale deployment my-app --replicas=3

# Wait for rollout
kubectl -n my-app rollout status deployment/my-app --timeout=5m

# Verify data integrity
kubectl -n my-app logs -l app=my-app --tail=20
# Look for successful startup logs and no data errors
```

#### Step 1.8: Clean Up Old PVC

Once the workload is stable and verified, delete the old Longhorn PVC:

```bash
kubectl -n my-app delete pvc orders-data

# Verify it's gone
kubectl -n my-app get pvc
# Output: orders-data-nest   Bound   ...
```

### Phase 2: Verify All Longhorn PVCs Migrated

Re-run the preflight script to confirm zero remaining Longhorn PVCs:

```bash
./scripts/migration/longhorn-preflight.sh local-alpha
# Output: No Longhorn PVCs found
```

If any Longhorn PVCs remain, repeat Phase 1 for each one before proceeding.

### Phase 3: Decommission Longhorn

Once all PVCs are migrated, remove the Longhorn operator to free cluster resources:

```bash
# Dry-run to see what will be removed
./scripts/migration/longhorn-drain.sh local-alpha true

# Execute the drain (no recovery possible after this)
./scripts/migration/longhorn-drain.sh local-alpha
```

The drain script:
1. Verifies zero remaining Longhorn PVCs (exits with error if any exist)
2. Scales Longhorn manager to zero replicas
3. Deletes Longhorn StorageClasses and CSIDriver
4. Removes the `longhorn-system` namespace
5. Cleans up node labels and annotations

**Warning:** This is destructive. Longhorn data not migrated beforehand is lost.

### Phase 4: Adopt Longhorn Replica Disks as Dark Drives

Longhorn replica disks become unprovisioned block devices after decommissioning. The Nest node-agent detects them and exposes them as `DarkDrive` resources:

```bash
# List discovered dark drives
kubectl get darkdrives.nest.penguintech.io --all-namespaces
# Output:
# NAME               STATUS      DEVICE        FILESYSTEM    SIZE
# dark-sdb-node01    discovered  /dev/sdb      ext4          200Gi
# dark-sdc-node02    discovered  /dev/sdc      ext4          200Gi

# Or view hardware inventory
kubectl get hardwareinventories.nest.penguintech.io --all-namespaces
```

**Adopting Dark Drives:**

Dark drives with foreign filesystems (ext4, xfs, etc.) require explicit `eraseConfirmed: true` to prevent accidental data loss. Use the Nest Operator UI or CLI to:

1. Review each dark drive in the hardware panel
2. Verify no critical data (old Longhorn replicas have been migrated)
3. Click "Adopt" or apply a HardwarePool adoption spec with `eraseConfirmed: true`
4. The node-agent securely erases the drive and adds it to the pool

This returns hardware to the infrastructure inventory for Ceph expansion.

### Phase 5 (Optional): Set Nest as Default StorageClass

If you want new PVCs to use Nest by default:

```bash
# Remove Longhorn default annotation (if still present)
kubectl patch storageclass longhorn \
  -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"false"}}}' 2>/dev/null || true

# Set Nest RBD as new default
kubectl patch storageclass nest-block \
  -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'

# Verify
kubectl get storageclass
# Output: nest-block (default)
```

## Advanced: Add Data Protection Policies

After migrating to Nest, add DataProtectionPolicies to enable automated backups and recovery:

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataProtectionPolicy
metadata:
  name: orders-protection
  namespace: my-app
spec:
  # Select PVCs to protect
  resourceSelector:
    matchLabels:
      app: my-app
  
  # Automated snapshots
  snapshots:
    schedule: "0 2 * * *"    # 2 AM UTC daily
    retention: 7              # Keep last 7 snapshots
    snapshotClass: ceph-snapshot
  
  # Velero backup integration
  veleroBackup:
    enabled: true
    schedule: "0 4 * * 0"     # 4 AM UTC on Sundays
    retention: "4 weeks"
    storageLocation: default
    volumeSnapshotLocation: ceph-snapshots
```

Apply and verify:

```bash
kubectl apply -f dataprotectionpolicy.yaml

# Check policy status
kubectl get dataprotectionpolicy -n my-app
# Output: orders-protection   Active
```

## Troubleshooting

### Migration Pod Stuck in Pending

**Symptoms:** Migration pod remains Pending after 2+ minutes.

**Diagnosis:**
```bash
kubectl describe pod nest-migrate-orders-data -n my-app
# Look for: "no matching CSI node driver" or "insufficient capacity"
```

**Solutions:**
- Verify `nest-csi-node` DaemonSet is running on all nodes: `kubectl get ds nest-csi-node -n nest`
- Check Ceph pool capacity: `kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph df`
- If pool is full, expand cluster or delete unused PVCs

### Rsync Exits with Non-Zero Exit Code

**Symptoms:** Migration pod logs show rsync error, data not synced.

**Diagnosis:**
```bash
kubectl logs nest-migrate-orders-data -n my-app | tail -30
# Common errors:
# - "Permission denied" (old PVC mounted read-only)
# - "Broken pipe" (pod killed mid-sync)
```

**Solutions:**
- Scale down the workload completely before migration
- Check if another pod is accessing the old PVC: `kubectl get pods -n my-app -o wide | grep orders-data`
- If pods are still running, scale to 0 and retry

### Target PVC Stays in Pending

**Symptoms:** New PVC remains Pending indefinitely.

**Diagnosis:**
```bash
kubectl describe pvc orders-data-nest -n my-app
# Look for: "waiting for first consumer" or CSI driver errors
```

**Solutions:**
- For `nest-longhorn-compat` (RWO): Create a temporary pod that mounts the PVC to trigger provisioning
- For `nest-filesystem` (RWX): PVCs should bind immediately; check CSI driver logs: `kubectl logs -n nest deploy/nest-csi-controller --tail=50`
- Check node capacity: `kubectl describe nodes | grep "Allocated resources"`

### Ceph Pool Errors

**Symptoms:** Rook-Ceph cluster is HEALTH_WARN or HEALTH_ERR.

**Diagnosis:**
```bash
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph health detail
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph osd df
```

**Solutions:**
- Add OSD capacity: expand Rook CephCluster spec `storage.nodes` or add raw devices
- Fix OSDs: identify failed OSDs, replace drives, and regenerate
- Contact Nest support if cluster remains unhealthy after basic fixes

## Rollback (If Migration Fails)

If a migration fails and you need to revert to Longhorn:

```bash
# 1. Scale the workload down
kubectl -n my-app scale deployment my-app --replicas=0

# 2. Update the manifest to use the original PVC
kubectl patch deployment my-app -n my-app -p \
  '{"spec":{"template":{"spec":{"volumes":[{"name":"orders-data","persistentVolumeClaim":{"claimName":"orders-data"}}]}}}}'

# 3. Scale the workload back up
kubectl -n my-app scale deployment my-app --replicas=3

# 4. Verify application health
kubectl -n my-app logs -l app=my-app --tail=20

# 5. Delete the failed Nest PVC
kubectl -n my-app delete pvc orders-data-nest
```

Longhorn PVC remains intact until explicitly deleted. Re-run migration once the issue is identified and fixed.

## FAQ

**Q: Do I need to migrate all Longhorn PVCs at once?**
A: No. Migrate one by one, or group by application. Longhorn and Nest coexist during migration.

**Q: Can I use `pv-migrate` instead of the migration script?**
A: Yes. `pv-migrate` supports live migration without downtime if your workload can tolerate brief delays. The migration script is simpler for stateless or batch workloads.

**Q: What happens to Longhorn replica disks after decommissioning?**
A: They become dark drives detected by the Nest node-agent. Adopt them into HardwarePool after confirming data has been migrated.

**Q: Can I reuse Longhorn's StorageClass name for Nest?**
A: Not recommended. Use distinct names (`nest-block`, `nest-filesystem`) to avoid confusion. If required, rename Longhorn's StorageClass before applying Nest.

**Q: Is there a way to test migration on a non-critical workload first?**
A: Yes. Start with test or staging workloads in a non-production namespace to validate the procedure before moving critical applications.
