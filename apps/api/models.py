"""Data models for Nest API."""
from dataclasses import dataclass, field
from typing import Any, Optional


@dataclass(slots=True)
class OperationRecord:
    """In-memory representation of a long-running operation."""

    id: str
    tenant: str
    op_type: str  # "snapshot", "restore", "migrate", "introspect"
    resource: str  # DataResource name
    phase: str  # "Pending", "Running", "Succeeded", "Failed"
    started_at: str  # ISO8601
    completed_at: Optional[str] = None
    error: Optional[str] = None
    result: Optional[dict[str, Any]] = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dict for JSON serialization."""
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


@dataclass(slots=True)
class DataResourceRecord:
    """In-memory P1 representation of a DataResource."""

    id: str
    name: str
    tenant: str
    resource_type: str  # pvc/block, pvc/file, object, nfs, iscsi, postgres, keyvalue
    engine_type: str
    storage_class: str
    driver_type: str
    origination: str  # managed | imported | external
    phase: str  # pending | provisioning | ready | failed | deleting
    created_at: str  # ISO8601
    updated_at: str  # ISO8601
    namespace: str = ""
    size_gi: int = 0
    # import fields
    import_conn_str: str = ""
    import_db_name: str = ""
    # external fields
    external_provider: str = ""  # aws|gcp|azure|cloudflare|vultr
    external_resource_id: str = ""
    external_endpoint: str = ""
    external_region: str = ""
    # health
    health_state: str = ""  # healthy|degraded|failed|unknown
    health_message: str = ""
    health_last_check: str = ""

    def to_dict(self) -> dict:
        """Convert to dict for JSON serialization."""
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


@dataclass(slots=True)
class VolumeSnapshotRecord:
    """In-memory representation of a VolumeSnapshot CR."""

    name: str
    tenant: str
    source_pvc: str
    snapshot_class: str
    ready_to_use: bool = False
    creation_time: str = ""
    size_bytes: int = 0

    def to_dict(self) -> dict[str, Any]:
        """Convert to dict for JSON serialization."""
        return {
            "name": self.name,
            "sourcePVC": self.source_pvc,
            "snapshotClass": self.snapshot_class,
            "readyToUse": self.ready_to_use,
            "creationTime": self.creation_time,
            "sizeBytes": self.size_bytes,
        }


@dataclass(slots=True)
class DataProtectionPolicyRecord:
    """In-memory representation of a DataProtectionPolicy CR."""

    name: str
    tenant: str
    snapshot_schedule: str = ""
    backup_schedule: str = ""
    destination: str = ""
    last_snapshot: str = ""
    last_backup: str = ""
    created_at: str = ""

    def to_dict(self) -> dict[str, Any]:
        """Convert to dict for JSON serialization."""
        return {
            "name": self.name,
            "snapshotSchedule": self.snapshot_schedule,
            "backupSchedule": self.backup_schedule,
            "destination": self.destination,
            "lastSnapshot": self.last_snapshot,
            "lastBackup": self.last_backup,
        }


@dataclass(slots=True)
class SearchPoolRecord:
    """In-memory representation of a SearchPool CR."""

    name: str
    phase: str = "Pending"  # Pending | Running | Ready | Failed
    endpoint: str = ""
    tenant_count: int = 0
    replicas: int = 1
    version: str = ""
    created_at: str = ""

    def to_dict(self) -> dict[str, Any]:
        """Convert to dict for JSON serialization."""
        return {
            "name": self.name,
            "phase": self.phase,
            "endpoint": self.endpoint,
            "tenantCount": self.tenant_count,
            "replicas": self.replicas,
            "version": self.version,
        }
