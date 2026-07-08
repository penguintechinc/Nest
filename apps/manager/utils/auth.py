"""JWT authentication middleware for Quart routes."""
import os
import functools
from datetime import datetime, timedelta, timezone
from quart import request, jsonify, g
from jose import jwt, JWTError

# FAIL CLOSED: JWT_SECRET must be explicitly set; no insecure default
JWT_SECRET = os.environ.get("JWT_SECRET", "")
if not JWT_SECRET:
    raise RuntimeError(
        "JWT_SECRET environment variable is required and must be set to a secure value. "
        "Do not use the default 'dev-secret-key-change-in-prod' in any environment."
    )

JWT_ALGORITHM = "HS256"
JWT_EXPIRY_HOURS = int(os.environ.get("JWT_EXPIRY_HOURS", "24"))


def create_token(user_id: int, email: str, role: str) -> str:
    payload = {
        "sub": str(user_id),
        "email": email,
        "role": role,
        "iat": datetime.now(timezone.utc),
        "exp": datetime.now(timezone.utc) + timedelta(hours=JWT_EXPIRY_HOURS),
    }
    return jwt.encode(payload, JWT_SECRET, algorithm=JWT_ALGORITHM)


def decode_token(token: str) -> dict:
    return jwt.decode(token, JWT_SECRET, algorithms=[JWT_ALGORITHM])


def require_auth(f):
    """Quart decorator that validates Bearer JWT token."""
    @functools.wraps(f)
    async def decorated(*args, **kwargs):
        auth_header = request.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            return jsonify({"error": "Missing or invalid Authorization header"}), 401
        token = auth_header[7:]
        try:
            payload = decode_token(token)
            g.user_id = int(payload["sub"])
            g.user_email = payload.get("email", "")
            g.user_role = payload.get("role", "viewer")
        except JWTError as e:
            return jsonify({"error": "Invalid or expired token"}), 401
        return await f(*args, **kwargs)
    return decorated


def require_role(role: str):
    """Decorator that requires a specific role (admin, maintainer, viewer)."""
    role_hierarchy = {"admin": 3, "maintainer": 2, "viewer": 1}

    def decorator(f):
        @functools.wraps(f)
        @require_auth
        async def decorated(*args, **kwargs):
            user_role = getattr(g, "user_role", "viewer")
            if role_hierarchy.get(user_role, 0) < role_hierarchy.get(role, 0):
                return jsonify({"error": "Insufficient permissions"}), 403
            return await f(*args, **kwargs)
        return decorated
    return decorator
