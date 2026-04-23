# Longhorn → Nest CSI Migration Guide

This guide walks through replacing Longhorn with the Nest CSI driver (backed by Rook-Ceph RBD)
for ReadWriteOnce (RWO) block volumes. Follow §6 of the Nest platform spec for the full
architectural rationale.

## Overview

Nest deploys its own CSI driver (`csi.nest.penguintech.io`) that provisions Ceph RBD images
via Rook-Ceph. For migration purposes Nest provides a `nest-longhorn-compat` StorageClass
that mirrors Longhorn's default access semantics (RWO, Retain).

**Phase-1 scope:** RWO block volumes only.  
**Not yet in P1:** ReadWriteMany (CephFS/RWX) — migrate those in P2+.

## Prerequisites

| Requirement | Check |
|---|---|
| Rook-Ceph deployed and healthy (`HEALTH_OK`) | `kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph status` |
| Nest deployed with CSI DaemonSet ready | `kubectl -n nest get ds nest-csi-node` |
| `jq` installed on migration host | `jq --version` |
| Source workloads can tolerate brief downtime | Scale to 0 during rsync, or use pv-migrate |

## Migration Sequence

### Step 1: Pre-flight Check

```bash
# Confirm Nest CSI is ready and inventory all Longhorn PVCs
./scripts/migration/longhorn-preflight.sh <kube-context>

# Limit to a specific namespace
./scripts/migration/longhorn-preflight.sh <kube-context> my-app-namespace
```

The script validates:
- CSIDriver `csi.nest.penguintech.io` is registered
- `nest-csi-node` DaemonSet is fully ready
- All Longhorn PVCs listed with namespace, name, size, and status
- Rook-Ceph cluster health

### Step 2: Migrate PVCs One-by-One

For each Longhorn PVC output by step 1:

```bash
# Scale down the workload first (prevents data corruption during rsync)
kubectl --context <ctx> -n <namespace> scale deploy/<workload> --replicas=0

# Migrate the PVC
./scripts/migration/longhorn-migrate-pvc.sh <kube-context> <namespace> <pvc-name>

# Example
./scripts/migration/longhorn-migrate-pvc.sh local-alpha my-app orders-data
```

The script:
1. Creates `<pvc-name>-nest` backed by `nest-longhorn-compat` StorageClass
2. Launches a migration pod that rsyncs data from the old PVC to the new one
3. Verifies byte counts match
4. Reports next steps

#### Update the workload

After migration, update the workload's `volumes` section:

```yaml
# Before
volumes:
- name: orders-data
  persistentVolumeClaim:
    claimName: orders-data

# After
volumes:
- name: orders-data
  persistentVolumeClaim:
    claimName: orders-data-nest
```

Scale the workload back up and verify:

```bash
kubectl --context <ctx> -n <namespace> scale deploy/<workload> --replicas=1
kubectl --context <ctx> -n <namespace> rollout status deploy/<workload>
```

Once the application is confirmed healthy, delete the old Longhorn PVC:

```bash
kubectl --context <ctx> -n <namespace> delete pvc orders-data
```

### Step 3: Verify All PVCs Migrated

Re-run the pre-flight script — it should report "No Longhorn PVCs found":

```bash
./scripts/migration/longhorn-preflight.sh <kube-context>
```

### Step 4: Decommission Longhorn

```bash
# Dry-run first to see what will be removed
./scripts/migration/longhorn-drain.sh <kube-context> true

# Execute
./scripts/migration/longhorn-drain.sh <kube-context>
```

The drain script:
1. Verifies zero remaining Longhorn PVCs (refuses if any exist)
2. Scales Longhorn manager to 0
3. Deletes Longhorn StorageClasses and CSIDriver
4. Removes the `longhorn-system` namespace
5. Cleans node labels

### Step 5: Adopt Ex-Longhorn Disks as Dark Drives

Longhorn replica disks are now unprovisioned block devices. The Nest node-agent will
detect them and create `DarkDrive` events (visible in the Operator UI hardware panel).

Review each discovered dark drive and adopt them into a `HardwarePool`:

```bash
# List discovered dark drives (if CRDs are registered)
kubectl get darkdrives.nest.penguintech.io --all-namespaces

# Or check hardware inventory
kubectl get hardwareinventories.nest.penguintech.io --all-namespaces
```

**Important:** Foreign-filesystem drives (`foreign-fs: ext4` or similar) require
`eraseConfirmed: true` in the adoption workflow — never auto-adopted.

### Step 6 (Optional): Set Nest as Default StorageClass

```bash
# Remove old default annotation from Longhorn (if it still exists)
kubectl patch storageclass longhorn \
  -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"false"}}}' 2>/dev/null || true

# Set Nest RBD as the new default
kubectl patch storageclass nest-rbd \
  -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
```

## StorageClass Reference

| StorageClass | Access Modes | Reclaim | Use |
|---|---|---|---|
| `nest-rbd` | RWO | Retain | New workloads (non-Longhorn) |
| `nest-longhorn-compat` | RWO | Retain | Longhorn PVC migration target |

## Rollback

If issues arise during migration, the old Longhorn PVC remains intact until you explicitly
delete it. To roll back:

1. Scale workload down
2. Update `claimName` back to the original PVC name
3. Scale workload up
4. Delete the failed `<pvc-name>-nest` PVC

Longhorn itself is only removed in step 4 (drain), which requires confirmation.

## Alternative: pv-migrate for Live Migration

For workloads that cannot tolerate downtime, use [`pv-migrate`](https://github.com/utkuozdemir/pv-migrate):

```bash
# Install pv-migrate
brew install pv-migrate   # or download from GitHub releases

# Live migration (workload keeps running)
pv-migrate migrate \
  --context <kube-context> \
  --source-namespace <namespace> \
  --dest-namespace <namespace> \
  --dest-storage-class nest-longhorn-compat \
  <source-pvc-name> <dest-pvc-name>
```

`pv-migrate` handles the PVC clone and uses rsync with `--delete` for delta sync.

## Troubleshooting

### Migration pod stuck in Pending
```bash
kubectl -n <namespace> describe pod nest-migrate-<pvc-name>
```
Usually means the target PVC hasn't bound yet (Ceph pool not ready, no capacity).

### rsync exits non-zero
```bash
kubectl -n <namespace> logs nest-migrate-<pvc-name>
```
Common cause: source PVC mounted read-only by another pod. Scale down the workload first.

### Target PVC stays in Pending
```bash
kubectl describe pvc <pvc-name>-nest -n <namespace>
# Check for: no matching CSI node driver
kubectl get csinode -o yaml | grep nest
```
Ensure `nest-csi-node` DaemonSet is running on all nodes.

### Ceph pool provisioning errors
```bash
kubectl -n rook-ceph logs deploy/rook-ceph-operator | tail -50
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph osd df
```
