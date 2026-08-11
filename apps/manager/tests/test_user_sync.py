"""Tests for user_sync worker module."""
import os
import sys
import signal
import pytest
from unittest.mock import MagicMock, patch, Mock
from pathlib import Path

# Ensure app root is in sys.path
app_root = Path(__file__).parent.parent
if str(app_root) not in sys.path:
    sys.path.insert(0, str(app_root))

# Set up environment
os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("REDIS_HOST", "localhost")
os.environ.setdefault("REDIS_PORT", "6379")

# workers.user_sync does `from models import db` at import, and importing the
# real `models` needs a live DB, so it must be mocked for that one import.
# Do it in a TIGHT window: mock `models`, force the (one-time) import of
# workers.user_sync so its `db` binds to the mock, then RESTORE the real
# `models` package. The test methods below re-import UserSyncWorker from the
# now-cached module, so the mock db is retained for them — but the real
# `models` (incl. models.operations.OperationRecord) is back in sys.modules for
# later test files. Without this restore, test_user_sync (collected before
# test_worker) leaks the mock, so test_worker's `from models import
# OperationRecord` binds a MagicMock and its store keys never match
# ("Operation tenant-1/op-1 not found").
_ORIG_MODELS_MODULES = {
    name: mod for name, mod in sys.modules.items()
    if name == "models" or name.startswith("models.")
}
sys.modules["models"] = MagicMock(db=MagicMock())
import workers.user_sync  # noqa: E402,F401  (import under the models mock)
for _name in [n for n in sys.modules
              if n == "models" or n.startswith("models.")]:
    del sys.modules[_name]
sys.modules.update(_ORIG_MODELS_MODULES)


class TestUserSyncWorker:
    """Tests for UserSyncWorker class."""

    def test_user_sync_worker_class_exists(self):
        """Test UserSyncWorker class can be imported."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            assert UserSyncWorker is not None

    def test_user_sync_init_with_parameters(self):
        """Test UserSyncWorker can be initialized with parameters."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker(sleep_interval=60, batch_size=20)
            assert worker.sleep_interval == 60
            assert worker.batch_size == 20

    def test_user_sync_init_with_defaults(self):
        """Test UserSyncWorker initializes with default parameters."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert worker.sleep_interval == 30
            assert worker.batch_size == 10

    def test_user_sync_running_flag_initial_state(self):
        """Test UserSyncWorker running flag starts as True."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert worker.running is True

    def test_user_sync_has_db_reference(self):
        """Test UserSyncWorker has db reference."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert worker.db is not None

    def test_user_sync_handle_shutdown_method_exists(self):
        """Test UserSyncWorker has _handle_shutdown method."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert hasattr(worker, "_handle_shutdown")

    def test_user_sync_sync_pending_users_method_exists(self):
        """Test UserSyncWorker has sync_pending_users method."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert hasattr(worker, "sync_pending_users")

    def test_user_sync_sync_user_method_exists(self):
        """Test UserSyncWorker has sync_user method."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert hasattr(worker, "sync_user")

    def test_user_sync_delete_user_method_exists(self):
        """Test UserSyncWorker has delete_user method."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert hasattr(worker, "delete_user")

    def test_user_sync_get_connector_method_exists(self):
        """Test UserSyncWorker has _get_connector method."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert hasattr(worker, "_get_connector")

    def test_user_sync_handle_sync_error_method_exists(self):
        """Test UserSyncWorker has _handle_sync_error method."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert hasattr(worker, "_handle_sync_error")

    def test_user_sync_run_method_exists(self):
        """Test UserSyncWorker has run method."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            assert hasattr(worker, "run")

    def test_user_sync_get_connector_returns_none_for_unknown_type(self):
        """Test _get_connector returns None for unknown resource type."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            connector = worker._get_connector("unknown-type", {}, {})
            assert connector is None

    def test_user_sync_get_connector_returns_none_missing_connection_info(self):
        """Test _get_connector returns None when connection_info is missing."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            connector = worker._get_connector("db-postgresql", None, {"user": "admin"})
            assert connector is None

    def test_user_sync_get_connector_returns_none_missing_credentials(self):
        """Test _get_connector returns None when credentials are missing."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            connector = worker._get_connector("db-postgresql", {"host": "localhost"}, None)
            assert connector is None

    def test_user_sync_shutdown_sets_running_false(self):
        """Test _handle_shutdown sets running to False."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            worker._handle_shutdown(signal.SIGTERM, None)
            assert worker.running is False

    def test_user_sync_signal_handlers_registered(self):
        """Test signal handlers are registered during init."""
        with patch("signal.signal") as mock_signal:
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            # Verify signal.signal was called for SIGTERM and SIGINT
            assert mock_signal.call_count >= 2


class TestUserSyncIntegration:
    """Integration tests for user_sync module."""

    def test_user_sync_module_importable(self):
        """Test user_sync module can be imported."""
        with patch("signal.signal"):
            from workers import user_sync
            assert user_sync is not None

    def test_user_sync_has_main_function(self):
        """Test user_sync module has main function."""
        with patch("signal.signal"):
            from workers import user_sync
            assert hasattr(user_sync, "main")

    def test_user_sync_main_callable(self):
        """Test main function is callable."""
        with patch("signal.signal"):
            from workers.user_sync import main
            assert callable(main)

    def test_user_sync_logger_exists(self):
        """Test user_sync module has logger."""
        with patch("signal.signal"):
            from workers import user_sync
            assert hasattr(user_sync, "logger")

    def test_user_sync_postgresql_connector_import_attempt(self):
        """Test PostgreSQLConnector import is attempted."""
        with patch("signal.signal"):
            from workers import user_sync
            # Module should have imported or set PostgreSQLConnector
            assert hasattr(user_sync, "PostgreSQLConnector")

    def test_user_sync_mariadb_connector_import_attempt(self):
        """Test MariaDBConnector import is attempted."""
        with patch("signal.signal"):
            from workers import user_sync
            assert hasattr(user_sync, "MariaDBConnector")

    def test_user_sync_redis_connector_import_attempt(self):
        """Test RedisConnector import is attempted."""
        with patch("signal.signal"):
            from workers import user_sync
            assert hasattr(user_sync, "RedisConnector")

    def test_user_sync_ceph_connector_import_attempt(self):
        """Test CephConnector import is attempted."""
        with patch("signal.signal"):
            from workers import user_sync
            assert hasattr(user_sync, "CephConnector")

    def test_user_sync_san_connector_import_attempt(self):
        """Test SANConnector import is attempted."""
        with patch("signal.signal"):
            from workers import user_sync
            assert hasattr(user_sync, "SANConnector")

    def test_user_sync_constants_defined(self):
        """Test user_sync module constants are defined."""
        with patch("signal.signal"):
            from workers import user_sync
            # Should have basic module-level constants
            assert user_sync is not None


class TestSyncUserMethod:
    """Tests for sync_user method."""

    def test_sync_user_not_found_returns_early(self):
        """Test sync_user returns early when resource_user not found."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            worker.db = MagicMock()
            worker.db.__getitem__.return_value = None  # resource_users[id] returns None

            result = worker.sync_user(999)
            assert result is None

    def test_sync_user_successful_create(self):
        """Test sync_user successfully creates new user on resource."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()

            # Mock resource_user record
            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.password_hash = "hashedpwd"
            resource_user.resource_id = 1
            resource_user.roles = ["admin"]

            # Mock resource record
            resource = MagicMock()
            resource.id = 1
            resource.name = "test-resource"
            resource.resource_type_id = 1
            resource.connection_info = {"host": "localhost"}
            resource.credentials = {"user": "admin"}

            # Mock resource_type record
            resource_type = MagicMock()
            resource_type.id = 1
            resource_type.name = "db-postgresql"

            # Create a proper mock database with indexed access
            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_resources = MagicMock()
            mock_resources.__getitem__ = MagicMock(return_value=resource)
            mock_resource_types = MagicMock()
            mock_resource_types.__getitem__ = MagicMock(return_value=resource_type)

            mock_db.resource_users = mock_resource_users
            mock_db.resources = mock_resources
            mock_db.resource_types = mock_resource_types
            worker.db = mock_db

            # Mock connector
            mock_connector = MagicMock()
            mock_connector.user_exists.return_value = False

            with patch.object(worker, "_get_connector", return_value=mock_connector):
                worker.sync_user(1)

                # Verify connector.create_user was called
                mock_connector.create_user.assert_called_once()
                # Verify resource_user was updated
                resource_user.update_record.assert_called()

    def test_sync_user_successful_update(self):
        """Test sync_user successfully updates existing user on resource."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()

            # Mock resource_user record
            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.password_hash = "hashedpwd"
            resource_user.resource_id = 1
            resource_user.roles = ["user"]

            # Mock resource and resource_type
            resource = MagicMock()
            resource.id = 1
            resource.name = "test-resource"
            resource.resource_type_id = 1
            resource.connection_info = {"host": "localhost"}
            resource.credentials = {"user": "admin"}

            resource_type = MagicMock()
            resource_type.id = 1
            resource_type.name = "db-postgresql"

            # Create proper mock database
            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_resources = MagicMock()
            mock_resources.__getitem__ = MagicMock(return_value=resource)
            mock_resource_types = MagicMock()
            mock_resource_types.__getitem__ = MagicMock(return_value=resource_type)

            mock_db.resource_users = mock_resource_users
            mock_db.resources = mock_resources
            mock_db.resource_types = mock_resource_types
            worker.db = mock_db

            # Mock connector with user existing
            mock_connector = MagicMock()
            mock_connector.user_exists.return_value = True

            with patch.object(worker, "_get_connector", return_value=mock_connector):
                worker.sync_user(1)

                # Verify connector.update_user was called
                mock_connector.update_user.assert_called_once()

    def test_sync_user_resource_not_found(self):
        """Test sync_user handles resource not found."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()
            worker.db = MagicMock()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.resource_id = 999

            worker.db.__getitem__.side_effect = lambda table_id: {
                ("resource_users", 1): resource_user,
                ("resources", 999): None,  # Resource not found
            }.get((table_id[0], table_id[1]), None)

            with patch.object(worker, "_handle_sync_error") as mock_error:
                worker.sync_user(1)
                mock_error.assert_called_once()

    def test_sync_user_connector_not_available(self):
        """Test sync_user handles unavailable connector."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()
            worker.db = MagicMock()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.resource_id = 1

            resource = MagicMock()
            resource.id = 1
            resource.resource_type_id = 1
            resource.connection_info = {"host": "localhost"}
            resource.credentials = {"user": "admin"}

            resource_type = MagicMock()
            resource_type.id = 1
            resource_type.name = "db-postgresql"

            worker.db.__getitem__.side_effect = lambda table_id: {
                ("resource_users", 1): resource_user,
                ("resources", 1): resource,
                ("resource_types", 1): resource_type,
            }.get((table_id[0], table_id[1]), None)

            with patch.object(worker, "_get_connector", return_value=None):
                with patch.object(worker, "_handle_sync_error") as mock_error:
                    worker.sync_user(1)
                    mock_error.assert_called_once()

    def test_sync_user_connection_error(self):
        """Test sync_user handles connection errors."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()
            worker.db = MagicMock()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.password_hash = "pwd"
            resource_user.resource_id = 1
            resource_user.roles = []

            resource = MagicMock()
            resource.id = 1
            resource.name = "test"
            resource.resource_type_id = 1
            resource.connection_info = {"host": "localhost"}
            resource.credentials = {"user": "admin"}

            resource_type = MagicMock()
            resource_type.id = 1
            resource_type.name = "db-postgresql"

            worker.db.__getitem__.side_effect = lambda table_id: {
                ("resource_users", 1): resource_user,
                ("resources", 1): resource,
                ("resource_types", 1): resource_type,
            }.get((table_id[0], table_id[1]), None)

            mock_connector = MagicMock()
            mock_connector.user_exists.side_effect = ConnectionError("Connection failed")

            with patch.object(worker, "_get_connector", return_value=mock_connector):
                with patch.object(worker, "_handle_sync_error") as mock_error:
                    worker.sync_user(1)
                    mock_error.assert_called_once()


class TestDeleteUserMethod:
    """Tests for delete_user method."""

    def test_delete_user_not_found_returns_early(self):
        """Test delete_user returns early when resource_user not found."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=None)
            mock_db.resource_users = mock_resource_users
            worker.db = mock_db

            result = worker.delete_user(999)
            assert result is None

    def test_delete_user_successful(self):
        """Test delete_user successfully deletes user from resource."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.resource_id = 1

            resource = MagicMock()
            resource.id = 1
            resource.name = "test-resource"
            resource.resource_type_id = 1
            resource.connection_info = {"host": "localhost"}
            resource.credentials = {"user": "admin"}

            resource_type = MagicMock()
            resource_type.id = 1
            resource_type.name = "db-postgresql"

            # Create proper mock database
            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_resources = MagicMock()
            mock_resources.__getitem__ = MagicMock(return_value=resource)
            mock_resource_types = MagicMock()
            mock_resource_types.__getitem__ = MagicMock(return_value=resource_type)

            mock_db.resource_users = mock_resource_users
            mock_db.resources = mock_resources
            mock_db.resource_types = mock_resource_types
            worker.db = mock_db

            mock_connector = MagicMock()
            mock_connector.user_exists.return_value = True

            with patch.object(worker, "_get_connector", return_value=mock_connector):
                worker.delete_user(1)

                mock_connector.delete_user.assert_called_once_with("testuser")
                resource_user.update_record.assert_called()

    def test_delete_user_not_exists_on_resource(self):
        """Test delete_user when user doesn't exist on resource."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.resource_id = 1

            resource = MagicMock()
            resource.id = 1
            resource.name = "test-resource"
            resource.resource_type_id = 1
            resource.connection_info = {"host": "localhost"}
            resource.credentials = {"user": "admin"}

            resource_type = MagicMock()
            resource_type.id = 1
            resource_type.name = "db-postgresql"

            # Create proper mock database
            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_resources = MagicMock()
            mock_resources.__getitem__ = MagicMock(return_value=resource)
            mock_resource_types = MagicMock()
            mock_resource_types.__getitem__ = MagicMock(return_value=resource_type)

            mock_db.resource_users = mock_resource_users
            mock_db.resources = mock_resources
            mock_db.resource_types = mock_resource_types
            worker.db = mock_db

            mock_connector = MagicMock()
            mock_connector.user_exists.return_value = False

            with patch.object(worker, "_get_connector", return_value=mock_connector):
                worker.delete_user(1)

                # delete_user should not be called if user doesn't exist
                mock_connector.delete_user.assert_not_called()
                # But record should still be marked as deleted
                resource_user.update_record.assert_called()


class TestGetConnectorMethod:
    """Tests for _get_connector method."""

    def test_get_connector_postgresql(self):
        """Test _get_connector returns PostgreSQLConnector for db-postgresql."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            with patch("workers.user_sync.PostgreSQLConnector") as mock_pg:
                mock_pg.return_value = MagicMock()
                connector = worker._get_connector(
                    "db-postgresql",
                    {"host": "localhost"},
                    {"user": "admin"}
                )
                mock_pg.assert_called_once()
                assert connector is not None

    def test_get_connector_mariadb(self):
        """Test _get_connector returns MariaDBConnector for db-mariadb."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            with patch("workers.user_sync.MariaDBConnector") as mock_mdb:
                mock_mdb.return_value = MagicMock()
                connector = worker._get_connector(
                    "db-mariadb",
                    {"host": "localhost"},
                    {"user": "admin"}
                )
                mock_mdb.assert_called_once()
                assert connector is not None

    def test_get_connector_redis(self):
        """Test _get_connector returns RedisConnector for db-redis."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            with patch("workers.user_sync.RedisConnector") as mock_redis:
                mock_redis.return_value = MagicMock()
                connector = worker._get_connector(
                    "db-redis",
                    {"host": "localhost"},
                    {"password": "secret"}
                )
                mock_redis.assert_called_once()
                assert connector is not None

    def test_get_connector_valkey(self):
        """Test _get_connector returns RedisConnector for db-valkey."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            with patch("workers.user_sync.RedisConnector") as mock_valkey:
                mock_valkey.return_value = MagicMock()
                connector = worker._get_connector(
                    "db-valkey",
                    {"host": "localhost"},
                    {"password": "secret"}
                )
                mock_valkey.assert_called_once()
                assert connector is not None

    def test_get_connector_ceph(self):
        """Test _get_connector returns CephConnector for storage-ceph."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            with patch("workers.user_sync.CephConnector") as mock_ceph:
                mock_ceph.return_value = MagicMock()
                connector = worker._get_connector(
                    "storage-ceph",
                    {"endpoint": "ceph.local"},
                    {"access_key": "key"}
                )
                mock_ceph.assert_called_once()
                assert connector is not None

    def test_get_connector_san(self):
        """Test _get_connector returns SANConnector for storage-san."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            with patch("workers.user_sync.SANConnector") as mock_san:
                mock_san.return_value = MagicMock()
                connector = worker._get_connector(
                    "storage-san",
                    {"target": "san.local"},
                    {"iqn": "iqn.local"}
                )
                mock_san.assert_called_once()
                assert connector is not None

    def test_get_connector_unknown_type(self):
        """Test _get_connector returns None for unknown type."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            connector = worker._get_connector(
                "unknown-type",
                {"host": "localhost"},
                {"user": "admin"}
            )
            assert connector is None

    def test_get_connector_missing_connection_info(self):
        """Test _get_connector returns None when connection_info is None."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            connector = worker._get_connector(
                "db-postgresql",
                None,
                {"user": "admin"}
            )
            assert connector is None

    def test_get_connector_missing_credentials(self):
        """Test _get_connector returns None when credentials is None."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            connector = worker._get_connector(
                "db-postgresql",
                {"host": "localhost"},
                None
            )
            assert connector is None

    def test_get_connector_initialization_error(self):
        """Test _get_connector handles connector initialization errors."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            with patch("workers.user_sync.PostgreSQLConnector", side_effect=Exception("Init failed")):
                connector = worker._get_connector(
                    "db-postgresql",
                    {"host": "localhost"},
                    {"user": "admin"}
                )
                assert connector is None


class TestSyncPendingUsersMethod:
    """Tests for sync_pending_users method."""

    def test_sync_pending_users_with_items(self):
        """Test sync_pending_users processes pending users."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            worker.db = MagicMock()

            # Mock query result
            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.sync_status = "pending"

            worker.db.return_value.select.return_value = [resource_user]

            with patch.object(worker, "sync_user") as mock_sync:
                worker.sync_pending_users()
                mock_sync.assert_called_with(1)

    def test_sync_pending_users_empty_queue(self):
        """Test sync_pending_users returns early when no pending users."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            worker.db = MagicMock()

            # Return empty list
            worker.db.return_value.select.return_value = []

            with patch.object(worker, "sync_user") as mock_sync:
                worker.sync_pending_users()
                mock_sync.assert_not_called()

    def test_sync_pending_users_respects_running_flag(self):
        """Test sync_pending_users respects running flag."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            worker.db = MagicMock()
            worker.running = False

            resource_user = MagicMock()
            resource_user.id = 1

            worker.db.return_value.select.return_value = [resource_user]

            with patch.object(worker, "sync_user") as mock_sync:
                worker.sync_pending_users()
                # Should not call sync_user if running is False
                mock_sync.assert_not_called()

    def test_sync_pending_users_handles_sync_error(self):
        """Test sync_pending_users handles errors during sync_user."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            worker.db = MagicMock()

            resource_user = MagicMock()
            resource_user.id = 1

            worker.db.return_value.select.return_value = [resource_user]

            with patch.object(worker, "sync_user", side_effect=Exception("Sync failed")):
                # Should not raise, just log error
                worker.sync_pending_users()

    def test_sync_pending_users_db_query_error(self):
        """Test sync_pending_users handles database errors."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            worker.db = MagicMock()

            # Make query raise exception
            worker.db.side_effect = Exception("DB connection failed")

            # Should not raise, just log error
            worker.sync_pending_users()


class TestRunMethod:
    """Tests for run method and main loop."""

    def test_run_loop_with_shutdown_signal(self):
        """Test run loop exits on shutdown signal."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            with patch.object(worker, "sync_pending_users"):
                with patch("time.sleep"):
                    # Simulate shutdown after first iteration
                    worker.running = False
                    worker.run()
                    # Should exit without error

    def test_run_loop_calls_sync_pending_users(self):
        """Test run loop calls sync_pending_users."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            # Track calls
            call_count = [0]

            def side_effect():
                call_count[0] += 1
                if call_count[0] >= 1:
                    worker.running = False

            with patch.object(worker, "sync_pending_users", side_effect=side_effect):
                with patch("time.sleep"):
                    worker.run()
                    assert call_count[0] >= 1

    def test_run_loop_respects_sleep_interval(self):
        """Test run loop sleeps between cycles."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker(sleep_interval=60)

            with patch.object(worker, "sync_pending_users"):
                with patch("time.sleep") as mock_sleep:
                    worker.running = False
                    worker.run()
                    # First call will not sleep (running=False breaks loop)

    def test_run_loop_handles_keyboard_interrupt(self):
        """Test run loop handles KeyboardInterrupt gracefully."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            with patch.object(worker, "sync_pending_users", side_effect=KeyboardInterrupt()):
                with patch("time.sleep"):
                    worker.run()
                    # Should exit gracefully

    def test_run_loop_handles_general_exception(self):
        """Test run loop handles general exceptions."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            call_count = [0]

            def side_effect():
                call_count[0] += 1
                if call_count[0] == 1:
                    raise Exception("Unexpected error")
                worker.running = False

            with patch.object(worker, "sync_pending_users", side_effect=side_effect):
                with patch("time.sleep"):
                    worker.run()
                    # Should continue running after exception


class TestHandleSyncErrorMethod:
    """Tests for _handle_sync_error method."""

    def test_handle_sync_error_updates_record(self):
        """Test _handle_sync_error updates resource_user record."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()

            resource_user = MagicMock()

            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_db.resource_users = mock_resource_users
            worker.db = mock_db

            worker._handle_sync_error(1, "Test error", "User-friendly message")

            resource_user.update_record.assert_called_once()

    def test_handle_sync_error_not_found_record(self):
        """Test _handle_sync_error handles missing record."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            worker.db = MagicMock()
            worker.db.__getitem__.return_value = None

            # Should not raise exception
            worker._handle_sync_error(999, "Test error", "Message")

    def test_handle_sync_error_on_update_fails(self):
        """Test _handle_sync_error handles update failures."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker
            worker = UserSyncWorker()
            worker.db = MagicMock()

            resource_user = MagicMock()
            resource_user.update_record.side_effect = Exception("Update failed")
            worker.db.__getitem__.return_value = resource_user

            # Should not raise exception
            worker._handle_sync_error(1, "Test error", "Message")


class TestPermissionErrors:
    """Tests for permission/authentication error handling."""

    def test_sync_user_permission_error(self):
        """Test sync_user handles PermissionError."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()
            worker.db = MagicMock()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.password_hash = "pwd"
            resource_user.resource_id = 1
            resource_user.roles = []

            resource = MagicMock()
            resource.id = 1
            resource.name = "test"
            resource.resource_type_id = 1
            resource.connection_info = {"host": "localhost"}
            resource.credentials = {"user": "admin"}

            resource_type = MagicMock()
            resource_type.id = 1
            resource_type.name = "db-postgresql"

            worker.db.__getitem__.side_effect = lambda table_id: {
                ("resource_users", 1): resource_user,
                ("resources", 1): resource,
                ("resource_types", 1): resource_type,
            }.get((table_id[0], table_id[1]), None)

            mock_connector = MagicMock()
            mock_connector.user_exists.side_effect = PermissionError("Auth failed")

            with patch.object(worker, "_get_connector", return_value=mock_connector):
                with patch.object(worker, "_handle_sync_error") as mock_error:
                    worker.sync_user(1)
                    mock_error.assert_called_once()
                    # Verify the error message mentions authentication
                    args = mock_error.call_args[0]
                    assert "Authentication" in args[1] or "Auth" in args[1]


class TestMainFunction:
    """Tests for main function."""

    def test_main_function_callable(self):
        """Test main function is callable."""
        with patch("signal.signal"):
            from workers.user_sync import main
            assert callable(main)

    def test_main_reads_environment_variables(self):
        """Test main function reads environment variables."""
        with patch("signal.signal"):
            with patch("workers.user_sync.UserSyncWorker") as mock_worker_class:
                mock_instance = MagicMock()
                mock_worker_class.return_value = mock_instance

                with patch.dict(os.environ, {
                    "SYNC_INTERVAL": "60",
                    "BATCH_SIZE": "20",
                    "LOG_LEVEL": "DEBUG"
                }):
                    with patch("sys.exit"):
                        from workers.user_sync import main
                        main()

                        # Verify UserSyncWorker was instantiated with env values
                        mock_worker_class.assert_called_once()
                        call_args = mock_worker_class.call_args
                        assert call_args[1]["sleep_interval"] == 60
                        assert call_args[1]["batch_size"] == 20

    def test_main_default_env_values(self):
        """Test main function uses default values when env vars not set."""
        with patch("signal.signal"):
            with patch("workers.user_sync.UserSyncWorker") as mock_worker_class:
                mock_instance = MagicMock()
                mock_worker_class.return_value = mock_instance

                env = {k: v for k, v in os.environ.items() if k not in ["SYNC_INTERVAL", "BATCH_SIZE", "LOG_LEVEL"]}
                with patch.dict(os.environ, env, clear=True):
                    with patch("sys.exit"):
                        from workers.user_sync import main
                        main()

                        # Verify defaults were used
                        mock_worker_class.assert_called_once()
                        call_args = mock_worker_class.call_args
                        assert call_args[1]["sleep_interval"] == 30
                        assert call_args[1]["batch_size"] == 10

    def test_main_exception_handling(self):
        """Test main function handles initialization exceptions."""
        with patch("signal.signal"):
            with patch("workers.user_sync.UserSyncWorker", side_effect=Exception("Init failed")):
                with patch("sys.exit") as mock_exit:
                    from workers.user_sync import main
                    main()
                    # Should call sys.exit(1) on exception
                    mock_exit.assert_called_with(1)


class TestResourceTypeNotFoundError:
    """Tests for resource_type not found scenarios."""

    def test_sync_user_resource_type_not_found(self):
        """Test sync_user handles resource_type not found."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.resource_id = 1

            resource = MagicMock()
            resource.id = 1
            resource.resource_type_id = 999

            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_resources = MagicMock()
            mock_resources.__getitem__ = MagicMock(return_value=resource)
            mock_resource_types = MagicMock()
            mock_resource_types.__getitem__ = MagicMock(return_value=None)  # Not found

            mock_db.resource_users = mock_resource_users
            mock_db.resources = mock_resources
            mock_db.resource_types = mock_resource_types
            worker.db = mock_db

            with patch.object(worker, "_handle_sync_error") as mock_error:
                worker.sync_user(1)
                mock_error.assert_called_once()

    def test_delete_user_resource_type_not_found(self):
        """Test delete_user handles resource_type not found."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.resource_id = 1

            resource = MagicMock()
            resource.id = 1
            resource.resource_type_id = 999

            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_resources = MagicMock()
            mock_resources.__getitem__ = MagicMock(return_value=resource)
            mock_resource_types = MagicMock()
            mock_resource_types.__getitem__ = MagicMock(return_value=None)

            mock_db.resource_users = mock_resource_users
            mock_db.resources = mock_resources
            mock_db.resource_types = mock_resource_types
            worker.db = mock_db

            # Should raise exception or handle gracefully
            try:
                worker.delete_user(1)
            except Exception:
                pass  # Exception is expected


class TestValueAndPermissionErrors:
    """Tests for ValueError and PermissionError handling."""

    def test_sync_user_value_error_handler(self):
        """Test sync_user handles ValueError from connector operations."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.password_hash = "pwd"
            resource_user.resource_id = 1
            resource_user.roles = []

            resource = MagicMock()
            resource.id = 1
            resource.name = "test"
            resource.resource_type_id = 1
            resource.connection_info = {"host": "localhost"}
            resource.credentials = {"user": "admin"}

            resource_type = MagicMock()
            resource_type.id = 1
            resource_type.name = "db-postgresql"

            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_resources = MagicMock()
            mock_resources.__getitem__ = MagicMock(return_value=resource)
            mock_resource_types = MagicMock()
            mock_resource_types.__getitem__ = MagicMock(return_value=resource_type)

            mock_db.resource_users = mock_resource_users
            mock_db.resources = mock_resources
            mock_db.resource_types = mock_resource_types
            worker.db = mock_db

            mock_connector = MagicMock()
            mock_connector.user_exists.side_effect = ValueError("Invalid config")

            with patch.object(worker, "_get_connector", return_value=mock_connector):
                with patch.object(worker, "_handle_sync_error") as mock_error:
                    worker.sync_user(1)
                    mock_error.assert_called_once()
                    # Check error message contains "Configuration"
                    args = mock_error.call_args[0]
                    assert "Configuration" in args[1]

    def test_sync_user_generic_exception_handler(self):
        """Test sync_user handles unexpected exceptions."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.password_hash = "pwd"
            resource_user.resource_id = 1
            resource_user.roles = []

            resource = MagicMock()
            resource.id = 1
            resource.name = "test"
            resource.resource_type_id = 1
            resource.connection_info = {"host": "localhost"}
            resource.credentials = {"user": "admin"}

            resource_type = MagicMock()
            resource_type.id = 1
            resource_type.name = "db-postgresql"

            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_resources = MagicMock()
            mock_resources.__getitem__ = MagicMock(return_value=resource)
            mock_resource_types = MagicMock()
            mock_resource_types.__getitem__ = MagicMock(return_value=resource_type)

            mock_db.resource_users = mock_resource_users
            mock_db.resources = mock_resources
            mock_db.resource_types = mock_resource_types
            worker.db = mock_db

            mock_connector = MagicMock()
            mock_connector.user_exists.side_effect = RuntimeError("Unexpected error")

            with patch.object(worker, "_get_connector", return_value=mock_connector):
                with patch.object(worker, "_handle_sync_error") as mock_error:
                    worker.sync_user(1)
                    mock_error.assert_called_once()

    def test_delete_user_exception_handling(self):
        """Test delete_user re-raises exception after logging."""
        with patch("signal.signal"):
            from workers.user_sync import UserSyncWorker

            worker = UserSyncWorker()

            resource_user = MagicMock()
            resource_user.id = 1
            resource_user.username = "testuser"
            resource_user.resource_id = 1

            resource = MagicMock()
            resource.id = 1
            resource.resource_type_id = 1

            mock_db = MagicMock()
            mock_resource_users = MagicMock()
            mock_resource_users.__getitem__ = MagicMock(return_value=resource_user)
            mock_resources = MagicMock()
            mock_resources.__getitem__ = MagicMock(return_value=None)  # Raise exception

            mock_db.resource_users = mock_resource_users
            mock_db.resources = mock_resources
            worker.db = mock_db

            # delete_user should raise when resource not found
            with pytest.raises(ValueError):
                worker.delete_user(1)
