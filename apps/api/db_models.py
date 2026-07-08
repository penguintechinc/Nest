"""SQLAlchemy ORM models for persistent storage."""

from typing import Any

from sqlalchemy import Column, Integer, String, Text
from sqlalchemy.orm import declarative_base

Base = declarative_base()


class DataResourceTable(Base):  # type: ignore[misc,valid-type]
    """SQLAlchemy model for DataResource persistence."""

    __tablename__ = "data_resources"

    id = Column(String(255), primary_key=True)
    tenant = Column(String(255), nullable=False, index=True)
    name = Column(String(255), nullable=False, index=True)
    resource_type = Column(String(100), nullable=False)
    engine_type = Column(String(100), nullable=False)
    storage_class = Column(String(255), nullable=False)
    driver_type = Column(String(100), nullable=False)
    origination = Column(String(50), nullable=False)
    phase = Column(String(50), nullable=False)
    created_at = Column(String(50), nullable=False)
    updated_at = Column(String(50), nullable=False)
    namespace = Column(String(255), default="")
    size_gi = Column(Integer, default=0)
    import_conn_str = Column(String(1024), default="")
    import_db_name = Column(String(255), default="")
    external_provider = Column(String(100), default="")
    external_resource_id = Column(String(255), default="")
    external_endpoint = Column(String(1024), default="")
    external_region = Column(String(100), default="")
    health_state = Column(String(50), default="")
    health_message = Column(Text, default="")
    health_last_check = Column(String(50), default="")

    def to_dict(self) -> dict[str, Any]:
        """Convert to dict for serialization."""
        return {
            "id": self.id,
            "name": self.name,
            "tenant": self.tenant,
            "resourceType": self.resource_type,
            "engineType": self.engine_type,
            "storageClass": self.storage_class,
            "driverType": self.driver_type,
            "origination": self.origination,
            "phase": self.phase,
            "createdAt": self.created_at,
            "updatedAt": self.updated_at,
            "namespace": self.namespace,
            "sizeGi": self.size_gi,
            "importConnStr": self.import_conn_str,
            "importDbName": self.import_db_name,
            "externalProvider": self.external_provider,
            "externalResourceId": self.external_resource_id,
            "externalEndpoint": self.external_endpoint,
            "externalRegion": self.external_region,
            "healthState": self.health_state,
            "healthMessage": self.health_message,
            "healthLastCheck": self.health_last_check,
        }


class OperationTable(Base):  # type: ignore[misc,valid-type]
    """SQLAlchemy model for Operation persistence."""

    __tablename__ = "operations"

    id = Column(String(255), primary_key=True)
    tenant = Column(String(255), nullable=False, index=True)
    op_type = Column(String(100), nullable=False)
    resource = Column(String(255), nullable=False)
    phase = Column(String(50), nullable=False)
    started_at = Column(String(50), nullable=False)
    completed_at = Column(String(50), default=None)
    error = Column(Text, default=None)
    result = Column(Text, default=None)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dict for serialization."""
        data: dict[str, Any] = {
            "id": self.id,
            "tenant": self.tenant,
            "type": self.op_type,
            "resource": self.resource,
            "phase": self.phase,
            "startedAt": self.started_at,
        }
        if self.completed_at:
            data["completedAt"] = self.completed_at
        if self.error:
            data["error"] = self.error
        if self.result:
            data["result"] = self.result
        return data


class VolumeSnapshotTable(Base):  # type: ignore[misc,valid-type]
    """SQLAlchemy model for VolumeSnapshot persistence."""

    __tablename__ = "volume_snapshots"

    name = Column(String(255), primary_key=True)
    tenant = Column(String(255), nullable=False, index=True)
    source_pvc = Column(String(255), nullable=False)
    snapshot_class = Column(String(255), nullable=False)
    ready_to_use = Column(Integer, default=0)
    creation_time = Column(String(50), default="")
    size_bytes = Column(Integer, default=0)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dict for serialization."""
        return {
            "name": self.name,
            "tenant": self.tenant,
            "sourcePVC": self.source_pvc,
            "snapshotClass": self.snapshot_class,
            "readyToUse": bool(self.ready_to_use),
            "creationTime": self.creation_time,
            "sizeBytes": self.size_bytes,
        }


class DataProtectionPolicyTable(Base):  # type: ignore[misc,valid-type]
    """SQLAlchemy model for DataProtectionPolicy persistence."""

    __tablename__ = "data_protection_policies"

    name = Column(String(255), primary_key=True)
    tenant = Column(String(255), nullable=False, index=True)
    snapshot_schedule = Column(String(255), default="")
    backup_schedule = Column(String(255), default="")
    destination = Column(String(255), default="")
    last_snapshot = Column(String(50), default="")
    last_backup = Column(String(50), default="")
    created_at = Column(String(50), default="")

    def to_dict(self) -> dict[str, Any]:
        """Convert to dict for serialization."""
        return {
            "name": self.name,
            "tenant": self.tenant,
            "snapshotSchedule": self.snapshot_schedule,
            "backupSchedule": self.backup_schedule,
            "destination": self.destination,
            "lastSnapshot": self.last_snapshot,
            "lastBackup": self.last_backup,
        }


class SearchPoolTable(Base):  # type: ignore[misc,valid-type]
    """SQLAlchemy model for SearchPool persistence."""

    __tablename__ = "search_pools"

    name = Column(String(255), primary_key=True)
    phase = Column(String(50), default="Pending")
    endpoint = Column(String(1024), default="")
    tenant_count = Column(Integer, default=0)
    replicas = Column(Integer, default=1)
    version = Column(String(100), default="")
    created_at = Column(String(50), default="")

    def to_dict(self) -> dict[str, Any]:
        """Convert to dict for serialization."""
        return {
            "name": self.name,
            "phase": self.phase,
            "endpoint": self.endpoint,
            "tenantCount": self.tenant_count,
            "replicas": self.replicas,
            "version": self.version,
        }
