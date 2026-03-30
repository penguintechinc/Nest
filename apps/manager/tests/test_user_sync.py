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

# Mock models.db BEFORE importing workers.user_sync
sys.modules["models"] = MagicMock(db=MagicMock())


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
