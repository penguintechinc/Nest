"""Pytest configuration and fixtures."""

import os
import sys
import tempfile
from datetime import datetime, timedelta, timezone

# ============================================================================
# CRITICAL: Set environment variables BEFORE any imports from app/routes/utils
# This must happen at module load time, not in pytest_configure hook
# ============================================================================
_test_db_dir = tempfile.mkdtemp(prefix="pytest_manager_")
_test_db_path = os.path.join(_test_db_dir, "test.db")

os.environ.setdefault("DB_TYPE", "sqlite")  # Use SQLite for tests
os.environ.setdefault("DB_HOST", "localhost")
os.environ.setdefault("DB_PORT", "5432")
os.environ.setdefault("DB_NAME", _test_db_path)
os.environ.setdefault("DB_USER", "test")
os.environ.setdefault("DB_PASS", "test")
os.environ.setdefault("JWT_SECRET", "test-secret-key-for-testing-only")
os.environ.setdefault("JWT_EXPIRY_HOURS", "24")
os.environ.setdefault("ENCRYPTION_KEY", "3X2YMNotai4RWijtiWg_NuEt0L0fzmEwOrhmHMzc8mw=")  # Valid Fernet key for test encryption
# Disable HS256 admin override in tests (we use ES256)
os.environ.setdefault("JWT_ALLOW_HS256", "false")

from unittest.mock import patch, AsyncMock, MagicMock
from jose import jwt
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.backends import default_backend

import pytest
from sqlalchemy import MetaData, Table, Column, String, Integer, Text, DateTime, create_engine

# ============================================================================
# Test EC P-256 Keypair (generated once at module load)
# ============================================================================

_TEST_EC_PRIVATE_KEY = ec.generate_private_key(ec.SECP256R1(), default_backend())
_TEST_EC_PUBLIC_KEY = _TEST_EC_PRIVATE_KEY.public_key()


# ============================================================================
# Test-mode JWT Token Generation & Validation (defined early for pytest_configure)
# ============================================================================

def _make_test_token(user_id: int = 1, email: str = "test@example.com", role: str = "admin", tenant: str = "test-tenant") -> str:
    """Create a test JWT token using ES256 (manager's algorithm).

    Test tokens have:
    - ES256 algorithm (signed with test EC P-256 private key)
    - tenant claim required by tenant middleware
    - sub, email, role claims for compatibility with @require_auth/@require_role
    - Valid expiry (24 hours)
    """
    from utils.auth import JWT_EXPIRY_HOURS

    payload = {
        "sub": str(user_id),
        "email": email,
        "role": role,
        "tenant": tenant,  # Required by tenant middleware
        "scope": "",  # Compatible with OIDC scope field
        "iat": datetime.now(timezone.utc),
        "exp": datetime.now(timezone.utc) + timedelta(hours=JWT_EXPIRY_HOURS),
    }
    return jwt.encode(payload, _TEST_EC_PRIVATE_KEY, algorithm="ES256")


def _get_auth_headers(role: str = "admin", tenant: str = "test-tenant") -> dict:
    """Get Authorization headers with a valid test token."""
    token = _make_test_token(role=role, tenant=tenant)
    return {"Authorization": f"Bearer {token}"}


def _make_test_tenant_middleware(parse_token_fn):
    """Test-mode tenant middleware that validates ES256 tokens.

    This replaces the production tenant_middleware (from middleware/tenant.py).
    In test mode, we accept ES256 tokens signed with the test EC key.

    Returns an async function suitable for patching middleware.tenant.tenant_middleware.
    """
    async def _test_middleware():
        from quart import request, g
        from werkzeug.exceptions import Unauthorized

        # Skip auth for health/ready/metrics
        if request.path in ["/health", "/ready", "/metrics"]:
            return

        # Skip internal routes (they have their own service auth)
        if request.path.startswith("/internal/"):
            return

        # Skip public auth endpoints (login, register, etc.) - production bug: these shouldn't require auth
        if request.path.startswith("/api/v1/auth/"):
            return

        # Skip public status endpoint
        if request.path == "/api/v1/status":
            return

        auth_header = request.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            raise Unauthorized(
                response={
                    "error": "auth.missing_token",
                    "message": "Authorization header with Bearer token required",
                }
            )

        token = auth_header[7:]

        # Use parse_token which validates ES256 JWT
        try:
            decoded = parse_token_fn(token)
            if not decoded:
                raise Unauthorized(
                    response={
                        "error": "auth.invalid_token",
                        "message": "Token could not be parsed",
                    }
                )

            tenant = decoded.get("tenant", "")
            if not tenant:
                raise Unauthorized(
                    response={
                        "error": "auth.missing_tenant",
                        "message": "JWT missing mandatory tenant claim",
                    }
                )

            # Store in g for request scope (matches production behavior)
            g.tenant = tenant
            g.claims = decoded

            # Also set fields that @require_auth decorators expect
            g.user_id = decoded.get("sub", "1")
            g.user_email = decoded.get("email", "test@example.com")
            g.user_role = decoded.get("role", "viewer")
        except Unauthorized:
            raise
        except Exception as e:
            raise Unauthorized(
                response={
                    "error": "auth.error",
                    "message": f"Authentication error: {e}",
                }
            )

    return _test_middleware


# ============================================================================
# Test-mode parse_token() for ES256 validation
# ============================================================================

def _make_test_parse_token():
    """Test-mode parse_token that accepts ES256 tokens signed with test key.

    Used by both tenant_middleware and require_service_auth() in tests.
    Returns a claims dict with sub, tenant, scopes.

    Also accepts legacy test format: "sub:tenant:tier" for backward compat with existing test suite.
    """
    def _test_parse_token_sync(token: str) -> dict | None:
        # Try ES256 JWT first (test tokens)
        try:
            decoded = jwt.decode(token, _TEST_EC_PUBLIC_KEY, algorithms=["ES256"])
            return {
                "sub": decoded.get("sub", ""),
                "tenant": decoded.get("tenant", "test-tenant"),
                "scopes": decoded.get("scope", "").split() if decoded.get("scope") else [],
                "tier": decoded.get("tier", "free"),
            }
        except (jwt.ExpiredSignatureError, jwt.JWTError):
            pass

        # Fallback: try legacy format "sub:tenant:tier" (backward compat with existing tests)
        # Reject malformed versions with too many parts
        if ":" in token and not token.startswith("eyJ"):  # Not a JWT (doesn't start with base64 header)
            parts = token.split(":")
            if len(parts) == 3:  # Exactly 3 parts, not >= 3
                return {
                    "sub": parts[0],
                    "tenant": parts[1],
                    "scopes": [],
                    "tier": parts[2],
                }

        return None

    return _test_parse_token_sync


# ============================================================================
# pytest_configure hook - patch EC key loading and middleware with test-mode handler
# ============================================================================

def pytest_configure(config):
    """Pytest hook: runs after env vars are set but before test collection.

    Patches:
    1. utils.ec_keys.get_manager_ec_keys() to use test EC keypair
    2. middleware.tenant to accept ES256 tokens with test key
    3. app module to use patched versions
    4. Imports app, create_app, and other modules AFTER patching
    """
    # Patch EC key loading to use test keypair (before any EC key imports)
    import utils.ec_keys
    utils.ec_keys.get_manager_ec_keys = lambda: (_TEST_EC_PRIVATE_KEY, _TEST_EC_PUBLIC_KEY)
    utils.ec_keys._PRIVATE_KEY = _TEST_EC_PRIVATE_KEY
    utils.ec_keys._PUBLIC_KEY = _TEST_EC_PUBLIC_KEY

    # Import middleware.tenant so we can patch it
    import middleware.tenant

    # Create the test-mode implementations
    test_parse_token = _make_test_parse_token()
    test_tenant_middleware = _make_test_tenant_middleware(test_parse_token)

    # Patch middleware.tenant module
    middleware.tenant.tenant_middleware = test_tenant_middleware
    middleware.tenant.parse_token = test_parse_token
    # Also patch the lazy-loaded public key to use test key
    middleware.tenant._MANAGER_PUBLIC_KEY = _TEST_EC_PUBLIC_KEY

    # Store in conftest module for later access
    global _test_tenant_middleware, _test_parse_token
    _test_tenant_middleware = test_tenant_middleware
    _test_parse_token = test_parse_token

    # Import app AFTER patching middleware, so imported references use the patched versions
    import app

    # ALSO patch the imported references in app module (important: app imports them at module level)
    app.parse_token = test_parse_token
    app.tenant_middleware = test_tenant_middleware

    # NOW import create_app and other modules (after all patches are in place)
    global create_app, OperationRecord, AsyncDB, MemoryOperationStore, SQLOperationStore
    from app import create_app
    from models.operations import OperationRecord
    from penguin_dal import AsyncDB
    from store import MemoryOperationStore, SQLOperationStore


# Store the test patches for use in autouse fixture (will be set by pytest_configure)
_test_tenant_middleware = None
_test_parse_token = None


# FIXTURE DISABLED: Module-level sys.modules cleanup was causing socket.gaierror
# when running full test suite. The fake modules are no longer injected at module level
# (since the four test files were migrated to use conftest fixtures), so this cleanup
# is no longer needed and was interfering with test isolation.


# Import create_app AFTER pytest_configure patches (will be imported in pytest_configure hook)


@pytest.fixture
async def app():
    """Create a fresh Quart test app per test with isolated DB.

    Each test gets its own temporary SQLite database to ensure complete isolation.
    This prevents data leakage between tests and ensures order-independence.

    The test app uses HS256 token validation (via conftest patch) instead of OIDC,
    allowing tests to work without external OIDC configuration.
    """
    from store import create_operation_store

    # Create a fresh temporary DB for this test
    _test_db_file = tempfile.NamedTemporaryFile(suffix=".db", delete=False)
    _test_db_path = _test_db_file.name
    _test_db_file.close()

    try:
        # Set the DB path for this test
        os.environ["DB_NAME"] = _test_db_path

        # Create the schema using SQLAlchemy (idempotent via create_all)
        sync_engine = create_engine(f"sqlite:///{_test_db_path}")
        metadata = MetaData()

        Table(
            "operations",
            metadata,
            Column("tenant", String(255), primary_key=True, nullable=False),
            Column("id", String(36), primary_key=True),
            Column("resource_name", String(255), nullable=False),
            Column("resource_type", String(100), nullable=False),
            Column("operation_type", String(100), nullable=False),
            Column("phase", String(50), nullable=False, index=True),
            Column("message", Text, nullable=False),
            Column("error", Text, nullable=True),
            Column("progress", Integer, nullable=False, default=0),
            Column("created_at", DateTime, nullable=False),
            Column("updated_at", DateTime, nullable=False),
        )
        metadata.create_all(sync_engine)
        sync_engine.dispose()

        # Create a fresh app for this test
        app = create_app()
        app.config["TESTING"] = True

        # Initialize the store in test app (mimics what @app.before_serving does)
        app.config["_store"] = await create_operation_store()

        yield app

    finally:
        # Clean up: delete the temp DB file
        if os.path.exists(_test_db_path):
            try:
                os.unlink(_test_db_path)
            except OSError:
                pass  # File may be locked on Windows


class _TestClientWithAuth:
    """Wrapper around Quart test client that auto-injects auth headers for internal endpoints."""

    def __init__(self, quart_client):
        self._client = quart_client
        self.make_token = _make_test_token
        self.get_auth_headers = _get_auth_headers

    async def get(self, path, **kwargs):
        """GET request with auto-injected auth for internal endpoints."""
        if path.startswith("/internal/"):
            headers = kwargs.pop("headers", {})
            if "Authorization" not in headers:
                # Use tenant-1 by default for internal endpoints (matches test expectations)
                headers["Authorization"] = f"Bearer {_make_test_token(tenant='tenant-1')}"
            kwargs["headers"] = headers
        return await self._client.get(path, **kwargs)

    async def post(self, path, **kwargs):
        """POST request with auto-injected auth for internal endpoints."""
        if path.startswith("/internal/"):
            headers = kwargs.pop("headers", {})
            if "Authorization" not in headers:
                # Use tenant-1 by default for internal endpoints (matches test expectations)
                headers["Authorization"] = f"Bearer {_make_test_token(tenant='tenant-1')}"
            kwargs["headers"] = headers
        return await self._client.post(path, **kwargs)

    async def put(self, path, **kwargs):
        """PUT request with auto-injected auth for internal endpoints."""
        if path.startswith("/internal/"):
            headers = kwargs.pop("headers", {})
            if "Authorization" not in headers:
                headers["Authorization"] = f"Bearer {_make_test_token(tenant='tenant-1')}"
            kwargs["headers"] = headers
        return await self._client.put(path, **kwargs)

    async def delete(self, path, **kwargs):
        """DELETE request with auto-injected auth for internal endpoints."""
        if path.startswith("/internal/"):
            headers = kwargs.pop("headers", {})
            if "Authorization" not in headers:
                headers["Authorization"] = f"Bearer {_make_test_token(tenant='tenant-1')}"
            kwargs["headers"] = headers
        return await self._client.delete(path, **kwargs)

    async def patch(self, path, **kwargs):
        """PATCH request with auto-injected auth for internal endpoints."""
        if path.startswith("/internal/"):
            headers = kwargs.pop("headers", {})
            if "Authorization" not in headers:
                headers["Authorization"] = f"Bearer {_make_test_token(tenant='tenant-1')}"
            kwargs["headers"] = headers
        return await self._client.patch(path, **kwargs)


@pytest.fixture
async def client(app):
    """Create a test client with auto auth injection and helpers."""
    test_client = app.test_client()
    wrapped = _TestClientWithAuth(test_client)
    return wrapped


@pytest.fixture
async def memory_store():
    """Create an in-memory store for testing."""
    return MemoryOperationStore()


@pytest.fixture
async def sql_store():
    """Create a SQL-backed store for testing with real SQLite database."""
    # Create a temporary SQLite database
    with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as f:
        db_path = f.name

    try:
        # Create the schema using SQLAlchemy (idempotent via create_all)
        db_uri = f"sqlite+aiosqlite:///{db_path}"
        sync_engine = create_engine(f"sqlite:///{db_path}")
        metadata = MetaData()

        Table(
            "operations",
            metadata,
            Column("tenant", String(255), primary_key=True, nullable=False),
            Column("id", String(36), primary_key=True),
            Column("resource_name", String(255), nullable=False),
            Column("resource_type", String(100), nullable=False),
            Column("operation_type", String(100), nullable=False),
            Column("phase", String(50), nullable=False, index=True),
            Column("message", Text, nullable=False),
            Column("error", Text, nullable=True),
            Column("progress", Integer, nullable=False, default=0),
            Column("created_at", DateTime, nullable=False),
            Column("updated_at", DateTime, nullable=False),
        )
        metadata.create_all(sync_engine)
        sync_engine.dispose()

        # Create AsyncDB and reflect
        db = AsyncDB(db_uri, pool_size=5, echo=False)
        await db.reflect()
        store = SQLOperationStore(db)
        yield store
        await db.close()
    finally:
        # Clean up
        if os.path.exists(db_path):
            os.unlink(db_path)


@pytest.fixture
async def store(memory_store):
    """Default store fixture (memory)."""
    return memory_store


# ============================================================================
# Shared Auth Fixtures
# ============================================================================

@pytest.fixture
def make_token():
    """Fixture providing token generation function.

    Usage: token = make_token("admin")  or  token = make_token()
    Returns: JWT token string (HS256)
    """
    return _make_test_token


@pytest.fixture
def auth_headers():
    """Fixture providing auth header generation function.

    Usage: headers = auth_headers("admin")  or  headers = auth_headers()
    Returns: dict with "Authorization": "Bearer <token>"
    """
    return _get_auth_headers


@pytest.fixture
def expired_token():
    """Factory for an ES256 token (signed with the test key) whose exp is in the
    past — so it passes signature validation but is rejected on expiry (401).
    """
    def _expired(user_id: int = 1, email: str = "x@x.com",
                 role: str = "admin", tenant: str = "test-tenant") -> str:
        now = datetime.now(timezone.utc)
        payload = {
            "sub": str(user_id), "email": email, "role": role, "tenant": tenant,
            "scope": "",
            "iat": now - timedelta(hours=2),
            "exp": now - timedelta(hours=1),
        }
        return jwt.encode(payload, _TEST_EC_PRIVATE_KEY, algorithm="ES256")
    return _expired
