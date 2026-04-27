"""Nest SDK type definitions."""
from dataclasses import dataclass, field
from typing import Optional, Dict


@dataclass(slots=True)
class DataResourceSpec:
    name: str
    type: str
    class_: str
    tenant: str
    labels: Dict[str, str] = field(default_factory=dict)


@dataclass(slots=True)
class DataResource:
    id: str
    name: str
    type: str
    class_: str
    tenant: str
    status: str
    endpoint: Optional[str] = None


@dataclass(slots=True)
class DatabaseSpec:
    name: str
    type: str
    class_: str
    tenant: str


@dataclass(slots=True)
class Database:
    id: str
    name: str
    type: str
    class_: str
    tenant: str
    status: str
    endpoint: Optional[str] = None
