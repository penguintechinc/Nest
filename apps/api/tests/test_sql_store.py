"""Tests for SQLStore with real SQLite database."""
import tempfile
from datetime import datetime, timezone
from pathlib import Path

import pytest
from penguin_dal.db import AsyncDB

from db_models import Base
from models import (DataProtectionPolicyRecord, DataResourceRecord,
                    OperationRecord, SearchPoolRecord, VolumeSnapshotRecord)
from store.sql_store import SQLStore


@pytest.fixture
async def temp_db() -> AsyncDB:
    """Create a temporary SQLite database with schema."""
    with tempfile.TemporaryDirectory() as tmpdir:
        db_file = Path(tmpdir) / "test.db"
        uri = f"sqlite+aiosqlite:///{db_file}"

        db = AsyncDB(uri, pool_size=5, echo=False)

        # Create tables using SQLAlchemy metadata
        async with db.engine.begin() as conn:
            await conn.run_sync(Base.metadata.create_all)

        # Reflect tables
        await db.reflect()

        yield db

        # Cleanup
        await db.close()


@pytest.fixture
async def sql_store(temp_db: AsyncDB) -> SQLStore:
    """Create a SQLStore instance for testing."""
    return SQLStore(temp_db)


class TestSQLStoreCRUD:
    """Test basic CRUD operations on SQLStore."""

    @pytest.mark.asyncio
    async def test_create_and_get_data_resource(self, sql_store: SQLStore):
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
            created_at=datetime.now(timezone.utc)
            .isoformat()
            .replace("+00:00", "Z"),
            updated_at=datetime.now(timezone.utc)
            .isoformat()
            .replace("+00:00", "Z"),
        )

        await sql_store.create_data_resource(dr)
        retrieved = await sql_store.get_data_resource("test-tenant", "test-dr")

        assert retrieved.id == "test-id"
        assert retrieved.name == "test-dr"
        assert retrieved.resource_type == "pvc/block"

    @pytest.mark.asyncio
    async def test_create_duplicate_fails(self, sql_store: SQLStore):
        """Test that creating duplicate DataResource fails."""
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

        await sql_store.create_data_resource(dr)

        with pytest.raises(ValueError, match="already exists"):
            await sql_store.create_data_resource(dr)

    @pytest.mark.asyncio
    async def test_get_nonexistent_fails(self, sql_store: SQLStore):
        """Test that getting nonexistent DataResource fails."""
        with pytest.raises(ValueError, match="not found"):
            await sql_store.get_data_resource("test-tenant", "nonexistent")

    @pytest.mark.asyncio
    async def test_delete_data_resource(self, sql_store: SQLStore):
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

        await sql_store.create_data_resource(dr)
        await sql_store.delete_data_resource("test-tenant", "test-dr")

        with pytest.raises(ValueError, match="not found"):
            await sql_store.get_data_resource("test-tenant", "test-dr")

    @pytest.mark.asyncio
    async def test_delete_nonexistent_fails(self, sql_store: SQLStore):
        """Test that deleting nonexistent DataResource fails."""
        with pytest.raises(ValueError, match="not found"):
            await sql_store.delete_data_resource("test-tenant", "nonexistent")


class TestSQLStoreTenantIsolation:
    """Test tenant isolation in SQLStore."""

    @pytest.mark.asyncio
    async def test_list_by_tenant(self, sql_store: SQLStore):
        """Test listing DataResources by tenant (isolation)."""
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

        await sql_store.create_data_resource(dr1)
        await sql_store.create_data_resource(dr2)
        await sql_store.create_data_resource(dr3)

        tenant1_drs = await sql_store.list_data_resources("tenant1")
        tenant2_drs = await sql_store.list_data_resources("tenant2")

        assert len(tenant1_drs) == 2
        assert len(tenant2_drs) == 1
        assert tenant1_drs[0].tenant == "tenant1"
        assert tenant2_drs[0].tenant == "tenant2"

    @pytest.mark.asyncio
    async def test_tenant_isolation_operations(self, sql_store: SQLStore):
        """Test that operations are isolated by tenant."""
        op1 = OperationRecord(
            id="op1",
            tenant="tenant1",
            op_type="snapshot",
            resource="dr1",
            phase="Running",
            started_at="2025-01-01T00:00:00Z",
        )
        op2 = OperationRecord(
            id="op2",
            tenant="tenant2",
            op_type="snapshot",
            resource="dr2",
            phase="Running",
            started_at="2025-01-01T00:00:00Z",
        )

        await sql_store.create_operation(op1)
        await sql_store.create_operation(op2)

        tenant1_ops = await sql_store.list_operations("tenant1")
        tenant2_ops = await sql_store.list_operations("tenant2")

        assert len(tenant1_ops) == 1
        assert len(tenant2_ops) == 1
        assert tenant1_ops[0].tenant == "tenant1"
        assert tenant2_ops[0].tenant == "tenant2"

    @pytest.mark.asyncio
    async def test_count_by_tenant(self, sql_store: SQLStore):
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

        await sql_store.create_data_resource(dr1)
        await sql_store.create_data_resource(dr2)

        count = await sql_store.count_data_resources("tenant1")
        assert count == 2

        count_empty = await sql_store.count_data_resources("tenant2")
        assert count_empty == 0


class TestSQLStoreUpdate:
    """Test update operations in SQLStore."""

    @pytest.mark.asyncio
    async def test_update_data_resource(self, sql_store: SQLStore):
        """Test updating a DataResource."""
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

        await sql_store.create_data_resource(dr)

        # Update the phase
        dr.phase = "ready"
        await sql_store.update_data_resource(dr)

        retrieved = await sql_store.get_data_resource("test-tenant", "test-dr")
        assert retrieved.phase == "ready"

    @pytest.mark.asyncio
    async def test_update_nonexistent_fails(self, sql_store: SQLStore):
        """Test that updating nonexistent DataResource fails."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        dr = DataResourceRecord(
            id="test-id",
            name="nonexistent",
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

        with pytest.raises(ValueError, match="not found"):
            await sql_store.update_data_resource(dr)

    @pytest.mark.asyncio
    async def test_update_health(self, sql_store: SQLStore):
        """Test updating health state of a DataResource."""
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

        await sql_store.create_data_resource(dr)
        await sql_store.update_data_resource_health(
            "test-tenant", "test-dr", "healthy"
        )

        retrieved = await sql_store.get_data_resource("test-tenant", "test-dr")
        assert retrieved.health_state == "healthy"


class TestSQLStoreOperations:
    """Test operation CRUD in SQLStore."""

    @pytest.mark.asyncio
    async def test_create_and_get_operation(self, sql_store: SQLStore):
        """Test creating and retrieving an operation."""
        op = OperationRecord(
            id="op-1",
            tenant="test-tenant",
            op_type="snapshot",
            resource="dr-1",
            phase="Running",
            started_at="2025-01-01T00:00:00Z",
        )

        await sql_store.create_operation(op)
        retrieved = await sql_store.get_operation("test-tenant", "op-1")

        assert retrieved.id == "op-1"
        assert retrieved.op_type == "snapshot"
        assert retrieved.phase == "Running"

    @pytest.mark.asyncio
    async def test_update_operation(self, sql_store: SQLStore):
        """Test updating an operation."""
        op = OperationRecord(
            id="op-1",
            tenant="test-tenant",
            op_type="snapshot",
            resource="dr-1",
            phase="Running",
            started_at="2025-01-01T00:00:00Z",
        )

        await sql_store.create_operation(op)

        # Update the phase
        op.phase = "Succeeded"
        op.completed_at = "2025-01-01T01:00:00Z"
        op.result = {"snapshot_id": "snap-1"}
        await sql_store.update_operation(op)

        retrieved = await sql_store.get_operation("test-tenant", "op-1")
        assert retrieved.phase == "Succeeded"
        assert retrieved.result == {"snapshot_id": "snap-1"}


class TestSQLStoreSnapshots:
    """Test VolumeSnapshot CRUD in SQLStore."""

    @pytest.mark.asyncio
    async def test_create_and_list_snapshots(self, sql_store: SQLStore):
        """Test creating and listing snapshots."""
        snap = await sql_store.create_snapshot(
            "test-tenant",
            "snap-1",
            "pvc-1",
            "csi-snapshot-class",
        )

        assert snap.name == "snap-1"
        assert snap.tenant == "test-tenant"

        snapshots = await sql_store.list_snapshots("test-tenant")
        assert len(snapshots) == 1
        assert snapshots[0].name == "snap-1"

    @pytest.mark.asyncio
    async def test_delete_snapshot(self, sql_store: SQLStore):
        """Test deleting a snapshot."""
        await sql_store.create_snapshot(
            "test-tenant",
            "snap-1",
            "pvc-1",
            "csi-snapshot-class",
        )

        await sql_store.delete_snapshot("test-tenant", "snap-1")

        snapshots = await sql_store.list_snapshots("test-tenant")
        assert len(snapshots) == 0


class TestSQLStorePolicies:
    """Test DataProtectionPolicy CRUD in SQLStore."""

    @pytest.mark.asyncio
    async def test_create_and_list_policies(self, sql_store: SQLStore):
        """Test creating and listing policies."""
        policy = await sql_store.create_protection_policy(
            "test-tenant",
            "policy-1",
            "0 2 * * *",
            "0 3 * * 0",
            "s3://backup-bucket",
        )

        assert policy.name == "policy-1"
        assert policy.snapshot_schedule == "0 2 * * *"

        policies = await sql_store.list_protection_policies("test-tenant")
        assert len(policies) == 1
        assert policies[0].name == "policy-1"

    @pytest.mark.asyncio
    async def test_delete_policy(self, sql_store: SQLStore):
        """Test deleting a policy."""
        await sql_store.create_protection_policy(
            "test-tenant",
            "policy-1",
            "0 2 * * *",
            "0 3 * * 0",
            "s3://backup-bucket",
        )

        await sql_store.delete_protection_policy("test-tenant", "policy-1")

        policies = await sql_store.list_protection_policies("test-tenant")
        assert len(policies) == 0


class TestSQLStoreSearchPools:
    """Test SearchPool CRUD in SQLStore."""

    @pytest.mark.asyncio
    async def test_create_and_get_search_pool(self, sql_store: SQLStore):
        """Test creating and getting a search pool."""
        pool = await sql_store.create_search_pool("pool-1", replicas=3)

        assert pool.name == "pool-1"
        assert pool.replicas == 3

        retrieved = await sql_store.get_search_pool("pool-1")
        assert retrieved.name == "pool-1"
        assert retrieved.replicas == 3

    @pytest.mark.asyncio
    async def test_list_search_pools(self, sql_store: SQLStore):
        """Test listing search pools."""
        await sql_store.create_search_pool("pool-1", replicas=3)
        await sql_store.create_search_pool("pool-2", replicas=1)

        pools = await sql_store.list_search_pools()
        assert len(pools) == 2

    @pytest.mark.asyncio
    async def test_delete_search_pool(self, sql_store: SQLStore):
        """Test deleting a search pool."""
        await sql_store.create_search_pool("pool-1", replicas=3)
        await sql_store.delete_search_pool("pool-1")

        pools = await sql_store.list_search_pools()
        assert len(pools) == 0


class TestSQLStoreDurability:
    """Test that data persists across store reconnections."""

    @pytest.mark.asyncio
    async def test_data_persists_after_reconnect(self):
        """Test that data survives a close/reopen cycle."""
        # Create initial db and store
        with tempfile.TemporaryDirectory() as tmpdir:
            db_file = Path(tmpdir) / "test.db"
            uri = f"sqlite+aiosqlite:///{db_file}"

            # First connection: create and insert
            db1 = AsyncDB(uri, pool_size=5, echo=False)
            async with db1.engine.begin() as conn:
                await conn.run_sync(Base.metadata.create_all)
            await db1.reflect()

            store1 = SQLStore(db1)
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
            await store1.create_data_resource(dr)
            await db1.close()

            # Second connection: verify data persists
            db2 = AsyncDB(uri, pool_size=5, echo=False)
            await db2.reflect()
            store2 = SQLStore(db2)

            retrieved = await store2.get_data_resource("test-tenant", "test-dr")
            assert retrieved.id == "test-id"
            assert retrieved.name == "test-dr"

            await db2.close()
