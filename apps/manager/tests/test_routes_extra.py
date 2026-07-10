"""Additional route tests covering stats, audit, and database_servers endpoints."""
import os
import sys
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

os.environ.setdefault("JWT_SECRET", "test-secret-key")

# Helper functions
def _make_db(count=0):
    """Create a mock DB object."""
    db = MagicMock()
    db.count = MagicMock(return_value=count)
    return db

def _make_token(role="admin"):
    """Create a valid JWT token for testing."""
    from utils.auth import create_token
    return create_token(user_id=1, email="test@example.com", role=role)

_GET_DB_EXTRA = [
    "routes.stats.get_db",
    "routes.audit.get_db",
    "routes.database_servers.get_db",
    "routes.user_profiles.get_db",
    "routes.permissions.get_db",
    "routes.managed_databases.get_db",
    "routes.security_rules.get_db",
    "routes.blocked_databases.get_db",
    "routes.temporary_access.get_db",
    "routes.sync.get_db",
    "routes.scaling.get_db",
    "routes.threat_intel.get_db",
    "routes.cloud.get_db",
    "routes.teams.get_db",
    "routes.analytics.get_db",
    "routes.license.get_db",
    "routes.sql_files.get_db",
]


def _cmp_mock():
    """A MagicMock whose comparison operators all return a MagicMock query fragment."""
    m = MagicMock()
    ret = MagicMock()
    ret.__and__ = MagicMock(return_value=ret)
    ret.__or__ = MagicMock(return_value=ret)
    m.__gt__ = MagicMock(return_value=ret)
    m.__lt__ = MagicMock(return_value=ret)
    m.__ge__ = MagicMock(return_value=ret)
    m.__le__ = MagicMock(return_value=ret)
    m.__eq__ = MagicMock(return_value=ret)
    m.__ne__ = MagicMock(return_value=ret)
    return m


def _table_mock():
    """Create a table-like MagicMock where columns support comparison operators."""
    tbl = MagicMock()
    for col in ("id", "active", "user_id", "server_id", "action", "created_at", "role"):
        setattr(tbl, col, _cmp_mock())
    return tbl


@pytest.fixture()
def db():
    """Per-test DB mock with all extra route targets patched."""
    mock = _make_db(count=0)
    # Override all table mocks with comparison-operator-aware versions
    for tbl_name in (
        "audit_log", "managed_database", "user_permission", "threat_indicator",
        "threat_intel_feed", "security_rule", "blocked_database", "sql_file",
        "temporary_access", "cloud_provider", "scaling_policy", "user_profile",
        "teams", "team_memberships", "database_server",
    ):
        setattr(mock, tbl_name, _table_mock())

    patches = [patch(t, return_value=mock) for t in _GET_DB_EXTRA]
    for p in patches:
        p.start()
    yield mock
    for p in patches:
        p.stop()


# ===========================================================================
# GET /api/v1/status  (stats.py — no auth)
# ===========================================================================

@pytest.mark.asyncio
async def test_status_no_auth(client):
    """Status endpoint requires no authentication."""
    resp = await client._client.get("/api/v1/status")
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "version" in data
    assert "service" in data
    assert data["service"] == "nest-manager"


@pytest.mark.asyncio
async def test_status_status_ok(client):
    resp = await client._client.get("/api/v1/status")
    data = await resp.get_json()
    assert data["status"] == "ok"


# ===========================================================================
# GET /api/v1/stats  (stats.py — auth required)
# ===========================================================================

@pytest.mark.asyncio
async def test_stats_authenticated(client, db):
    db.return_value.count.return_value = 5
    token = _make_token()
    resp = await client.get(
        "/api/v1/stats",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data
    assert "servers" in data["data"]
    assert "managed_databases" in data["data"]


@pytest.mark.asyncio
async def test_stats_unauthenticated(client):
    resp = await client.get("/api/v1/stats")
    assert resp.status_code == 401


# ===========================================================================
# GET /api/v1/audit-log  (audit.py — auth required)
# ===========================================================================

@pytest.mark.asyncio
async def test_audit_log_authenticated(client, db):
    rows_mock = MagicMock()
    rows_mock.__iter__ = MagicMock(return_value=iter([]))
    db.return_value.select.return_value = rows_mock
    db.return_value.count.return_value = 0
    token = _make_token()
    resp = await client.get(
        "/api/v1/audit-log",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data
    assert "meta" in data


@pytest.mark.asyncio
async def test_audit_log_unauthenticated(client):
    resp = await client.get("/api/v1/audit-log")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_audit_log_pagination(client, db):
    rows_mock = MagicMock()
    rows_mock.__iter__ = MagicMock(return_value=iter([]))
    db.return_value.select.return_value = rows_mock
    db.return_value.count.return_value = 100
    token = _make_token()
    resp = await client.get(
        "/api/v1/audit-log?page=3&per_page=10",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["meta"]["page"] == 3
    assert data["meta"]["per_page"] == 10


@pytest.mark.asyncio
async def test_audit_log_filters(client, db):
    """Filters (user_id, server_id, action, from, to) are passed without error."""
    rows_mock = MagicMock()
    rows_mock.__iter__ = MagicMock(return_value=iter([]))
    db.return_value.select.return_value = rows_mock
    db.return_value.count.return_value = 0
    token = _make_token()
    resp = await client.get(
        "/api/v1/audit-log?user_id=1&server_id=2&action=login&from=2024-01-01&to=2024-12-31",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_audit_log_invalid_page_clamped(client, db):
    rows_mock = MagicMock()
    rows_mock.__iter__ = MagicMock(return_value=iter([]))
    db.return_value.select.return_value = rows_mock
    db.return_value.count.return_value = 0
    token = _make_token()
    resp = await client.get(
        "/api/v1/audit-log?page=0&per_page=999",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["meta"]["page"] == 1
    assert data["meta"]["per_page"] == 50


# ===========================================================================
# GET /api/v1/servers/<id>  (database_servers.py)
# ===========================================================================

@pytest.mark.asyncio
async def test_get_server_found(client, db):
    server_dict = {"id": 1, "name": "pg", "host": "db.local", "port": 5432, "db_type": "pg", "active": True}
    server_row = MagicMock()
    server_row.as_dict.return_value = server_dict
    db.database_server.__getitem__ = MagicMock(return_value=server_row)
    token = _make_token()
    resp = await client.get(
        "/api/v1/servers/1",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_get_server_not_found(client, db):
    db.database_server.__getitem__ = MagicMock(return_value=None)
    token = _make_token()
    resp = await client.get(
        "/api/v1/servers/999",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_get_server_unauthenticated(client):
    resp = await client.get("/api/v1/servers/1")
    assert resp.status_code == 401


# ===========================================================================
# PUT /api/v1/servers/<id>
# ===========================================================================

@pytest.mark.asyncio
async def test_update_server_not_found(client, db):
    db.database_server.__getitem__ = MagicMock(return_value=None)
    token = _make_token()
    resp = await client.put(
        "/api/v1/servers/999",
        json={"name": "new-name"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_update_server_no_body(client, db):
    token = _make_token()
    resp = await client.put(
        "/api/v1/servers/1",
        data=b"",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        },
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_update_server_success(client, db):
    server_dict = {"id": 1, "name": "updated", "host": "db.local", "port": 5432, "db_type": "pg", "active": True}
    server_row = MagicMock()
    server_row.as_dict.return_value = server_dict
    db.database_server.__getitem__ = MagicMock(return_value=server_row)
    db.return_value.update = MagicMock()
    token = _make_token()
    resp = await client.put(
        "/api/v1/servers/1",
        json={"name": "updated"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200


# ===========================================================================
# DELETE /api/v1/servers/<id>  (requires admin)
# ===========================================================================

@pytest.mark.asyncio
async def test_delete_server_success(client, db):
    server_row = MagicMock()
    db.database_server.__getitem__ = MagicMock(return_value=server_row)
    db.return_value.update = MagicMock()
    token = _make_token(role="admin")
    resp = await client.delete(
        "/api/v1/servers/1",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "message" in data


@pytest.mark.asyncio
async def test_delete_server_not_found(client, db):
    db.database_server.__getitem__ = MagicMock(return_value=None)
    token = _make_token(role="admin")
    resp = await client.delete(
        "/api/v1/servers/999",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_delete_server_requires_admin(client, db):
    token = _make_token(role="viewer")
    resp = await client.delete(
        "/api/v1/servers/1",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_delete_server_unauthenticated(client):
    resp = await client.delete("/api/v1/servers/1")
    assert resp.status_code == 401
