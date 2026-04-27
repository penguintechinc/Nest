"""Nest HTTP client with context manager support."""
from __future__ import annotations

import os
from typing import Optional, List, Any
import urllib.request
import urllib.error
import json

from .types import DataResource, DataResourceSpec, Database, DatabaseSpec


class NestClient:
    """Client for the Nest storage platform REST API.

    Usage:
        with NestClient("https://nest.acme.com", token="sk-...") as client:
            resources = client.data_resources.list(tenant="acme")
    """

    def __init__(
        self,
        base_url: Optional[str] = None,
        token: Optional[str] = None,
    ) -> None:
        self.base_url = (base_url or os.environ.get("NEST_ENDPOINT", "")).rstrip("/")
        self.token = token or os.environ.get("NEST_TOKEN", "")
        self.data_resources = DataResourceAPI(self)
        self.databases = DatabaseAPI(self)

    def __enter__(self) -> "NestClient":
        return self

    def __exit__(self, *_: Any) -> None:
        pass

    def _request(self, method: str, path: str, body: Any = None) -> Any:
        url = self.base_url + path
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(
            url,
            data=data,
            headers={
                "Authorization": f"Bearer {self.token}",
                "Content-Type": "application/json",
            },
            method=method,
        )
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return json.loads(resp.read())
        except urllib.error.HTTPError as e:
            body_text = e.read().decode()
            raise RuntimeError(f"Nest API {e.code}: {body_text}") from e


class DataResourceAPI:
    def __init__(self, client: NestClient) -> None:
        self._c = client

    def list(self, tenant: str) -> List[DataResource]:
        data = self._c._request("GET", f"/api/v1/tenants/{tenant}/dataresources")
        return [
            DataResource(
                **{k.replace("class", "class_"): v for k, v in r.items()}
            )
            for r in data.get("dataresources", [])
        ]

    def create(self, spec: DataResourceSpec) -> str:
        payload = {
            "name": spec.name,
            "type": spec.type,
            "class": spec.class_,
            "tenant": spec.tenant,
        }
        self._c._request("POST", f"/api/v1/tenants/{spec.tenant}/dataresources", payload)
        return ""

    def delete(self, tenant: str, name: str) -> None:
        self._c._request("DELETE", f"/api/v1/tenants/{tenant}/dataresources/{name}")


class DatabaseAPI:
    def __init__(self, client: NestClient) -> None:
        self._c = client

    def list(self, tenant: str) -> List[Database]:
        data = self._c._request("GET", f"/api/v1/tenants/{tenant}/databases")
        return [Database(**r) for r in data.get("databases", [])]

    def create(self, spec: DatabaseSpec) -> str:
        payload = {
            "name": spec.name,
            "type": spec.type,
            "class": spec.class_,
            "tenant": spec.tenant,
        }
        self._c._request("POST", f"/api/v1/tenants/{spec.tenant}/databases", payload)
        return ""
