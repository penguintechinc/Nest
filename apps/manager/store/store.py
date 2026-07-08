"""In-memory operation store with thread-safe operations."""

import asyncio
from typing import Any

from models.operations import OperationRecord


class OperationStore:
    """Abstract base class for operation storage."""

    async def create_operation(self, operation: OperationRecord) -> None:
        """Create a new operation. Raises ValueError if duplicate."""
        raise NotImplementedError

    async def get_operation(self, tenant: str, operation_id: str) -> OperationRecord:
        """Get a single operation. Raises ValueError if not found."""
        raise NotImplementedError

    async def list_by_tenant(self, tenant: str) -> list[OperationRecord]:
        """List all operations for a tenant."""
        raise NotImplementedError

    async def update_operation(self, operation: OperationRecord) -> None:
        """Update an existing operation (full replace)."""
        raise NotImplementedError

    async def list_pending_or_running(self) -> list[OperationRecord]:
        """List all operations in pending or running phase."""
        raise NotImplementedError


class MemoryOperationStore(OperationStore):
    """In-memory thread-safe operation store using asyncio.Lock."""

    def __init__(self: Any) -> None:
        """Initialize the in-memory store."""
        self._data: dict[str, OperationRecord] = {}
        self._lock = asyncio.Lock()

    async def create_operation(self, operation: OperationRecord) -> None:
        """Create a new operation. Raises ValueError if duplicate."""
        key = f"{operation.tenant}/{operation.id}"
        async with self._lock:
            if key in self._data:
                raise ValueError(f"Operation {key} already exists")
            self._data[key] = operation

    async def get_operation(self, tenant: str, operation_id: str) -> OperationRecord:
        """Get a single operation. Raises ValueError if not found."""
        key = f"{tenant}/{operation_id}"
        async with self._lock:
            if key not in self._data:
                raise ValueError(f"Operation {key} not found")
            return self._data[key]

    async def list_by_tenant(self, tenant: str) -> list[OperationRecord]:
        """List all operations for a tenant."""
        async with self._lock:
            return [op for op in self._data.values() if op.tenant == tenant]

    async def update_operation(self, operation: OperationRecord) -> None:
        """Update an existing operation (full replace)."""
        key = f"{operation.tenant}/{operation.id}"
        async with self._lock:
            if key not in self._data:
                raise ValueError(f"Operation {key} not found")
            self._data[key] = operation

    async def list_pending_or_running(self) -> list[OperationRecord]:
        """List all operations in pending or running phase."""
        async with self._lock:
            return [
                op for op in self._data.values() if op.phase in ("pending", "running")
            ]
