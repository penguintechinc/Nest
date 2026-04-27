"""Tests for the background worker."""

import asyncio
from datetime import datetime, timedelta, timezone

import pytest

from models import OperationRecord
from store.store import MemoryOperationStore
from worker import run


@pytest.fixture
def store():
    """Create a fresh store for each test."""
    return MemoryOperationStore()


@pytest.mark.asyncio
async def test_worker_pending_to_running(store: MemoryOperationStore) -> None:
    """Test that worker transitions pending operations to running."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    op = OperationRecord(
        id="op-1",
        tenant="tenant-1",
        resource_name="res-1",
        resource_type="pvc",
        operation_type="create",
        phase="pending",
        message="Operation created",
        created_at=now,
        updated_at=now,
    )

    await store.create_operation(op)

    # Start the worker
    worker_task = asyncio.create_task(run(store))

    # Let it run for a bit
    await asyncio.sleep(3)

    # Check that the operation transitioned to running
    updated_op = await store.get_operation("tenant-1", "op-1")
    assert updated_op.phase == "running"

    worker_task.cancel()
    try:
        await worker_task
    except asyncio.CancelledError:
        pass


@pytest.mark.asyncio
async def test_worker_running_to_succeeded(store: MemoryOperationStore) -> None:
    """Test that worker transitions running operations to succeeded after 3s."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    op = OperationRecord(
        id="op-1",
        tenant="tenant-1",
        resource_name="res-1",
        resource_type="pvc",
        operation_type="create",
        phase="running",
        message="Operation in progress",
        created_at=now,
        updated_at=now,
    )

    await store.create_operation(op)

    # Start the worker
    worker_task = asyncio.create_task(run(store))

    # Let it run long enough to transition to succeeded (>3s)
    await asyncio.sleep(5)

    # Check that the operation transitioned to succeeded
    updated_op = await store.get_operation("tenant-1", "op-1")
    assert updated_op.phase == "succeeded"
    assert updated_op.progress == 100

    worker_task.cancel()
    try:
        await worker_task
    except asyncio.CancelledError:
        pass


@pytest.mark.asyncio
async def test_worker_full_lifecycle(store: MemoryOperationStore) -> None:
    """Test the full lifecycle: pending → running → succeeded."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    op = OperationRecord(
        id="op-1",
        tenant="tenant-1",
        resource_name="res-1",
        resource_type="pvc",
        operation_type="create",
        phase="pending",
        message="Operation created",
        created_at=now,
        updated_at=now,
    )

    await store.create_operation(op)

    # Start the worker
    worker_task = asyncio.create_task(run(store))

    # Let it run through the full lifecycle
    await asyncio.sleep(9)

    # Check that the operation reached succeeded
    updated_op = await store.get_operation("tenant-1", "op-1")
    assert updated_op.phase == "succeeded"
    assert updated_op.progress == 100

    worker_task.cancel()
    try:
        await worker_task
    except asyncio.CancelledError:
        pass


@pytest.mark.asyncio
async def test_worker_handles_cancellation(store: MemoryOperationStore) -> None:
    """Test that worker handles CancelledError gracefully."""
    worker_task = asyncio.create_task(run(store))

    # Let it start
    await asyncio.sleep(0.1)

    # Cancel it
    worker_task.cancel()

    # Should not raise
    try:
        await worker_task
    except asyncio.CancelledError:
        pass

    # Task should be done
    assert worker_task.done()
