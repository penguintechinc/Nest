"""
Authentication routes — ported from apps/api/controllers/auth.go.

Port notes:
- Go used SHA-256 (models/user.go BeforeSave/VerifyPassword) for password hashing.
  The existing users table stores werkzeug pbkdf2 hashes created by flask-security-too
  (password_hash column). We check werkzeug first; if that fails we fall back to
  SHA-256 so tokens minted by the old Go service remain valid during migration.
- JWT secret and algorithm match utils/auth.py (JWT_SECRET env var, HS256).
- Login accepts username OR email (Go accepted username only; we extend to email
  for parity with flask-security-too conventions used elsewhere in the manager).
- POST /auth/logout is stateless — JWT is client-side; server returns 200.
- POST /auth/register requires admin role, matching Go's CreateUser handler.
"""

import asyncio
import hashlib
import logging
from typing import Any

from quart import Blueprint, g, jsonify, request
from werkzeug.security import check_password_hash, generate_password_hash

from penguin_dal.quart_ext import get_db
from utils.auth import create_token, require_auth, require_role

logger = logging.getLogger(__name__)

auth_bp = Blueprint("auth", __name__, url_prefix="/api/v1")


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _verify_password(stored_hash: str, plaintext: str) -> bool:
    """Check password against stored hash.

    Supports two formats in priority order:
    1. Werkzeug pbkdf2/scrypt hashes (flask-security-too / manager-native accounts).
    2. SHA-256 hex strings (legacy accounts created by the Go API service).
    """
    # Werkzeug hashes start with "pbkdf2:" or "scrypt:" etc.
    if stored_hash.startswith(("pbkdf2:", "scrypt:", "bcrypt:")):
        return check_password_hash(stored_hash, plaintext)
    # Fall back to SHA-256 for Go-API–created accounts
    sha_hash = hashlib.sha256(plaintext.encode()).hexdigest()
    return stored_hash == sha_hash


def _user_to_dict(user: Any) -> dict:
    """Serialize a PyDAL user Row to a safe response dict (no password)."""
    return {
        "id": int(user.id),
        "username": user.username,
        "email": user.email,
        "first_name": user.first_name or "",
        "last_name": user.last_name or "",
        "role": user.role if hasattr(user, "role") else "viewer",
        "is_active": bool(user.is_active),
    }


def _lookup_user(identifier: str) -> Any | None:


    db = get_db()
    """Return a user row by username or email, or None if not found."""
    row = db(
        (db.users.username == identifier) | (db.users.email == identifier)
    ).select(limitby=(0, 1)).first()
    return row


# ---------------------------------------------------------------------------
# POST /api/v1/auth/login
# ---------------------------------------------------------------------------

@auth_bp.route("/auth/login", methods=["POST"])
async def login() -> tuple:
    """Validate credentials and return a JWT token.

    Accepts JSON: {"username": "...", "password": "..."}
    username may be a username or email address.
    """
    body = await request.get_json(silent=True)
    if not body:
        return jsonify({"error": "invalid request body"}), 400

    identifier = (body.get("username") or "").strip()
    password = (body.get("password") or "").strip()

    if not identifier or not password:
        return jsonify({"error": "username and password are required"}), 400

    # PyDAL query runs synchronously — offload to thread.
    def _do_lookup() -> Any | None:
        db = get_db()
        return _lookup_user(identifier)

    user = await asyncio.to_thread(_do_lookup)

    if user is None:
        return jsonify({"error": "invalid username or password"}), 401

    if not bool(user.is_active):
        return jsonify({"error": "account is disabled"}), 401

    if not _verify_password(user.password_hash, password):
        return jsonify({"error": "invalid username or password"}), 401

    # Determine role for token; fall back to "viewer" for legacy rows.
    role: str = getattr(user, "role", None) or "viewer"

    token = create_token(user_id=int(user.id), email=user.email, role=role)

    logger.info("User logged in: id=%d username=%s", int(user.id), user.username)

    return jsonify({
        "token": token,
        "user": _user_to_dict(user),
    }), 200


# ---------------------------------------------------------------------------
# POST /api/v1/auth/logout
# ---------------------------------------------------------------------------

@auth_bp.route("/auth/logout", methods=["POST"])
@require_auth
async def logout() -> tuple:
    """Stateless logout — instructs the client to discard its token."""
    logger.info("User logged out: id=%d", g.user_id)
    return jsonify({"message": "successfully logged out"}), 200


# ---------------------------------------------------------------------------
# GET /api/v1/auth/me
# ---------------------------------------------------------------------------

@auth_bp.route("/auth/me", methods=["GET"])
@require_auth
async def me() -> tuple:
    """Return the current authenticated user's profile."""
    user_id: int = g.user_id

    def _do_lookup() -> Any | None:

        db = get_db()
        return db(db.users.id == user_id).select(limitby=(0, 1)).first()

    user = await asyncio.to_thread(_do_lookup)

    if user is None:
        return jsonify({"error": "user not found"}), 404

    return jsonify({"user": _user_to_dict(user)}), 200


# ---------------------------------------------------------------------------
# POST /api/v1/auth/register
# ---------------------------------------------------------------------------

@auth_bp.route("/auth/register", methods=["POST"])
@require_role("admin")
async def register() -> tuple:
    """Create a new user account.  Requires admin role.

    Accepts JSON: {"username": "...", "email": "...", "password": "...",
                   "first_name": "...", "last_name": "...", "role": "..."}
    """
    body = await request.get_json(silent=True)
    if not body:
        return jsonify({"error": "invalid request body"}), 400

    username: str = (body.get("username") or "").strip()
    email: str = (body.get("email") or "").strip()
    password: str = (body.get("password") or "").strip()
    first_name: str = (body.get("first_name") or "").strip()
    last_name: str = (body.get("last_name") or "").strip()
    role: str = (body.get("role") or "viewer").strip()

    if not username or not email or not password:
        return jsonify({"error": "username, email, and password are required"}), 400

    valid_roles = {"admin", "maintainer", "viewer"}
    if role not in valid_roles:
        return jsonify({"error": f"role must be one of {sorted(valid_roles)}"}), 400

    # Hash password with werkzeug (pbkdf2:sha256).
    password_hash = generate_password_hash(password)

    def _do_create() -> dict:

        db = get_db()
        # Check uniqueness.
        existing = db(
            (db.users.username == username) | (db.users.email == email)
        ).select(limitby=(0, 1)).first()
        if existing is not None:
            return {"conflict": True}

        user_id = db.users.insert(
            username=username,
            email=email,
            password_hash=password_hash,
            first_name=first_name,
            last_name=last_name,
            is_active=True,
        )
        db.commit()
        user = db(db.users.id == user_id).select(limitby=(0, 1)).first()
        return {"user": user}

    result = await asyncio.to_thread(_do_create)

    if result.get("conflict"):
        return jsonify({"error": "username or email already exists"}), 409

    user = result["user"]
    logger.info(
        "New user created: id=%d username=%s by admin id=%d",
        int(user.id), user.username, g.user_id,
    )
    return jsonify(_user_to_dict(user)), 201
