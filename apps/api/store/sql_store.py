"""SQLStore: durable store backed by SQL database using penguin-dal."""

import json
import os
from typing import Any, Optional, Protocol

from penguin_dal.db import AsyncDB

from models import (DataProtectionPolicyRecord, DataResourceRecord,
                    OperationRecord, SearchPoolRecord, VolumeSnapshotRecord)
from store import Store


class Row(Protocol):
    """Protocol for database row objects."""

    def __getattr__(self, name: str) -> Any:
        """Get attribute by name."""
        ...


class SQLStore(Store):
    """Durable SQL-backed store using penguin-dal for runtime operations."""

    def __init__(self, db: AsyncDB) -> None:
        """Initialize SQLStore with an AsyncDB instance.

        Args:
            db: AsyncDB instance with reflected tables.
        """
        self.db = db

    @staticmethod
    def create_from_env() -> "SQLStore":
        """Factory method to create SQLStore from environment variables.

        Supports DB_TYPE in {postgresql, mysql, sqlite}.
        Uses connection pooling and retry logic.

        Environment variables:
        - DB_TYPE: postgresql | mysql | sqlite (default: sqlite)
        - DB_HOST: Database host (default: localhost)
        - DB_PORT: Database port
        - DB_NAME: Database name (default: nest_db)
        - DB_USER: Database user
        - DB_PASS: Database password
        - DB_POOL_SIZE: Connection pool size (default: 10)
        """
        db_type = os.getenv("DB_TYPE", "sqlite").lower()
        db_host = os.getenv("DB_HOST", "localhost")
        db_port = os.getenv("DB_PORT", "")
        db_name = os.getenv("DB_NAME", "nest_db")
        db_user = os.getenv("DB_USER", "")
        db_pass = os.getenv("DB_PASS", "")
        pool_size = int(os.getenv("DB_POOL_SIZE", "10"))

        if db_type == "postgresql":
            port = db_port or "5432"
            uri = f"postgresql+asyncpg://{db_user}:{db_pass}@{db_host}:{port}/{db_name}"
        elif db_type == "mysql":
            port = db_port or "3306"
            uri = f"mysql+aiomysql://{db_user}:{db_pass}@{db_host}:{port}/{db_name}"
        elif db_type == "sqlite":
            # SQLite uses local file path or :memory:
            db_file = os.getenv("DB_FILE", "nest.db")
            uri = f"sqlite+aiosqlite:///{db_file}"
        else:
            raise ValueError(f"Unsupported DB_TYPE: {db_type}")

        db = AsyncDB(uri, pool_size=pool_size, echo=False)
        return SQLStore(db)

    async def _ensure_reflected(self) -> None:
        """Ensure tables are reflected (call once after creation)."""
        if not self.db._reflected:
            await self.db.reflect()

    # DataResource methods

    async def list_data_resources(self, tenant: str) -> list[DataResourceRecord]:
        """List all DataResources for a tenant."""
        await self._ensure_reflected()
        rows = await self.db(self.db.data_resources.tenant == tenant).select()
        return [self._row_to_data_resource(row) for row in rows]

    async def create_data_resource(self, dr: DataResourceRecord) -> None:
        """Create a new DataResource."""
        await self._ensure_reflected()
        # Check if already exists
        existing = await self.db(
            (self.db.data_resources.tenant == dr.tenant) & (self.db.data_resources.name == dr.name)
        ).select()
        if existing:
            raise ValueError(f"DataResource {dr.name} already exists")

        await self.db.data_resources.async_insert(
            id=dr.id,
            tenant=dr.tenant,
            name=dr.name,
            resource_type=dr.resource_type,
            engine_type=dr.engine_type,
            storage_class=dr.storage_class,
            driver_type=dr.driver_type,
            origination=dr.origination,
            phase=dr.phase,
            created_at=dr.created_at,
            updated_at=dr.updated_at,
            namespace=dr.namespace,
            size_gi=dr.size_gi,
            import_conn_str=dr.import_conn_str,
            import_db_name=dr.import_db_name,
            external_provider=dr.external_provider,
            external_resource_id=dr.external_resource_id,
            external_endpoint=dr.external_endpoint,
            external_region=dr.external_region,
            health_state=dr.health_state,
            health_message=dr.health_message,
            health_last_check=dr.health_last_check,
        )

    async def get_data_resource(self, tenant: str, name: str) -> Optional[DataResourceRecord]:
        """Get a DataResource by name."""
        await self._ensure_reflected()
        rows = await self.db(
            (self.db.data_resources.tenant == tenant) & (self.db.data_resources.name == name)
        ).select()
        if not rows:
            raise ValueError(f"DataResource {name} not found")
        return self._row_to_data_resource(rows[0])

    async def delete_data_resource(self, tenant: str, name: str) -> None:
        """Delete a DataResource."""
        await self._ensure_reflected()
        count = await self.db(
            (self.db.data_resources.tenant == tenant) & (self.db.data_resources.name == name)
        ).delete()
        if count == 0:
            raise ValueError(f"DataResource {name} not found")

    async def count_data_resources(self, tenant: str) -> int:
        """Count DataResources for a tenant."""
        await self._ensure_reflected()
        count = await self.db(self.db.data_resources.tenant == tenant).count()
        return count

    async def update_data_resource(self, dr: DataResourceRecord) -> None:
        """Update a DataResource."""
        await self._ensure_reflected()
        count = await self.db(
            (self.db.data_resources.tenant == dr.tenant) & (self.db.data_resources.name == dr.name)
        ).update(
            id=dr.id,
            resource_type=dr.resource_type,
            engine_type=dr.engine_type,
            storage_class=dr.storage_class,
            driver_type=dr.driver_type,
            origination=dr.origination,
            phase=dr.phase,
            created_at=dr.created_at,
            updated_at=dr.updated_at,
            namespace=dr.namespace,
            size_gi=dr.size_gi,
            import_conn_str=dr.import_conn_str,
            import_db_name=dr.import_db_name,
            external_provider=dr.external_provider,
            external_resource_id=dr.external_resource_id,
            external_endpoint=dr.external_endpoint,
            external_region=dr.external_region,
            health_state=dr.health_state,
            health_message=dr.health_message,
            health_last_check=dr.health_last_check,
        )
        if count == 0:
            raise ValueError(f"DataResource {dr.name} not found")

    async def update_data_resource_health(self, tenant: str, name: str, health: str) -> None:
        """Update health state of a DataResource."""
        await self._ensure_reflected()
        count = await self.db(
            (self.db.data_resources.tenant == tenant) & (self.db.data_resources.name == name)
        ).update(health_state=health)
        if count == 0:
            raise ValueError(f"DataResource {name} not found")

    # Operation methods

    async def create_operation(self, op: OperationRecord) -> None:
        """Create a new operation."""
        await self._ensure_reflected()
        # Check if already exists
        existing = await self.db(
            (self.db.operations.tenant == op.tenant) & (self.db.operations.id == op.id)
        ).select()
        if existing:
            raise ValueError(f"Operation {op.id} already exists")

        await self.db.operations.async_insert(
            id=op.id,
            tenant=op.tenant,
            op_type=op.op_type,
            resource=op.resource,
            phase=op.phase,
            started_at=op.started_at,
            completed_at=op.completed_at,
            error=op.error,
            result=json.dumps(op.result) if op.result else None,
        )

    async def get_operation(self, tenant: str, op_id: str) -> OperationRecord:
        """Get an operation by ID."""
        await self._ensure_reflected()
        rows = await self.db(
            (self.db.operations.tenant == tenant) & (self.db.operations.id == op_id)
        ).select()
        if not rows:
            raise ValueError(f"Operation {op_id} not found")
        return self._row_to_operation(rows[0])

    async def update_operation(self, op: OperationRecord) -> None:
        """Update an operation."""
        await self._ensure_reflected()
        count = await self.db(
            (self.db.operations.tenant == op.tenant) & (self.db.operations.id == op.id)
        ).update(
            op_type=op.op_type,
            resource=op.resource,
            phase=op.phase,
            started_at=op.started_at,
            completed_at=op.completed_at,
            error=op.error,
            result=json.dumps(op.result) if op.result else None,
        )
        if count == 0:
            raise ValueError(f"Operation {op.id} not found")

    async def list_operations(self, tenant: str) -> list[OperationRecord]:
        """List all operations for a tenant."""
        await self._ensure_reflected()
        rows = await self.db(self.db.operations.tenant == tenant).select()
        return [self._row_to_operation(row) for row in rows]

    # VolumeSnapshot methods

    async def list_snapshots(self, tenant: str) -> list[VolumeSnapshotRecord]:
        """List all VolumeSnapshots for a tenant."""
        await self._ensure_reflected()
        rows = await self.db(self.db.volume_snapshots.tenant == tenant).select()
        return [self._row_to_snapshot(row) for row in rows]

    async def create_snapshot(
        self, tenant: str, name: str, source_pvc: str, snapshot_class: str
    ) -> VolumeSnapshotRecord:
        """Create a new VolumeSnapshot."""
        await self._ensure_reflected()
        # Check if already exists
        existing = await self.db(
            (self.db.volume_snapshots.tenant == tenant) & (self.db.volume_snapshots.name == name)
        ).select()
        if existing:
            raise ValueError(f"VolumeSnapshot {name} already exists")

        await self.db.volume_snapshots.async_insert(
            name=name,
            tenant=tenant,
            source_pvc=source_pvc,
            snapshot_class=snapshot_class,
            ready_to_use=1,
            creation_time="",
            size_bytes=0,
        )

        return VolumeSnapshotRecord(
            name=name,
            tenant=tenant,
            source_pvc=source_pvc,
            snapshot_class=snapshot_class,
            ready_to_use=True,
            creation_time="",
        )

    async def delete_snapshot(self, tenant: str, name: str) -> None:
        """Delete a VolumeSnapshot."""
        await self._ensure_reflected()
        count = await self.db(
            (self.db.volume_snapshots.tenant == tenant) & (self.db.volume_snapshots.name == name)
        ).delete()
        if count == 0:
            raise ValueError(f"VolumeSnapshot {name} not found")

    # DataProtectionPolicy methods

    async def list_protection_policies(self, tenant: str) -> list[DataProtectionPolicyRecord]:
        """List all DataProtectionPolicy CRs for a tenant."""
        await self._ensure_reflected()
        rows = await self.db(self.db.data_protection_policies.tenant == tenant).select()
        return [self._row_to_policy(row) for row in rows]

    async def create_protection_policy(
        self,
        tenant: str,
        name: str,
        snapshot_schedule: str,
        backup_schedule: str,
        destination: str,
    ) -> DataProtectionPolicyRecord:
        """Create a new DataProtectionPolicy."""
        await self._ensure_reflected()
        # Check if already exists
        existing = await self.db(
            (self.db.data_protection_policies.tenant == tenant)
            & (self.db.data_protection_policies.name == name)
        ).select()
        if existing:
            raise ValueError(f"DataProtectionPolicy {name} already exists")

        await self.db.data_protection_policies.async_insert(
            name=name,
            tenant=tenant,
            snapshot_schedule=snapshot_schedule,
            backup_schedule=backup_schedule,
            destination=destination,
            last_snapshot="",
            last_backup="",
            created_at="",
        )

        return DataProtectionPolicyRecord(
            name=name,
            tenant=tenant,
            snapshot_schedule=snapshot_schedule,
            backup_schedule=backup_schedule,
            destination=destination,
            created_at="",
        )

    async def delete_protection_policy(self, tenant: str, name: str) -> None:
        """Delete a DataProtectionPolicy."""
        await self._ensure_reflected()
        count = await self.db(
            (self.db.data_protection_policies.tenant == tenant)
            & (self.db.data_protection_policies.name == name)
        ).delete()
        if count == 0:
            raise ValueError(f"DataProtectionPolicy {name} not found")

    # SearchPool methods (not tenant-specific)

    async def list_search_pools(self) -> list[SearchPoolRecord]:
        """List all SearchPool CRs (not tenant-specific)."""
        await self._ensure_reflected()
        rows = await self.db(self.db.search_pools.name != "").select()
        return [self._row_to_search_pool(row) for row in rows]

    async def create_search_pool(self, name: str, replicas: int = 1) -> SearchPoolRecord:
        """Create a new SearchPool CR."""
        await self._ensure_reflected()
        # Check if already exists
        existing = await self.db(self.db.search_pools.name == name).select()
        if existing:
            raise ValueError(f"SearchPool {name} already exists")

        await self.db.search_pools.async_insert(
            name=name,
            phase="Pending",
            endpoint="",
            tenant_count=0,
            replicas=replicas,
            version="",
            created_at="",
        )

        return SearchPoolRecord(
            name=name,
            phase="Pending",
            replicas=replicas,
            created_at="",
        )

    async def get_search_pool(self, name: str) -> SearchPoolRecord:
        """Get a SearchPool CR by name."""
        await self._ensure_reflected()
        rows = await self.db(self.db.search_pools.name == name).select()
        if not rows:
            raise ValueError(f"SearchPool {name} not found")
        return self._row_to_search_pool(rows[0])

    async def delete_search_pool(self, name: str) -> None:
        """Delete a SearchPool CR."""
        await self._ensure_reflected()
        count = await self.db(self.db.search_pools.name == name).delete()
        if count == 0:
            raise ValueError(f"SearchPool {name} not found")

    # Helper methods for row conversion

    @staticmethod
    def _row_to_data_resource(row: Row) -> DataResourceRecord:
        """Convert a database row to DataResourceRecord."""
        return DataResourceRecord(
            id=str(row.id),
            name=str(row.name),
            tenant=str(row.tenant),
            resource_type=str(row.resource_type),
            engine_type=str(row.engine_type),
            storage_class=str(row.storage_class),
            driver_type=str(row.driver_type),
            origination=str(row.origination),
            phase=str(row.phase),
            created_at=str(row.created_at),
            updated_at=str(row.updated_at),
            namespace=str(row.namespace or ""),
            size_gi=int(row.size_gi or 0),
            import_conn_str=str(row.import_conn_str or ""),
            import_db_name=str(row.import_db_name or ""),
            external_provider=str(row.external_provider or ""),
            external_resource_id=str(row.external_resource_id or ""),
            external_endpoint=str(row.external_endpoint or ""),
            external_region=str(row.external_region or ""),
            health_state=str(row.health_state or ""),
            health_message=str(row.health_message or ""),
            health_last_check=str(row.health_last_check or ""),
        )

    @staticmethod
    def _row_to_operation(row: Row) -> OperationRecord:
        """Convert a database row to OperationRecord."""
        result = None
        if row.result:
            try:
                result = json.loads(str(row.result))
            except (json.JSONDecodeError, TypeError):
                result = None

        return OperationRecord(
            id=str(row.id),
            tenant=str(row.tenant),
            op_type=str(row.op_type),
            resource=str(row.resource),
            phase=str(row.phase),
            started_at=str(row.started_at),
            completed_at=str(row.completed_at) if row.completed_at else None,
            error=str(row.error) if row.error else None,
            result=result,
        )

    @staticmethod
    def _row_to_snapshot(row: Row) -> VolumeSnapshotRecord:
        """Convert a database row to VolumeSnapshotRecord."""
        return VolumeSnapshotRecord(
            name=str(row.name),
            tenant=str(row.tenant),
            source_pvc=str(row.source_pvc),
            snapshot_class=str(row.snapshot_class),
            ready_to_use=bool(row.ready_to_use),
            creation_time=str(row.creation_time or ""),
            size_bytes=int(row.size_bytes or 0),
        )

    @staticmethod
    def _row_to_policy(row: Row) -> DataProtectionPolicyRecord:
        """Convert a database row to DataProtectionPolicyRecord."""
        return DataProtectionPolicyRecord(
            name=str(row.name),
            tenant=str(row.tenant),
            snapshot_schedule=str(row.snapshot_schedule or ""),
            backup_schedule=str(row.backup_schedule or ""),
            destination=str(row.destination or ""),
            last_snapshot=str(row.last_snapshot or ""),
            last_backup=str(row.last_backup or ""),
            created_at=str(row.created_at or ""),
        )

    @staticmethod
    def _row_to_search_pool(row: Row) -> SearchPoolRecord:
        """Convert a database row to SearchPoolRecord."""
        return SearchPoolRecord(
            name=str(row.name),
            phase=str(row.phase or "Pending"),
            endpoint=str(row.endpoint or ""),
            tenant_count=int(row.tenant_count or 0),
            replicas=int(row.replicas or 1),
            version=str(row.version or ""),
            created_at=str(row.created_at or ""),
        )
