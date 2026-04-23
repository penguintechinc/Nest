#!/usr/bin/env bash
# Migrate a single Longhorn PVC to Nest CSI (Ceph RBD).
#
# Strategy (§6 of the spec):
#   1. Create a new PVC backed by nest-longhorn-compat StorageClass
#   2. Rsync data from old PVC to new PVC via a temporary migration pod
#   3. Update the workload to use the new PVC
#   4. Delete the old PVC (or retain for verification)
#
# Pre-requisites:
#   - Nest CSI driver deployed and registered
#   - Rook-Ceph healthy
#   - Workload scaled to 0 during data copy (or use pv-migrate for live copy)
set -euo pipefail

CONTEXT="${1:-}"
NAMESPACE="${2:-}"
PVC_NAME="${3:-}"
TARGET_SC="${4:-nest-longhorn-compat}"
DRY_RUN="${5:-false}"

usage() {
  echo "Usage: $0 <kube-context> <namespace> <pvc-name> [target-storageclass] [dry-run]"
  echo "  kube-context          Required. Kubernetes context"
  echo "  namespace             Required. Namespace of the PVC"
  echo "  pvc-name              Required. Name of the Longhorn PVC to migrate"
  echo "  target-storageclass   Optional. Default: nest-longhorn-compat"
  echo "  dry-run               Optional. Set to 'true' to show plan without executing"
  exit 1
}

[[ -z "$CONTEXT" || -z "$NAMESPACE" || -z "$PVC_NAME" ]] && usage

KC="kubectl --context $CONTEXT"
NEW_PVC_NAME="${PVC_NAME}-nest"
MIGRATION_POD="nest-migrate-${PVC_NAME}"

log()  { echo "[$(date -u +%H:%M:%S)] $*"; }
warn() { echo "[$(date -u +%H:%M:%S)] ⚠ $*" >&2; }
die()  { echo "[$(date -u +%H:%M:%S)] ✗ $*" >&2; exit 1; }

# ── Validate source PVC ──────────────────────────────────────────────────────
log "Validating source PVC: $NAMESPACE/$PVC_NAME"
PVC_JSON=$($KC -n "$NAMESPACE" get pvc "$PVC_NAME" -o json 2>/dev/null) \
  || die "PVC $NAMESPACE/$PVC_NAME not found"

CURRENT_SC=$(echo "$PVC_JSON" | jq -r '.spec.storageClassName')
PVC_SIZE=$(echo "$PVC_JSON" | jq -r '.spec.resources.requests.storage')
PVC_STATUS=$(echo "$PVC_JSON" | jq -r '.status.phase')
ACCESS_MODES=$(echo "$PVC_JSON" | jq -r '.spec.accessModes[]' | tr '\n' ',')

log "  Source:        $NAMESPACE/$PVC_NAME"
log "  StorageClass:  $CURRENT_SC"
log "  Size:          $PVC_SIZE"
log "  Status:        $PVC_STATUS"
log "  AccessModes:   ${ACCESS_MODES%,}"

if [[ "$CURRENT_SC" != "longhorn"* ]]; then
  warn "PVC $PVC_NAME has storageClass '$CURRENT_SC', expected 'longhorn*'. Proceeding anyway."
fi

# ── Check access modes ────────────────────────────────────────────────────────
if echo "$ACCESS_MODES" | grep -q "ReadWriteMany"; then
  die "PVC has ReadWriteMany access mode. Nest RBD only supports RWO in P1. Use nest-cephfs for RWX (P2+)."
fi

# ── Show plan ─────────────────────────────────────────────────────────────────
log ""
log "Migration plan:"
log "  Source PVC:    $NAMESPACE/$PVC_NAME  (StorageClass: $CURRENT_SC)"
log "  Target PVC:    $NAMESPACE/$NEW_PVC_NAME  (StorageClass: $TARGET_SC)"
log "  Size:          $PVC_SIZE"
log "  Method:        rsync via migration pod"
log ""

[[ "$DRY_RUN" == "true" ]] && { log "Dry-run mode — no changes made."; exit 0; }

# ── Create target PVC ─────────────────────────────────────────────────────────
log "Creating target PVC: $NEW_PVC_NAME"
if $KC -n "$NAMESPACE" get pvc "$NEW_PVC_NAME" &>/dev/null; then
  warn "Target PVC $NEW_PVC_NAME already exists — skipping creation"
else
  $KC apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${NEW_PVC_NAME}
  namespace: ${NAMESPACE}
  labels:
    nest.penguintech.io/migrated-from: ${PVC_NAME}
    nest.penguintech.io/migration-source-sc: ${CURRENT_SC}
  annotations:
    nest.penguintech.io/migration-timestamp: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    nest.penguintech.io/migration-source: "${NAMESPACE}/${PVC_NAME}"
spec:
  storageClassName: ${TARGET_SC}
  accessModes: $(echo "$PVC_JSON" | jq '.spec.accessModes')
  resources:
    requests:
      storage: ${PVC_SIZE}
EOF
  log "  ✓ Target PVC created"
fi

# ── Wait for target PVC to bind ───────────────────────────────────────────────
log "Waiting for target PVC to bind..."
DEADLINE=$(($(date +%s) + 120))
while [[ $(date +%s) -lt $DEADLINE ]]; do
  STATUS=$($KC -n "$NAMESPACE" get pvc "$NEW_PVC_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
  if [[ "$STATUS" == "Bound" ]]; then
    log "  ✓ PVC bound"
    break
  fi
  echo -n "  Waiting ($STATUS)... "
  sleep 5
done
FINAL_STATUS=$($KC -n "$NAMESPACE" get pvc "$NEW_PVC_NAME" -o jsonpath='{.status.phase}')
[[ "$FINAL_STATUS" != "Bound" ]] && die "Target PVC did not bind within 2 minutes (status: $FINAL_STATUS)"

# ── Run rsync migration pod ───────────────────────────────────────────────────
log "Running data migration pod: $MIGRATION_POD"
$KC apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: ${MIGRATION_POD}
  namespace: ${NAMESPACE}
  labels:
    nest.penguintech.io/migration-pod: "true"
    nest.penguintech.io/source-pvc: ${PVC_NAME}
    nest.penguintech.io/target-pvc: ${NEW_PVC_NAME}
spec:
  restartPolicy: Never
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000
  containers:
  - name: rsync
    image: debian:bookworm-slim@sha256:f9c6a2fd2ddbc23e336b6257a5245e31f996953ef06cd13a59fa0a1df2d5c252
    command:
    - /bin/bash
    - -c
    - |
      apt-get install -qq -y rsync > /dev/null 2>&1
      echo "Starting rsync: /source/ → /target/"
      rsync -avH --info=progress2 --delete /source/ /target/
      echo "Rsync complete. Verifying..."
      SRC_SIZE=\$(du -sb /source | awk '{print \$1}')
      DST_SIZE=\$(du -sb /target | awk '{print \$1}')
      echo "Source: \${SRC_SIZE} bytes, Target: \${DST_SIZE} bytes"
      if [[ "\$SRC_SIZE" -eq "\$DST_SIZE" ]]; then
        echo "✓ Size match verified"
        exit 0
      else
        echo "✗ Size mismatch! Source: \${SRC_SIZE}, Target: \${DST_SIZE}"
        exit 1
      fi
    resources:
      requests:
        cpu: 200m
        memory: 128Mi
      limits:
        cpu: 1000m
        memory: 512Mi
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: false
      capabilities:
        drop: [ALL]
    volumeMounts:
    - name: source
      mountPath: /source
      readOnly: true
    - name: target
      mountPath: /target
  volumes:
  - name: source
    persistentVolumeClaim:
      claimName: ${PVC_NAME}
      readOnly: true
  - name: target
    persistentVolumeClaim:
      claimName: ${NEW_PVC_NAME}
EOF

# ── Wait for migration to complete ────────────────────────────────────────────
log "Waiting for migration pod to complete (this may take a while for large volumes)..."
$KC -n "$NAMESPACE" wait --for=condition=Ready pod/"$MIGRATION_POD" --timeout=120s 2>/dev/null || true
$KC -n "$NAMESPACE" wait --for=jsonpath='{.status.phase}'=Succeeded pod/"$MIGRATION_POD" --timeout=3600s \
  || die "Migration pod failed or timed out. Check: kubectl -n $NAMESPACE logs $MIGRATION_POD"

log "  ✓ Migration complete"

# ── Show migration logs ───────────────────────────────────────────────────────
log "Migration logs:"
$KC -n "$NAMESPACE" logs "$MIGRATION_POD" | tail -10 | sed 's/^/  /'

# ── Cleanup migration pod ─────────────────────────────────────────────────────
log "Cleaning up migration pod..."
$KC -n "$NAMESPACE" delete pod "$MIGRATION_POD" --wait=false
log "  ✓ Pod deletion requested"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
log "=== Migration Complete ==="
log "  Source PVC: $NAMESPACE/$PVC_NAME (Longhorn — keep until workload verified)"
log "  Target PVC: $NAMESPACE/$NEW_PVC_NAME (Nest CSI / $TARGET_SC)"
log ""
log "Next steps:"
log "  1. Update your workload to reference '$NEW_PVC_NAME'"
log "  2. Verify data integrity and application function"
log "  3. Once verified, delete the old PVC: kubectl -n $NAMESPACE delete pvc $PVC_NAME"
log "  4. Repeat for remaining PVCs, then run longhorn-drain.sh"
