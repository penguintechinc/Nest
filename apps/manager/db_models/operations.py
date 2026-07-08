"""SQLAlchemy model for the operations table.

This model defines the schema for long-running operations that need persistence.
Used by SQLOperationStore for durable operation state.

IMPORTANT: Used ONLY for Alembic autogenerate and create_all().
Runtime queries use penguin-dal AsyncDB.
"""

from datetime import datetime

from sqlalchemy import (
    Column,
    String,
    Integer,
    Text,
    DateTime,
)

from . import Base


class Operation(Base):
    """Operations table for storing long-running operation state."""

    __tablename__ = "operations"

    tenant = Column(String(255), primary_key=True, nullable=False)
    id = Column(String(36), primary_key=True)
    resource_name = Column(String(255), nullable=False)
    resource_type = Column(String(100), nullable=False)
    operation_type = Column(String(100), nullable=False)
    phase = Column(String(50), nullable=False, index=True)
    message = Column(Text, nullable=False)
    error = Column(Text, nullable=True)
    progress = Column(Integer, nullable=False, default=0)
    created_at = Column(DateTime, nullable=False, default=datetime.utcnow)
    updated_at = Column(DateTime, nullable=False, default=datetime.utcnow, onupdate=datetime.utcnow)
