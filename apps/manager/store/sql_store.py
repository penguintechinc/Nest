"""Durable SQL-based operation store using penguin-dal AsyncDB."""

from __future__ import annotations

from datetime import datetime
from typing import Any

from penguin_dal import AsyncDB

from models.operations import OperationRecord
from store.store import OperationStore


class SQLOperationStore(OperationStore):
    """Durable operation store backed by SQL database via penguin-dal AsyncDB."""

    def __init__(self, db: AsyncDB) -> None:
        """Initialize the SQL store.

        Args:
            db: Initialized AsyncDB instance with reflected tables.
        """
        self._db = db

    @staticmethod
    def _parse_iso_datetime(iso_str: str) -> datetime:
        """Parse ISO format datetime string to datetime object.

        Handles both formats: with and without Z suffix.
        """
        # Remove 'Z' suffix if present
        iso_str_clean = iso_str.rstrip("Z")
        # Parse the ISO format
        return datetime.fromisoformat(iso_str_clean)

    async def create_operation(self, operation: OperationRecord) -> None:
        """Create a new operation. Raises ValueError if duplicate."""
        try:
            # Check if operation already exists
            key = f"{operation.tenant}:{operation.id}"
            exists = await self._db(
                (self._db.operations.tenant == operation.tenant)
                & (self._db.operations.id == operation.id)
            ).exists()
            if exists:
                raise ValueError(f"Operation {key} already exists")

            # Insert the operation
            await self._db.operations.async_insert(
                id=operation.id,
                tenant=operation.tenant,
                resource_name=operation.resource_name,
                resource_type=operation.resource_type,
                operation_type=operation.operation_type,
                phase=operation.phase,
                message=operation.message,
                error=operation.error,
                progress=operation.progress,
                created_at=self._parse_iso_datetime(operation.created_at),
                updated_at=self._parse_iso_datetime(operation.updated_at),
            )
        except ValueError:
            raise
        except Exception as e:
            raise ValueError(f"Failed to create operation: {e}") from e

    async def get_operation(self, tenant: str, operation_id: str) -> OperationRecord:
        """Get a single operation. Raises ValueError if not found."""
        try:
            rows = await self._db(
                (self._db.operations.tenant == tenant)
                & (self._db.operations.id == operation_id)
            ).select()
            if not rows:
                key = f"{tenant}:{operation_id}"
                raise ValueError(f"Operation {key} not found")
            row = rows.first()
            return self._row_to_record(row)
        except ValueError:
            raise
        except Exception as e:
            raise ValueError(f"Failed to get operation: {e}") from e

    async def list_by_tenant(self, tenant: str) -> list[OperationRecord]:
        """List all operations for a tenant."""
        try:
            rows = await self._db(self._db.operations.tenant == tenant).select()
            return [self._row_to_record(row) for row in rows]
        except Exception as e:
            raise ValueError(
                f"Failed to list operations for tenant {tenant}: {e}"
            ) from e

    async def update_operation(self, operation: OperationRecord) -> None:
        """Update an existing operation (full replace)."""
        try:
            # Check if operation exists
            exists = await self._db(
                (self._db.operations.tenant == operation.tenant)
                & (self._db.operations.id == operation.id)
            ).exists()
            if not exists:
                key = f"{operation.tenant}:{operation.id}"
                raise ValueError(f"Operation {key} not found")

            # Update all fields
            count = await self._db(
                (self._db.operations.tenant == operation.tenant)
                & (self._db.operations.id == operation.id)
            ).update(
                resource_name=operation.resource_name,
                resource_type=operation.resource_type,
                operation_type=operation.operation_type,
                phase=operation.phase,
                message=operation.message,
                error=operation.error,
                progress=operation.progress,
                updated_at=self._parse_iso_datetime(operation.updated_at),
            )
            if count != 1:
                raise ValueError(f"Update affected {count} rows, expected 1")
        except ValueError:
            raise
        except Exception as e:
            raise ValueError(f"Failed to update operation: {e}") from e

    async def list_pending_or_running(self) -> list[OperationRecord]:
        """List all operations in pending or running phase."""
        try:
            rows = await self._db(
                (self._db.operations.phase == "pending")
                | (self._db.operations.phase == "running")
            ).select()
            return [self._row_to_record(row) for row in rows]
        except Exception as e:
            raise ValueError(f"Failed to list pending/running operations: {e}") from e

    @staticmethod
    def _row_to_record(row: Any) -> OperationRecord:
        """Convert a database row to an OperationRecord."""
        return OperationRecord(
            id=row.id,
            tenant=row.tenant,
            resource_name=row.resource_name,
            resource_type=row.resource_type,
            operation_type=row.operation_type,
            phase=row.phase,
            message=row.message,
            error=row.error or "",
            progress=row.progress,
            created_at=(
                row.created_at.isoformat()
                if hasattr(row.created_at, "isoformat")
                else str(row.created_at)
            ),
            updated_at=(
                row.updated_at.isoformat()
                if hasattr(row.updated_at, "isoformat")
                else str(row.updated_at)
            ),
        )
