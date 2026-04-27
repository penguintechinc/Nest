"""In-memory store for DataResources."""
import asyncio
from abc import ABC, abstractmethod
from typing import Optional

from models import DataResourceRecord


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


class MemoryStore(Store):
    """Simple in-memory store for P1 local dev."""

    def __init__(self):
        """Initialize the store."""
        self._resources: dict[str, DataResourceRecord] = {}
        self._lock = asyncio.Lock()

    def _key(self, tenant: str, name: str) -> str:
        """Generate a key for tenant/name."""
        return f"{tenant}/{name}"

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
