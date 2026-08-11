"""JWT authentication middleware for Quart routes.

Implements JWT algorithm policy:
  1. Default: ES256 (Elliptic Curve P-256)
  2. Fallback: RS256 (RSA, from OIDC_JWKS_URL)
  3. Symmetric (HS256) only if JWT_ALLOW_HS256=true (admin override)
"""

import os
import functools
import logging
import json
import time
from datetime import datetime, timedelta, timezone
from typing import Optional, Any
from dataclasses import dataclass

import requests
from quart import request, jsonify, g
from jose import jwt, JWTError
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives import serialization

from .ec_keys import get_manager_ec_keys, get_ec_public_key_pem

log = logging.getLogger(__name__)

# ============================================================================
# Configuration
# ============================================================================

JWT_EXPIRY_HOURS = int(os.environ.get("JWT_EXPIRY_HOURS", "24"))
JWT_ALLOW_HS256 = os.environ.get("JWT_ALLOW_HS256", "false").lower() in ("true", "1", "yes")
JWT_SECRET = os.environ.get("JWT_SECRET", "")

# Load ES256 key (manager's own signing key)
_MANAGER_PRIVATE_KEY, _MANAGER_PUBLIC_KEY = get_manager_ec_keys()


# ============================================================================
# JWKS Cache (for RS256 validation)
# ============================================================================

@dataclass(slots=True)
class JWKSCache:
    """Cached JWKS data with TTL."""
    keys: list[dict]
    timestamp: float


_JWKS_CACHE: Optional[JWKSCache] = None
_JWKS_TTL = 300  # 5 minutes


def _get_jwks_keys() -> Optional[list[dict]]:
    """Fetch and cache JWKS keys with 5-minute TTL.

    Returns None if OIDC_JWKS_URL is not configured.
    """
    global _JWKS_CACHE

    jwks_url = os.getenv("OIDC_JWKS_URL", "").strip()
    if not jwks_url:
        return None

    now = time.time()
    # Check cache validity
    if _JWKS_CACHE and (now - _JWKS_CACHE.timestamp) < _JWKS_TTL:
        return _JWKS_CACHE.keys

    # Fetch fresh JWKS
    try:
        resp = requests.get(jwks_url, timeout=5)
        resp.raise_for_status()
        data = resp.json()
        keys = data.get("keys", [])

        # Update cache
        _JWKS_CACHE = JWKSCache(keys=keys, timestamp=now)

        return keys
    except Exception as e:
        log.error("Failed to fetch JWKS from %s: %s", jwks_url, e)
        # Fall back to cached keys if available
        return _JWKS_CACHE.keys if _JWKS_CACHE else None


def _get_key_from_jwks(kid: str, keys: list[dict]) -> Any:
    """Resolve a key by kid from JWKS keys (RSA format)."""
    from jwt.algorithms import RSAAlgorithm

    for key in keys:
        if key.get("kid") == kid:
            if key.get("kty") == "RSA":
                return RSAAlgorithm.from_jwk(json.dumps(key))
    raise jwt.exceptions.PyJWKClientError(
        f"Unable to find a signing key that matches: {kid}"
    )


# ============================================================================
# Token Creation (ES256 only)
# ============================================================================

def create_token(user_id: int, email: str, role: str, tenant: str = "default") -> str:
    """Create JWT token signed with ES256 (manager's private key).

    Args:
        user_id: User identifier
        email: User email
        role: User role (admin, maintainer, viewer)
        tenant: Tenant claim (required for tenant isolation)

    Returns:
        JWT token string signed with ES256.
    """
    payload = {
        "sub": str(user_id),
        "email": email,
        "role": role,
        "tenant": tenant,
        "iat": datetime.now(timezone.utc),
        "exp": datetime.now(timezone.utc) + timedelta(hours=JWT_EXPIRY_HOURS),
    }
    return jwt.encode(
        payload,
        _MANAGER_PRIVATE_KEY,
        algorithm="ES256"
    )


# ============================================================================
# Token Verification (Policy: ES256 → RS256 → HS256 with admin flag)
# ============================================================================

def decode_token(token: str) -> dict:
    """Decode and verify JWT token per algorithm policy.

    Policy order:
      1. ES256 (manager's own key) — try first
      2. RS256 (OIDC_JWKS_URL) — if ES256 fails and OIDC configured
      3. HS256 (symmetric) — only if JWT_ALLOW_HS256=true

    Args:
        token: JWT token string

    Returns:
        Decoded token payload dict

    Raises:
        JWTError: If token is invalid or doesn't match any algorithm policy
    """
    if not token:
        raise JWTError("Token is empty")

    # Get unverified header to inspect algorithm
    try:
        header = jwt.get_unverified_header(token)
        alg = header.get("alg", "unknown")
    except Exception as e:
        raise JWTError(f"Failed to parse token header: {e}") from e

    issuer = os.getenv("OIDC_ISSUER", "").strip()
    audience = os.getenv("OIDC_AUDIENCE", "nest-manager").strip()
    jwks_url = os.getenv("OIDC_JWKS_URL", "").strip()

    # ========================================================================
    # 1. Try ES256 (manager's own key)
    # ========================================================================
    if alg == "ES256":
        try:
            decoded = jwt.decode(
                token,
                _MANAGER_PUBLIC_KEY,
                algorithms=["ES256"],
                options={"verify_aud": False},  # Manager tokens don't have aud
            )
            log.debug("Token verified with ES256 (manager key)")
            return decoded
        except JWTError as e:
            log.debug("ES256 verification failed: %s", e)
            # Continue to next policy (RS256)

    # ========================================================================
    # 2. Try RS256 (OIDC_JWKS_URL)
    # ========================================================================
    if alg == "RS256" and jwks_url:
        try:
            # Get JWKS keys (cached with 5-min TTL)
            keys = _get_jwks_keys()
            if not keys:
                log.warning("Failed to fetch JWKS keys for RS256 verification")
            else:
                # Get the key ID from token header
                kid = header.get("kid")
                if not kid:
                    log.warning("RS256 token missing 'kid' in header")
                else:
                    try:
                        key = _get_key_from_jwks(kid, keys)
                        decoded = jwt.decode(
                            token,
                            key,
                            algorithms=["RS256"],
                            audience=audience,
                            issuer=issuer,
                        )
                        log.debug("Token verified with RS256 (OIDC key)")
                        return decoded
                    except jwt.exceptions.PyJWKClientError as e:
                        log.warning("Failed to resolve RS256 key: %s", e)
        except JWTError as e:
            log.debug("RS256 verification failed: %s", e)
            # Continue to next policy (HS256)

    # ========================================================================
    # 3. Try HS256 (symmetric, admin override only)
    # ========================================================================
    if alg == "HS256" and JWT_ALLOW_HS256 and JWT_SECRET:
        try:
            decoded = jwt.decode(
                token,
                JWT_SECRET,
                algorithms=["HS256"],
                options={"verify_aud": False},
            )
            log.warning("Token verified with HS256 (symmetric, admin override)")
            return decoded
        except JWTError as e:
            log.debug("HS256 verification failed: %s", e)
            # Fall through to error

    # ========================================================================
    # Policy rejected the token
    # ========================================================================
    if alg == "HS256" and not JWT_ALLOW_HS256:
        raise JWTError(
            "HS256 (symmetric) algorithm rejected by policy. "
            "Enable with JWT_ALLOW_HS256=true (admin override only)."
        )

    raise JWTError(f"Token verification failed with algorithm {alg}: not accepted by policy")


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
