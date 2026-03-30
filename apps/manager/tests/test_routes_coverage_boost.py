"""Coverage boost tests for cloud, permissions, security_rules, sql_files, blocked_databases, temporary_access routes."""
import os
import sys
import types
import pytest
import pytest_asyncio
from unittest.mock import AsyncMock, MagicMock, patch
from datetime import datetime, timezone, timedelta

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


def _install_fake_modules():
    """Inject fake heavy-weight modules so app.py imports cleanly."""
    # penguin_dal.quart_ext
    fake_quart_ext = types.ModuleType("penguin_dal.quart_ext")
    fake_quart_ext.init_dal = MagicMock(return_value=None)
    fake_quart_ext.get_db = MagicMock()
    sys.modules["penguin_dal.quart_ext"] = fake_quart_ext

    # clients.dblb_grpc
    fake_dblb = types.ModuleType("clients.dblb_grpc")
    fake_dblb.get_dblb_client = MagicMock(return_value=MagicMock())
    fake_dblb.init_dblb_client = MagicMock(return_value=None)
    fake_dblb.DblbGrpcClient = MagicMock()
    sys.modules["clients.dblb_grpc"] = fake_dblb

    # Worker modules
    for mod_name in [
        "workers.threat_intel_poller",
        "workers.db_health_checker",
        "workers.scaling_evaluator",
        "workers.backup_scheduler",
        "workers.cert_rotation",
        "workers.stats_collector",
        "workers.user_sync",
    ]:
        fake_w = types.ModuleType(mod_name)
        fake_w.threat_intel_poller_loop = AsyncMock()
        fake_w.db_health_checker_loop = AsyncMock()
        fake_w.scaling_evaluator_loop = AsyncMock()
        sys.modules.setdefault(mod_name, fake_w)


_install_fake_modules()

# Import the app now that fakes are in place
sys.modules.pop("app", None)
import app as _app_module

_application = _app_module.app
_application.config["TESTING"] = True


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_db() -> MagicMock:
    """Build a fresh MagicMock DB."""
    db = MagicMock()

    # Setup table objects with insert/getitem and comparison support
    for table_name in [
        "cloud_provider",
        "cloud_instance",
        "user_permission",
        "security_rule",
        "sql_file",
        "blocked_database",
        "temporary_access_token",
    ]:
        table = MagicMock()
        table.insert = MagicMock(return_value=1)
        table.__getitem__ = MagicMock()
        # Support comparison operators for queries like db.table.id > 0
        table.id = MagicMock()
        table.id.__gt__ = MagicMock(return_value=True)
        table.id.__lt__ = MagicMock(return_value=False)
        table.id.__eq__ = MagicMock(return_value=True)
        table.id.__ne__ = MagicMock(return_value=False)
        setattr(db, table_name, table)

    # Make db(...) chainable for queries
    def _db_query(condition):
        query = MagicMock()
        query.select = MagicMock(return_value=[])
        query.delete = MagicMock(return_value=None)
        query.update = MagicMock(return_value=None)
        query.count = MagicMock(return_value=0)
        return query

    db.side_effect = _db_query

    db.commit = MagicMock(return_value=None)
    db.__getitem__ = MagicMock()

    return db


def _create_token(role="admin"):
    """Create a valid JWT token for testing."""
    from utils.auth import create_token
    return create_token(user_id=1, email="test@example.com", role=role)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def mock_db():
    """Create a mock database with table support."""
    return _make_db()


@pytest.fixture
def app_client(mock_db):
    """Create test client with mocked database."""
    # Patch all get_db references
    with patch("routes.cloud.get_db", return_value=mock_db):
        with patch("routes.permissions.get_db", return_value=mock_db):
            with patch("routes.security_rules.get_db", return_value=mock_db):
                with patch("routes.sql_files.get_db", return_value=mock_db):
                    with patch("routes.blocked_databases.get_db", return_value=mock_db):
                        with patch("routes.temporary_access.get_db", return_value=mock_db):
                            yield _application.test_client()


# ---------------------------------------------------------------------------
# CLOUD PROVIDER TESTS
# ---------------------------------------------------------------------------


class TestCloudProviders:
    """Test cloud provider routes."""

    @pytest.mark.asyncio
    async def test_list_providers_success(self, app_client, mock_db):
        """List providers returns 200 with data."""
        token = _create_token("admin")
        provider_row = MagicMock()
        provider_row.as_dict = MagicMock(
            return_value={"id": 1, "name": "aws", "provider_type": "cloud"}
        )

        def _mock_select(orderby=None):
            return [provider_row]

        mock_query = MagicMock()
        mock_query.select = _mock_select
        mock_db.side_effect = lambda *a, **k: mock_query

        response = await app_client.get(
            "/api/v1/cloud/providers",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_create_provider_success(self, app_client, mock_db):
        """Create provider with valid data returns 201."""
        token = _create_token("admin")
        provider_row = MagicMock()
        provider_row.as_dict = MagicMock(
            return_value={"id": 1, "name": "aws", "provider_type": "cloud"}
        )
        mock_db.cloud_provider.__getitem__.return_value = provider_row

        response = await app_client.post(
            "/api/v1/cloud/providers",
            json={"name": "aws", "provider_type": "cloud"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code in [200, 201]

    @pytest.mark.asyncio
    async def test_create_provider_missing_fields(self, app_client):
        """Create provider without required fields returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/cloud/providers",
            json={"name": "aws"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_get_provider_success(self, app_client, mock_db):
        """Get provider returns 200 when found."""
        token = _create_token("admin")
        provider_row = MagicMock()
        provider_row.as_dict = MagicMock(
            return_value={"id": 1, "name": "aws", "provider_type": "cloud"}
        )
        mock_db.cloud_provider.__getitem__.return_value = provider_row

        response = await app_client.get(
            "/api/v1/cloud/providers/1",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_get_provider_not_found(self, app_client, mock_db):
        """Get provider returns 404 when not found."""
        token = _create_token("admin")
        mock_db.cloud_provider.__getitem__.return_value = None

        response = await app_client.get(
            "/api/v1/cloud/providers/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_update_provider_not_found(self, app_client, mock_db):
        """Update provider returns 404 when not found."""
        token = _create_token("admin")
        mock_db.cloud_provider.__getitem__.return_value = None

        response = await app_client.put(
            "/api/v1/cloud/providers/999",
            json={"name": "updated"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_delete_provider_not_found(self, app_client, mock_db):
        """Delete provider returns 404 when not found."""
        token = _create_token("admin")
        mock_db.cloud_provider.__getitem__.return_value = None

        response = await app_client.delete(
            "/api/v1/cloud/providers/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_list_instances_success(self, app_client, mock_db):
        """List instances returns 200."""
        token = _create_token("admin")
        instance_row = MagicMock()
        instance_row.as_dict = MagicMock(return_value={"id": 1, "instance_id": "i-123"})

        def _mock_select(orderby=None):
            return [instance_row]

        mock_query = MagicMock()
        mock_query.select = _mock_select
        mock_db.side_effect = lambda *a, **k: mock_query

        response = await app_client.get(
            "/api/v1/cloud/instances",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_create_instance_missing_fields(self, app_client):
        """Create instance without required fields returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/cloud/instances",
            json={"provider_id": 1},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_update_instance_not_found(self, app_client, mock_db):
        """Update instance returns 404 when not found."""
        token = _create_token("admin")
        mock_db.cloud_instance.__getitem__.return_value = None

        response = await app_client.put(
            "/api/v1/cloud/instances/999",
            json={"status": "running"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404


# ---------------------------------------------------------------------------
# PERMISSIONS TESTS
# ---------------------------------------------------------------------------


class TestPermissions:
    """Test permission routes."""

    @pytest.mark.asyncio
    async def test_list_permissions_success(self, app_client, mock_db):
        """List permissions returns 200."""
        token = _create_token("admin")
        perm_row = MagicMock()
        perm_row.as_dict = MagicMock(return_value={"id": 1, "user_id": 1})

        def _mock_select(orderby=None):
            return [perm_row]

        mock_query = MagicMock()
        mock_query.select = _mock_select
        mock_db.side_effect = lambda *a, **k: mock_query

        response = await app_client.get(
            "/api/v1/permissions",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_list_permissions_filtered_by_user(self, app_client, mock_db):
        """List permissions filtered by user_id."""
        token = _create_token("admin")
        perm_row = MagicMock()
        perm_row.as_dict = MagicMock(return_value={"id": 1, "user_id": 5})

        def _mock_select(orderby=None):
            return [perm_row]

        mock_query = MagicMock()
        mock_query.select = _mock_select
        mock_db.side_effect = lambda *a, **k: mock_query

        response = await app_client.get(
            "/api/v1/permissions?user_id=5",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_create_permission_success(self, app_client, mock_db):
        """Create permission returns 201."""
        token = _create_token("admin")
        perm_row = MagicMock()
        perm_row.as_dict = MagicMock(return_value={"id": 1, "user_id": 1, "server_id": 1})
        mock_db.user_permission.__getitem__.return_value = perm_row

        response = await app_client.post(
            "/api/v1/permissions",
            json={"user_id": 1, "server_id": 1, "permission_level": "read"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 201

    @pytest.mark.asyncio
    async def test_create_permission_missing_fields(self, app_client):
        """Create permission without required fields returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/permissions",
            json={"user_id": 1},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_get_permission_success(self, app_client, mock_db):
        """Get permission returns 200 when found."""
        token = _create_token("admin")
        perm_row = MagicMock()
        perm_row.as_dict = MagicMock(return_value={"id": 1, "user_id": 1})
        mock_db.user_permission.__getitem__.return_value = perm_row

        response = await app_client.get(
            "/api/v1/permissions/1",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_get_permission_not_found(self, app_client, mock_db):
        """Get permission returns 404 when not found."""
        token = _create_token("admin")
        mock_db.user_permission.__getitem__.return_value = None

        response = await app_client.get(
            "/api/v1/permissions/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_delete_permission_success(self, app_client, mock_db):
        """Delete permission returns 200 when successful."""
        token = _create_token("admin")
        perm_row = MagicMock()
        mock_db.user_permission.__getitem__.return_value = perm_row

        response = await app_client.delete(
            "/api/v1/permissions/1",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_delete_permission_not_found(self, app_client, mock_db):
        """Delete permission returns 404 when not found."""
        token = _create_token("admin")
        mock_db.user_permission.__getitem__.return_value = None

        response = await app_client.delete(
            "/api/v1/permissions/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_get_user_permissions(self, app_client, mock_db):
        """Get all permissions for a user returns 200."""
        token = _create_token("admin")
        perm_row = MagicMock()
        perm_row.as_dict = MagicMock(return_value={"id": 1, "user_id": 5})

        def _mock_select(orderby=None):
            return [perm_row]

        mock_query = MagicMock()
        mock_query.select = _mock_select
        mock_db.side_effect = lambda *a, **k: mock_query

        response = await app_client.get(
            "/api/v1/users/5/permissions",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200


# ---------------------------------------------------------------------------
# SECURITY RULES TESTS
# ---------------------------------------------------------------------------


class TestSecurityRules:
    """Test security rule routes."""

    @pytest.mark.asyncio
    async def test_list_security_rules_success(self, app_client, mock_db):
        """List rules returns 200."""
        token = _create_token("admin")
        rule_row = MagicMock()
        rule_row.as_dict = MagicMock(return_value={"id": 1, "name": "rule1"})

        def _mock_select(orderby=None):
            return [rule_row]

        mock_query = MagicMock()
        mock_query.select = _mock_select
        mock_db.side_effect = lambda *a, **k: mock_query

        response = await app_client.get(
            "/api/v1/security-rules",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_create_security_rule_success(self, app_client, mock_db):
        """Create rule returns 201."""
        token = _create_token("admin")
        rule_row = MagicMock()
        rule_row.as_dict = MagicMock(
            return_value={"id": 1, "name": "rule1", "rule_type": "ip"}
        )
        mock_db.security_rule.__getitem__.return_value = rule_row

        response = await app_client.post(
            "/api/v1/security-rules",
            json={
                "name": "rule1",
                "rule_type": "ip",
                "action": "block",
                "priority": 1,
            },
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 201

    @pytest.mark.asyncio
    async def test_create_security_rule_missing_fields(self, app_client):
        """Create rule without required fields returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/security-rules",
            json={"name": "rule1"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_get_security_rule_success(self, app_client, mock_db):
        """Get rule returns 200 when found."""
        token = _create_token("admin")
        rule_row = MagicMock()
        rule_row.as_dict = MagicMock(return_value={"id": 1, "name": "rule1"})
        mock_db.security_rule.__getitem__.return_value = rule_row

        response = await app_client.get(
            "/api/v1/security-rules/1",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_get_security_rule_not_found(self, app_client, mock_db):
        """Get rule returns 404 when not found."""
        token = _create_token("admin")
        mock_db.security_rule.__getitem__.return_value = None

        response = await app_client.get(
            "/api/v1/security-rules/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_update_security_rule_not_found(self, app_client, mock_db):
        """Update rule returns 404 when not found."""
        token = _create_token("admin")
        mock_db.security_rule.__getitem__.return_value = None

        response = await app_client.put(
            "/api/v1/security-rules/999",
            json={"name": "updated"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_delete_security_rule_not_found(self, app_client, mock_db):
        """Delete rule returns 404 when not found."""
        token = _create_token("admin")
        mock_db.security_rule.__getitem__.return_value = None

        response = await app_client.delete(
            "/api/v1/security-rules/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404


# ---------------------------------------------------------------------------
# SQL FILES TESTS
# ---------------------------------------------------------------------------


class TestSqlFiles:
    """Test SQL file routes."""

    @pytest.mark.asyncio
    async def test_list_sql_files_success(self, app_client, mock_db):
        """List SQL files returns 200."""
        token = _create_token("admin")
        file_row = MagicMock()
        file_row.as_dict = MagicMock(return_value={"id": 1, "filename": "test.sql"})

        def _mock_select(*args, **kwargs):
            return [file_row]

        mock_query = MagicMock()
        mock_query.select = _mock_select
        mock_db.side_effect = lambda *a, **k: mock_query

        response = await app_client.get(
            "/api/v1/sql-files",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_upload_sql_file_success(self, app_client, mock_db):
        """Upload SQL file returns 201."""
        token = _create_token("admin")
        file_row = MagicMock()
        file_row.as_dict = MagicMock(
            return_value={"id": 1, "filename": "test.sql", "sha256_hash": "abc123"}
        )
        mock_db.sql_file.__getitem__.return_value = file_row

        response = await app_client.post(
            "/api/v1/sql-files",
            json={"filename": "test.sql", "content": "SELECT * FROM users;"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 201

    @pytest.mark.asyncio
    async def test_upload_sql_file_missing_filename(self, app_client):
        """Upload SQL file without filename returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/sql-files",
            json={"content": "SELECT * FROM users;"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_upload_sql_file_missing_content(self, app_client):
        """Upload SQL file without content returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/sql-files",
            json={"filename": "test.sql"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_get_sql_file_success(self, app_client, mock_db):
        """Get SQL file returns 200 when found."""
        token = _create_token("admin")
        file_row = MagicMock()
        file_row.as_dict = MagicMock(return_value={"id": 1, "filename": "test.sql"})
        mock_db.sql_file.__getitem__.return_value = file_row

        response = await app_client.get(
            "/api/v1/sql-files/1",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_get_sql_file_not_found(self, app_client, mock_db):
        """Get SQL file returns 404 when not found."""
        token = _create_token("admin")
        mock_db.sql_file.__getitem__.return_value = None

        response = await app_client.get(
            "/api/v1/sql-files/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_delete_sql_file_not_found(self, app_client, mock_db):
        """Delete SQL file returns 404 when not found."""
        token = _create_token("admin")
        mock_db.sql_file.__getitem__.return_value = None

        response = await app_client.delete(
            "/api/v1/sql-files/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_execute_sql_file_not_implemented(self, app_client, mock_db):
        """Execute SQL file returns 501."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/sql-files/1/execute",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 501


# ---------------------------------------------------------------------------
# BLOCKED DATABASES TESTS
# ---------------------------------------------------------------------------


class TestBlockedDatabases:
    """Test blocked database routes."""

    @pytest.mark.asyncio
    async def test_list_blocked_databases_success(self, app_client, mock_db):
        """List blocked databases returns 200."""
        token = _create_token("admin")
        block_row = MagicMock()
        block_row.as_dict = MagicMock(return_value={"id": 1, "db_name": "prod"})

        def _mock_select(orderby=None):
            return [block_row]

        mock_query = MagicMock()
        mock_query.select = _mock_select
        mock_db.side_effect = lambda *a, **k: mock_query

        response = await app_client.get(
            "/api/v1/blocked-databases",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_add_blocked_database_success(self, app_client, mock_db):
        """Add blocked database returns 201."""
        token = _create_token("admin")
        block_row = MagicMock()
        block_row.as_dict = MagicMock(
            return_value={"id": 1, "db_name": "prod", "reason": "protect"}
        )
        mock_db.blocked_database.__getitem__.return_value = block_row

        response = await app_client.post(
            "/api/v1/blocked-databases",
            json={"db_name": "prod", "reason": "protect"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 201

    @pytest.mark.asyncio
    async def test_add_blocked_database_missing_fields(self, app_client):
        """Add blocked database without required fields returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/blocked-databases",
            json={"reason": "protect"},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_get_blocked_database_success(self, app_client, mock_db):
        """Get blocked database returns 200 when found."""
        token = _create_token("admin")
        block_row = MagicMock()
        block_row.as_dict = MagicMock(return_value={"id": 1, "db_name": "prod"})
        mock_db.blocked_database.__getitem__.return_value = block_row

        response = await app_client.get(
            "/api/v1/blocked-databases/1",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_get_blocked_database_not_found(self, app_client, mock_db):
        """Get blocked database returns 404 when not found."""
        token = _create_token("admin")
        mock_db.blocked_database.__getitem__.return_value = None

        response = await app_client.get(
            "/api/v1/blocked-databases/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_remove_blocked_database_not_found(self, app_client, mock_db):
        """Remove blocked database returns 404 when not found."""
        token = _create_token("admin")
        mock_db.blocked_database.__getitem__.return_value = None

        response = await app_client.delete(
            "/api/v1/blocked-databases/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404


# ---------------------------------------------------------------------------
# TEMPORARY ACCESS TESTS
# ---------------------------------------------------------------------------


class TestTemporaryAccess:
    """Test temporary access token routes."""

    @pytest.mark.asyncio
    async def test_list_temp_tokens_success(self, app_client, mock_db):
        """List tokens returns 200."""
        token = _create_token("admin")
        token_row = MagicMock()
        token_row.as_dict = MagicMock(return_value={"id": 1, "token": "abc123"})

        def _mock_select(orderby=None):
            return [token_row]

        mock_query = MagicMock()
        mock_query.select = _mock_select
        mock_db.side_effect = lambda *a, **k: mock_query

        response = await app_client.get(
            "/api/v1/temporary-access",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_create_temp_token_success(self, app_client, mock_db):
        """Create token returns 201."""
        token = _create_token("admin")
        token_row = MagicMock()
        token_row.as_dict = MagicMock(return_value={"id": 1, "token": "abc123"})
        mock_db.temporary_access_token.__getitem__.return_value = token_row

        response = await app_client.post(
            "/api/v1/temporary-access",
            json={"user_id": 1, "server_id": 1, "ttl_hours": 24},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 201

    @pytest.mark.asyncio
    async def test_create_temp_token_invalid_ttl_too_low(self, app_client):
        """Create token with ttl_hours < 1 returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/temporary-access",
            json={"user_id": 1, "server_id": 1, "ttl_hours": 0},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_create_temp_token_invalid_ttl_too_high(self, app_client):
        """Create token with ttl_hours > 720 returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/temporary-access",
            json={"user_id": 1, "server_id": 1, "ttl_hours": 721},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_create_temp_token_missing_fields(self, app_client):
        """Create token without required fields returns 400."""
        token = _create_token("admin")
        response = await app_client.post(
            "/api/v1/temporary-access",
            json={"user_id": 1},
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 400

    @pytest.mark.asyncio
    async def test_get_temp_token_success(self, app_client, mock_db):
        """Get token returns 200 when found."""
        token = _create_token("admin")
        token_row = MagicMock()
        token_row.as_dict = MagicMock(return_value={"id": 1, "token": "abc123"})
        mock_db.temporary_access_token.__getitem__.return_value = token_row

        response = await app_client.get(
            "/api/v1/temporary-access/1",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_get_temp_token_not_found(self, app_client, mock_db):
        """Get token returns 404 when not found."""
        token = _create_token("admin")
        mock_db.temporary_access_token.__getitem__.return_value = None

        response = await app_client.get(
            "/api/v1/temporary-access/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_delete_temp_token_not_found(self, app_client, mock_db):
        """Delete token returns 404 when not found."""
        token = _create_token("admin")
        mock_db.temporary_access_token.__getitem__.return_value = None

        response = await app_client.delete(
            "/api/v1/temporary-access/999",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404

    @pytest.mark.asyncio
    async def test_revoke_temp_token_success(self, app_client, mock_db):
        """Revoke token returns 200 when successful."""
        token = _create_token("admin")
        token_row = MagicMock()
        token_row.as_dict = MagicMock(
            return_value={"id": 1, "token": "abc123", "used_at": datetime.now(timezone.utc)}
        )
        mock_db.temporary_access_token.__getitem__.return_value = token_row

        response = await app_client.post(
            "/api/v1/temporary-access/1/revoke",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 200

    @pytest.mark.asyncio
    async def test_revoke_temp_token_not_found(self, app_client, mock_db):
        """Revoke token returns 404 when not found."""
        token = _create_token("admin")
        mock_db.temporary_access_token.__getitem__.return_value = None

        response = await app_client.post(
            "/api/v1/temporary-access/999/revoke",
            headers={"Authorization": f"Bearer {token}"},
        )
        assert response.status_code == 404
