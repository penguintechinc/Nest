"""SQLAlchemy model definitions for schema management (Alembic only).

IMPORTANT: These models are used EXCLUSIVELY for:
  - Alembic autogenerate (schema detection)
  - SQLAlchemy Base.metadata.create_all() at startup (idempotent)

They are NEVER used for runtime database operations. All runtime queries
use PyDAL with migrate=False.

Import order matters: existing tables first, then articdbm tables that
may reference them via ForeignKey.
"""

from sqlalchemy.orm import DeclarativeBase


class Base(DeclarativeBase):
    """Shared declarative base for all SQLAlchemy models."""
    pass


# Import existing table models (baseline schema)
from .existing import *  # noqa: F401, F403, E402

# Import new ArticDBM feature tables
from .articdbm import *  # noqa: F401, F403, E402

# Import operations table model
from .operations import *  # noqa: F401, F403, E402
