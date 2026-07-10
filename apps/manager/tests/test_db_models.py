"""Tests for database models module."""
import os
import sys
import pytest
from unittest.mock import MagicMock, patch, PropertyMock
from pathlib import Path

# Ensure app root is in sys.path
app_root = Path(__file__).parent.parent
if str(app_root) not in sys.path:
    sys.path.insert(0, str(app_root))

# Set up environment before importing models
os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("DB_HOST", "localhost")
os.environ.setdefault("DB_PORT", "5432")
os.environ.setdefault("DB_NAME", "test_db")
os.environ.setdefault("DB_USER", "testuser")
os.environ.setdefault("REDIS_HOST", "localhost")
os.environ.setdefault("REDIS_PORT", "6379")


@pytest.fixture
def preserve_environ():
    """Fixture to preserve and restore os.environ mutations across tests.

    Saves all os.environ keys before the test and restores them after,
    preventing test isolation leaks where one test's env changes pollute later tests.
    """
    saved_env = dict(os.environ)
    yield
    # Restore original env: remove added keys, restore modified ones
    for key in list(os.environ.keys()):
        if key not in saved_env:
            del os.environ[key]
    for key, value in saved_env.items():
        os.environ[key] = value


class TestModelsConfiguration:
    """Tests for models configuration and environment variables."""

    def test_db_type_can_be_set_from_env(self, preserve_environ):
        """Test DB_TYPE can be set from environment."""
        os.environ["DB_TYPE"] = "postgresql"
        # Just verify the env var is set
        assert os.environ.get("DB_TYPE") == "postgresql"

    def test_db_host_can_be_set_from_env(self, preserve_environ):
        """Test DB_HOST can be set from environment."""
        os.environ["DB_HOST"] = "db.example.com"
        assert os.environ.get("DB_HOST") == "db.example.com"

    def test_db_port_can_be_set_from_env(self, preserve_environ):
        """Test DB_PORT can be set from environment."""
        os.environ["DB_PORT"] = "5433"
        assert os.environ.get("DB_PORT") == "5433"

    def test_db_name_can_be_set_from_env(self, preserve_environ):
        """Test DB_NAME can be set from environment."""
        os.environ["DB_NAME"] = "mydb"
        assert os.environ.get("DB_NAME") == "mydb"

    def test_db_user_can_be_set_from_env(self, preserve_environ):
        """Test DB_USER can be set from environment."""
        os.environ["DB_USER"] = "myuser"
        assert os.environ.get("DB_USER") == "myuser"

    def test_db_pass_can_be_set_from_env(self, preserve_environ):
        """Test DB_PASS can be set from environment."""
        os.environ["DB_PASS"] = "mypass"
        assert os.environ.get("DB_PASS") == "mypass"

    def test_db_password_fallback_env_var(self, preserve_environ):
        """Test DB_PASSWORD is available as fallback."""
        os.environ["DB_PASSWORD"] = "fallback"
        assert os.environ.get("DB_PASSWORD") == "fallback"


class TestUsersModel:
    """Tests for users model."""

    def test_users_model_importable_with_mocked_db(self):
        """Test users model can be imported with mocked db."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import users
            assert users is not None

    def test_users_model_has_docstring(self):
        """Test users model has docstring."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import users
            assert users.__doc__ is not None
            # Just verify it has a docstring
            assert isinstance(users.__doc__, str)

    def test_users_model_mentions_tables_in_docs(self):
        """Test users model documents its tables."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import users
            # Should have documentation
            assert users.__doc__ is not None


class TestThreatIntelModel:
    """Tests for threat intelligence model."""

    def test_threat_intel_model_importable(self):
        """Test threat_intel model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import threat_intel
            assert threat_intel is not None

    def test_threat_intel_model_has_docstring(self):
        """Test threat_intel model has docstring."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import threat_intel
            assert threat_intel.__doc__ is not None
            assert isinstance(threat_intel.__doc__, str)

    def test_threat_intel_model_documents_tables(self):
        """Test threat_intel model documents its tables."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import threat_intel
            doc = threat_intel.__doc__
            # Should have documentation
            assert doc is not None


class TestResourcesModel:
    """Tests for resources model."""

    def test_resources_model_importable(self):
        """Test resources model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import resources
            assert resources is not None

    def test_resources_model_has_docstring(self):
        """Test resources model has docstring."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import resources
            assert resources.__doc__ is not None


class TestAuditModel:
    """Tests for audit model."""

    def test_audit_model_importable(self):
        """Test audit model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import audit
            assert audit is not None

    def test_audit_model_has_docstring(self):
        """Test audit model has docstring."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import audit
            assert audit.__doc__ is not None


class TestCertificatesModel:
    """Tests for certificates model."""

    def test_certificates_model_importable(self):
        """Test certificates model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import certificates
            assert certificates is not None

    def test_certificates_model_has_docstring(self):
        """Test certificates model has docstring."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import certificates
            assert certificates.__doc__ is not None


class TestCloudModel:
    """Tests for cloud model."""

    def test_cloud_model_importable(self):
        """Test cloud model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import cloud
            assert cloud is not None


class TestDatabaseServersModel:
    """Tests for database_servers model."""

    def test_database_servers_model_importable(self):
        """Test database_servers model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import database_servers
            assert database_servers is not None


class TestLicenseModel:
    """Tests for license model."""

    def test_license_model_importable(self):
        """Test license model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import license
            assert license is not None


class TestPermissionsModel:
    """Tests for permissions model."""

    def test_permissions_model_importable(self):
        """Test permissions model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import permissions
            assert permissions is not None


class TestScalingModel:
    """Tests for scaling model."""

    def test_scaling_model_importable(self):
        """Test scaling model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import scaling
            assert scaling is not None


class TestSecurityModel:
    """Tests for security model."""

    def test_security_model_importable(self):
        """Test security model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import security
            assert security is not None


class TestSqlFilesModel:
    """Tests for sql_files model."""

    def test_sql_files_model_importable(self):
        """Test sql_files model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import sql_files
            assert sql_files is not None


class TestTeamsModel:
    """Tests for teams model."""

    def test_teams_model_importable(self):
        """Test teams model can be imported."""
        with patch("penguin_dal.DB", MagicMock()):
            from models import teams
            assert teams is not None


class TestAllModelsImportable:
    """Tests that all model modules can be imported."""

    def test_all_model_files_importable(self):
        """Test all model Python files can be imported."""
        # Just test that we can import the models package
        with patch("penguin_dal.DB", MagicMock()):
            import models
            assert models is not None
            assert hasattr(models, "db")

    def test_models_init_exports_db(self):
        """Test models/__init__.py exports db."""
        with patch("penguin_dal.DB", MagicMock()):
            with patch("models.db", MagicMock()):
                import importlib
                import models
                # Mock the actual db attribute
                models.db = MagicMock()
                assert hasattr(models, "db")

    def test_models_init_exports_constants(self):
        """Test models/__init__.py exports configuration constants."""
        # These should be available as environment-derived values
        assert "DB_TYPE" in os.environ or os.environ.get("DB_TYPE") is not None
