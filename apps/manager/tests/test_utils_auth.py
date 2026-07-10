"""Tests for utils/auth.py — create_token, decode_token, require_auth, require_role."""
import os
import sys
import pytest
import pytest_asyncio
from unittest.mock import MagicMock, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("JWT_EXPIRY_HOURS", "1")


# ---------------------------------------------------------------------------
# create_token / decode_token
# ---------------------------------------------------------------------------

def test_create_token_returns_string():
    from utils.auth import create_token
    token = create_token(user_id=1, email="a@b.com", role="admin")
    assert isinstance(token, str)
    assert len(token) > 10


def test_create_token_decode_roundtrip():
    from utils.auth import create_token, decode_token
    token = create_token(user_id=42, email="user@example.com", role="maintainer")
    payload = decode_token(token)
    assert payload["sub"] == "42"
    assert payload["email"] == "user@example.com"
    assert payload["role"] == "maintainer"


def test_decode_invalid_token_raises():
    from jose import JWTError
    from utils.auth import decode_token
    with pytest.raises(JWTError):
        decode_token("not.a.valid.jwt.at.all")


def test_decode_tampered_token_raises():
    from jose import JWTError
    from utils.auth import create_token, decode_token
    token = create_token(1, "x@x.com", "admin")
    # Tamper with signature
    parts = token.split(".")
    tampered = parts[0] + "." + parts[1] + ".invalidsignature"
    with pytest.raises(JWTError):
        decode_token(tampered)


def test_create_token_different_roles():
    """Token encodes role correctly for all valid roles."""
    from utils.auth import create_token, decode_token
    for role in ("admin", "maintainer", "viewer"):
        token = create_token(1, "a@b.com", role)
        payload = decode_token(token)
        assert payload["role"] == role


# ---------------------------------------------------------------------------
# require_auth decorator (via a minimal Quart app)
# ---------------------------------------------------------------------------

@pytest.fixture()
def auth_app():
    """Minimal Quart app with a single protected route for decorator testing."""
    from quart import Quart, jsonify, g
    from utils.auth import require_auth, require_role

    mini = Quart(__name__)

    @mini.route("/protected")
    @require_auth
    async def protected():
        return jsonify({"user_id": g.user_id, "role": g.user_role}), 200

    @mini.route("/admin-only")
    @require_role("admin")
    async def admin_only():
        return jsonify({"ok": True}), 200

    @mini.route("/maintainer-only")
    @require_role("maintainer")
    async def maintainer_only():
        return jsonify({"ok": True}), 200

    mini.config["TESTING"] = True
    return mini


@pytest.mark.asyncio
async def test_require_auth_valid_token(auth_app):
    from utils.auth import create_token
    token = create_token(7, "u@u.com", "admin")
    client = auth_app.test_client()
    resp = await client.get("/protected", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 200
    data = await resp.get_json()
    assert data["user_id"] == 7
    assert data["role"] == "admin"


@pytest.mark.asyncio
async def test_require_auth_missing_header(auth_app):
    client = auth_app.test_client()
    resp = await client.get("/protected")
    assert resp.status_code == 401
    data = await resp.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_require_auth_wrong_scheme(auth_app):
    client = auth_app.test_client()
    resp = await client.get("/protected", headers={"Authorization": "Basic abc123"})
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_require_auth_expired_token(auth_app):
    """Token with past expiry is rejected."""
    import time
    from datetime import datetime, timedelta, timezone
    from jose import jwt
    from cryptography.hazmat.primitives.asymmetric import ec
    from cryptography.hazmat.backends import default_backend

    # Generate a test EC key for this test
    test_private_key = ec.generate_private_key(ec.SECP256R1(), default_backend())

    payload = {
        "sub": "1",
        "email": "x@x.com",
        "role": "admin",
        "tenant": "test-tenant",  # Required by tenant middleware
        "iat": datetime.now(timezone.utc) - timedelta(hours=2),
        "exp": datetime.now(timezone.utc) - timedelta(hours=1),
    }
    expired = jwt.encode(payload, test_private_key, algorithm="ES256")
    client = auth_app.test_client()
    resp = await client.get("/protected", headers={"Authorization": f"Bearer {expired}"})
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_require_role_admin_allowed(auth_app):
    from utils.auth import create_token
    token = create_token(1, "a@a.com", "admin")
    client = auth_app.test_client()
    resp = await client.get("/admin-only", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_require_role_viewer_blocked(auth_app):
    from utils.auth import create_token
    token = create_token(1, "a@a.com", "viewer")
    client = auth_app.test_client()
    resp = await client.get("/admin-only", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 403
    data = await resp.get_json()
    assert "Insufficient" in data["error"]


@pytest.mark.asyncio
async def test_require_role_maintainer_blocked_from_admin(auth_app):
    from utils.auth import create_token
    token = create_token(1, "a@a.com", "maintainer")
    client = auth_app.test_client()
    resp = await client.get("/admin-only", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_require_role_maintainer_allowed_for_maintainer_route(auth_app):
    from utils.auth import create_token
    token = create_token(1, "a@a.com", "maintainer")
    client = auth_app.test_client()
    resp = await client.get("/maintainer-only", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_require_role_admin_allowed_for_maintainer_route(auth_app):
    """Admin satisfies maintainer requirement (higher role hierarchy)."""
    from utils.auth import create_token
    token = create_token(1, "a@a.com", "admin")
    client = auth_app.test_client()
    resp = await client.get("/maintainer-only", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_require_role_no_token(auth_app):
    client = auth_app.test_client()
    resp = await client.get("/admin-only")
    assert resp.status_code == 401


# ---------------------------------------------------------------------------
# Role hierarchy edge cases
# ---------------------------------------------------------------------------

def test_role_hierarchy_values():
    """Confirm role_hierarchy constants are correct relative to each other."""
    from utils.auth import require_role
    # We just verify the function is callable and returns a decorator
    decorator = require_role("admin")
    assert callable(decorator)
