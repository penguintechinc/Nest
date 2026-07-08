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

# Validate required credentials
if not DB_HOST:
    raise RuntimeError("DB_HOST environment variable is required")
if not DB_NAME:
    raise RuntimeError("DB_NAME environment variable is required")
if not DB_USER:
    raise RuntimeError("DB_USER environment variable is required")
if not DB_PASSWORD:
    raise RuntimeError("DB_PASS (or DB_PASSWORD) environment variable is required")

DB_URI = f"{DB_TYPE}://{DB_USER}:{DB_PASSWORD}@{DB_HOST}:{DB_PORT}/{DB_NAME}"

# ---------------------------------------------------------------------------
# DB instance (synchronous; used in sync contexts and module-level init)
# For Quart async request handlers, use get_db() from penguin_dal.quart_ext
# Includes connection pooling per penguin-dal standards
# ---------------------------------------------------------------------------
db = DB(
    DB_URI,
    pool_size=20,
    max_overflow=10,
    pool_recycle=3600  # Recycle connections after 1 hour
)

__all__ = [
    "db",
    "DB_URI",
    "DB_TYPE",
    "DB_HOST",
    "DB_PORT",
    "DB_NAME",
    "DB_USER",
]
