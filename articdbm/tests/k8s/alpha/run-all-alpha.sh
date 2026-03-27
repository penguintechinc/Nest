#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
NAMESPACE="articdbm-alpha"
RELEASE_NAME="articdbm-alpha"
HELM_DIR="$PROJECT_ROOT/k8s/helm/articdbm"

echo "=== ArticDBM Alpha Deployment & Test ==="
echo "Project root: $PROJECT_ROOT"
echo "Helm chart: $HELM_DIR"
echo ""

# Step 1: Ensure microk8s addons
echo "--- Enabling required microk8s addons ---"
microk8s enable dns storage ingress 2>/dev/null || true

# Step 2: Deploy with Helm
echo "--- Deploying ArticDBM to alpha namespace ---"
microk8s helm3 upgrade --install "$RELEASE_NAME" "$HELM_DIR" \
    -f "$HELM_DIR/values-alpha.yaml" \
    -n "$NAMESPACE" \
    --create-namespace \
    --wait \
    --timeout 5m

# Step 3: Wait for pods to be ready
echo "--- Waiting for pods to be ready ---"
microk8s kubectl wait --for=condition=ready pod \
    -l app.kubernetes.io/instance="$RELEASE_NAME" \
    -n "$NAMESPACE" \
    --timeout=120s

echo "--- Pod status ---"
microk8s kubectl get pods -n "$NAMESPACE"

# Step 4: Port-forward services for testing
echo "--- Setting up port-forwards ---"
# Kill any existing port-forwards
pkill -f "kubectl.*port-forward.*$NAMESPACE" 2>/dev/null || true
sleep 1

microk8s kubectl port-forward -n "$NAMESPACE" svc/"$RELEASE_NAME"-manager 8000:8000 &
PF_MANAGER_PID=$!

microk8s kubectl port-forward -n "$NAMESPACE" svc/"$RELEASE_NAME"-proxy 9091:9091 &
PF_PROXY_PID=$!

# Wait for port-forwards to establish
sleep 3

# Step 5: Run tests
echo "--- Running test suite ---"
cd "$PROJECT_ROOT"

TEST_EXIT_CODE=0
export RUN_INTEGRATION_TESTS=1
export PROXY_METRICS_URL="http://localhost:9091"
python3 -m pytest tests/smoke/ tests/api/ tests/page/ tests/integration/ -v --timeout=120 \
    || TEST_EXIT_CODE=$?

# Step 6: Cleanup port-forwards
echo "--- Cleaning up port-forwards ---"
kill $PF_MANAGER_PID 2>/dev/null || true
kill $PF_PROXY_PID 2>/dev/null || true

# Step 7: Report
echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "=== ALL ALPHA TESTS PASSED ==="
else
    echo "=== ALPHA TESTS FAILED (exit code: $TEST_EXIT_CODE) ==="
fi

exit $TEST_EXIT_CODE
