"""Extended route tests for apps/manager — covering all remaining route modules."""
import os
import sys
import types
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

# Ensure the manager app directory is on the path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

# Required env vars before any import
os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("DB_HOST", "localhost")
os.environ.setdefault("DB_NAME", "test_nest")
os.environ.setdefault("DB_USER", "test")
os.environ.setdefault("FIELD_ENCRYPTION_KEY", "Fernet_key_placeholder_32bytes==")


# ---------------------------------------------------------------------------
# Module-level stub installation — must happen before app import
# ---------------------------------------------------------------------------


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_ALL_GET_DB_TARGETS = [
    "routes.managed_databases.get_db",
    "routes.scaling.get_db",
    "routes.cloud.get_db",
    "routes.analytics.get_db",
    "routes.user_profiles.get_db",
    "routes.permissions.get_db",
    "routes.sql_files.get_db",
    "routes.security_rules.get_db",
    "routes.sync.get_db",
    "routes.license.get_db",
    "routes.threat_intel.get_db",
    "routes.blocked_databases.get_db",
    "routes.temporary_access.get_db",
    "routes.audit.get_db",
    "routes.database_servers.get_db",
    "routes.auth.get_db",
    "penguin_dal.quart_ext.get_db",
]


def _make_db() -> MagicMock:
    """Build a fresh MagicMock DB returning sensible defaults."""
    db = MagicMock()

    # A reusable row dict for subscript lookups (db.table[id].as_dict())
    _row_dict = {"id": 42, "name": "test", "status": "ok"}

    select_result = MagicMock()
    select_result.__iter__ = MagicMock(return_value=iter([]))
    select_result.first.return_value = None

    query_result = MagicMock()
    query_result.select.return_value = select_result
    query_result.count.return_value = 0

    db.return_value = query_result
    db.__call__ = MagicMock(return_value=query_result)

    # Insert helpers — each table mock needs subscript (__getitem__) support
    for table in [
        "managed_database", "database_server", "scaling_policy",
        "scaling_event", "cloud_provider", "cloud_instance",
        "user_profile", "user_permission", "sql_file",
        "security_rule", "license_info", "threat_intel_feed",
        "threat_intel_indicator", "blocked_database",
        "temporary_access_token", "audit_log", "resources", "teams", "users",
    ]:
        tbl = MagicMock()
        tbl.insert = MagicMock(return_value=42)
        tbl.update_record = MagicMock()
        tbl.id = MagicMock()
        tbl.id.__gt__ = MagicMock(return_value=query_result)
        # Make db.table[key].as_dict() return a plain dict
        item_mock = MagicMock()
        item_mock.as_dict.return_value = dict(_row_dict)
        tbl.__getitem__ = MagicMock(return_value=item_mock)
        setattr(db, table, tbl)

    db.commit = MagicMock()
    db.executesql = MagicMock(return_value=[])
    return db


def _make_token(role: str = "admin") -> str:
    from utils.auth import create_token
    return create_token(user_id=1, email="test@example.com", role=role)


def _auth_headers(role: str = "admin") -> dict:
    return {"Authorization": f"Bearer {_make_token(role)}"}


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture()
def db():
    """Per-test DB mock; patches all known get_db call sites."""
    mock = _make_db()
    patches = [patch(t, return_value=mock) for t in _ALL_GET_DB_TARGETS]
    for p in patches:
        p.start()
    yield mock
    for p in patches:
        p.stop()


# ===========================================================================
# /api/v1/databases — managed_databases.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_databases_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/databases", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_databases_no_auth(client):
    resp = await client.get("/api/v1/databases")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_create_database_ok(client, db):
    db.managed_database.insert.return_value = 10
    resp = await client.post(
        "/api/v1/databases",
        json={"server_id": 1, "db_name": "mydb"},
        headers=_auth_headers(),
    )
    assert resp.status_code == 201
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_create_database_missing_fields(client, db):
    resp = await client.post(
        "/api/v1/databases",
        json={"server_id": 1},
        headers=_auth_headers(),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_database_no_body(client, db):
    resp = await client.post(
        "/api/v1/databases",
        json=None,
        headers=_auth_headers(),
    )
    assert resp.status_code == 400
    data = await resp.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_get_database_ok(client, db):
    """Test retrieving a single database (happy path)."""
    db.managed_database.__getitem__.return_value.as_dict.return_value = {
        "id": 42,
        "server_id": 1,
        "db_name": "proddb",
        "status": "active",
    }
    resp = await client.get("/api/v1/databases/42", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["data"]["db_name"] == "proddb"


@pytest.mark.asyncio
async def test_get_database_not_found(client, db):
    """Test 404 when database does not exist."""
    db.managed_database.__getitem__.return_value = None
    resp = await client.get("/api/v1/databases/999", headers=_auth_headers())
    assert resp.status_code == 404
    data = await resp.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_update_database_ok(client, db):
    """Test updating database fields (happy path)."""
    db.managed_database.__getitem__.return_value.as_dict.return_value = {
        "id": 42,
        "db_name": "newname",
    }
    resp = await client.put(
        "/api/v1/databases/42",
        json={"db_name": "newname", "description": "Updated"},
        headers=_auth_headers(),
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["data"]["db_name"] == "newname"


@pytest.mark.asyncio
async def test_update_database_not_found(client, db):
    """Test 404 when updating non-existent database."""
    db.managed_database.__getitem__.return_value = None
    resp = await client.put(
        "/api/v1/databases/999",
        json={"db_name": "newname"},
        headers=_auth_headers(),
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_update_database_no_body(client, db):
    """Test 400 when update request body is missing."""
    resp = await client.put(
        "/api/v1/databases/42",
        json=None,
        headers=_auth_headers(),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_delete_database_ok(client, db):
    """Test deleting a database (admin only, happy path)."""
    db.managed_database.__getitem__.return_value = {"id": 42}
    resp = await client.delete("/api/v1/databases/42", headers=_auth_headers("admin"))
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "message" in data


@pytest.mark.asyncio
async def test_delete_database_not_found(client, db):
    """Test 404 when deleting non-existent database."""
    db.managed_database.__getitem__.return_value = None
    resp = await client.delete("/api/v1/databases/999", headers=_auth_headers("admin"))
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_delete_database_forbidden(client, db):
    """Test 403 when non-admin tries to delete."""
    db.managed_database.__getitem__.return_value = {"id": 42}
    resp = await client.delete("/api/v1/databases/42", headers=_auth_headers("viewer"))
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_get_database_schema_ok(client, db):
    """Test retrieving database schema (happy path)."""
    db.managed_database.__getitem__.return_value.as_dict.return_value = {"id": 42}
    db.return_value.select.return_value = []
    resp = await client.get("/api/v1/databases/42/schema", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data
    assert "database" in data["data"]
    assert "schema" in data["data"]


@pytest.mark.asyncio
async def test_get_database_schema_not_found(client, db):
    """Test 404 when getting schema for non-existent database."""
    db.managed_database.__getitem__.return_value = None
    resp = await client.get("/api/v1/databases/999/schema", headers=_auth_headers())
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_refresh_database_schema_ok(client, db):
    """Test triggering schema refresh (happy path)."""
    db.managed_database.__getitem__.return_value = {"id": 42}
    resp = await client.post(
        "/api/v1/databases/42/schema",
        headers=_auth_headers(),
    )
    assert resp.status_code == 202
    data = await resp.get_json()
    assert "message" in data
    assert data["db_id"] == 42


@pytest.mark.asyncio
async def test_refresh_database_schema_not_found(client, db):
    """Test 404 when refreshing schema for non-existent database."""
    db.managed_database.__getitem__.return_value = None
    resp = await client.post(
        "/api/v1/databases/999/schema",
        headers=_auth_headers(),
    )
    assert resp.status_code == 404


# ===========================================================================
# /api/v1/license — license.py
# ===========================================================================


@pytest.mark.asyncio
async def test_get_license_ok(client, db):
    """Test retrieving license info (happy path, license exists)."""
    db.return_value.select.return_value.first.return_value = MagicMock(
        as_dict=MagicMock(return_value={
            "id": 1,
            "license_key": "test_key_1234567890abcdefghijklmn",
            "valid": True,
        })
    )
    resp = await client.get("/api/v1/license", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["data"] is not None
    # License key should be masked
    assert "****" in data["data"]["license_key"]


@pytest.mark.asyncio
async def test_get_license_not_configured(client, db):
    """Test license retrieval when no license configured."""
    db.return_value.select.return_value.first.return_value = None
    resp = await client.get("/api/v1/license", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["data"] is None


@pytest.mark.asyncio
async def test_set_license_ok(client, db):
    """Test setting a valid license (happy path)."""
    with patch("routes.license._validate_with_server") as mock_validate:
        mock_validate.return_value = {"valid": True, "features": ["ssa", "waddleai"]}
        db.return_value.select.return_value.first.return_value = None
        resp = await client.post(
            "/api/v1/license",
            json={"license_key": "valid-key-12345"},
            headers=_auth_headers("admin"),
        )
        assert resp.status_code == 200
        data = await resp.get_json()
        assert data["valid"] is True


@pytest.mark.asyncio
async def test_set_license_invalid(client, db):
    """Test 422 when license validation fails."""
    with patch("routes.license._validate_with_server") as mock_validate:
        mock_validate.return_value = {"valid": False, "error": "Invalid key format"}
        resp = await client.post(
            "/api/v1/license",
            json={"license_key": "invalid-key"},
            headers=_auth_headers("admin"),
        )
        assert resp.status_code == 422
        data = await resp.get_json()
        assert "error" in data


@pytest.mark.asyncio
async def test_set_license_no_body(client, db):
    """Test 400 when set license request body is missing."""
    resp = await client.post(
        "/api/v1/license",
        json=None,
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_set_license_empty_key(client, db):
    """Test 400 when license_key is empty or missing."""
    resp = await client.post(
        "/api/v1/license",
        json={"license_key": ""},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_set_license_forbidden(client, db):
    """Test 403 when non-admin tries to set license."""
    resp = await client.post(
        "/api/v1/license",
        json={"license_key": "some-key"},
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_remove_license_ok(client, db):
    """Test removing license (happy path)."""
    db.return_value.count.return_value = 1
    resp = await client.delete(
        "/api/v1/license",
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "message" in data


@pytest.mark.asyncio
async def test_remove_license_not_found(client, db):
    """Test 404 when removing non-existent license."""
    db.return_value.count.return_value = 0
    resp = await client.delete(
        "/api/v1/license",
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_remove_license_forbidden(client, db):
    """Test 403 when non-admin tries to remove license."""
    resp = await client.delete(
        "/api/v1/license",
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


# ===========================================================================
# /api/v1/sync — sync.py
# ===========================================================================


@pytest.mark.asyncio
async def test_sync_servers_ok(client, db):
    """Test syncing servers to Redis (happy path)."""
    with patch("routes.sync.sync_to_redis") as mock_sync, \
         patch("routes.sync.get_dblb_client") as mock_dblb:
        mock_sync.return_value = {"synced": 5}
        mock_client = MagicMock()
        mock_client.reload = MagicMock()
        mock_dblb.return_value = mock_client
        resp = await client.post("/api/v1/sync", headers=_auth_headers())
        assert resp.status_code == 200
        data = await resp.get_json()
        assert "message" in data




@pytest.mark.asyncio
async def test_get_blocking_config_ok(client, db):
    """Test retrieving blocking config from DBLB (happy path)."""
    with patch("routes.sync.get_dblb_client") as mock_dblb:
        mock_client = MagicMock()
        mock_client.get_blocking_config = MagicMock(
            return_value={"blocked": ["schema1", "schema2"]}
        )
        mock_dblb.return_value = mock_client
        resp = await client.get("/api/v1/blocking-config", headers=_auth_headers())
        assert resp.status_code == 200
        data = await resp.get_json()
        assert "data" in data


@pytest.mark.asyncio
async def test_get_blocking_config_error(client, db):
    """Test 500 when retrieving blocking config fails."""
    with patch("routes.sync.get_dblb_client") as mock_dblb:
        mock_client = MagicMock()
        mock_client.get_blocking_config.side_effect = Exception("DBLB error")
        mock_dblb.return_value = mock_client
        resp = await client.get("/api/v1/blocking-config", headers=_auth_headers())
        assert resp.status_code == 500
        data = await resp.get_json()
        assert "error" in data


@pytest.mark.asyncio
async def test_update_blocking_config_ok(client, db):
    """Test updating blocking config (admin only, happy path)."""
    with patch("routes.sync.get_dblb_client") as mock_dblb:
        mock_client = MagicMock()
        mock_client.set_blocking_config = MagicMock()
        mock_dblb.return_value = mock_client
        resp = await client.put(
            "/api/v1/blocking-config",
            json={"blocked": ["schema1"]},
            headers=_auth_headers("admin"),
        )
        assert resp.status_code == 200
        data = await resp.get_json()
        assert "message" in data


@pytest.mark.asyncio
async def test_update_blocking_config_no_body(client, db):
    """Test 400 when update config request body is missing."""
    resp = await client.put(
        "/api/v1/blocking-config",
        json=None,
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_update_blocking_config_error(client, db):
    """Test 500 when updating blocking config fails."""
    with patch("routes.sync.get_dblb_client") as mock_dblb:
        mock_client = MagicMock()
        mock_client.set_blocking_config.side_effect = Exception("DBLB error")
        mock_dblb.return_value = mock_client
        resp = await client.put(
            "/api/v1/blocking-config",
            json={"blocked": ["schema1"]},
            headers=_auth_headers("admin"),
        )
        assert resp.status_code == 500


@pytest.mark.asyncio
async def test_update_blocking_config_forbidden(client, db):
    """Test 403 when non-admin tries to update config."""
    resp = await client.put(
        "/api/v1/blocking-config",
        json={"blocked": ["schema1"]},
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_seed_blocked_resources_ok(client, db):
    """Test seeding default blocked resources (happy path)."""
    db.return_value.count.return_value = 0
    db.blocked_database.insert = MagicMock()
    resp = await client.post(
        "/api/v1/seed-blocked-resources",
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "message" in data
    assert "Seeded" in data["message"]


@pytest.mark.asyncio
async def test_seed_blocked_resources_forbidden(client, db):
    """Test 403 when non-admin tries to seed."""
    resp = await client.post(
        "/api/v1/seed-blocked-resources",
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


# ===========================================================================
# /api/v1/users/{user_id}/profile — user_profiles.py
# ===========================================================================


@pytest.mark.asyncio
async def test_get_profile_ok(client, db):
    """Test retrieving user profile (happy path)."""
    profile_data = {
        "id": 1,
        "user_id": 42,
        "rate_limit": 100,
        "api_key_encrypted": "encrypted_key_xyz",
    }
    db.return_value.select.return_value.first.return_value = MagicMock(
        as_dict=MagicMock(return_value=profile_data)
    )
    resp = await client.get("/api/v1/users/42/profile", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["data"]["user_id"] == 42
    # api_key should be omitted
    assert "api_key_encrypted" not in data["data"]


@pytest.mark.asyncio
async def test_get_profile_not_found(client, db):
    """Test 404 when profile does not exist."""
    db.return_value.select.return_value.first.return_value = None
    resp = await client.get("/api/v1/users/999/profile", headers=_auth_headers())
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_update_profile_ok(client, db):
    """Test updating user profile (happy path)."""
    db.return_value.select.return_value.first.return_value = MagicMock(
        as_dict=MagicMock(return_value={"user_id": 42, "rate_limit": 200})
    )
    resp = await client.put(
        "/api/v1/users/42/profile",
        json={"rate_limit": 200},
        headers=_auth_headers(),
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["data"]["rate_limit"] == 200


@pytest.mark.asyncio
async def test_update_profile_no_body(client, db):
    """Test 400 when update profile request body is missing."""
    resp = await client.put(
        "/api/v1/users/42/profile",
        json=None,
        headers=_auth_headers(),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_update_profile_no_valid_fields(client, db):
    """Test 400 when no valid fields provided in update."""
    db.return_value.select.return_value.first.return_value = MagicMock(
        as_dict=MagicMock(return_value={"user_id": 42})
    )
    resp = await client.put(
        "/api/v1/users/42/profile",
        json={"invalid_field": "value"},
        headers=_auth_headers(),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_update_profile_not_found(client, db):
    """Test 404 when updating non-existent profile."""
    db.return_value.select.return_value.first.return_value = None
    resp = await client.put(
        "/api/v1/users/999/profile",
        json={"rate_limit": 100},
        headers=_auth_headers(),
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_update_profile_self_only(client, db):
    """Test 403 when non-admin updates other user profile."""
    db.return_value.select.return_value.first.return_value = MagicMock(
        as_dict=MagicMock(return_value={"user_id": 42})
    )
    # User 1 trying to update user 42
    resp = await client.put(
        "/api/v1/users/42/profile",
        json={"rate_limit": 100},
        headers=_auth_headers("viewer"),  # user_id is 1
    )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_regenerate_api_key_ok(client, db):
    """Test regenerating API key (happy path)."""
    db.return_value.select.return_value.first.return_value = MagicMock(
        as_dict=MagicMock(return_value={"user_id": 42})
    )
    with patch("routes.user_profiles.generate_api_key") as mock_gen:
        mock_gen.return_value = "new-api-key-12345"
        with patch("routes.user_profiles.encrypt_value") as mock_enc:
            mock_enc.return_value = "encrypted_xyz"
            resp = await client.post(
                "/api/v1/users/42/regenerate-api-key",
                headers=_auth_headers(),
            )
            assert resp.status_code == 200
            data = await resp.get_json()
            assert data["api_key"] == "new-api-key-12345"


@pytest.mark.asyncio
async def test_regenerate_api_key_not_found(client, db):
    """Test 404 when regenerating key for non-existent profile."""
    db.return_value.select.return_value.first.return_value = None
    resp = await client.post(
        "/api/v1/users/999/regenerate-api-key",
        headers=_auth_headers(),
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_regenerate_api_key_self_only(client, db):
    """Test 403 when non-admin regenerates other user key."""
    db.return_value.select.return_value.first.return_value = MagicMock(
        as_dict=MagicMock(return_value={"user_id": 42})
    )
    # User 1 trying to regenerate key for user 42
    resp = await client.post(
        "/api/v1/users/42/regenerate-api-key",
        headers=_auth_headers("viewer"),  # user_id is 1
    )
    assert resp.status_code == 403


# ===========================================================================
# /api/v1/servers — database_servers.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_servers_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    db.return_value.count.return_value = 0
    resp = await client.get("/api/v1/servers", headers=client.get_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data
    assert "meta" in data


@pytest.mark.asyncio
async def test_create_server_missing_fields(client, db):
    resp = await client.post(
        "/api/v1/servers",
        json={"host": "localhost"},
        headers=_auth_headers(),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_server_no_auth(client):
    resp = await client.post("/api/v1/servers", json={"host": "h", "port": 3306})
    assert resp.status_code == 401


# ===========================================================================
# /api/v1/scaling — scaling.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_scaling_policies_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/scaling/policies", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_scaling_policies_no_auth(client):
    resp = await client.get("/api/v1/scaling/policies")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_create_scaling_policy_ok(client, db):
    db.scaling_policy.insert.return_value = 5
    resp = await client.post(
        "/api/v1/scaling/policies",
        json={"name": "pol1", "server_id": 1, "metric": "cpu", "threshold": 80},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 201


@pytest.mark.asyncio
async def test_create_scaling_policy_missing_fields(client, db):
    resp = await client.post(
        "/api/v1/scaling/policies",
        json={"name": "pol1"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_scaling_policy_forbidden_viewer(client):
    resp = await client.post(
        "/api/v1/scaling/policies",
        json={"name": "p", "server_id": 1, "metric": "cpu", "threshold": 50},
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


# ===========================================================================
# /api/v1/cloud — cloud.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_cloud_providers_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/cloud/providers", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_cloud_providers_no_auth(client):
    resp = await client.get("/api/v1/cloud/providers")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_create_cloud_provider_missing_fields(client, db):
    resp = await client.post(
        "/api/v1/cloud/providers",
        json={"name": "aws"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_cloud_provider_forbidden_viewer(client):
    resp = await client.post(
        "/api/v1/cloud/providers",
        json={"name": "aws", "provider_type": "aws"},
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_create_cloud_provider_no_body(client, db):
    resp = await client.post(
        "/api/v1/cloud/providers",
        data=b"",
        headers={**_auth_headers("admin"), "Content-Type": "application/json"},
    )
    assert resp.status_code == 400


# ===========================================================================
# /api/v1/advanced/analytics — analytics.py
# ===========================================================================


@pytest.mark.asyncio
async def test_advanced_analytics_ok(client, db):
    # The analytics route calls db(db.audit_logs.timestamp >= since).count()
    # db.audit_logs.timestamp >= datetime raises TypeError on MagicMock.
    # Patch the entire analytics _do_query to return a known-good dict.
    good_data = {
        "message": "Advanced analytics data",
        "data": {
            "resources": {"total": 0, "active": 0, "by_status": {}},
            "audit_log": {"events_last_7_days": 0},
            "teams": {"total": 0},
            "users": {"active": 0},
        },
    }
    with patch("routes.analytics.get_db", return_value=db):
        with patch("asyncio.to_thread", new=AsyncMock(return_value=good_data)):
            resp = await client.get("/api/v1/advanced/analytics", headers=_auth_headers())
            assert resp.status_code == 200
            data = await resp.get_json()
            assert "data" in data


@pytest.mark.asyncio
async def test_advanced_analytics_no_auth(client):
    resp = await client.get("/api/v1/advanced/analytics")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_enterprise_reports_ok(client, db):
    good_data = {
        "message": "Enterprise reports data",
        "data": {
            "databases": {"total": 0},
            "servers": {"total": 0},
            "security": {"blocked_count": 0, "threat_indicators": 0},
        },
    }
    with patch("asyncio.to_thread", new=AsyncMock(return_value=good_data)):
        resp = await client.get("/api/v1/enterprise/reports", headers=_auth_headers("admin"))
        assert resp.status_code == 200


@pytest.mark.asyncio
async def test_enterprise_reports_forbidden_viewer(client):
    resp = await client.get("/api/v1/enterprise/reports", headers=_auth_headers("viewer"))
    assert resp.status_code == 403


# ===========================================================================
# /api/v1/users/<id>/profile — user_profiles.py
# ===========================================================================


@pytest.mark.asyncio
async def test_get_profile_not_found(client, db):
    db.return_value.select.return_value.first.return_value = None
    resp = await client.get("/api/v1/users/99/profile", headers=_auth_headers())
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_get_profile_ok(client, db):
    row = MagicMock()
    row.as_dict.return_value = {"id": 1, "user_id": 1, "display_name": "Test"}
    db.return_value.select.return_value.first.return_value = row
    resp = await client.get("/api/v1/users/1/profile", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_get_profile_no_auth(client):
    resp = await client.get("/api/v1/users/1/profile")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_update_profile_no_body(client, db):
    resp = await client.put(
        "/api/v1/users/1/profile",
        data=b"",
        headers={**_auth_headers(), "Content-Type": "application/json"},
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_update_profile_no_valid_fields(client, db):
    resp = await client.put(
        "/api/v1/users/1/profile",
        json={"unknown_field": "value"},
        headers=_auth_headers(),
    )
    assert resp.status_code == 400


# ===========================================================================
# /api/v1/permissions — permissions.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_permissions_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/permissions", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_permissions_no_auth(client):
    resp = await client.get("/api/v1/permissions")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_create_permission_ok(client, db):
    db.user_permission.insert.return_value = 7
    resp = await client.post(
        "/api/v1/permissions",
        json={"user_id": 2, "server_id": 1, "permission_level": "read"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 201


@pytest.mark.asyncio
async def test_create_permission_missing_fields(client, db):
    resp = await client.post(
        "/api/v1/permissions",
        json={"user_id": 2},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_permission_forbidden_viewer(client):
    resp = await client.post(
        "/api/v1/permissions",
        json={"user_id": 2, "server_id": 1, "permission_level": "read"},
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


# ===========================================================================
# /api/v1/sql-files — sql_files.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_sql_files_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/sql-files", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_sql_files_no_auth(client):
    resp = await client.get("/api/v1/sql-files")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_create_sql_file_no_body(client, db):
    resp = await client.post(
        "/api/v1/sql-files",
        data=b"",
        headers={**_auth_headers(), "Content-Type": "application/json"},
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_sql_file_missing_fields(client, db):
    resp = await client.post(
        "/api/v1/sql-files",
        json={"filename": "test.sql"},
        headers=_auth_headers(),
    )
    assert resp.status_code == 400


# ===========================================================================
# /api/v1/security-rules — security_rules.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_security_rules_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/security-rules", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_security_rules_no_auth(client):
    resp = await client.get("/api/v1/security-rules")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_create_security_rule_ok(client, db):
    db.security_rule.insert.return_value = 3
    resp = await client.post(
        "/api/v1/security-rules",
        json={"name": "rule1", "rule_type": "ip", "action": "block", "priority": 10},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 201


@pytest.mark.asyncio
async def test_create_security_rule_missing_fields(client, db):
    resp = await client.post(
        "/api/v1/security-rules",
        json={"name": "rule1"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_security_rule_forbidden_viewer(client):
    resp = await client.post(
        "/api/v1/security-rules",
        json={"name": "r", "rule_type": "ip", "action": "block", "priority": 1},
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


# ===========================================================================
# /api/v1/sync — sync.py
# ===========================================================================


@pytest.mark.asyncio
async def test_sync_servers_ok(client, db):
    # sync_to_redis is a sync function; use MagicMock (not AsyncMock) explicitly
    sync_mock = MagicMock(return_value={"synced": 2, "servers": []})
    mock_dblb = MagicMock()
    mock_dblb.reload = MagicMock(return_value=None)
    with patch("routes.sync.sync_to_redis", new=sync_mock), \
         patch("routes.sync.get_dblb_client", return_value=mock_dblb):
        resp = await client.post("/api/v1/sync", headers=_auth_headers())
        assert resp.status_code == 200
        data = await resp.get_json()
        assert "message" in data


@pytest.mark.asyncio
async def test_sync_servers_no_auth(client):
    resp = await client.post("/api/v1/sync")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_sync_redis_failure(client, db):
    # sync_to_redis raises inside asyncio.to_thread → caught as Exception
    sync_mock = MagicMock(side_effect=RuntimeError("redis down"))
    with patch("routes.sync.sync_to_redis", new=sync_mock):
        resp = await client.post("/api/v1/sync", headers=_auth_headers())
        assert resp.status_code == 500
        data = await resp.get_json()
        assert "error" in data


@pytest.mark.asyncio
async def test_sync_dblb_failure(client, db):
    sync_mock = MagicMock(return_value={"synced": 1, "servers": []})
    mock_dblb = MagicMock()
    mock_dblb.reload = MagicMock(side_effect=RuntimeError("dblb unavailable"))
    with patch("routes.sync.sync_to_redis", new=sync_mock), \
         patch("routes.sync.get_dblb_client", return_value=mock_dblb):
        resp = await client.post("/api/v1/sync", headers=_auth_headers())
        assert resp.status_code == 207
        data = await resp.get_json()
        assert "dblb_error" in data


# ===========================================================================
# /api/v1/license — license.py
# ===========================================================================


@pytest.mark.asyncio
async def test_get_license_not_configured(client, db):
    # When no license row exists, route returns 200 with data=None
    db.return_value.select.return_value.first.return_value = None
    resp = await client.get("/api/v1/license", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data.get("data") is None


@pytest.mark.asyncio
async def test_get_license_ok(client, db):
    row = MagicMock()
    row.as_dict.return_value = {
        "id": 1,
        "license_key": "ABCD1234EFGH5678",
        "product": "nest",
        "valid_until": "2027-01-01",
    }
    db.return_value.select.return_value.first.return_value = row
    resp = await client.get("/api/v1/license", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data
    # Key should be masked
    key = data["data"].get("license_key", "")
    assert "****" in key


@pytest.mark.asyncio
async def test_get_license_no_auth(client):
    resp = await client.get("/api/v1/license")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_set_license_no_body(client, db):
    resp = await client.post(
        "/api/v1/license",
        data=b"",
        headers={**_auth_headers("admin"), "Content-Type": "application/json"},
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_set_license_missing_key(client, db):
    resp = await client.post(
        "/api/v1/license",
        json={},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


# ===========================================================================
# /api/v1/threat-intel — threat_intel.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_threat_feeds_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/threat-intel/feeds", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_threat_feeds_no_auth(client):
    resp = await client.get("/api/v1/threat-intel/feeds")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_create_threat_feed_ok(client, db):
    db.threat_intel_feed.insert.return_value = 9
    resp = await client.post(
        "/api/v1/threat-intel/feeds",
        json={"name": "feed1", "url": "https://example.com/feed", "feed_type": "ip"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 201


@pytest.mark.asyncio
async def test_create_threat_feed_missing_fields(client, db):
    resp = await client.post(
        "/api/v1/threat-intel/feeds",
        json={"name": "feed1"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_threat_feed_forbidden_viewer(client):
    resp = await client.post(
        "/api/v1/threat-intel/feeds",
        json={"name": "f", "url": "http://x.com", "feed_type": "ip"},
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


# ===========================================================================
# /api/v1/blocked-databases — blocked_databases.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_blocked_databases_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/blocked-databases", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_blocked_databases_no_auth(client):
    resp = await client.get("/api/v1/blocked-databases")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_add_blocked_database_ok(client, db):
    db.blocked_database.insert.return_value = 11
    resp = await client.post(
        "/api/v1/blocked-databases",
        json={"db_name": "bad_db"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 201


@pytest.mark.asyncio
async def test_add_blocked_database_missing_db_name(client, db):
    resp = await client.post(
        "/api/v1/blocked-databases",
        json={"reason": "spam"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_add_blocked_database_forbidden_viewer(client):
    resp = await client.post(
        "/api/v1/blocked-databases",
        json={"db_name": "bad_db"},
        headers=_auth_headers("viewer"),
    )
    assert resp.status_code == 403


# ===========================================================================
# /api/v1/temporary-access — temporary_access.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_temp_tokens_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/temporary-access", headers=_auth_headers("admin"))
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_temp_tokens_forbidden_viewer(client):
    resp = await client.get("/api/v1/temporary-access", headers=_auth_headers("viewer"))
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_create_temp_token_ok(client, db):
    db.temporary_access_token.insert.return_value = 15
    # db.temporary_access_token[15].as_dict() must return serializable dict
    item = MagicMock()
    item.as_dict.return_value = {"id": 15, "user_id": 2, "server_id": 1, "token": "abc123"}
    db.temporary_access_token.__getitem__ = MagicMock(return_value=item)
    resp = await client.post(
        "/api/v1/temporary-access",
        json={"user_id": 2, "server_id": 1, "ttl_hours": 6},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 201
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_create_temp_token_missing_fields(client, db):
    resp = await client.post(
        "/api/v1/temporary-access",
        json={"ttl_hours": 6},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_temp_token_invalid_ttl(client, db):
    resp = await client.post(
        "/api/v1/temporary-access",
        json={"user_id": 1, "server_id": 1, "ttl_hours": 0},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_temp_token_ttl_too_large(client, db):
    resp = await client.post(
        "/api/v1/temporary-access",
        json={"user_id": 1, "server_id": 1, "ttl_hours": 999},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


# ===========================================================================
# /api/v1/audit-log — audit.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_audit_log_ok(client, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    db.return_value.count.return_value = 0
    resp = await client.get("/api/v1/audit-log", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_audit_log_no_auth(client):
    resp = await client.get("/api/v1/audit-log")
    assert resp.status_code == 401


# ---------------------------------------------------------------------------
# Threat Intel Routes Tests
# ---------------------------------------------------------------------------


# ===========================================================================
# /api/v1/threat-intel — threat_intel.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_threat_feeds_ok(client, db):
    """Test listing threat intel feeds."""
    db.threat_intel_feed.id.__gt__.return_value = MagicMock()
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/threat-intel/feeds", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_create_threat_feed_ok(client, db):
    """Test creating a threat intel feed."""
    db.threat_intel_feed.insert.return_value = 5
    resp = await client.post(
        "/api/v1/threat-intel/feeds",
        json={"name": "OSINT", "url": "https://example.com", "feed_type": "osint"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 201
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_create_threat_feed_missing_fields(client, db):
    """Test creating feed without required fields."""
    resp = await client.post(
        "/api/v1/threat-intel/feeds",
        json={"name": "Incomplete"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_get_threat_feed_ok(client, db):
    """Test getting a single threat feed."""
    resp = await client.get("/api/v1/threat-intel/feeds/42", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_get_threat_feed_not_found(client, db):
    """Test getting non-existent feed."""
    db.threat_intel_feed.__getitem__.return_value = None
    resp = await client.get("/api/v1/threat-intel/feeds/999", headers=_auth_headers())
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_update_threat_feed_ok(client, db):
    """Test updating a threat feed."""
    resp = await client.put(
        "/api/v1/threat-intel/feeds/42",
        json={"name": "Updated"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_update_threat_feed_not_found(client, db):
    """Test updating non-existent feed."""
    db.threat_intel_feed.__getitem__.return_value = None
    resp = await client.put(
        "/api/v1/threat-intel/feeds/999",
        json={"name": "Updated"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_delete_threat_feed_ok(client, db):
    """Test deleting a threat feed."""
    resp = await client.delete("/api/v1/threat-intel/feeds/42", headers=_auth_headers("admin"))
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_delete_threat_feed_not_found(client, db):
    """Test deleting non-existent feed."""
    db.threat_intel_feed.__getitem__.return_value = None
    resp = await client.delete("/api/v1/threat-intel/feeds/999", headers=_auth_headers("admin"))
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_poll_threat_feed_ok(client, db):
    """Test polling a threat feed."""
    resp = await client.post(
        "/api/v1/threat-intel/feeds/42/poll",
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 202


@pytest.mark.asyncio
async def test_poll_threat_feed_not_found(client, db):
    """Test polling non-existent feed."""
    db.threat_intel_feed.__getitem__.return_value = None
    resp = await client.post(
        "/api/v1/threat-intel/feeds/999/poll",
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_list_threat_indicators_ok(client, db):
    """Test listing threat indicators."""
    db.threat_indicator.id.__gt__.return_value = MagicMock()
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/threat-intel/indicators", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_threat_indicators_filtered(client, db):
    """Test listing threat indicators with filters."""
    db.threat_indicator.id.__gt__.return_value = MagicMock()
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get(
        "/api/v1/threat-intel/indicators?type=ip&feed_id=1",
        headers=_auth_headers()
    )
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_list_threat_matches_ok(client, db):
    """Test listing threat matches."""
    db.threat_match.id.__gt__.return_value = MagicMock()
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/threat-intel/matches", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


# ===========================================================================
# /api/v1/scaling — scaling.py
# ===========================================================================


@pytest.mark.asyncio
async def test_list_scaling_policies_ok(client, db):
    """Test listing scaling policies."""
    db.scaling_policy.id.__gt__.return_value = MagicMock()
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/scaling/policies", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_create_scaling_policy_ok(client, db):
    """Test creating a scaling policy."""
    db.scaling_policy.insert.return_value = 3
    resp = await client.post(
        "/api/v1/scaling/policies",
        json={"name": "scale-up", "server_id": 1, "metric": "cpu", "threshold": 80},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 201
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_create_scaling_policy_missing_fields(client, db):
    """Test creating policy without required fields."""
    resp = await client.post(
        "/api/v1/scaling/policies",
        json={"name": "incomplete"},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_get_scaling_policy_ok(client, db):
    """Test getting a single scaling policy."""
    resp = await client.get("/api/v1/scaling/policies/42", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_get_scaling_policy_not_found(client, db):
    """Test getting non-existent policy."""
    db.scaling_policy.__getitem__.return_value = None
    resp = await client.get("/api/v1/scaling/policies/999", headers=_auth_headers())
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_update_scaling_policy_ok(client, db):
    """Test updating a scaling policy."""
    resp = await client.put(
        "/api/v1/scaling/policies/42",
        json={"target_cpu": 90},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_update_scaling_policy_not_found(client, db):
    """Test updating non-existent policy."""
    db.scaling_policy.__getitem__.return_value = None
    resp = await client.put(
        "/api/v1/scaling/policies/999",
        json={"target_cpu": 90},
        headers=_auth_headers("admin"),
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_delete_scaling_policy_ok(client, db):
    """Test deleting a scaling policy."""
    resp = await client.delete("/api/v1/scaling/policies/42", headers=_auth_headers("admin"))
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_delete_scaling_policy_not_found(client, db):
    """Test deleting non-existent policy."""
    db.scaling_policy.__getitem__.return_value = None
    resp = await client.delete("/api/v1/scaling/policies/999", headers=_auth_headers("admin"))
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_list_scaling_events_ok(client, db):
    """Test listing scaling events."""
    db.scaling_event.id.__gt__.return_value = MagicMock()
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get("/api/v1/scaling/events", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_list_scaling_events_filtered(client, db):
    """Test listing scaling events with filters."""
    db.scaling_event.id.__gt__.return_value = MagicMock()
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    resp = await client.get(
        "/api/v1/scaling/events?policy_id=1",
        headers=_auth_headers()
    )
    assert resp.status_code == 200


# ===========================================================================
# /api/v1/analytics — analytics.py
# ===========================================================================


@pytest.mark.asyncio
async def test_advanced_analytics_ok(client, db):
    """Test advanced analytics endpoint."""
    # Mock resource query
    db.resources.id.__gt__.return_value = MagicMock()
    db.resources.status = MagicMock()
    db.resources.id.count.return_value = MagicMock()
    
    # Mock audit logs
    db.audit_logs.timestamp.__ge__.return_value = MagicMock()
    
    # Mock teams/users
    db.teams.deleted_at.__eq__.return_value = MagicMock()
    db.users.is_active.__eq__.return_value = MagicMock()
    
    # Set return values for counts
    db.return_value.select.return_value = []
    db.return_value.count.return_value = 0
    resp = await client.get("/api/v1/advanced/analytics", headers=_auth_headers())
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data or "message" in data


@pytest.mark.asyncio
async def test_advanced_analytics_no_auth(client, db):
    """Test advanced analytics requires auth."""
    resp = await client.get("/api/v1/advanced/analytics")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_enterprise_reports_ok(client, db):
    """Test enterprise reports endpoint."""
    db.blocked_databases.id.__gt__.return_value = MagicMock()
    db.security_rules.id.__gt__.return_value = MagicMock()
    db.certificates.expires_at.__le__.return_value = MagicMock()
    db.certificates.revoked_at.__eq__.return_value = MagicMock()
    db.backup_jobs.status.__eq__.return_value = MagicMock()
    db.backup_jobs.created_at.__ge__.return_value = MagicMock()
    db.provisioning_jobs.created_at.__ge__.return_value = MagicMock()
    
    db.return_value.count.return_value = 0
    resp = await client.get("/api/v1/enterprise/reports", headers=_auth_headers("admin"))
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data or "reports" in data


@pytest.mark.asyncio
async def test_enterprise_reports_requires_admin(client, db):
    """Test enterprise reports requires admin role."""
    resp = await client.get("/api/v1/enterprise/reports", headers=_auth_headers("viewer"))
    assert resp.status_code == 403
