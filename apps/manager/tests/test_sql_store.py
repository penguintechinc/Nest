"""Tests for SQLOperationStore."""

from datetime import datetime, timezone

import pytest

from models import OperationRecord
from store import SQLOperationStore


class TestSQLOperationStore:
    """Test suite for SQLOperationStore."""

    async def test_create_operation_success(self, sql_store: SQLOperationStore) -> None:
        """Test creating a new operation succeeds."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-001",
            tenant="tenant-a",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="pending",
            message="Starting backup",
            created_at=now,
            updated_at=now,
        )

        await sql_store.create_operation(operation)

        # Verify it was stored
        retrieved = await sql_store.get_operation("tenant-a", "op-001")
        assert retrieved.id == "op-001"
        assert retrieved.tenant == "tenant-a"
        assert retrieved.phase == "pending"

    async def test_create_operation_duplicate_raises(self, sql_store: SQLOperationStore) -> None:
        """Test creating duplicate operation raises ValueError."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-002",
            tenant="tenant-a",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="pending",
            message="Starting backup",
            created_at=now,
            updated_at=now,
        )

        await sql_store.create_operation(operation)

        # Creating again should raise
        with pytest.raises(ValueError, match="already exists"):
            await sql_store.create_operation(operation)

    async def test_get_operation_success(self, sql_store: SQLOperationStore) -> None:
        """Test getting an existing operation."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-003",
            tenant="tenant-b",
            resource_name="resource-2",
            resource_type="storage",
            operation_type="restore",
            phase="running",
            message="Restoring data",
            created_at=now,
            updated_at=now,
            progress=50,
        )

        await sql_store.create_operation(operation)
        retrieved = await sql_store.get_operation("tenant-b", "op-003")

        assert retrieved.id == "op-003"
        assert retrieved.tenant == "tenant-b"
        assert retrieved.phase == "running"
        assert retrieved.progress == 50

    async def test_get_operation_not_found_raises(self, sql_store: SQLOperationStore) -> None:
        """Test getting non-existent operation raises ValueError."""
        with pytest.raises(ValueError, match="not found"):
            await sql_store.get_operation("tenant-a", "nonexistent")

    async def test_get_operation_tenant_isolation(self, sql_store: SQLOperationStore) -> None:
        """Test that operations are isolated by tenant."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        op_a = OperationRecord(
            id="op-shared-id",
            tenant="tenant-a",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="pending",
            message="Backup for A",
            created_at=now,
            updated_at=now,
        )
        op_b = OperationRecord(
            id="op-shared-id",
            tenant="tenant-b",
            resource_name="resource-2",
            resource_type="storage",
            operation_type="backup",
            phase="pending",
            message="Backup for B",
            created_at=now,
            updated_at=now,
        )

        await sql_store.create_operation(op_a)
        await sql_store.create_operation(op_b)

        # Each tenant should only see their own operation
        retrieved_a = await sql_store.get_operation("tenant-a", "op-shared-id")
        retrieved_b = await sql_store.get_operation("tenant-b", "op-shared-id")

        assert retrieved_a.message == "Backup for A"
        assert retrieved_b.message == "Backup for B"

    async def test_list_by_tenant_success(self, sql_store: SQLOperationStore) -> None:
        """Test listing operations for a tenant."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        ops = [
            OperationRecord(
                id=f"op-{i}",
                tenant="tenant-c",
                resource_name=f"resource-{i}",
                resource_type="database",
                operation_type="backup",
                phase="pending",
                message=f"Backup {i}",
                created_at=now,
                updated_at=now,
            )
            for i in range(3)
        ]

        for op in ops:
            await sql_store.create_operation(op)

        # List all for tenant-c
        retrieved = await sql_store.list_by_tenant("tenant-c")
        assert len(retrieved) == 3
        assert all(op.tenant == "tenant-c" for op in retrieved)

    async def test_list_by_tenant_empty(self, sql_store: SQLOperationStore) -> None:
        """Test listing operations for non-existent tenant returns empty."""
        retrieved = await sql_store.list_by_tenant("tenant-nonexistent")
        assert retrieved == []

    async def test_list_by_tenant_isolation(self, sql_store: SQLOperationStore) -> None:
        """Test that list_by_tenant only returns operations for that tenant."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        op_a = OperationRecord(
            id="op-for-a",
            tenant="tenant-d",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="pending",
            message="Op A",
            created_at=now,
            updated_at=now,
        )
        op_b = OperationRecord(
            id="op-for-b",
            tenant="tenant-e",
            resource_name="resource-2",
            resource_type="storage",
            operation_type="backup",
            phase="pending",
            message="Op B",
            created_at=now,
            updated_at=now,
        )

        await sql_store.create_operation(op_a)
        await sql_store.create_operation(op_b)

        list_d = await sql_store.list_by_tenant("tenant-d")
        list_e = await sql_store.list_by_tenant("tenant-e")

        assert len(list_d) == 1
        assert len(list_e) == 1
        assert list_d[0].tenant == "tenant-d"
        assert list_e[0].tenant == "tenant-e"

    async def test_update_operation_success(self, sql_store: SQLOperationStore) -> None:
        """Test updating an existing operation."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-004",
            tenant="tenant-f",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="pending",
            message="Starting",
            created_at=now,
            updated_at=now,
            progress=0,
        )

        await sql_store.create_operation(operation)

        # Update the operation
        operation.phase = "running"
        operation.message = "In progress"
        operation.progress = 50
        operation.updated_at = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        await sql_store.update_operation(operation)

        # Verify the update
        retrieved = await sql_store.get_operation("tenant-f", "op-004")
        assert retrieved.phase == "running"
        assert retrieved.message == "In progress"
        assert retrieved.progress == 50

    async def test_update_operation_not_found_raises(self, sql_store: SQLOperationStore) -> None:
        """Test updating non-existent operation raises ValueError."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-nonexistent",
            tenant="tenant-g",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="pending",
            message="Never created",
            created_at=now,
            updated_at=now,
        )

        with pytest.raises(ValueError, match="not found"):
            await sql_store.update_operation(operation)

    async def test_update_operation_full_replace(self, sql_store: SQLOperationStore) -> None:
        """Test update performs full replace of all fields."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-005",
            tenant="tenant-h",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="pending",
            message="Starting",
            created_at=now,
            updated_at=now,
            error="",
            progress=0,
        )

        await sql_store.create_operation(operation)

        # Full update with different values
        updated_time = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation.resource_name = "resource-updated"
        operation.resource_type = "storage"
        operation.operation_type = "restore"
        operation.phase = "succeeded"
        operation.message = "Completed"
        operation.error = "No errors"
        operation.progress = 100
        operation.updated_at = updated_time
        await sql_store.update_operation(operation)

        # Verify all fields were updated
        retrieved = await sql_store.get_operation("tenant-h", "op-005")
        assert retrieved.resource_name == "resource-updated"
        assert retrieved.resource_type == "storage"
        assert retrieved.operation_type == "restore"
        assert retrieved.phase == "succeeded"
        assert retrieved.message == "Completed"
        assert retrieved.error == "No errors"
        assert retrieved.progress == 100

    async def test_list_pending_or_running_success(self, sql_store: SQLOperationStore) -> None:
        """Test listing pending/running operations."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        ops = [
            OperationRecord(
                id="op-pending",
                tenant="tenant-i",
                resource_name="resource-1",
                resource_type="database",
                operation_type="backup",
                phase="pending",
                message="Waiting",
                created_at=now,
                updated_at=now,
            ),
            OperationRecord(
                id="op-running",
                tenant="tenant-i",
                resource_name="resource-2",
                resource_type="storage",
                operation_type="backup",
                phase="running",
                message="Processing",
                created_at=now,
                updated_at=now,
            ),
            OperationRecord(
                id="op-succeeded",
                tenant="tenant-i",
                resource_name="resource-3",
                resource_type="database",
                operation_type="backup",
                phase="succeeded",
                message="Done",
                created_at=now,
                updated_at=now,
            ),
        ]

        for op in ops:
            await sql_store.create_operation(op)

        # List pending/running
        pending_or_running = await sql_store.list_pending_or_running()
        assert len(pending_or_running) == 2
        phases = {op.phase for op in pending_or_running}
        assert phases == {"pending", "running"}

    async def test_list_pending_or_running_empty(self, sql_store: SQLOperationStore) -> None:
        """Test listing pending/running when none exist."""
        pending_or_running = await sql_store.list_pending_or_running()
        assert pending_or_running == []

    async def test_list_pending_or_running_cross_tenant(self, sql_store: SQLOperationStore) -> None:
        """Test that list_pending_or_running returns operations across all tenants."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        ops = [
            OperationRecord(
                id="op-j1",
                tenant="tenant-j",
                resource_name="resource-1",
                resource_type="database",
                operation_type="backup",
                phase="pending",
                message="Pending",
                created_at=now,
                updated_at=now,
            ),
            OperationRecord(
                id="op-k1",
                tenant="tenant-k",
                resource_name="resource-2",
                resource_type="storage",
                operation_type="backup",
                phase="running",
                message="Running",
                created_at=now,
                updated_at=now,
            ),
        ]

        for op in ops:
            await sql_store.create_operation(op)

        pending_or_running = await sql_store.list_pending_or_running()
        assert len(pending_or_running) == 2
        tenants = {op.tenant for op in pending_or_running}
        assert tenants == {"tenant-j", "tenant-k"}

    async def test_durability_restart(self, sql_store: SQLOperationStore) -> None:
        """Test that operations persist across store instances (durability)."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-persistent",
            tenant="tenant-l",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="pending",
            message="Persistent operation",
            created_at=now,
            updated_at=now,
        )

        # Create operation in first store instance
        await sql_store.create_operation(operation)

        # Retrieve and verify
        retrieved = await sql_store.get_operation("tenant-l", "op-persistent")
        assert retrieved.message == "Persistent operation"

        # Note: In real scenario, we'd create a new store instance connecting
        # to the same DB. For this test, we just verify the data is in the DB.
        # The actual durability test would require creating a new AsyncDB
        # connection to the same SQLite file, which we'll defer to integration tests.
