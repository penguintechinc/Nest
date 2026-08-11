"""
NEST Manager Models — penguin-dal Database Layer

Initializes PostgreSQL connection via penguin-dal (schema auto-reflected).
Schema is managed by Alembic — see apps/manager/migrations/.

DO NOT add define_table() calls here. Tables are created by Alembic migrations
and auto-reflected by penguin-dal at runtime.
"""
from __future__ import annotations

import os

from penguin_dal import DB
from models.operations import OperationRecord  # noqa: F401

# ---------------------------------------------------------------------------
# Database configuration from environment variables
# FAIL CLOSED: All credentials must be explicitly configured
# ---------------------------------------------------------------------------
DB_TYPE = os.getenv("DB_TYPE", "").strip() or "postgresql"
DB_HOST = os.getenv("DB_HOST", "").strip()
DB_PORT = os.getenv("DB_PORT", "").strip() or "5432"
DB_NAME = os.getenv("DB_NAME", "").strip()
DB_USER = os.getenv("DB_USER", "").strip()
DB_PASSWORD = os.getenv("DB_PASS", os.getenv("DB_PASSWORD", "")).strip()

# Allow imports with minimal env requirements - only validate when all creds present
# This allows importing OperationRecord and other classes even in test/import-only contexts
_has_all_creds = all([DB_HOST, DB_NAME, DB_USER, DB_PASSWORD])

if not _has_all_creds:
    # Import-only mode (e.g., importing OperationRecord in tests)
    DB_URI = "sqlite:///:memory:"
    db = None  # type: ignore[assignment]
else:
    DB_URI = f"{DB_TYPE}://{DB_USER}:{DB_PASSWORD}@{DB_HOST}:{DB_PORT}/{DB_NAME}"
    # ---------------------------------------------------------------------------
    # DB instance (synchronous; used in sync contexts and module-level init)
    # For Quart async request handlers, use get_db() from penguin_dal.quart_ext
    # Includes connection pooling per penguin-dal standards
    # ---------------------------------------------------------------------------
    db = DB(
        DB_URI,
        pool_size=20,
        echo=False,
        reflect=False,  # Reflection will happen at runtime when tables exist
    )

__all__ = [
    "db",
    "DB_URI",
    "DB_TYPE",
    "DB_HOST",
    "DB_PORT",
    "DB_NAME",
    "DB_USER",
    "OperationRecord",
]
