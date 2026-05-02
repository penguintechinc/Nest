"""Tests for MemoryStore."""
from datetime import datetime, timezone

import pytest

from models import DataResourceRecord
from store import MemoryStore


@pytest.mark.asyncio
async def test_create_and_get(store):
    """Test creating and retrieving a DataResource."""
    dr = DataResourceRecord(
        id="test-id",
        name="test-dr",
        tenant="test-tenant",
        resource_type="pvc/block",
        engine_type="block",
        storage_class="nest-block",
        driver_type="csi",
        origination="managed",
        phase="pending",
        created_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        updated_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    )

    await store.create_data_resource(dr)
    retrieved = await store.get_data_resource("test-tenant", "test-dr")

    assert retrieved.id == "test-id"
    assert retrieved.name == "test-dr"
    assert retrieved.resource_type == "pvc/block"


@pytest.mark.asyncio
async def test_create_duplicate_fails(store):
    """Test that creating duplicate DataResource fails."""
    dr = DataResourceRecord(
        id="test-id",
        name="test-dr",
        tenant="test-tenant",
        resource_type="pvc/block",
        engine_type="block",
        storage_class="nest-block",
        driver_type="csi",
        origination="managed",
        phase="pending",
        created_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        updated_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    )

    await store.create_data_resource(dr)

    with pytest.raises(ValueError, match="already exists"):
        await store.create_data_resource(dr)


@pytest.mark.asyncio
async def test_get_nonexistent_fails(store):
    """Test that getting nonexistent DataResource fails."""
    with pytest.raises(ValueError, match="not found"):
        await store.get_data_resource("test-tenant", "nonexistent")


@pytest.mark.asyncio
async def test_list_by_tenant(store):
    """Test listing DataResources by tenant."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    dr1 = DataResourceRecord(
        id="id1",
        name="dr1",
        tenant="tenant1",
        resource_type="pvc/block",
        engine_type="block",
        storage_class="nest-block",
        driver_type="csi",
        origination="managed",
        phase="pending",
        created_at=now,
        updated_at=now,
    )
    dr2 = DataResourceRecord(
        id="id2",
        name="dr2",
        tenant="tenant1",
        resource_type="pvc/file",
        engine_type="filesystem",
        storage_class="nest-file",
        driver_type="csi",
        origination="managed",
        phase="pending",
        created_at=now,
        updated_at=now,
    )
    dr3 = DataResourceRecord(
        id="id3",
        name="dr3",
        tenant="tenant2",
        resource_type="postgres",
        engine_type="postgres",
        storage_class="",
        driver_type="cnpg",
        origination="managed",
        phase="pending",
        created_at=now,
        updated_at=now,
    )

    await store.create_data_resource(dr1)
    await store.create_data_resource(dr2)
    await store.create_data_resource(dr3)

    tenant1_drs = await store.list_data_resources("tenant1")
    tenant2_drs = await store.list_data_resources("tenant2")

    assert len(tenant1_drs) == 2
    assert len(tenant2_drs) == 1
    assert tenant1_drs[0].tenant == "tenant1"
    assert tenant2_drs[0].tenant == "tenant2"


@pytest.mark.asyncio
async def test_count(store):
    """Test counting DataResources by tenant."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    dr1 = DataResourceRecord(
        id="id1",
        name="dr1",
        tenant="tenant1",
        resource_type="pvc/block",
        engine_type="block",
        storage_class="nest-block",
        driver_type="csi",
        origination="managed",
        phase="pending",
        created_at=now,
        updated_at=now,
    )
    dr2 = DataResourceRecord(
        id="id2",
        name="dr2",
        tenant="tenant1",
        resource_type="pvc/file",
        engine_type="filesystem",
        storage_class="nest-file",
        driver_type="csi",
        origination="managed",
        phase="pending",
        created_at=now,
        updated_at=now,
    )

    await store.create_data_resource(dr1)
    await store.create_data_resource(dr2)

    count = await store.count_data_resources("tenant1")
    assert count == 2

    count_empty = await store.count_data_resources("tenant2")
    assert count_empty == 0


@pytest.mark.asyncio
async def test_delete(store):
    """Test deleting a DataResource."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    dr = DataResourceRecord(
        id="test-id",
        name="test-dr",
        tenant="test-tenant",
        resource_type="pvc/block",
        engine_type="block",
        storage_class="nest-block",
        driver_type="csi",
        origination="managed",
        phase="pending",
        created_at=now,
        updated_at=now,
    )

    await store.create_data_resource(dr)
    await store.delete_data_resource("test-tenant", "test-dr")

    with pytest.raises(ValueError, match="not found"):
        await store.get_data_resource("test-tenant", "test-dr")


@pytest.mark.asyncio
async def test_delete_nonexistent_fails(store):
    """Test that deleting nonexistent DataResource fails."""
    with pytest.raises(ValueError, match="not found"):
        await store.delete_data_resource("test-tenant", "nonexistent")
