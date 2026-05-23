"""Tests for cloud.py and sql_files.py routes."""
import os
import sys
import types
import json
import pytest
import pytest_asyncio
from unittest.mock import AsyncMock, MagicMock, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("DB_HOST", "localhost")
os.environ.setdefault("DB_NAME", "test_nest")
os.environ.setdefault("DB_USER", "test")
os.environ.setdefault("FIELD_ENCRYPTION_KEY", "Fernet_key_placeholder_32bytes==")


def _install_fake_modules():
    """Inject fake heavy-weight modules so app.py imports cleanly."""
    fake_quart_ext = types.ModuleType("penguin_dal.quart_ext")
    fake_quart_ext.init_dal = MagicMock(return_value=None)
    fake_quart_ext.get_db = MagicMock()
    sys.modules["penguin_dal.quart_ext"] = fake_quart_ext

    fake_dblb = types.ModuleType("clients.dblb_grpc")
    fake_dblb.get_dblb_client = MagicMock(return_value=MagicMock())
    fake_dblb.init_dblb_client = MagicMock(return_value=None)
    fake_dblb.DblbGrpcClient = MagicMock()
    sys.modules["clients.dblb_grpc"] = fake_dblb

    for mod_name, fn_names in [
        ("workers.threat_intel_poller", ["threat_intel_poller_loop"]),
        ("workers.db_health_checker", ["db_health_checker_loop"]),
        ("workers.scaling_evaluator", ["scaling_evaluator_loop"]),
    ]:
        fake_w = types.ModuleType(mod_name)
        for fn in fn_names:
            setattr(fake_w, fn, AsyncMock())
        sys.modules.setdefault(mod_name, fake_w)


_install_fake_modules()

sys.modules.pop("app", None)
import app as _app_module

_application = _app_module.app
_application.config["TESTING"] = True

_GET_DB_TARGETS = [
    "routes.auth.get_db",
    "routes.cloud.get_db",
    "routes.sql_files.get_db",
]


def _make_db(user=None, count=0) -> MagicMock:
    """Build a fresh MagicMock DB returning *user* from any .select().first()."""
    db = MagicMock()

    # Support query building like db(...).select(...)
    select_result = MagicMock()
    select_result.first.return_value = user
    select_result.__iter__ = MagicMock(return_value=iter([]))

    query_result = MagicMock()
    query_result.select.return_value = select_result
    query_result.count.return_value = count
    query_result.__iter__ = MagicMock(return_value=iter([]))

    # When db(...) is called, return the query_result
    db.return_value = query_result

    # Support comparison operators on fields
    cloud_provider_id = MagicMock()
    cloud_provider_id.__gt__ = MagicMock(return_value=True)
    cloud_provider_id.__lt__ = MagicMock(return_value=False)
    cloud_provider_id.__eq__ = MagicMock(return_value=False)

    sql_file_id = MagicMock()
    sql_file_id.__gt__ = MagicMock(return_value=True)
    sql_file_id.__lt__ = MagicMock(return_value=False)

    db.users = MagicMock()
    db.users.id = MagicMock()
    db.users.username = MagicMock()
    db.users.email = MagicMock()
    db.users.insert = MagicMock(return_value=1)

    db.cloud_provider = MagicMock()
    db.cloud_provider.insert = MagicMock(return_value=42)
    db.cloud_provider.id = cloud_provider_id
    db.cloud_provider.name = MagicMock()
    db.cloud_provider.__getitem__ = MagicMock(
        return_value=MagicMock(as_dict=MagicMock(return_value={"id": 42}))
    )
    db.cloud_provider.active = MagicMock()

    db.sql_file = MagicMock()
    db.sql_file.insert = MagicMock(return_value=100)
    db.sql_file.id = sql_file_id
    db.sql_file.__getitem__ = MagicMock(
        return_value=MagicMock(as_dict=MagicMock(return_value={"id": 100}))
    )
    db.sql_file.active = MagicMock()

    db.threat_intel_feed = MagicMock()
    db.threat_intel_indicator = MagicMock()
    db.scaling_event = MagicMock()
    db.database_server = MagicMock()
    db.scaling_policy = MagicMock()

    db.commit = MagicMock()
    db.executesql = MagicMock(return_value=[])
    return db


def _make_user(
    user_id: int = 1,
    username: str = "testuser",
    email: str = "test@example.com",
    role: str = "admin",
    is_active: bool = True,
    password_hash: str = None,
) -> MagicMock:
    """Build a mock user record."""
    from werkzeug.security import generate_password_hash
    user = MagicMock()
    user.id = user_id
    user.username = username
    user.email = email
    user.role = role
    user.is_active = is_active
    user.password_hash = password_hash or generate_password_hash("testpassword")
    user.as_dict = MagicMock(
        return_value={
            "id": user_id,
            "username": username,
            "email": email,
            "role": role,
            "is_active": is_active,
        }
    )
    return user


def _make_token(role: str = "admin") -> str:
    from utils.auth import create_token
    return create_token(user_id=1, email="test@example.com", role=role)


@pytest.fixture()
def app():
    return _application


@pytest.fixture()
def db(request):
    """Per-test DB mock; patches all known get_db call sites."""
    mock = _make_db()
    patches = [patch(t, return_value=mock) for t in _GET_DB_TARGETS]
    for p in patches:
        p.start()
    yield mock
    for p in patches:
        p.stop()


class TestCloudProviderRoutes:
    """Tests for /cloud/providers endpoints."""

    @pytest.mark.asyncio
    async def test_list_providers_success(self, app, db):
        """Test GET /api/v1/cloud/providers returns provider list."""
        provider_row = MagicMock()
        provider_row.as_dict = MagicMock(
            return_value={
                "id": 42,
                "name": "AWS",
                "provider_type": "aws",
                "region": "us-east-1",
                "credentials_encrypted": "***",
            }
        )
        db.return_value.select.return_value = [provider_row]

        token = _make_token("admin")
        client = app.test_client()
        resp = await client.get(
            "/api/v1/cloud/providers", headers={"Authorization": f"Bearer {token}"}
        )
        assert resp.status_code == 200
        data = await resp.get_json()
        assert "data" in data

    @pytest.mark.asyncio
    async def test_create_provider_success(self, app, db):
        """Test POST /api/v1/cloud/providers creates a provider."""
        provider_row = MagicMock()
        provider_row.as_dict = MagicMock(
            return_value={
                "id": 42,
                "name": "AWS",
                "provider_type": "aws",
                "region": "us-east-1",
            }
        )
        db.cloud_provider.__getitem__.return_value = provider_row

        token = _make_token("admin")
        client = app.test_client()
        with patch("routes.cloud.encrypt_value", return_value="encrypted_creds"):
            payload = {
                "name": "AWS",
                "provider_type": "aws",
                "region": "us-east-1",
                "credentials": "some_creds",
            }
            resp = await client.post(
                "/api/v1/cloud/providers",
                json=payload,
                headers={"Authorization": f"Bearer {token}"},
            )
            assert resp.status_code in [200, 201]

    @pytest.mark.asyncio
    async def test_create_provider_missing_fields(self, app, db):
        """Test POST /api/v1/cloud/providers rejects missing fields."""
        token = _make_token("admin")
        client = app.test_client()
        payload = {"name": "AWS"}
        resp = await client.post(
            "/api/v1/cloud/providers",
            json=payload,
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_create_provider_no_body(self, app, db):
        """Test POST /api/v1/cloud/providers rejects empty body."""
        token = _make_token("admin")
        client = app.test_client()
        resp = await client.post(
            "/api/v1/cloud/providers",
            json=None,
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_list_providers_empty(self, app, db):
        """Test GET /api/v1/cloud/providers returns empty list."""
        db.return_value.select.return_value = []

        token = _make_token("admin")
        client = app.test_client()
        resp = await client.get(
            "/api/v1/cloud/providers", headers={"Authorization": f"Bearer {token}"}
        )
        assert resp.status_code == 200
        data = await resp.get_json()
        assert "data" in data


class TestSqlFileRoutes:
    """Tests for /sql-files endpoints."""

    @pytest.mark.asyncio
    async def test_list_sql_files_success(self, app, db):
        """Test GET /api/v1/sql-files returns file list."""
        file_row = MagicMock()
        file_row.as_dict = MagicMock(
            return_value={
                "id": 100,
                "filename": "query.sql",
                "sha256_hash": "abc123",
                "file_size": 256,
                "created_at": "2025-01-22T00:00:00Z",
                "created_by": 1,
                "is_valid": True,
            }
        )
        db.return_value.select.return_value = [file_row]

        token = _make_token("admin")
        client = app.test_client()
        resp = await client.get(
            "/api/v1/sql-files", headers={"Authorization": f"Bearer {token}"}
        )
        assert resp.status_code == 200
        data = await resp.get_json()
        assert "data" in data

    @pytest.mark.asyncio
    async def test_upload_sql_file_success(self, app, db):
        """Test POST /api/v1/sql-files uploads a SQL file."""
        file_row = MagicMock()
        file_row.as_dict = MagicMock(
            return_value={"id": 100, "filename": "query.sql"}
        )
        db.sql_file.__getitem__.return_value = file_row

        token = _make_token("admin")
        client = app.test_client()
        payload = {"filename": "query.sql", "content": "SELECT * FROM users;"}
        resp = await client.post(
            "/api/v1/sql-files",
            json=payload,
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code in [200, 201, 202]

    @pytest.mark.asyncio
    async def test_upload_sql_file_missing_filename(self, app, db):
        """Test POST /api/v1/sql-files rejects missing filename."""
        token = _make_token("admin")
        client = app.test_client()
        payload = {"content": "SELECT * FROM users;"}
        resp = await client.post(
            "/api/v1/sql-files",
            json=payload,
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_upload_sql_file_missing_content(self, app, db):
        """Test POST /api/v1/sql-files rejects missing content."""
        token = _make_token("admin")
        client = app.test_client()
        payload = {"filename": "query.sql"}
        resp = await client.post(
            "/api/v1/sql-files",
            json=payload,
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_upload_sql_file_no_body(self, app, db):
        """Test POST /api/v1/sql-files rejects empty body."""
        token = _make_token("admin")
        client = app.test_client()
        resp = await client.post(
            "/api/v1/sql-files",
            json=None,
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_list_sql_files_empty(self, app, db):
        """Test GET /api/v1/sql-files returns empty list."""
        db.return_value.select.return_value = []

        token = _make_token("admin")
        client = app.test_client()
        resp = await client.get(
            "/api/v1/sql-files", headers={"Authorization": f"Bearer {token}"}
        )
        assert resp.status_code == 200

    @pytest.mark.asyncio
    async def test_upload_sql_file_large_content(self, app, db):
        """Test POST /api/v1/sql-files with large SQL content."""
        file_row = MagicMock()
        file_row.as_dict = MagicMock(
            return_value={"id": 100, "filename": "large.sql"}
        )
        db.sql_file.__getitem__.return_value = file_row

        token = _make_token("admin")
        client = app.test_client()
        large_sql = "SELECT * FROM users;\n" * 1000
        payload = {"filename": "large.sql", "content": large_sql}
        resp = await client.post(
            "/api/v1/sql-files",
            json=payload,
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code in [200, 201, 202]

    @pytest.mark.asyncio
    async def test_upload_sql_file_whitespace_filename(self, app, db):
        """Test POST /api/v1/sql-files rejects whitespace-only filename."""
        token = _make_token("admin")
        client = app.test_client()
        payload = {"filename": "   ", "content": "SELECT 1;"}
        resp = await client.post(
            "/api/v1/sql-files",
            json=payload,
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 400
