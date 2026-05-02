# Migrating from Longhorn to Nest

## Overview

Longhorn is being phased out in favor of Nest's Ceph-backed storage infrastructure. This runbook provides a complete operational procedure for discovering Longhorn volumes, generating Nest DataResource manifests, validating data migration, and decommissioning Longhorn with minimal disruption.

**When to migrate:**
- Longhorn cluster experiencing performance issues or resource constraints
- Consolidating to Ceph for HA, multi-node replication, and enterprise features
- Modernizing storage backend to Nest CSI driver (block/file/object unified model)

**Scope:** This runbook covers ReadWriteOnce (RWO) block volumes (pvc/block). ReadWriteMany (RWX) volumes migrate to Nest filesystem type in a separate phase. Object storage migrations handled separately per bucket.

**Key principle:** Longhorn volumes remain untouched until explicitly deleted. Rollback is safe at any point until cleanup.

---

## Prerequisites

### Infrastructure
- **Kubernetes cluster:** 1.26+ with both Longhorn and Nest installed
- **Nest control plane:** Deployed and healthy (CRDs, CSI driver, controller)
- **Rook-Ceph:** Running with CephBlockPool and CephFilesystem ready
- **Storage pools:** Verify Ceph cluster health before starting

### Tools
- `kubectl` (1.26+) configured for cluster access
- `nestctl` CLI: `curl https://releases.nest.penguintech.io/nestctl | bash && sudo install -m 0755 nestctl /usr/local/bin/`
- `jq` for JSON parsing
- Optional: `velero` CLI for data migration via snapshots

### Validation Checklist
```bash
# 1. Verify Nest CSI driver is installed
kubectl get csidriver csi.nest.penguintech.io -o yaml

# 2. Check Ceph cluster health
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph status
# Must show: "health HEALTH_OK"

# 3. Verify CephBlockPool exists and is healthy
kubectl -n rook-ceph get cephblockpools -o yaml | grep -A5 "name:"

# 4. Verify CephFilesystem exists (for RWX volumes)
kubectl -n rook-ceph get cephfilesystems -o yaml | grep -A5 "name:"

# 5. Inventory all Longhorn PVCs
kubectl get pvc -A | grep longhorn
# Record namespace, name, size, access mode for each
```

---

## Phase 1: Discovery & Planning

### Step 1.1: Scan for Longhorn Volumes

Run the discovery command to inventory all Longhorn-backed PVCs:

```bash
# Dry-run discovery (no changes)
nestctl migrate longhorn discover \
  --tenant mycompany \
  --kubeconfig ~/.kube/config \
  --context <cluster-context>
```

**Expected output:**
```
Discovering Longhorn volumes in cluster dal2-beta...
Total Longhorn PVCs found: 5

Volume Inventory:
┌─────────────────┬────────┬────────────┬─────────┐
│ NAME            │ STATUS │ SIZE       │ MODES   │
├─────────────────┼────────┼────────────┼─────────┤
│ data-db         │ Bound  │ 100Gi      │ RWO     │
│ shared-logs     │ Bound  │ 50Gi       │ RWX     │
│ cache-data      │ Bound  │ 20Gi       │ RWO     │
│ archive-vol     │ Bound  │ 500Gi      │ RWO     │
│ uploads         │ Bound  │ 1Ti        │ RWX     │
└─────────────────┴────────┴────────────┴─────────┘

Type Recommendations:
data-db         (RWO) → pvc/block       [Ceph RBD, 1 replica]
shared-logs     (RWX) → filesystem      [Ceph CephFS, shared-capable]
cache-data      (RWO) → pvc/block       [Ceph RBD, 1 replica]
archive-vol     (RWO) → pvc/block       [Ceph RBD, 1 replica]
uploads         (RWX) → filesystem      [Ceph CephFS, shared-capable]

Migration Status:
✓ Nest CSI driver ready
✓ Rook-Ceph cluster healthy (HEALTH_OK)
✓ 5 volumes ready for migration
```

**What to verify:**
- All PVCs listed with correct size and access mode
- Type recommendations appropriate (RWO → block, RWX → filesystem)
- Nest CSI driver status: READY
- Ceph cluster health: HEALTH_OK

**If discovery fails:**
```bash
# Check for Longhorn in non-default namespace
kubectl get ns | grep longhorn

# Manual PVC inventory if nestctl unavailable
kubectl get pvc -A -o json | jq '.items[] | 
  select(.spec.storageClassName=="longhorn") | 
  {name:.metadata.name, ns:.metadata.namespace, 
   size:.spec.resources.requests.storage, 
   mode:.spec.accessModes[]}'
```

### Step 1.2: Document Current State

Create a migration plan document:

```bash
# Save inventory for reference
kubectl get pvc -A | grep longhorn > /tmp/longhorn-inventory.txt

# For each critical volume, verify current data integrity
# Example: database backup pre-migration
kubectl exec -n myapp deployment/postgres -- \
  pg_dump mydb > /tmp/mydb-premigration.sql

# Archive current application config
kubectl get -A all -o yaml > /tmp/cluster-state-premigration.yaml
```

---

## Phase 2: Manifest Generation

### Step 2.1: Generate DataResource Manifests

Create Nest DataResource manifests from the discovery output:

```bash
# Generate manifests (no application)
nestctl migrate longhorn generate \
  --tenant mycompany \
  --kubeconfig ~/.kube/config \
  --context <cluster-context> \
  --output ./migration-manifests

# Expected structure:
# migration-manifests/
# ├── dataresources/
# │   ├── data-db.yaml
# │   ├── shared-logs.yaml
# │   ├── cache-data.yaml
# │   ├── archive-vol.yaml
# │   └── uploads.yaml
# ├── pvc-mapping.yaml
# └── migration-report.txt
```

### Step 2.2: Review Generated DataResources

Examine each generated manifest before applying:

```bash
# View data-db.yaml (block volume)
cat migration-manifests/dataresources/data-db.yaml
```

**Expected content:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: data-db
  namespace: myapp
spec:
  type: pvc/block
  tenant: mycompany
  class: block-standard
  size:
    storage: 100Gi
  dataProtectionPolicy: standard-backup
  reclaimPolicy: Retain
status:
  phase: Pending
```

**Customization options:**

- **Change StorageClass:** Edit `class` field to match performance requirements:
  - `block-standard` (default): Ceph RBD, 1 replica
  - `block-ha`: 3 replicas (higher cost/reliability)
  - `block-nvme`: NVMe-backed (performance-critical data)

- **Adjust reclaimPolicy:**
  - `Retain` (default): PVC persists after deletion (safe for migration)
  - `Delete`: Auto-delete data (use only after validation)

- **Enable backup:** Add DataProtectionPolicy reference:
  ```yaml
  dataProtectionPolicy: standard-backup  # Hourly snapshots + daily backups
  ```

- **Add filesystem permissions** (if needed):
  ```yaml
  annotations:
    nest.penguintech.io/permissions-mode: "0755"
  ```

### Step 2.3: Validate Manifests

```bash
# Dry-run apply to validate syntax
kubectl apply -f ./migration-manifests/dataresources/ --dry-run=client

# Check for any API validation errors (should see "created (dry run)")
# If errors appear, edit manifests before proceeding
```

---

## Phase 3: Apply DataResources

### Step 3.1: Create Nest DataResources

Apply the validated manifests:

```bash
# Apply all DataResource manifests
kubectl apply -f ./migration-manifests/dataresources/

# Verify creation
kubectl get dataresources -n myapp -o wide
```

**Expected output:**
```
NAME          TYPE        PHASE     CAPACITY   AGE
data-db       pvc/block   Pending   100Gi      0s
shared-logs   filesystem  Pending   50Gi       0s
cache-data    pvc/block   Pending   100Gi      0s
archive-vol   pvc/block   Pending   500Gi      0s
uploads       filesystem  Pending   1Ti        0s
```

### Step 3.2: Wait for DataResources to Bind

Monitor the provisioning process:

```bash
# Watch until all reach "Bound" phase
kubectl get dataresources -n myapp --watch

# Or check status of individual resources
kubectl describe dataresource data-db -n myapp
```

**Expected progression:**
1. `Pending` — CSI driver received request, creating backing storage
2. `Available` — Underlying storage ready, PVC created
3. `Bound` — PVC bound to pod/workload (when pod references it)
4. `Ready` — DataResource fully operational

**Binding typically takes 10-30 seconds.** If a DataResource stays in `Pending` for >2 minutes:

```bash
# Check resource events
kubectl describe dataresource data-db -n myapp

# Check for CSI driver errors
kubectl logs -n nest deployment/nest-csi-controller | tail -50

# Verify Ceph pools have capacity
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph df

# Check node labels (CSI requires node selection)
kubectl get nodes --show-labels | grep ceph
```

---

## Phase 4: Data Migration

### Step 4.1: Choose Migration Strategy

Select based on downtime tolerance and data volume:

**Strategy A: Velero Snapshot (Recommended for <1TB)**
- Zero-downtime
- Automated consistency checks
- Works with stateful apps (databases, etc.)
- Cost: slightly higher (Velero overhead)

**Strategy B: rsync Block Copy (For RWO only)**
- Direct block-level copy
- Requires brief downtime
- Suitable for file-based volumes
- Faster than Velero for large single volumes

**Strategy C: Application-Level Replication (For databases)**
- Live replication (no downtime)
- Database-specific (PostgreSQL logical replication, MySQL binary logs)
- Highest complexity

**Strategy D: Warm Migration (For low-frequency data)**
- Copy during off-peak hours
- Verify copy, then flag volume as read-only
- Switch workload references
- Works for archives/backups

### Step 4.2: Velero Snapshot Migration (Recommended)

```bash
# 1. Ensure Velero is installed with BackupStorageLocation
kubectl get backupstoragelocations -n velero

# 2. Create backup of Longhorn volumes
velero backup create longhorn-migration-backup \
  --include-namespaces myapp \
  --wait

# Monitor backup progress
velero backup get longhorn-migration-backup
velero backup logs longhorn-migration-backup

# Expected: "Backup completed with status: Completed"
```

```bash
# 3. Restore to Nest-backed PVCs
# Create restore mapping Longhorn PVC → Nest DataResource
cat > /tmp/restore-mapping.yaml <<EOF
# Velero restore specification
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: longhorn-to-nest-restore
  namespace: velero
spec:
  backupName: longhorn-migration-backup
  namespaceMapping:
    myapp: myapp
  pvcMapping:
    - source:
        namespace: myapp
        name: data-db
      target:
        namespace: myapp
        name: data-db-nest  # Maps to Nest DataResource
EOF

kubectl apply -f /tmp/restore-mapping.yaml

# Monitor restore
velero restore get longhorn-to-nest-restore --details
```

```bash
# 4. Verify restored data
# Check PVC binding
kubectl get pvc -n myapp | grep data-db-nest

# Mount in a test pod and verify contents
kubectl run test-pod --image=debian --rm -it -- \
  bash -c "apt-get update && apt-get install -y postgresql-client && \
  psql -h <db-host> -U postgres -c 'SELECT COUNT(*) FROM <table>;'"
```

### Step 4.3: rsync Block Copy (Alternative)

For individual RWO volumes without Velero:

```bash
# 1. Scale down workload using the volume
kubectl scale deployment myapp --replicas=0 -n myapp
kubectl wait --for=condition=ready pod -l app=myapp --timeout=120s 2>/dev/null || true

# 2. Create migration pod with both PVCs mounted
cat > /tmp/rsync-migration.yaml <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: migrate-data-db
  namespace: myapp
spec:
  containers:
  - name: migrate
    image: alpine:3.18
    command: ["sh", "-c", "apk add rsync && rsync -av --delete /source/ /dest/"]
    volumeMounts:
    - name: source
      mountPath: /source
    - name: dest
      mountPath: /dest
    resources:
      requests:
        cpu: 1
        memory: 512Mi
      limits:
        cpu: 2
        memory: 1Gi
  volumes:
  - name: source
    persistentVolumeClaim:
      claimName: data-db        # Old Longhorn PVC
  - name: dest
    persistentVolumeClaim:
      claimName: data-db-nest   # New Nest DataResource
  restartPolicy: Never
EOF

kubectl apply -f /tmp/rsync-migration.yaml

# 3. Monitor copy progress
kubectl logs -f migrate-data-db -n myapp

# Expected final output: "sent XXX bytes  received YYY bytes"
```

```bash
# 4. Verify byte counts match
# Get source size
kubectl exec -n myapp migration-pod -- du -sb /source | awk '{print $1}'

# Get dest size
kubectl exec -n myapp migration-pod -- du -sb /dest | awk '{print $1}'

# Sizes must match exactly
# If different, rsync failed and will output error details in logs
```

### Step 4.4: Application-Level Replication (For Databases)

PostgreSQL live replication example:

```bash
# 1. On source (Longhorn) database, create replication slot
kubectl exec -n myapp deployment/postgres -- psql -U postgres -c \
  "CREATE PUBLICATION repl FOR ALL TABLES;"

# 2. On destination (Nest) database, configure as standby
# This assumes fresh Nest PVC with PostgreSQL initialized
kubectl exec -n myapp deployment/postgres-replica -- psql -U postgres -c \
  "CREATE SUBSCRIPTION repl CONNECTION 'host=postgres-source user=postgres' \
   PUBLICATION repl WITH (copy_data=true);"

# 3. Monitor replication lag
kubectl exec -n myapp deployment/postgres-replica -- psql -U postgres -c \
  "SELECT slot_name, restart_lsn, confirmed_flush_lsn FROM pg_replication_slots;"

# 4. Once caught up, switch application to replica
# Update service endpoints to point to replica
kubectl patch svc postgres -n myapp -p '{"spec":{"selector":{"app":"postgres-replica"}}}'
```

---

## Phase 5: Workload Cutover

### Step 5.1: Update Application Manifests

Update each workload to reference Nest DataResources instead of Longhorn PVCs:

```bash
# Backup current manifests
kubectl get deployment,statefulset -n myapp -o yaml > /tmp/workloads-original.yaml

# Edit deployment to reference new Nest PVC
kubectl patch deployment myapp -n myapp --type merge -p \
  '{
    "spec": {
      "template": {
        "spec": {
          "volumes": [
            {
              "name": "data",
              "persistentVolumeClaim": {
                "claimName": "data-db-nest"
              }
            }
          ]
        }
      }
    }
  }'
```

Or manually edit the manifest:

```yaml
# BEFORE (Longhorn)
spec:
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: data-db          # Old Longhorn PVC

# AFTER (Nest)
spec:
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: data-db-nest     # New Nest DataResource
```

### Step 5.2: Trigger Pod Restart

Force pods to restart with new PVC binding:

```bash
# Scale down
kubectl scale deployment myapp --replicas=0 -n myapp

# Wait for pods to terminate
kubectl wait --for=delete pod -l app=myapp --timeout=60s

# Scale up (pods will mount new Nest PVC)
kubectl scale deployment myapp --replicas=3 -n myapp

# Monitor rollout
kubectl rollout status deployment/myapp -n myapp --timeout=5m
```

### Step 5.3: Validate Data Access

Verify workload can read/write to Nest storage:

```bash
# Check pod startup logs for mount errors
kubectl logs -f deployment/myapp -n myapp --all-containers=true

# Look for errors like:
# - "mount error: permission denied"
# - "I/O error on device"
# - "file not found"

# Test application functionality
kubectl port-forward service/myapp 3000:80 -n myapp &
curl http://localhost:3000/api/health
# Should return 200 OK

# Test data write
curl -X POST http://localhost:3000/api/data -d '{"test":"data"}'

# Test data read
curl http://localhost:3000/api/data
```

---

## Phase 6: Validation & Verification

### Step 6.1: Data Integrity Checks

```bash
# For PostgreSQL, verify row counts
LONGHORN_COUNT=$(kubectl exec -n myapp deployment/postgres -- \
  psql -U postgres -t -c "SELECT COUNT(*) FROM <table>;")

NEST_COUNT=$(kubectl exec -n myapp deployment/postgres-new -- \
  psql -U postgres -t -c "SELECT COUNT(*) FROM <table>;")

if [ "$LONGHORN_COUNT" = "$NEST_COUNT" ]; then
  echo "✓ Data integrity verified: $LONGHORN_COUNT rows"
else
  echo "✗ Mismatch! Longhorn: $LONGHORN_COUNT, Nest: $NEST_COUNT"
fi

# For file storage, compare checksums
kubectl exec -n myapp pod/old-app -- \
  find /data -type f -exec md5sum {} \; > /tmp/longhorn-sums.txt

kubectl exec -n myapp pod/new-app -- \
  find /data -type f -exec md5sum {} \; > /tmp/nest-sums.txt

diff /tmp/longhorn-sums.txt /tmp/nest-sums.txt
# Should be identical
```

### Step 6.2: Performance Validation

Monitor I/O performance for 24 hours post-migration:

```bash
# Setup monitoring
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: migration-monitoring
  namespace: myapp
data:
  script.sh: |
    while true; do
      kubectl top pod -n myapp --containers
      sleep 60
    done
EOF

# Run performance baseline
kubectl exec -n myapp deployment/myapp -- \
  /app/perf-test.sh > /tmp/nest-perf.txt

# Compare with pre-migration baseline
# Look for: CPU usage, memory pressure, I/O latency
```

### Step 6.3: Monitor for 7+ Days

Keep Longhorn volumes intact during observation period:

```bash
# Automated monitoring check (run daily)
kubectl get pvc -A | grep longhorn
# Should still list all original PVCs (not yet deleted)

# Application health checks
kubectl get deployment,statefulset -n myapp -o wide
# All should show READY=true

# Check for warnings/errors in logs
kubectl logs -n myapp -l app=myapp --all-containers=true \
  --since=24h | grep -i "error\|warn" | head -20
```

---

## Phase 7: Rollback (If Needed)

If critical issues emerge, rollback is safe because Longhorn volumes persist:

### Step 7.1: Emergency Rollback

```bash
# 1. Scale down new workload
kubectl scale deployment myapp --replicas=0 -n myapp

# 2. Revert volume references
kubectl patch deployment myapp -n myapp --type merge -p \
  '{
    "spec": {
      "template": {
        "spec": {
          "volumes": [
            {
              "name": "data",
              "persistentVolumeClaim": {
                "claimName": "data-db"  # Back to Longhorn
              }
            }
          ]
        }
      }
    }
  }'

# 3. Restart pods
kubectl scale deployment myapp --replicas=3 -n myapp
kubectl rollout status deployment/myapp -n myapp

# 4. Verify on Longhorn
kubectl logs -f deployment/myapp -n myapp
# Should show successful mount of Longhorn PVC
```

### Step 7.2: Root Cause Analysis

If rollback necessary:

```bash
# Collect diagnostic data
kubectl describe dataresource data-db -n myapp
kubectl logs -n nest deployment/nest-csi-controller
kubectl logs -n nest deployment/nest-controller-manager

# Check Ceph health
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph status

# Review events
kubectl get events -n myapp --sort-by='.lastTimestamp' | tail -50

# Save for analysis
kubectl get all -n myapp -o yaml > /tmp/rollback-state.yaml
```

---

## Phase 8: Cleanup

Only execute after 7+ days of successful operation and approval to decommission Longhorn:

### Step 8.1: Delete Longhorn PVCs

```bash
# 1. Verify workloads are stable
kubectl get deployment,statefulset -n myapp -o wide
# All must show AVAILABLE replicas > 0

# 2. Delete old Longhorn PVCs
for pvc in data-db cache-data archive-vol; do
  kubectl delete pvc $pvc -n myapp
done

# Verify deletion
kubectl get pvc -n myapp | grep longhorn
# Should return empty
```

### Step 8.2: Run Nest Migration Cleanup

```bash
# Nest-provided cleanup command
nestctl migrate longhorn cleanup \
  --tenant mycompany \
  --kubeconfig ~/.kube/config \
  --context <cluster-context>

# This:
# - Removes Longhorn StorageClasses
# - Deregisters Longhorn CSIDriver
# - Scales down Longhorn manager deployment
# - Cleans up Longhorn CRDs
```

### Step 8.3: Decommission Longhorn (Optional)

If Longhorn is no longer needed elsewhere:

```bash
# Remove Longhorn Helm release
helm uninstall longhorn -n longhorn-system

# Delete Longhorn namespace
kubectl delete namespace longhorn-system

# Verify removal
kubectl get storageclass | grep longhorn
# Should be empty
```

---

## Troubleshooting

### DataResource Stuck in Pending

```bash
# Collect diagnostics
kubectl describe dataresource data-db -n myapp

# Common causes and fixes:
# 1. Ceph pool full
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph df
# Look for utilization > 80%; free up space or add OSD

# 2. CSI driver not ready
kubectl get daemonset -n nest nest-csi-node -o wide
# Must show DESIRED=READY

# 3. Node affinity issues
kubectl get nodes --show-labels | grep ceph

# 4. StorageClass does not exist
kubectl get storageclass | grep nest
```

### Pod Cannot Mount Nest Volume

```bash
# Check mount errors
kubectl logs <pod-name> -n myapp --previous

# Common errors:
# "permission denied": Fix file permissions
kubectl exec -n myapp <pod-name> -- chmod 755 /mnt/data

# "resource busy": Wait for unmount (usually 30 sec)
# or force pod deletion
kubectl delete pod <pod-name> --grace-period=0 --force -n myapp

# "no such device": CSI driver error
kubectl logs -n nest deployment/nest-csi-controller | grep error
```

### Data Mismatch After Migration

```bash
# Re-sync with Velero
velero restore create \
  --from-backup longhorn-migration-backup \
  --overwrite \
  longhorn-to-nest-restore-retry

# Or re-run rsync with --delete flag
kubectl run retry-sync --image=alpine --rm -it -- \
  sh -c "apk add rsync && rsync -av --delete /old /new"
```

### Workload Crashes After Cutover

```bash
# Check application logs
kubectl logs -n myapp deployment/myapp --tail=100

# Verify Nest PVC is writable
kubectl exec -n myapp deployment/myapp -- \
  touch /mnt/data/test-write.txt

# Check filesystem
kubectl exec -n myapp deployment/myapp -- df -h /mnt/data

# Restart pod to refresh mount
kubectl rollout restart deployment/myapp -n myapp
```

---

## Success Criteria

Migration is complete when:

- ✓ All Longhorn PVCs successfully copied to Nest DataResources
- ✓ All workloads running on Nest PVCs without errors
- ✓ Data integrity verified (checksums, row counts match)
- ✓ Performance baseline met or exceeded (I/O latency, throughput)
- ✓ 7+ days of stable operation without issues
- ✓ Old Longhorn PVCs deleted and Longhorn decommissioned

---

## Post-Migration

### Documentation Updates

- [ ] Update runbooks to reference Nest storage instead of Longhorn
- [ ] Document Nest StorageClass options for future deployments
- [ ] Archive migration logs for compliance/audit trails
- [ ] Update monitoring/alerting rules to track Ceph metrics

### Optimization

- [ ] Review DataResource class assignments; upgrade to `block-ha` for critical data
- [ ] Enable DataProtectionPolicy for automated backups
- [ ] Configure Ceph RBD snapshots for point-in-time recovery
- [ ] Tune Ceph replication factor based on availability requirements

### Knowledge Transfer

- [ ] Conduct post-mortem on migration process
- [ ] Document lessons learned and process improvements
- [ ] Train team on Nest storage management, troubleshooting
- [ ] Update disaster recovery procedures to use Nest

---

## References

- Nest Platform Spec: `/Users/penguinz/code/nest/docs/spec/nest-p1.md`
- Longhorn to Nest CSI Migration Guide: `/Users/penguinz/code/nest/docs/migration/longhorn-to-nest.md`
- Ceph Administration: https://docs.ceph.com/en/latest/
- Rook-Ceph Operator: https://rook.io/docs/rook/latest/
- Velero Backup & Restore: https://velero.io/docs/
