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
            created_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
            updated_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
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
        await sql_store.update_data_resource_health("test-tenant", "test-dr", "healthy")

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


class TestSQLStoreFactoryMethod:
    """Test SQLStore.create_from_env factory method."""

    def test_create_from_env_sqlite(self, monkeypatch):
        """Test creating SQLStore from env with SQLite."""
        monkeypatch.setenv("DB_TYPE", "sqlite")
        monkeypatch.setenv("DB_FILE", ":memory:")
        store = SQLStore.create_from_env()
        assert store is not None
        assert isinstance(store, SQLStore)

    def test_create_from_env_invalid_db_type(self, monkeypatch):
        """Test that invalid DB_TYPE raises ValueError."""
        monkeypatch.setenv("DB_TYPE", "invalid_db")
        with pytest.raises(ValueError, match="Unsupported DB_TYPE"):
            SQLStore.create_from_env()

    def test_create_from_env_with_pool_size(self, monkeypatch):
        """Test that pool size env var is used."""
        monkeypatch.setenv("DB_TYPE", "sqlite")
        monkeypatch.setenv("DB_FILE", ":memory:")
        monkeypatch.setenv("DB_POOL_SIZE", "20")
        store = SQLStore.create_from_env()
        assert store is not None


class TestSQLStoreEmptyLists:
    """Test empty list returns."""

    @pytest.mark.asyncio
    async def test_list_data_resources_empty_tenant(self, sql_store: SQLStore):
        """Test listing DataResources for tenant with no resources."""
        resources = await sql_store.list_data_resources("nonexistent-tenant")
        assert resources == []

    @pytest.mark.asyncio
    async def test_list_operations_empty_tenant(self, sql_store: SQLStore):
        """Test listing operations for tenant with no operations."""
        operations = await sql_store.list_operations("nonexistent-tenant")
        assert operations == []

    @pytest.mark.asyncio
    async def test_list_snapshots_empty_tenant(self, sql_store: SQLStore):
        """Test listing snapshots for tenant with no snapshots."""
        snapshots = await sql_store.list_snapshots("nonexistent-tenant")
        assert snapshots == []

    @pytest.mark.asyncio
    async def test_list_protection_policies_empty_tenant(self, sql_store: SQLStore):
        """Test listing policies for tenant with no policies."""
        policies = await sql_store.list_protection_policies("nonexistent-tenant")
        assert policies == []

    @pytest.mark.asyncio
    async def test_list_search_pools_empty(self, sql_store: SQLStore):
        """Test listing search pools when none exist."""
        pools = await sql_store.list_search_pools()
        assert pools == []

    @pytest.mark.asyncio
    async def test_count_data_resources_empty_tenant(self, sql_store: SQLStore):
        """Test counting DataResources for empty tenant."""
        count = await sql_store.count_data_resources("nonexistent-tenant")
        assert count == 0


class TestSQLStoreUpdateDelete404:
    """Test error returns for update/delete on nonexistent items."""

    @pytest.mark.asyncio
    async def test_update_operation_nonexistent(self, sql_store: SQLStore):
        """Test updating nonexistent operation fails."""
        op = OperationRecord(
            id="nonexistent",
            tenant="test-tenant",
            op_type="snapshot",
            resource="dr-1",
            phase="Running",
            started_at="2025-01-01T00:00:00Z",
        )
        with pytest.raises(ValueError, match="not found"):
            await sql_store.update_operation(op)

    @pytest.mark.asyncio
    async def test_delete_operation_nonexistent(self, sql_store: SQLStore):
        """Test deleting nonexistent operation returns empty (via get)."""
        with pytest.raises(ValueError, match="not found"):
            await sql_store.get_operation("test-tenant", "nonexistent")

    @pytest.mark.asyncio
    async def test_update_health_nonexistent(self, sql_store: SQLStore):
        """Test updating health of nonexistent resource fails."""
        with pytest.raises(ValueError, match="not found"):
            await sql_store.update_data_resource_health("test-tenant", "nonexistent", "degraded")

    @pytest.mark.asyncio
    async def test_delete_snapshot_nonexistent(self, sql_store: SQLStore):
        """Test deleting nonexistent snapshot fails."""
        with pytest.raises(ValueError, match="not found"):
            await sql_store.delete_snapshot("test-tenant", "nonexistent")

    @pytest.mark.asyncio
    async def test_delete_protection_policy_nonexistent(self, sql_store: SQLStore):
        """Test deleting nonexistent policy fails."""
        with pytest.raises(ValueError, match="not found"):
            await sql_store.delete_protection_policy("test-tenant", "nonexistent")

    @pytest.mark.asyncio
    async def test_get_search_pool_nonexistent(self, sql_store: SQLStore):
        """Test getting nonexistent search pool fails."""
        with pytest.raises(ValueError, match="not found"):
            await sql_store.get_search_pool("nonexistent")

    @pytest.mark.asyncio
    async def test_delete_search_pool_nonexistent(self, sql_store: SQLStore):
        """Test deleting nonexistent search pool fails."""
        with pytest.raises(ValueError, match="not found"):
            await sql_store.delete_search_pool("nonexistent")


class TestSQLStoreEnsureReflected:
    """Test _ensure_reflected idempotence."""

    @pytest.mark.asyncio
    async def test_ensure_reflected_idempotent(self, sql_store: SQLStore):
        """Test that _ensure_reflected can be called multiple times."""
        # First call reflects
        await sql_store._ensure_reflected()
        # Second call should be no-op
        await sql_store._ensure_reflected()
        # Third call still safe
        await sql_store._ensure_reflected()
        # Should still be able to use the store
        count = await sql_store.count_data_resources("test-tenant")
        assert count == 0


class TestSQLStoreMultipleEntityInteraction:
    """Test interactions between multiple entity types."""

    @pytest.mark.asyncio
    async def test_create_all_entities(self, sql_store: SQLStore):
        """Test creating one of each entity type."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

        # DataResource
        dr = DataResourceRecord(
            id="id-1",
            name="dr-1",
            tenant="tenant1",
            resource_type="pvc/block",
            engine_type="block",
            storage_class="nest-block",
            driver_type="csi",
            origination="managed",
            phase="ready",
            created_at=now,
            updated_at=now,
        )
        await sql_store.create_data_resource(dr)

        # Operation
        op = OperationRecord(
            id="op-1",
            tenant="tenant1",
            op_type="snapshot",
            resource="dr-1",
            phase="Succeeded",
            started_at=now,
            completed_at=now,
        )
        await sql_store.create_operation(op)

        # VolumeSnapshot
        snap = await sql_store.create_snapshot("tenant1", "snap-1", "pvc-1", "csi-class")

        # DataProtectionPolicy
        policy = await sql_store.create_protection_policy(
            "tenant1", "policy-1", "0 2 * * *", "0 3 * * 0", "s3://bucket"
        )

        # SearchPool (not tenant-specific)
        pool = await sql_store.create_search_pool("pool-1", replicas=3)

        # Verify all created
        drs = await sql_store.list_data_resources("tenant1")
        ops = await sql_store.list_operations("tenant1")
        snaps = await sql_store.list_snapshots("tenant1")
        policies = await sql_store.list_protection_policies("tenant1")
        pools = await sql_store.list_search_pools()

        assert len(drs) == 1
        assert len(ops) == 1
        assert len(snaps) == 1
        assert len(policies) == 1
        assert len(pools) == 1


class TestSQLStoreRowConversion:
    """Test row conversion methods handle edge cases."""

    @pytest.mark.asyncio
    async def test_data_resource_with_all_fields(self, sql_store: SQLStore):
        """Test creating and retrieving DataResource with all optional fields."""
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        dr = DataResourceRecord(
            id="full-id",
            name="full-dr",
            tenant="tenant1",
            resource_type="postgres",
            engine_type="postgres",
            storage_class="premium",
            driver_type="cnpg",
            origination="imported",
            phase="ready",
            created_at=now,
            updated_at=now,
            namespace="custom-ns",
            size_gi=100,
            import_conn_str="postgresql://user:pass@host/db",
            import_db_name="mydb",
            external_provider="aws",
            external_resource_id="arn:aws:rds:...",
            external_endpoint="rds.amazonaws.com",
            external_region="us-east-1",
            health_state="healthy",
            health_message="All checks passed",
            health_last_check=now,
        )
        await sql_store.create_data_resource(dr)
        retrieved = await sql_store.get_data_resource("tenant1", "full-dr")

        assert retrieved.namespace == "custom-ns"
        assert retrieved.size_gi == 100
        assert retrieved.import_conn_str == "postgresql://user:pass@host/db"
        assert retrieved.external_provider == "aws"
        assert retrieved.health_state == "healthy"

    @pytest.mark.asyncio
    async def test_operation_with_result_json(self, sql_store: SQLStore):
        """Test operation with JSON result field."""
        op = OperationRecord(
            id="op-json",
            tenant="tenant1",
            op_type="snapshot",
            resource="dr-1",
            phase="Succeeded",
            started_at="2025-01-01T00:00:00Z",
            completed_at="2025-01-01T01:00:00Z",
            result={"snapshot_id": "snap-123", "size_bytes": 1024, "tags": ["prod"]},
        )
        await sql_store.create_operation(op)
        retrieved = await sql_store.get_operation("tenant1", "op-json")

        assert retrieved.result is not None
        assert retrieved.result["snapshot_id"] == "snap-123"
        assert retrieved.result["tags"] == ["prod"]

    @pytest.mark.asyncio
    async def test_operation_with_error(self, sql_store: SQLStore):
        """Test operation with error field."""
        op = OperationRecord(
            id="op-error",
            tenant="tenant1",
            op_type="snapshot",
            resource="dr-1",
            phase="Failed",
            started_at="2025-01-01T00:00:00Z",
            error="Snapshot failed: out of space",
        )
        await sql_store.create_operation(op)
        retrieved = await sql_store.get_operation("tenant1", "op-error")

        assert retrieved.error == "Snapshot failed: out of space"
        assert retrieved.phase == "Failed"
