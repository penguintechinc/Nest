#!/bin/bash
# Manual-only migration script. NEVER call from application startup.
#
# Usage:
#   ./scripts/migrate.sh                  # upgrade head (default)
#   ./scripts/migrate.sh current          # show current revision
#   ./scripts/migrate.sh history          # show full chain
#   ./scripts/migrate.sh downgrade -1     # roll back one step
#   ./scripts/migrate.sh heads            # show all head revisions
#
# Required environment variables:
#   DB_URI   — full database URI (e.g. postgresql://user:pass@host/db)
#
# Optional (for schema version archiving to S3-compatible storage):
#   SCHEMA_STORAGE_BUCKET    — S3 bucket name
#   SCHEMA_STORAGE_ENDPOINT  — S3-compatible endpoint URL
#
# This script runs alembic from apps/manager/ where alembic.ini is located.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANAGER_DIR="${SCRIPT_DIR}/../apps/manager"

if [ ! -f "${MANAGER_DIR}/migrations/alembic.ini" ]; then
    echo "[migrate] ERROR: alembic.ini not found at ${MANAGER_DIR}/migrations/alembic.ini" >&2
    exit 1
fi

# DB_URI must be set — never fall through to a default in production
if [ -z "${DB_URI:-}" ]; then
    echo "[migrate] ERROR: DB_URI environment variable is not set." >&2
    echo "[migrate] Set DB_URI before running migrations, e.g.:" >&2
    echo "[migrate]   export DB_URI=postgresql://user:pass@host:5432/dbname" >&2
    exit 1
fi

COMMAND=${1:-upgrade head}

cd "${MANAGER_DIR}"

echo "[migrate] Working directory: $(pwd)"
echo "[migrate] Running: alembic ${COMMAND}"
# Word-splitting on COMMAND is intentional here (multi-word commands like "upgrade head")
# shellcheck disable=SC2086
alembic ${COMMAND}

# Upload migration state to S3-compatible storage for schema versioning.
# This step is optional and skipped if storage env vars are not set.
if [ -n "${SCHEMA_STORAGE_BUCKET:-}" ] && [ -n "${SCHEMA_STORAGE_ENDPOINT:-}" ]; then
    REVISION=$(alembic current 2>/dev/null | grep -oP '\w{12}' | head -1 || echo "unknown")
    TIMESTAMP=$(date -u +%Y%m%dT%H%M%SZ)
    echo "[migrate] Uploading schema version to object storage: ${REVISION}"
    aws s3 cp migrations/versions/ \
        "s3://${SCHEMA_STORAGE_BUCKET}/schema-versions/${TIMESTAMP}/" \
        --recursive \
        --endpoint-url "${SCHEMA_STORAGE_ENDPOINT}" \
        --no-sign-request 2>/dev/null \
        || echo "[migrate] Warning: S3 upload skipped (check credentials or endpoint)"
    echo "[migrate] Schema version: ${REVISION} @ ${TIMESTAMP}"
fi

echo "[migrate] Done."
