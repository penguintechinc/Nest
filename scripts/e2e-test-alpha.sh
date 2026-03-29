#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== NEST Alpha E2E Tests ==="
echo "Target: local-alpha cluster"

# Step 1: Smoke check
echo ""
echo "--- Step 1: Smoke Check ---"
HEALTH_URL="${PLAYWRIGHT_BASE_URL:-http://nest.localhost.local}/api/v1/healthz"
echo "Checking health: $HEALTH_URL"
if ! curl -sf "$HEALTH_URL" > /dev/null 2>&1; then
  # Try localhost:3000 fallback
  HEALTH_URL="http://localhost:3000/api/v1/healthz"
  if ! curl -sf "$HEALTH_URL" > /dev/null 2>&1; then
    echo "ERROR: Health check failed. Is the app deployed?"
    echo "Deploy with: kubectl apply --context local-alpha -k k8s/kustomize/overlays/alpha"
    exit 1
  fi
fi
echo "Health check passed"

# Step 2: Run Playwright tests
echo ""
echo "--- Step 2: Playwright E2E Tests ---"
cd "$PROJECT_ROOT/web"

# Install browsers if needed
if [ ! -d "$HOME/.cache/ms-playwright" ]; then
  echo "Installing Playwright browsers..."
  npx playwright install --with-deps chromium
fi

# Run tests
npx playwright test --reporter=list

echo ""
echo "=== E2E Tests Complete ==="
