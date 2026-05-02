#!/usr/bin/env bash
# Longhorn → Nest CSI pre-flight validation.
# Run this before starting the migration to identify all PVCs, check Ceph health,
# and confirm Nest CSI driver is ready.
set -euo pipefail

CONTEXT="${1:-}"
NAMESPACE="${2:-}"
NEST_NS="${3:-nest}"

usage() {
  echo "Usage: $0 <kube-context> [namespace] [nest-namespace]"
  echo "  kube-context   Required. e.g. dal2-beta or local-alpha"
  echo "  namespace       Optional. Limit scan to a specific namespace (default: all)"
  echo "  nest-namespace  Optional. Namespace where Nest is deployed (default: nest)"
  exit 1
}

[[ -z "$CONTEXT" ]] && usage

KC="kubectl --context $CONTEXT"

echo "=== Nest CSI Pre-flight: Longhorn Migration ==="
echo "Context:       $CONTEXT"
echo "Namespace:     ${NAMESPACE:-<all>}"
echo "Nest NS:       $NEST_NS"
echo ""

# ── 1. Confirm Nest CSI driver is registered ──────────────────────────────────
echo "--- [1/5] Checking Nest CSI driver registration ---"
if $KC get csidriver csi.nest.penguintech.io &>/dev/null; then
  echo "  ✓ CSIDriver csi.nest.penguintech.io found"
else
  echo "  ✗ CSIDriver csi.nest.penguintech.io NOT found"
  echo "    Deploy Nest first: kubectl kustomize k8s/kustomize/overlays/alpha | kubectl --context $CONTEXT apply -f -"
  exit 1
fi

# ── 2. Check Nest CSI DaemonSet health ────────────────────────────────────────
echo "--- [2/5] Checking Nest CSI DaemonSet health ---"
DESIRED=$($KC -n "$NEST_NS" get daemonset nest-csi-node -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null || echo "0")
READY=$($KC -n "$NEST_NS" get daemonset nest-csi-node -o jsonpath='{.status.numberReady}' 2>/dev/null || echo "0")
if [[ "$DESIRED" -eq 0 ]]; then
  echo "  ✗ nest-csi-node DaemonSet not found in namespace $NEST_NS"
  exit 1
elif [[ "$READY" -lt "$DESIRED" ]]; then
  echo "  ⚠ DaemonSet not fully ready: $READY/$DESIRED nodes ready"
else
  echo "  ✓ DaemonSet ready: $READY/$DESIRED nodes"
fi

# ── 3. Inventory Longhorn PVCs ────────────────────────────────────────────────
echo "--- [3/5] Inventorying Longhorn PVCs ---"
NS_FLAG=""
[[ -n "$NAMESPACE" ]] && NS_FLAG="-n $NAMESPACE" || NS_FLAG="--all-namespaces"

LONGHORN_PVCS=$($KC get pvc $NS_FLAG -o json \
  | jq -r '.items[] | select(.spec.storageClassName == "longhorn" or (.spec.storageClassName | startswith("longhorn"))) | [.metadata.namespace, .metadata.name, .spec.storageClassName, .status.capacity.storage, .status.phase] | @tsv' 2>/dev/null || true)

if [[ -z "$LONGHORN_PVCS" ]]; then
  echo "  ✓ No Longhorn PVCs found"
else
  COUNT=$(echo "$LONGHORN_PVCS" | wc -l | tr -d ' ')
  echo "  Found $COUNT Longhorn PVC(s):"
  printf "  %-30s %-40s %-20s %-10s %-10s\n" "NAMESPACE" "NAME" "STORAGECLASS" "SIZE" "STATUS"
  while IFS=$'\t' read -r ns name sc size phase; do
    printf "  %-30s %-40s %-20s %-10s %-10s\n" "$ns" "$name" "$sc" "$size" "$phase"
  done <<< "$LONGHORN_PVCS"
fi
echo ""

# ── 4. Check for bound PVs (identify replicas for re-adoption) ────────────────
echo "--- [4/5] Checking Longhorn PVs ---"
LONGHORN_PVS=$($KC get pv -o json \
  | jq -r '.items[] | select(.spec.storageClassName == "longhorn" or (.spec.storageClassName | startswith("longhorn"))) | [.metadata.name, .spec.storageClassName, .spec.capacity.storage, .status.phase, .spec.claimRef.namespace // "-", .spec.claimRef.name // "-"] | @tsv' 2>/dev/null || true)

if [[ -n "$LONGHORN_PVS" ]]; then
  PV_COUNT=$(echo "$LONGHORN_PVS" | wc -l | tr -d ' ')
  echo "  Found $PV_COUNT Longhorn PV(s)"
fi

# ── 5. Verify Ceph/Rook health ────────────────────────────────────────────────
echo "--- [5/5] Checking Rook-Ceph cluster health ---"
CEPH_NS="rook-ceph"
CEPH_HEALTH=$($KC -n "$CEPH_NS" exec deploy/rook-ceph-tools -- ceph status -f json 2>/dev/null \
  | jq -r '.health.status' 2>/dev/null || echo "UNREACHABLE")

if [[ "$CEPH_HEALTH" == "HEALTH_OK" ]]; then
  echo "  ✓ Ceph cluster health: HEALTH_OK"
elif [[ "$CEPH_HEALTH" == "UNREACHABLE" ]]; then
  echo "  ⚠ Could not reach Ceph tools pod; ensure rook-ceph is deployed"
else
  echo "  ⚠ Ceph health: $CEPH_HEALTH (migration can proceed but investigate)"
fi

# ── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "=== Pre-flight Summary ==="
if [[ -z "$LONGHORN_PVCS" ]]; then
  echo "No Longhorn PVCs to migrate. Nothing to do."
else
  echo "Ready to migrate. Run each PVC through longhorn-migrate-pvc.sh:"
  echo ""
  while IFS=$'\t' read -r ns name _ _ _; do
    echo "  ./scripts/migration/longhorn-migrate-pvc.sh $CONTEXT $ns $name nest-longhorn-compat"
  done <<< "$LONGHORN_PVCS"
  echo ""
  echo "After all PVCs migrated, run longhorn-drain.sh to decommission Longhorn."
fi
