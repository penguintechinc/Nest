"""Operation record data structure (no DB imports)."""

from dataclasses import dataclass


@dataclass(slots=True)
class OperationRecord:
    """Represents a long-running operation."""

    id: str
    tenant: str
    resource_name: str
    resource_type: str
    operation_type: str
    phase: str
    message: str
    created_at: str
    updated_at: str
    error: str = ""
    progress: int = 0
