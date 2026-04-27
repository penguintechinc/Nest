"""Data models for Nest API."""
from dataclasses import dataclass, field
from typing import Optional


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
