#!/usr/bin/env bash
# Decommission Longhorn after all PVCs have been migrated to Nest CSI.
# Follows the §6 migration sequence from the Nest spec.
#
# WARNING: This is irreversible. Run longhorn-preflight.sh first to confirm
# there are no remaining Longhorn PVCs before running this script.
set -euo pipefail

CONTEXT="${1:-}"
DRY_RUN="${2:-false}"
LONGHORN_NS="${3:-longhorn-system}"

usage() {
  echo "Usage: $0 <kube-context> [dry-run] [longhorn-namespace]"
  echo "  kube-context       Required. Kubernetes context"
  echo "  dry-run            Optional. 'true' to show plan only (default: false)"
  echo "  longhorn-namespace Optional. Default: longhorn-system"
  echo ""
  echo "  Run longhorn-preflight.sh first to verify no Longhorn PVCs remain."
  exit 1
}

[[ -z "$CONTEXT" ]] && usage

KC="kubectl --context $CONTEXT"

log()  { echo "[$(date -u +%H:%M:%S)] $*"; }
warn() { echo "[$(date -u +%H:%M:%S)] ⚠ $*" >&2; }
die()  { echo "[$(date -u +%H:%M:%S)] ✗ $*" >&2; exit 1; }

echo "=== Longhorn Decommission ==="
echo "Context:           $CONTEXT"
echo "Longhorn NS:       $LONGHORN_NS"
echo "Dry-run:           $DRY_RUN"
echo ""

# ── Safety: verify no remaining Longhorn PVCs ─────────────────────────────────
log "Checking for remaining Longhorn PVCs..."
REMAINING=$($KC get pvc --all-namespaces -o json \
  | jq -r '.items[] | select(.spec.storageClassName == "longhorn" or (.spec.storageClassName | startswith("longhorn"))) | "\(.metadata.namespace)/\(.metadata.name)"' 2>/dev/null || true)

if [[ -n "$REMAINING" ]]; then
  die "Found remaining Longhorn PVCs — migrate these first:\n$REMAINING"
fi
log "  ✓ No remaining Longhorn PVCs"

# ── Show Longhorn state ────────────────────────────────────────────────────────
log "Current Longhorn state:"
$KC -n "$LONGHORN_NS" get pods 2>/dev/null | head -20 | sed 's/^/  /' || warn "Longhorn pods not found"

if [[ "$DRY_RUN" == "true" ]]; then
  log ""
  log "Dry-run mode — no changes made. Would decommission:"
  log "  1. Scale Longhorn Manager to 0 replicas"
  log "  2. Delete Longhorn StorageClass(es)"
  log "  3. Delete Longhorn CSIDriver"
  log "  4. Delete Longhorn namespace $LONGHORN_NS"
  log "  5. Remove Longhorn node labels"
  exit 0
fi

# ── Confirm destructive action ────────────────────────────────────────────────
echo ""
echo "This will permanently remove Longhorn from cluster '$CONTEXT'."
echo "All Longhorn data replicas will become dark drives for Nest re-adoption (§4 of spec)."
echo ""
read -r -p "Type 'yes-remove-longhorn' to confirm: " CONFIRM
[[ "$CONFIRM" != "yes-remove-longhorn" ]] && die "Aborted"

# ── Scale Longhorn to zero ────────────────────────────────────────────────────
log "Scaling Longhorn manager to zero..."
$KC -n "$LONGHORN_NS" scale deploy longhorn-manager --replicas=0 2>/dev/null && log "  ✓ Scaled to 0" || warn "longhorn-manager not found, skipping"

# ── Delete Longhorn StorageClasses ────────────────────────────────────────────
log "Deleting Longhorn StorageClasses..."
$KC get storageclass -o json \
  | jq -r '.items[] | select(.provisioner | startswith("driver.longhorn.io") or startswith("rancher.io/longhorn")) | .metadata.name' \
  | while read -r sc; do
    $KC delete storageclass "$sc" && log "  ✓ Deleted StorageClass: $sc" || warn "Failed to delete $sc"
  done

# ── Delete Longhorn CSIDriver ─────────────────────────────────────────────────
log "Deleting Longhorn CSIDriver..."
$KC get csidriver -o json \
  | jq -r '.items[] | select(.metadata.name | startswith("driver.longhorn.io") or startswith("rancher.io/longhorn")) | .metadata.name' \
  | while read -r csi; do
    $KC delete csidriver "$csi" && log "  ✓ Deleted CSIDriver: $csi" || warn "Failed to delete $csi"
  done

# ── Delete Longhorn namespace ─────────────────────────────────────────────────
log "Deleting Longhorn namespace $LONGHORN_NS (may take several minutes)..."
$KC delete namespace "$LONGHORN_NS" --timeout=300s 2>/dev/null \
  && log "  ✓ Namespace deleted" \
  || warn "Namespace deletion timed out or namespace not found"

# ── Remove Longhorn node labels ───────────────────────────────────────────────
log "Removing Longhorn node labels..."
$KC get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | while read -r node; do
  $KC label node "$node" \
    node.longhorn.io/create-default-disk- \
    longhorn.io/node- \
    2>/dev/null || true
done
log "  ✓ Node labels cleaned"

# ── Annotate dark drives in HardwareInventory ────────────────────────────────
log "Annotating HardwareInventory CRs to re-scan for new dark drives..."
log "  (Longhorn replica disks are now unprovisioned — Nest node-agent will detect them)"
log "  After verification, use the Nest UI to adopt them into a HardwarePool."

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
log "=== Longhorn Decommission Complete ==="
log "  Longhorn has been removed from cluster $CONTEXT"
log ""
log "Next steps:"
log "  1. Check Nest UI for newly discovered dark drives (§4 of spec)"
log "  2. Adopt the ex-Longhorn disks into a HardwarePool"
log "  3. Verify all workloads are healthy on Nest CSI"
log "  4. Confirm Nest StorageClass is set as cluster default if desired:"
log "     kubectl patch storageclass nest-rbd -p '{\"metadata\":{\"annotations\":{\"storageclass.kubernetes.io/is-default-class\":\"true\"}}}'"
