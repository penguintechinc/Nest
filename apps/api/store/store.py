"""In-memory store for DataResources and Operations."""
import asyncio
from abc import ABC, abstractmethod
from typing import Optional

from models import (
    DataResourceRecord,
    OperationRecord,
    VolumeSnapshotRecord,
    DataProtectionPolicyRecord,
    SearchPoolRecord,
)


class Store(ABC):
    """Data access interface for the API server."""

    @abstractmethod
    async def list_data_resources(
        self, tenant: str
    ) -> list[DataResourceRecord]:
        """List all DataResources for a tenant."""
        pass

    @abstractmethod
    async def create_data_resource(self, dr: DataResourceRecord) -> None:
        """Create a new DataResource."""
        pass

    @abstractmethod
    async def get_data_resource(
        self, tenant: str, name: str
    ) -> Optional[DataResourceRecord]:
        """Get a DataResource by name."""
        pass

    @abstractmethod
    async def delete_data_resource(self, tenant: str, name: str) -> None:
        """Delete a DataResource."""
        pass

    @abstractmethod
    async def count_data_resources(self, tenant: str) -> int:
        """Count DataResources for a tenant."""
        pass

    @abstractmethod
    async def update_data_resource(self, dr: DataResourceRecord) -> None:
        """Update a DataResource."""
        pass

    @abstractmethod
    async def update_data_resource_health(
        self, tenant: str, name: str, health: str
    ) -> None:
        """Update health state of a DataResource."""
        pass

    @abstractmethod
    async def create_operation(self, op: OperationRecord) -> None:
        """Create a new operation."""
        pass

    @abstractmethod
    async def get_operation(self, tenant: str, op_id: str) -> OperationRecord:
        """Get an operation by ID."""
        pass

    @abstractmethod
    async def update_operation(self, op: OperationRecord) -> None:
        """Update an operation."""
        pass

    @abstractmethod
    async def list_operations(self, tenant: str) -> list[OperationRecord]:
        """List all operations for a tenant."""
        pass

    @abstractmethod
    async def list_snapshots(self, tenant: str) -> list[VolumeSnapshotRecord]:
        """List all VolumeSnapshots for a tenant."""
        pass

    @abstractmethod
    async def create_snapshot(
        self, tenant: str, name: str, source_pvc: str, snapshot_class: str
    ) -> VolumeSnapshotRecord:
        """Create a new VolumeSnapshot."""
        pass

    @abstractmethod
    async def delete_snapshot(self, tenant: str, name: str) -> None:
        """Delete a VolumeSnapshot."""
        pass

    @abstractmethod
    async def list_protection_policies(
        self, tenant: str
    ) -> list[DataProtectionPolicyRecord]:
        """List all DataProtectionPolicy CRs for a tenant."""
        pass

    @abstractmethod
    async def create_protection_policy(
        self,
        tenant: str,
        name: str,
        snapshot_schedule: str,
        backup_schedule: str,
        destination: str,
    ) -> DataProtectionPolicyRecord:
        """Create a new DataProtectionPolicy."""
        pass

    @abstractmethod
    async def delete_protection_policy(self, tenant: str, name: str) -> None:
        """Delete a DataProtectionPolicy."""
        pass

    @abstractmethod
    async def list_search_pools(self) -> list[SearchPoolRecord]:
        """List all SearchPool CRs (not tenant-specific)."""
        pass

    @abstractmethod
    async def create_search_pool(
        self, name: str, replicas: int = 1
    ) -> SearchPoolRecord:
        """Create a new SearchPool CR."""
        pass

    @abstractmethod
    async def get_search_pool(self, name: str) -> SearchPoolRecord:
        """Get a SearchPool CR by name."""
        pass

    @abstractmethod
    async def delete_search_pool(self, name: str) -> None:
        """Delete a SearchPool CR."""
        pass


class MemoryStore(Store):
    """Simple in-memory store for P1 local dev."""

    def __init__(self) -> None:
        """Initialize the store."""
        self._resources: dict[str, DataResourceRecord] = {}
        self._operations: dict[str, OperationRecord] = {}
        self._snapshots: dict[str, VolumeSnapshotRecord] = {}
        self._policies: dict[str, DataProtectionPolicyRecord] = {}
        self._search_pools: dict[str, SearchPoolRecord] = {}
        self._lock = asyncio.Lock()

    def _key(self, tenant: str, name: str) -> str:
        """Generate a key for tenant/name."""
        return f"{tenant}/{name}"

    def _op_key(self, tenant: str, op_id: str) -> str:
        """Generate a key for tenant/op_id."""
        return f"{tenant}/{op_id}"

    async def list_data_resources(
        self, tenant: str
    ) -> list[DataResourceRecord]:
        """List all DataResources for a tenant."""
        async with self._lock:
            return [
                dr for dr in self._resources.values() if dr.tenant == tenant
            ]

    async def create_data_resource(self, dr: DataResourceRecord) -> None:
        """Create a new DataResource."""
        async with self._lock:
            key = self._key(dr.tenant, dr.name)
            if key in self._resources:
                raise ValueError(f"DataResource {dr.name} already exists")
            self._resources[key] = dr

    async def get_data_resource(
        self, tenant: str, name: str
    ) -> Optional[DataResourceRecord]:
        """Get a DataResource by name."""
        async with self._lock:
            key = self._key(tenant, name)
            if key not in self._resources:
                raise ValueError(f"DataResource {name} not found")
            return self._resources[key]

    async def delete_data_resource(self, tenant: str, name: str) -> None:
        """Delete a DataResource."""
        async with self._lock:
            key = self._key(tenant, name)
            if key not in self._resources:
                raise ValueError(f"DataResource {name} not found")
            del self._resources[key]

    async def count_data_resources(self, tenant: str) -> int:
        """Count DataResources for a tenant."""
        async with self._lock:
            return sum(1 for dr in self._resources.values() if dr.tenant == tenant)

    async def update_data_resource(self, dr: DataResourceRecord) -> None:
        """Update a DataResource."""
        async with self._lock:
            key = self._key(dr.tenant, dr.name)
            if key not in self._resources:
                raise ValueError(f"DataResource {dr.name} not found")
            self._resources[key] = dr

    async def update_data_resource_health(
        self, tenant: str, name: str, health: str
    ) -> None:
        """Update health state of a DataResource."""
        async with self._lock:
            key = self._key(tenant, name)
            if key not in self._resources:
                raise ValueError(f"DataResource {name} not found")
            self._resources[key].health_state = health

    async def create_operation(self, op: OperationRecord) -> None:
        """Create a new operation."""
        async with self._lock:
            key = self._op_key(op.tenant, op.id)
            if key in self._operations:
                raise ValueError(f"Operation {op.id} already exists")
            self._operations[key] = op

    async def get_operation(self, tenant: str, op_id: str) -> OperationRecord:
        """Get an operation by ID."""
        async with self._lock:
            key = self._op_key(tenant, op_id)
            if key not in self._operations:
                raise ValueError(f"Operation {op_id} not found")
            return self._operations[key]

    async def update_operation(self, op: OperationRecord) -> None:
        """Update an operation."""
        async with self._lock:
            key = self._op_key(op.tenant, op.id)
            if key not in self._operations:
                raise ValueError(f"Operation {op.id} not found")
            self._operations[key] = op

    async def list_operations(self, tenant: str) -> list[OperationRecord]:
        """List all operations for a tenant."""
        async with self._lock:
            return [
                op for op in self._operations.values() if op.tenant == tenant
            ]

    async def list_snapshots(self, tenant: str) -> list[VolumeSnapshotRecord]:
        """List all VolumeSnapshots for a tenant."""
        async with self._lock:
            return [
                s for s in self._snapshots.values() if s.tenant == tenant
            ]

    async def create_snapshot(
        self, tenant: str, name: str, source_pvc: str, snapshot_class: str
    ) -> VolumeSnapshotRecord:
        """Create a new VolumeSnapshot."""
        async with self._lock:
            key = self._key(tenant, name)
            if key in self._snapshots:
                raise ValueError(f"VolumeSnapshot {name} already exists")
            snap = VolumeSnapshotRecord(
                name=name,
                tenant=tenant,
                source_pvc=source_pvc,
                snapshot_class=snapshot_class,
                ready_to_use=True,
                creation_time="",
            )
            self._snapshots[key] = snap
            return snap

    async def delete_snapshot(self, tenant: str, name: str) -> None:
        """Delete a VolumeSnapshot."""
        async with self._lock:
            key = self._key(tenant, name)
            if key not in self._snapshots:
                raise ValueError(f"VolumeSnapshot {name} not found")
            del self._snapshots[key]

    async def list_protection_policies(
        self, tenant: str
    ) -> list[DataProtectionPolicyRecord]:
        """List all DataProtectionPolicy CRs for a tenant."""
        async with self._lock:
            return [
                p for p in self._policies.values() if p.tenant == tenant
            ]

    async def create_protection_policy(
        self,
        tenant: str,
        name: str,
        snapshot_schedule: str,
        backup_schedule: str,
        destination: str,
    ) -> DataProtectionPolicyRecord:
        """Create a new DataProtectionPolicy."""
        async with self._lock:
            key = self._key(tenant, name)
            if key in self._policies:
                raise ValueError(f"DataProtectionPolicy {name} already exists")
            policy = DataProtectionPolicyRecord(
                name=name,
                tenant=tenant,
                snapshot_schedule=snapshot_schedule,
                backup_schedule=backup_schedule,
                destination=destination,
                created_at="",
            )
            self._policies[key] = policy
            return policy

    async def delete_protection_policy(self, tenant: str, name: str) -> None:
        """Delete a DataProtectionPolicy."""
        async with self._lock:
            key = self._key(tenant, name)
            if key not in self._policies:
                raise ValueError(f"DataProtectionPolicy {name} not found")
            del self._policies[key]

    async def list_search_pools(self) -> list[SearchPoolRecord]:
        """List all SearchPool CRs (not tenant-specific)."""
        async with self._lock:
            return list(self._search_pools.values())

    async def create_search_pool(
        self, name: str, replicas: int = 1
    ) -> SearchPoolRecord:
        """Create a new SearchPool CR."""
        async with self._lock:
            if name in self._search_pools:
                raise ValueError(f"SearchPool {name} already exists")
            pool = SearchPoolRecord(
                name=name,
                phase="Pending",
                replicas=replicas,
                created_at="",
            )
            self._search_pools[name] = pool
            return pool

    async def get_search_pool(self, name: str) -> SearchPoolRecord:
        """Get a SearchPool CR by name."""
        async with self._lock:
            if name not in self._search_pools:
                raise ValueError(f"SearchPool {name} not found")
            return self._search_pools[name]

    async def delete_search_pool(self, name: str) -> None:
        """Delete a SearchPool CR."""
        async with self._lock:
            if name not in self._search_pools:
                raise ValueError(f"SearchPool {name} not found")
            del self._search_pools[name]
