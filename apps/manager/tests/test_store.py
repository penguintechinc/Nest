"""Tests for the MemoryOperationStore."""

from datetime import datetime, timezone

import pytest

from models import OperationRecord
from store.store import MemoryOperationStore


@pytest.fixture
def store():
    """Create a fresh store for each test."""
    return MemoryOperationStore()


@pytest.mark.asyncio
async def test_create_operation(store: MemoryOperationStore) -> None:
    """Test creating a new operation."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    op = OperationRecord(
        id="op-123",
        tenant="tenant-1",
        resource_name="my-pvc",
        resource_type="pvc",
        operation_type="create",
        phase="pending",
        message="Operation created",
        created_at=now,
        updated_at=now,
    )

    await store.create_operation(op)
    retrieved = await store.get_operation("tenant-1", "op-123")
    assert retrieved.id == "op-123"
    assert retrieved.phase == "pending"


@pytest.mark.asyncio
async def test_create_duplicate_operation(store: MemoryOperationStore) -> None:
    """Test that creating a duplicate operation raises ValueError."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    op = OperationRecord(
        id="op-123",
        tenant="tenant-1",
        resource_name="my-pvc",
        resource_type="pvc",
        operation_type="create",
        phase="pending",
        message="Operation created",
        created_at=now,
        updated_at=now,
    )

    await store.create_operation(op)

    with pytest.raises(ValueError, match="already exists"):
        await store.create_operation(op)


@pytest.mark.asyncio
async def test_get_nonexistent_operation(store: MemoryOperationStore) -> None:
    """Test that getting a nonexistent operation raises ValueError."""
    with pytest.raises(ValueError, match="not found"):
        await store.get_operation("tenant-1", "op-nonexistent")


@pytest.mark.asyncio
async def test_list_by_tenant(store: MemoryOperationStore) -> None:
    """Test listing operations by tenant."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    op1 = OperationRecord(
        id="op-1",
        tenant="tenant-1",
        resource_name="res-1",
        resource_type="pvc",
        operation_type="create",
        phase="pending",
        message="Op 1",
        created_at=now,
        updated_at=now,
    )

    op2 = OperationRecord(
        id="op-2",
        tenant="tenant-1",
        resource_name="res-2",
        resource_type="pvc",
        operation_type="delete",
        phase="running",
        message="Op 2",
        created_at=now,
        updated_at=now,
    )

    op3 = OperationRecord(
        id="op-3",
        tenant="tenant-2",
        resource_name="res-3",
        resource_type="pvc",
        operation_type="create",
        phase="pending",
        message="Op 3",
        created_at=now,
        updated_at=now,
    )

    await store.create_operation(op1)
    await store.create_operation(op2)
    await store.create_operation(op3)

    tenant1_ops = await store.list_by_tenant("tenant-1")
    assert len(tenant1_ops) == 2
    assert all(op.tenant == "tenant-1" for op in tenant1_ops)

    tenant2_ops = await store.list_by_tenant("tenant-2")
    assert len(tenant2_ops) == 1
    assert tenant2_ops[0].tenant == "tenant-2"


@pytest.mark.asyncio
async def test_update_operation(store: MemoryOperationStore) -> None:
    """Test updating an operation."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    op = OperationRecord(
        id="op-123",
        tenant="tenant-1",
        resource_name="my-pvc",
        resource_type="pvc",
        operation_type="create",
        phase="pending",
        message="Operation created",
        created_at=now,
        updated_at=now,
    )

    await store.create_operation(op)

    op.phase = "running"
    op.message = "Operation in progress"
    await store.update_operation(op)

    retrieved = await store.get_operation("tenant-1", "op-123")
    assert retrieved.phase == "running"
    assert retrieved.message == "Operation in progress"


@pytest.mark.asyncio
async def test_update_nonexistent_operation(
    store: MemoryOperationStore,
) -> None:
    """Test that updating a nonexistent operation raises ValueError."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    op = OperationRecord(
        id="op-nonexistent",
        tenant="tenant-1",
        resource_name="my-pvc",
        resource_type="pvc",
        operation_type="create",
        phase="pending",
        message="Operation created",
        created_at=now,
        updated_at=now,
    )

    with pytest.raises(ValueError, match="not found"):
        await store.update_operation(op)


@pytest.mark.asyncio
async def test_list_pending_or_running(store: MemoryOperationStore) -> None:
    """Test listing pending or running operations."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    pending_op = OperationRecord(
        id="op-pending",
        tenant="tenant-1",
        resource_name="res-1",
        resource_type="pvc",
        operation_type="create",
        phase="pending",
        message="Pending",
        created_at=now,
        updated_at=now,
    )

    running_op = OperationRecord(
        id="op-running",
        tenant="tenant-1",
        resource_name="res-2",
        resource_type="pvc",
        operation_type="delete",
        phase="running",
        message="Running",
        created_at=now,
        updated_at=now,
    )

    succeeded_op = OperationRecord(
        id="op-succeeded",
        tenant="tenant-1",
        resource_name="res-3",
        resource_type="pvc",
        operation_type="create",
        phase="succeeded",
        message="Succeeded",
        created_at=now,
        updated_at=now,
    )

    await store.create_operation(pending_op)
    await store.create_operation(running_op)
    await store.create_operation(succeeded_op)

    pending_or_running = await store.list_pending_or_running()
    assert len(pending_or_running) == 2
    phases = {op.phase for op in pending_or_running}
    assert phases == {"pending", "running"}
