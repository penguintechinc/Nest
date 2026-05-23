#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="articdbm-alpha"
RELEASE_NAME="articdbm-alpha"

echo "=== Tearing down ArticDBM Alpha ==="

# Kill any port-forwards
pkill -f "kubectl.*port-forward.*$NAMESPACE" 2>/dev/null || true

# Uninstall helm release
echo "--- Uninstalling Helm release ---"
microk8s helm3 uninstall "$RELEASE_NAME" -n "$NAMESPACE" 2>/dev/null || true

# Delete namespace
echo "--- Deleting namespace ---"
microk8s kubectl delete namespace "$NAMESPACE" 2>/dev/null || true

echo "=== Alpha teardown complete ==="
