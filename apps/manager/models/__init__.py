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
# ---------------------------------------------------------------------------
DB_TYPE = os.getenv("DB_TYPE", "postgresql")
DB_HOST = os.getenv("DB_HOST", "localhost")
DB_PORT = os.getenv("DB_PORT", "5432")
DB_NAME = os.getenv("DB_NAME", "nest")
DB_USER = os.getenv("DB_USER", "nest")
DB_PASSWORD = os.getenv("DB_PASS", os.getenv("DB_PASSWORD", "nest"))

DB_URI = f"{DB_TYPE}://{DB_USER}:{DB_PASSWORD}@{DB_HOST}:{DB_PORT}/{DB_NAME}"

# ---------------------------------------------------------------------------
# DB instance (synchronous; used in sync contexts and module-level init)
# For Quart async request handlers, use get_db() from penguin_dal.quart_ext
# ---------------------------------------------------------------------------
db = DB(DB_URI)

__all__ = [
    "db",
    "DB_URI",
    "DB_TYPE",
    "DB_HOST",
    "DB_PORT",
    "DB_NAME",
    "DB_USER",
]
