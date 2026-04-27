"""Pulumi Python provider for Nest storage platform.

This is a stub; full implementation uses pulumi.ResourceProvider.

Usage:
    import pulumi_nest as nest

    postgres = nest.DataResource("my-postgres",
        type="postgres",
        class_="postgres-ha-3",
        tenant="acme",
    )

    pulumi.export("endpoint", postgres.endpoint)
"""
from __future__ import annotations


class DataResource:
	"""A Nest DataResource managed by Pulumi."""

	def __init__(
		self,
		name: str,
		*,
		type: str,
		class_: str,
		tenant: str,
	) -> None:
		self.name = name
		self.type = type
		self.class_ = class_
		self.tenant = tenant
		# In production: call Nest API to provision and register with Pulumi state
		self.id: str = ""
		self.endpoint: str = ""

	def __repr__(self) -> str:
		return f"DataResource(name={self.name!r}, type={self.type!r}, class={self.class_!r})"


class Credential:
	"""A Nest Credential (connection string) managed by Pulumi."""

	def __init__(
		self,
		name: str,
		*,
		resource_id: str,
		tenant: str,
	) -> None:
		self.name = name
		self.resource_id = resource_id
		self.tenant = tenant
		self.id: str = ""
		self.connection_string: str = ""


class HardwarePool:
	"""A Nest HardwarePool managed by Pulumi."""

	def __init__(
		self,
		name: str,
		*,
		tenant: str,
		storage_class: str = "sata-warm",
	) -> None:
		self.name = name
		self.tenant = tenant
		self.storage_class = storage_class
		self.id: str = ""
