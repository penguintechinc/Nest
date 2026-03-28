"""Comprehensive route tests for apps/manager covering auth, teams, and core routes."""
import os
import sys
import types
import pytest
import pytest_asyncio
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

    # Worker modules (imported inside before_serving)
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

# Import the app now that fakes are in place
sys.modules.pop("app", None)
import app as _app_module  # noqa: E402

_application = _app_module.app
_application.config["TESTING"] = True

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_GET_DB_TARGETS = [
    "routes.auth.get_db",
    "routes.database_servers.get_db",
]


def _make_db(user=None, count=0) -> MagicMock:
    """Build a fresh MagicMock DB returning *user* from any .select().first()."""
    db = MagicMock()
    select_result = MagicMock()
    select_result.first.return_value = user
    query_result = MagicMock()
    query_result.select.return_value = select_result
    query_result.count.return_value = count
    db.return_value = query_result   # db(...) returns query_result
    db.users = MagicMock()
    db.users.id = MagicMock()
    db.users.username = MagicMock()
    db.users.email = MagicMock()
    db.users.insert = MagicMock(return_value=1)
    db.database_server = MagicMock()
    db.database_server.insert = MagicMock(return_value=42)
    db.team_memberships = MagicMock()
    db.teams = MagicMock()
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
    from werkzeug.security import generate_password_hash
    u = MagicMock()
    u.id = user_id
    u.username = username
    u.email = email
    u.role = role
    u.is_active = is_active
    u.password_hash = password_hash or generate_password_hash("testpassword")
    u.first_name = "Test"
    u.last_name = "User"
    return u


def _make_token(role: str = "admin") -> str:
    from utils.auth import create_token
    return create_token(user_id=1, email="test@example.com", role=role)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


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


# ===========================================================================
# Health / built-in endpoints
# ===========================================================================

@pytest.mark.asyncio
async def test_healthz(app):
    client = app.test_client()
    resp = await client.get("/healthz")
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["status"] == "healthy"


@pytest.mark.asyncio
async def test_metrics_endpoint(app):
    client = app.test_client()
    resp = await client.get("/metrics")
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_404_handler(app):
    client = app.test_client()
    resp = await client.get("/no/such/path/xyz")
    assert resp.status_code == 404
    data = await resp.get_json()
    assert "error" in data


# ===========================================================================
# Auth helper unit tests (no HTTP)
# ===========================================================================

def test_verify_password_werkzeug():
    from werkzeug.security import generate_password_hash
    from routes.auth import _verify_password
    h = generate_password_hash("secret")
    assert _verify_password(h, "secret") is True
    assert _verify_password(h, "wrong") is False


def test_verify_password_sha256_legacy():
    import hashlib
    from routes.auth import _verify_password
    sha = hashlib.sha256("mypassword".encode()).hexdigest()
    assert _verify_password(sha, "mypassword") is True
    assert _verify_password(sha, "notmypassword") is False


def test_user_to_dict():
    from routes.auth import _user_to_dict
    user = _make_user(role="maintainer")
    result = _user_to_dict(user)
    assert result["id"] == 1
    assert result["email"] == "test@example.com"
    assert result["role"] == "maintainer"
    assert "password_hash" not in result
    assert "password" not in result


def test_user_to_dict_no_role_attr():
    """Falls back to 'viewer' when user lacks role attribute."""
    from routes.auth import _user_to_dict
    user = _make_user()
    del user.role
    result = _user_to_dict(user)
    assert result["role"] == "viewer"


def test_user_to_dict_is_active_truthy():
    from routes.auth import _user_to_dict
    user = _make_user(is_active=True)
    assert _user_to_dict(user)["is_active"] is True


def test_user_to_dict_is_active_falsy():
    from routes.auth import _user_to_dict
    user = _make_user(is_active=False)
    assert _user_to_dict(user)["is_active"] is False


# ===========================================================================
# POST /api/v1/auth/login
# ===========================================================================

@pytest.mark.asyncio
async def test_login_success(app, db):
    from werkzeug.security import generate_password_hash
    user = _make_user(password_hash=generate_password_hash("password123"))
    db.return_value.select.return_value.first.return_value = user
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/login",
        json={"username": "testuser", "password": "password123"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "token" in data
    assert "user" in data


@pytest.mark.asyncio
async def test_login_missing_body(app):
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/login",
        data=b"",
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_login_missing_password(app):
    client = app.test_client()
    resp = await client.post("/api/v1/auth/login", json={"username": "user"})
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_login_missing_username(app):
    client = app.test_client()
    resp = await client.post("/api/v1/auth/login", json={"password": "pass"})
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_login_both_empty_strings(app):
    """Whitespace-only credentials are also rejected."""
    client = app.test_client()
    resp = await client.post("/api/v1/auth/login", json={"username": "  ", "password": "  "})
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_login_user_not_found(app, db):
    db.return_value.select.return_value.first.return_value = None
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/login",
        json={"username": "ghost", "password": "pw"},
    )
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_login_inactive_user(app, db):
    from werkzeug.security import generate_password_hash
    user = _make_user(is_active=False, password_hash=generate_password_hash("pw"))
    db.return_value.select.return_value.first.return_value = user
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/login",
        json={"username": "testuser", "password": "pw"},
    )
    assert resp.status_code == 401
    data = await resp.get_json()
    assert "disabled" in data["error"]


@pytest.mark.asyncio
async def test_login_wrong_password(app, db):
    from werkzeug.security import generate_password_hash
    user = _make_user(password_hash=generate_password_hash("correct"))
    db.return_value.select.return_value.first.return_value = user
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/login",
        json={"username": "testuser", "password": "wrong"},
    )
    assert resp.status_code == 401
    data = await resp.get_json()
    assert "invalid" in data["error"]


@pytest.mark.asyncio
async def test_login_sha256_legacy(app, db):
    """Login works for legacy SHA-256 password hash accounts."""
    import hashlib
    sha = hashlib.sha256("legacy123".encode()).hexdigest()
    user = _make_user(password_hash=sha)
    db.return_value.select.return_value.first.return_value = user
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/login",
        json={"username": "testuser", "password": "legacy123"},
    )
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_login_no_role_defaults_viewer(app, db):
    """User without role attribute gets viewer token."""
    from werkzeug.security import generate_password_hash
    user = _make_user(password_hash=generate_password_hash("pw"))
    del user.role  # remove role attribute
    db.return_value.select.return_value.first.return_value = user
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/login",
        json={"username": "testuser", "password": "pw"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "token" in data


# ===========================================================================
# POST /api/v1/auth/logout
# ===========================================================================

@pytest.mark.asyncio
async def test_logout_success(app):
    token = _make_token()
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/logout",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "message" in data


@pytest.mark.asyncio
async def test_logout_no_token(app):
    client = app.test_client()
    resp = await client.post("/api/v1/auth/logout")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_logout_invalid_token(app):
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/logout",
        headers={"Authorization": "Bearer not.a.valid.jwt"},
    )
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_logout_basic_scheme_rejected(app):
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/logout",
        headers={"Authorization": "Basic dXNlcjpwYXNz"},
    )
    assert resp.status_code == 401


# ===========================================================================
# GET /api/v1/auth/me
# ===========================================================================

@pytest.mark.asyncio
async def test_me_success(app, db):
    user = _make_user()
    db.return_value.select.return_value.first.return_value = user
    token = _make_token()
    client = app.test_client()
    resp = await client.get(
        "/api/v1/auth/me",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "user" in data
    assert data["user"]["email"] == "test@example.com"


@pytest.mark.asyncio
async def test_me_user_not_found(app, db):
    db.return_value.select.return_value.first.return_value = None
    token = _make_token()
    client = app.test_client()
    resp = await client.get(
        "/api/v1/auth/me",
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_me_unauthenticated(app):
    client = app.test_client()
    resp = await client.get("/api/v1/auth/me")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_me_expired_token(app):
    from datetime import datetime, timedelta, timezone
    from jose import jwt
    from utils.auth import JWT_SECRET, JWT_ALGORITHM
    payload = {
        "sub": "1", "email": "x@x.com", "role": "admin",
        "iat": datetime.now(timezone.utc) - timedelta(hours=2),
        "exp": datetime.now(timezone.utc) - timedelta(hours=1),
    }
    expired = jwt.encode(payload, JWT_SECRET, algorithm=JWT_ALGORITHM)
    client = app.test_client()
    resp = await client.get(
        "/api/v1/auth/me",
        headers={"Authorization": f"Bearer {expired}"},
    )
    assert resp.status_code == 401


# ===========================================================================
# POST /api/v1/auth/register
# ===========================================================================

@pytest.mark.asyncio
async def test_register_success(app, db):
    """Admin creates a new user."""
    new_user = _make_user(user_id=2, username="newuser", email="new@example.com")
    # Two DB calls: conflict check (None) then fetch after insert (new_user)
    db.return_value.select.return_value.first.side_effect = [None, new_user]
    db.users.insert.return_value = 2
    token = _make_token(role="admin")
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        json={
            "username": "newuser",
            "email": "new@example.com",
            "password": "Passw0rd!",
            "role": "viewer",
        },
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 201
    data = await resp.get_json()
    assert data["username"] == "newuser"


@pytest.mark.asyncio
async def test_register_missing_fields(app):
    token = _make_token(role="admin")
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        json={"username": "u"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_register_invalid_role(app):
    token = _make_token(role="admin")
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        json={"username": "u", "email": "e@e.com", "password": "p", "role": "superuser"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 400
    data = await resp.get_json()
    assert "role" in data["error"]


@pytest.mark.asyncio
async def test_register_conflict(app, db):
    existing = _make_user()
    db.return_value.select.return_value.first.return_value = existing
    token = _make_token(role="admin")
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        json={"username": "testuser", "email": "test@example.com", "password": "Passw0rd!"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 409


@pytest.mark.asyncio
async def test_register_requires_admin(app):
    token = _make_token(role="viewer")
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        json={"username": "u", "email": "u@u.com", "password": "p"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_register_maintainer_rejected(app):
    token = _make_token(role="maintainer")
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        json={"username": "u", "email": "u@u.com", "password": "p"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_register_no_token(app):
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        json={"username": "u", "email": "u@u.com", "password": "p"},
    )
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_register_no_body(app):
    token = _make_token(role="admin")
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        data=b"",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        },
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_register_all_valid_roles(app, db):
    """Admin can register with each valid role value."""
    for role in ("admin", "maintainer", "viewer"):
        new_user = _make_user(user_id=10, username=f"user_{role}", role=role)
        db.return_value.select.return_value.first.side_effect = [None, new_user]
        db.users.insert.return_value = 10
        token = _make_token(role="admin")
        client = app.test_client()
        resp = await client.post(
            "/api/v1/auth/register",
            json={
                "username": f"user_{role}",
                "email": f"{role}@example.com",
                "password": "Passw0rd!",
                "role": role,
            },
            headers={"Authorization": f"Bearer {token}"},
        )
        assert resp.status_code == 201, f"Failed for role={role}: {await resp.get_json()}"


# ===========================================================================
# GET /api/v1/servers
# ===========================================================================

@pytest.mark.asyncio
async def test_list_servers_200(app, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    db.return_value.count.return_value = 0
    client = app.test_client()
    resp = await client.get("/api/v1/servers")
    assert resp.status_code == 200
    data = await resp.get_json()
    assert "data" in data
    assert "meta" in data


@pytest.mark.asyncio
async def test_list_servers_pagination(app, db):
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    db.return_value.count.return_value = 15
    client = app.test_client()
    resp = await client.get("/api/v1/servers?page=2&per_page=5")
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["meta"]["page"] == 2
    assert data["meta"]["per_page"] == 5


@pytest.mark.asyncio
async def test_list_servers_invalid_page_clamped(app, db):
    """page=-1 is clamped to 1; per_page=999 clamped to 20."""
    db.return_value.select.return_value.__iter__ = MagicMock(return_value=iter([]))
    db.return_value.count.return_value = 0
    client = app.test_client()
    resp = await client.get("/api/v1/servers?page=-1&per_page=999")
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["meta"]["page"] == 1
    assert data["meta"]["per_page"] == 20


# ===========================================================================
# POST /api/v1/servers
# ===========================================================================

@pytest.mark.asyncio
async def test_create_server_success(app, db):
    # Route does: db.database_server[server_id].as_dict()
    server_dict = {
        "id": 42, "name": "pg-prod", "host": "db.local",
        "port": 5432, "db_type": "postgresql", "active": True,
    }
    server_row = MagicMock()
    server_row.as_dict.return_value = server_dict
    db.database_server.insert.return_value = 42
    db.database_server.__getitem__ = MagicMock(return_value=server_row)
    token = _make_token()
    client = app.test_client()
    resp = await client.post(
        "/api/v1/servers",
        json={"name": "pg-prod", "host": "db.local", "port": 5432, "db_type": "postgresql"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 201
    data = await resp.get_json()
    assert "data" in data


@pytest.mark.asyncio
async def test_create_server_missing_fields(app):
    token = _make_token()
    client = app.test_client()
    resp = await client.post(
        "/api/v1/servers",
        json={"name": "only-name"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 400
    data = await resp.get_json()
    assert "Missing" in data["error"]


@pytest.mark.asyncio
async def test_create_server_no_body(app):
    token = _make_token()
    client = app.test_client()
    resp = await client.post(
        "/api/v1/servers",
        data=b"",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        },
    )
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_create_server_unauthenticated(app):
    client = app.test_client()
    resp = await client.post(
        "/api/v1/servers",
        json={"name": "x", "host": "h", "port": 5432, "db_type": "pg"},
    )
    assert resp.status_code == 401


# ===========================================================================
# require_auth / require_role decorator behaviour
# ===========================================================================

@pytest.mark.asyncio
async def test_require_auth_no_header(app):
    client = app.test_client()
    resp = await client.post("/api/v1/auth/logout")
    assert resp.status_code == 401
    data = await resp.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_require_auth_bad_scheme(app):
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/logout",
        headers={"Authorization": "Basic dXNlcjpwYXNz"},
    )
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_require_role_maintainer_blocked_admin_route(app):
    token = _make_token(role="maintainer")
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        json={"username": "u", "email": "e@e.com", "password": "p"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_require_role_viewer_blocked_admin_route(app):
    token = _make_token(role="viewer")
    client = app.test_client()
    resp = await client.post(
        "/api/v1/auth/register",
        json={"username": "u", "email": "e@e.com", "password": "p"},
        headers={"Authorization": f"Bearer {token}"},
    )
    assert resp.status_code == 403
    data = await resp.get_json()
    assert "Insufficient" in data["error"]
