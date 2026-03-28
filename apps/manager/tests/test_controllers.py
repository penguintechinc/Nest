"""Controller unit tests — certificates, external_ops, provisioning."""
import os
import sys
import types
import json
import pytest
from unittest.mock import MagicMock, patch, PropertyMock

# Ensure the manager app directory is on the path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("FIELD_ENCRYPTION_KEY", "Fernet_key_placeholder_32bytes==")
os.environ.setdefault("ENCRYPTION_KEY", "")


# ---------------------------------------------------------------------------
# Stub heavy modules before importing controllers
# ---------------------------------------------------------------------------

def _stub_modules():
    """Install all stubs needed for controller imports."""

    # penguin_dal.quart_ext
    fake_quart_ext = types.ModuleType("penguin_dal.quart_ext")
    fake_quart_ext.get_db = MagicMock()
    fake_quart_ext.init_dal = MagicMock()
    sys.modules["penguin_dal.quart_ext"] = fake_quart_ext

    # lib.ca_manager
    fake_ca = types.ModuleType("lib.ca_manager")
    fake_ca.CAManager = MagicMock()
    fake_ca.CAManagerException = Exception
    sys.modules["lib.ca_manager"] = fake_ca
    sys.modules["lib"] = types.ModuleType("lib")

    # lib.k8s_client (certificates variant)
    fake_k8s = types.ModuleType("lib.k8s_client")
    fake_k8s.KubernetesClient = MagicMock()
    fake_k8s.KubernetesClientException = Exception
    fake_k8s.K8sClient = MagicMock()
    fake_k8s.K8sException = Exception
    sys.modules["lib.k8s_client"] = fake_k8s

    # Resource connector stubs for external_ops
    for connector in [
        "lib.resource_connectors",
        "lib.resource_connectors.postgresql",
        "lib.resource_connectors.mariadb",
        "lib.resource_connectors.redis",
        "lib.resource_connectors.ceph",
        "lib.resource_connectors.san",
    ]:
        mod = types.ModuleType(connector)
        # Each connector stub exposes a class with the last segment name
        class_name = connector.split(".")[-1].capitalize() + "Connector"
        setattr(mod, class_name, MagicMock())
        sys.modules[connector] = mod

    # Specific connector classes used in external_ops
    sys.modules["lib.resource_connectors.postgresql"].PostgreSQLConnector = MagicMock()
    sys.modules["lib.resource_connectors.mariadb"].MariaDBConnector = MagicMock()
    sys.modules["lib.resource_connectors.redis"].RedisConnector = MagicMock()
    sys.modules["lib.resource_connectors.ceph"].CephConnector = MagicMock()
    sys.modules["lib.resource_connectors.san"].SANConnector = MagicMock()

    # jinja2 (used in provisioning)
    try:
        import jinja2  # noqa: F401
    except ImportError:
        fake_jinja = types.ModuleType("jinja2")
        fake_jinja.Environment = MagicMock()
        fake_jinja.FileSystemLoader = MagicMock()
        fake_jinja.TemplateNotFound = Exception
        sys.modules["jinja2"] = fake_jinja

    # cryptography.fernet (used in provisioning)
    try:
        from cryptography.fernet import Fernet  # noqa: F401
    except ImportError:
        fake_fernet_mod = types.ModuleType("cryptography.fernet")
        fake_fernet = MagicMock()
        fake_fernet.generate_key.return_value = b"fake_key_32byteslong_padding_here"
        fake_fernet_mod.Fernet = fake_fernet
        sys.modules["cryptography"] = types.ModuleType("cryptography")
        sys.modules["cryptography.fernet"] = fake_fernet_mod


_stub_modules()


# ---------------------------------------------------------------------------
# EncryptionManager and CredentialGenerator (provisioning.py)
# ---------------------------------------------------------------------------

class TestEncryptionManager:
    def _get_class(self):
        from cryptography.fernet import Fernet
        key = Fernet.generate_key()
        # Now import the real class
        import importlib
        spec = importlib.util.spec_from_file_location(
            "provisioning",
            os.path.join(os.path.dirname(__file__), "..", "controllers", "provisioning.py"),
        )
        return key

    def test_encrypt_decrypt_roundtrip(self):
        """EncryptionManager encrypts then decrypts to original value."""
        try:
            from cryptography.fernet import Fernet
        except ImportError:
            pytest.skip("cryptography not available")

        key = Fernet.generate_key()
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import EncryptionManager
            mgr = EncryptionManager(key=key.decode())
            plaintext = "super_secret_password"
            encrypted = mgr.encrypt(plaintext)
            assert encrypted != plaintext
            decrypted = mgr.decrypt(encrypted)
            assert decrypted == plaintext

    def test_encrypt_returns_string(self):
        """Encrypted value is a string."""
        try:
            from cryptography.fernet import Fernet
        except ImportError:
            pytest.skip("cryptography not available")

        key = Fernet.generate_key()
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import EncryptionManager
            mgr = EncryptionManager(key=key.decode())
            result = mgr.encrypt("hello")
            assert isinstance(result, str)


class TestCredentialGenerator:
    def test_generate_password_default_length(self):
        """Default password is 32 chars."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import CredentialGenerator
            pwd = CredentialGenerator.generate_password()
            assert len(pwd) == 32

    def test_generate_password_custom_length(self):
        """Custom password length is respected."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import CredentialGenerator
            pwd = CredentialGenerator.generate_password(length=16)
            assert len(pwd) == 16

    def test_generate_password_is_string(self):
        """Password is a string."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import CredentialGenerator
            pwd = CredentialGenerator.generate_password()
            assert isinstance(pwd, str)

    def test_generate_username_has_prefix(self):
        """Username starts with prefix."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import CredentialGenerator
            uname = CredentialGenerator.generate_username(prefix="db")
            assert uname.startswith("db_")

    def test_generate_username_default_prefix(self):
        """Default prefix is 'user'."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import CredentialGenerator
            uname = CredentialGenerator.generate_username()
            assert uname.startswith("user_")

    def test_generate_api_token_length(self):
        """API token is the right hex length (length//2 bytes → length hex chars)."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import CredentialGenerator
            token = CredentialGenerator.generate_api_token(length=32)
            assert len(token) == 32

    def test_generate_api_token_is_string(self):
        """API token is a string."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import CredentialGenerator
            token = CredentialGenerator.generate_api_token()
            assert isinstance(token, str)

    def test_passwords_are_unique(self):
        """Two generated passwords differ (probabilistically)."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import CredentialGenerator
            p1 = CredentialGenerator.generate_password()
            p2 = CredentialGenerator.generate_password()
            assert p1 != p2


# ---------------------------------------------------------------------------
# TemplateRenderer (provisioning.py)
# ---------------------------------------------------------------------------

class TestTemplateRenderer:
    def test_render_statefulset_unsupported_type_raises(self):
        """Unsupported resource type raises ValueError."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import TemplateRenderer
            renderer = TemplateRenderer(template_dir="/tmp/fake_templates")
            with pytest.raises((ValueError, Exception)):
                renderer.render_statefulset_template("unknown-type", {})

    def test_render_statefulset_known_type_attempts_render(self):
        """Known resource type calls render_template."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import TemplateRenderer
            renderer = TemplateRenderer(template_dir="/tmp/fake_templates")
            renderer.render_template = MagicMock(return_value="yaml: content")
            result = renderer.render_statefulset_template("db-postgresql", {"key": "val"})
            renderer.render_template.assert_called_once_with(
                "statefulset/postgresql.yaml", {"key": "val"}
            )
            assert result == "yaml: content"

    def test_render_statefulset_supported_types_map(self):
        """All expected resource types have a template mapping."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import TemplateRenderer
            renderer = TemplateRenderer(template_dir="/tmp/fake_templates")
            renderer.render_template = MagicMock(return_value="")
            for rtype in ("db-postgresql", "db-redis", "db-mariadb", "db-valkey"):
                renderer.render_statefulset_template(rtype, {})
            assert renderer.render_template.call_count == 4


# ---------------------------------------------------------------------------
# ExternalOpsController — static helpers
# ---------------------------------------------------------------------------

class TestExternalOpsController:
    def test_load_resource_raises_when_not_found(self):
        """_load_resource raises InvalidResourceError if resource missing."""
        db = MagicMock()
        # Make db.resources[id] return a falsy object
        falsy_resource = MagicMock()
        falsy_resource.__bool__ = MagicMock(return_value=False)
        db.resources.__getitem__ = MagicMock(return_value=falsy_resource)

        with patch("controllers.external_ops.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController, InvalidResourceError
            with pytest.raises(InvalidResourceError):
                ExternalOpsController._load_resource(999)

    def test_load_resource_returns_record_when_found(self):
        """_load_resource returns the resource record when it exists."""
        resource = MagicMock()
        # MagicMock is truthy by default, so not resource == False
        db = MagicMock()
        db.resources.__getitem__ = MagicMock(return_value=resource)

        with patch("controllers.external_ops.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController
            result = ExternalOpsController._load_resource(1)
            assert result == resource

    def test_validate_lifecycle_mode_partial_passes(self):
        """'partial' lifecycle mode passes validation."""
        resource = MagicMock()
        resource.lifecycle_mode = "partial"
        db = MagicMock()

        with patch("penguin_dal.quart_ext.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController
            # Should not raise
            ExternalOpsController._validate_lifecycle_mode(resource)

    def test_validate_lifecycle_mode_monitor_only_passes(self):
        """'monitor_only' lifecycle mode passes validation."""
        resource = MagicMock()
        resource.lifecycle_mode = "monitor_only"
        db = MagicMock()

        with patch("penguin_dal.quart_ext.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController
            # Should not raise
            ExternalOpsController._validate_lifecycle_mode(resource)

    def test_validate_lifecycle_mode_full_raises(self):
        """Unsupported lifecycle mode raises InvalidResourceError."""
        resource = MagicMock()
        resource.lifecycle_mode = "full"
        db = MagicMock()

        with patch("controllers.external_ops.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController, InvalidResourceError
            with pytest.raises(InvalidResourceError):
                ExternalOpsController._validate_lifecycle_mode(resource)

    def test_get_connector_class_postgresql(self):
        """PostgreSQL resource type returns PostgreSQLConnector."""
        db = MagicMock()
        with patch("penguin_dal.quart_ext.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController
            cls = ExternalOpsController._get_connector_class("db-postgresql")
            assert cls is not None

    def test_get_connector_class_unsupported_raises(self):
        """Unknown resource type raises ConnectorError."""
        db = MagicMock()
        with patch("penguin_dal.quart_ext.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController, ConnectorError
            with pytest.raises(ConnectorError):
                ExternalOpsController._get_connector_class("db-unknown-type")

    def test_get_connector_class_all_supported_types(self):
        """All supported types return a connector class."""
        db = MagicMock()
        supported = [
            "db-postgresql", "db-mariadb", "db-redis",
            "storage-ceph", "storage-san",
        ]
        with patch("penguin_dal.quart_ext.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController
            for rtype in supported:
                cls = ExternalOpsController._get_connector_class(rtype)
                assert cls is not None, f"No connector for {rtype}"

    def test_initialize_connector_with_json_string_credentials(self):
        """_initialize_connector parses JSON string credentials."""
        db = MagicMock()

        resource = MagicMock()
        resource.connection_info = {"host": "localhost"}
        resource.credentials = '{"password": "secret"}'

        mock_connector_class = MagicMock()
        mock_instance = MagicMock()
        mock_connector_class.return_value = mock_instance

        with patch("penguin_dal.quart_ext.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController
            result = ExternalOpsController._initialize_connector(mock_connector_class, resource)
            assert result == mock_instance

    def test_initialize_connector_with_empty_credentials(self):
        """_initialize_connector handles empty credentials dict."""
        db = MagicMock()

        resource = MagicMock()
        resource.connection_info = {}
        resource.credentials = {}

        mock_connector_class = MagicMock()
        mock_instance = MagicMock()
        mock_connector_class.return_value = mock_instance

        with patch("penguin_dal.quart_ext.get_db", return_value=db):
            from controllers.external_ops import ExternalOpsController
            result = ExternalOpsController._initialize_connector(mock_connector_class, resource)
            assert result == mock_instance


# ---------------------------------------------------------------------------
# CertificatesController RBAC helpers
# ---------------------------------------------------------------------------

class TestCertificatesController:
    def _make_db(self):
        db = MagicMock()
        db.team_memberships = MagicMock()
        db.team_memberships.user_id = MagicMock()
        db.team_memberships.team_id = MagicMock()
        db.team_memberships.role = MagicMock()
        db.teams = MagicMock()
        db.teams.is_global = MagicMock()
        db.teams.id = MagicMock()
        return db

    def _make_controller(self, db=None):
        if db is None:
            db = self._make_db()
        from controllers.certificates import CertificatesController
        ctrl = CertificatesController.__new__(CertificatesController)
        ctrl.db = db
        ctrl.k8s_client = MagicMock()
        ctrl.ca_manager = MagicMock()
        return ctrl

    def test_is_global_admin_true_when_admin_role(self):
        """Returns True when user has admin role in global team."""
        membership = MagicMock()
        membership.role = "admin"

        db = self._make_db()
        select_result = MagicMock()
        select_result.first.return_value = membership
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        ctrl = self._make_controller(db)
        assert ctrl._is_global_admin(1) is True

    def test_is_global_admin_false_when_no_membership(self):
        """Returns falsy when no global team membership (None or False)."""
        db = self._make_db()
        select_result = MagicMock()
        select_result.first.return_value = None
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        ctrl = self._make_controller(db)
        assert not ctrl._is_global_admin(1)

    def test_is_global_admin_false_when_not_admin_role(self):
        """Returns falsy when user has non-admin role."""
        membership = MagicMock()
        membership.role = "member"

        db = self._make_db()
        select_result = MagicMock()
        select_result.first.return_value = membership
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        ctrl = self._make_controller(db)
        assert not ctrl._is_global_admin(1)

    def test_get_user_team_role_returns_role_when_member(self):
        """Returns the user's role when they are a team member."""
        membership = MagicMock()
        membership.role = "admin"

        db = self._make_db()
        select_result = MagicMock()
        select_result.first.return_value = membership
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        ctrl = self._make_controller(db)
        role = ctrl._get_user_team_role(user_id=1, team_id=1)
        assert role == "admin"

    def test_get_user_team_role_returns_none_when_not_member(self):
        """Returns None when user is not in the team."""
        db = self._make_db()
        select_result = MagicMock()
        select_result.first.return_value = None
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        ctrl = self._make_controller(db)
        role = ctrl._get_user_team_role(user_id=1, team_id=1)
        assert role is None

    def test_check_ca_access_raises_for_non_admin(self):
        """_check_ca_access raises CertificateAccessDenied for non-admin."""
        from controllers.certificates import CertificateAccessDenied
        db = self._make_db()
        select_result = MagicMock()
        select_result.first.return_value = None
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        ctrl = self._make_controller(db)
        with pytest.raises(CertificateAccessDenied):
            ctrl._check_ca_access(user_id=1)

    def test_check_ca_access_passes_for_global_admin(self):
        """_check_ca_access does not raise for global admin."""
        membership = MagicMock()
        membership.role = "admin"

        db = self._make_db()
        select_result = MagicMock()
        select_result.first.return_value = membership
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        ctrl = self._make_controller(db)
        # Should not raise
        ctrl._check_ca_access(user_id=1)

    def test_check_certificate_access_global_admin_passes(self):
        """Global admin can always manage certificates."""
        membership = MagicMock()
        membership.role = "admin"

        db = self._make_db()
        select_result = MagicMock()
        select_result.first.return_value = membership
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        ctrl = self._make_controller(db)
        # Should not raise
        ctrl._check_certificate_access(user_id=1, team_id=1)

    def test_check_certificate_access_team_admin_passes(self):
        """Team admin can manage certificates in their team."""
        from controllers.certificates import CertificatesController
        db = self._make_db()
        ctrl = self._make_controller(db)
        ctrl._is_global_admin = MagicMock(return_value=False)
        ctrl._get_user_team_role = MagicMock(return_value="admin")
        # Should not raise
        ctrl._check_certificate_access(user_id=2, team_id=1)

    def test_check_certificate_access_non_admin_raises(self):
        """Non-admin team member cannot manage certificates."""
        from controllers.certificates import CertificateAccessDenied
        db = self._make_db()
        ctrl = self._make_controller(db)
        ctrl._is_global_admin = MagicMock(return_value=False)
        ctrl._get_user_team_role = MagicMock(return_value="member")
        with pytest.raises(CertificateAccessDenied):
            ctrl._check_certificate_access(user_id=2, team_id=1)

    def test_check_certificate_view_non_member_raises(self):
        """Non-member cannot view certificates."""
        from controllers.certificates import CertificateAccessDenied
        db = self._make_db()
        ctrl = self._make_controller(db)
        ctrl._is_global_admin = MagicMock(return_value=False)
        ctrl._get_user_team_role = MagicMock(return_value=None)
        with pytest.raises(CertificateAccessDenied):
            ctrl._check_certificate_view(user_id=2, team_id=1)

    def test_check_certificate_view_member_passes(self):
        """Any team member can view certificates."""
        db = self._make_db()
        ctrl = self._make_controller(db)
        ctrl._is_global_admin = MagicMock(return_value=False)
        ctrl._get_user_team_role = MagicMock(return_value="viewer")
        # Should not raise
        ctrl._check_certificate_view(user_id=2, team_id=1)

    def test_check_certificate_view_global_admin_passes(self):
        """Global admin can always view certificates."""
        db = self._make_db()
        ctrl = self._make_controller(db)
        ctrl._is_global_admin = MagicMock(return_value=True)
        # Should not raise (no team role check needed)
        ctrl._check_certificate_view(user_id=1, team_id=1)


# ---------------------------------------------------------------------------
# ExternalOpsController — error classes
# ---------------------------------------------------------------------------

class TestExternalOpsErrors:
    def test_invalid_resource_error_is_subclass(self):
        """InvalidResourceError inherits from ExternalOpsControllerError."""
        from controllers.external_ops import (
            InvalidResourceError,
            ExternalOpsControllerError,
        )
        assert issubclass(InvalidResourceError, ExternalOpsControllerError)

    def test_connector_error_is_subclass(self):
        """ConnectorError inherits from ExternalOpsControllerError."""
        from controllers.external_ops import (
            ConnectorError,
            ExternalOpsControllerError,
        )
        assert issubclass(ConnectorError, ExternalOpsControllerError)

    def test_external_ops_error_is_exception(self):
        """ExternalOpsControllerError is an Exception."""
        from controllers.external_ops import ExternalOpsControllerError
        assert issubclass(ExternalOpsControllerError, Exception)


# ---------------------------------------------------------------------------
# ProvisioningController — SUPPORTED_RESOURCE_TYPES
# ---------------------------------------------------------------------------

class TestProvisioningController:
    def test_supported_resource_types_contains_postgresql(self):
        """db-postgresql is in supported types."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import ProvisioningController
            assert "db-postgresql" in ProvisioningController.SUPPORTED_RESOURCE_TYPES

    def test_supported_resource_types_contains_redis(self):
        """db-redis is in supported types."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import ProvisioningController
            assert "db-redis" in ProvisioningController.SUPPORTED_RESOURCE_TYPES

    def test_supported_resource_types_non_empty(self):
        """Supported resource types list is non-empty."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import ProvisioningController
            assert len(ProvisioningController.SUPPORTED_RESOURCE_TYPES) > 0


# ---------------------------------------------------------------------------
# ProvisioningStatus dataclass
# ---------------------------------------------------------------------------

class TestProvisioningStatus:
    def test_provisioning_status_creation(self):
        """ProvisioningStatus dataclass holds expected fields."""
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import ProvisioningStatus
            status = ProvisioningStatus(
                resource_id=1,
                status="provisioning",
                namespace="nest",
                k8s_resource_name="postgresql-1",
            )
            assert status.resource_id == 1
            assert status.status == "provisioning"
            assert status.namespace == "nest"
            assert status.k8s_resource_name == "postgresql-1"
            assert status.connection_info is None
            assert status.error_message is None

    def test_provisioning_status_with_all_fields(self):
        """ProvisioningStatus accepts all optional fields."""
        from datetime import datetime
        with patch("penguin_dal.quart_ext.get_db", return_value=MagicMock()):
            from controllers.provisioning import ProvisioningStatus
            now = datetime.utcnow()
            status = ProvisioningStatus(
                resource_id=2,
                status="ready",
                namespace="nest",
                k8s_resource_name="redis-2",
                connection_info={"host": "localhost", "port": 6379},
                error_message=None,
                created_at=now,
                updated_at=now,
            )
            assert status.connection_info == {"host": "localhost", "port": 6379}
            assert status.created_at == now
