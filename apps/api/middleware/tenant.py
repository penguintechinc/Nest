"""Tenant middleware for authentication."""
import json
import logging
import os
import time
import uuid
from dataclasses import dataclass
from functools import wraps
from typing import Optional

import jwt
import requests
from quart import g, request
from werkzeug.exceptions import Unauthorized, Forbidden

log = logging.getLogger(__name__)


@dataclass(slots=True)
class Claims:
    """JWT claims subset Nest cares about."""

    sub: str
    tenant: str
    scopes: list[str]
    tier: str


TENANT_KEY = "nest_tenant"
CLAIMS_KEY = "nest_claims"

# JWKS cache: { "jwks_data": {}, "timestamp": float }
_JWKS_CACHE: dict = {}
_JWKS_TTL = 300  # 5 minutes


def _get_jwks_keys() -> Optional[list]:
    """Fetch and cache JWKS keys with 5-minute TTL.

    Returns None if OIDC_JWKS_URL is not configured.
    """
    jwks_url = os.getenv("OIDC_JWKS_URL", "").strip()
    if not jwks_url:
        return None

    now = time.time()
    # Check cache validity
    if _JWKS_CACHE and (now - _JWKS_CACHE.get("timestamp", 0)) < _JWKS_TTL:
        return _JWKS_CACHE.get("keys", [])

    # Fetch fresh JWKS
    try:
        resp = requests.get(jwks_url, timeout=5)
        resp.raise_for_status()
        data = resp.json()
        keys = data.get("keys", [])

        # Update cache
        _JWKS_CACHE.update({"keys": keys, "timestamp": now})

        return keys
    except Exception as e:
        log.error(f"Failed to fetch JWKS from {jwks_url}: {e}")
        # Fall back to cached keys if available
        return _JWKS_CACHE.get("keys")


def _get_key_from_jwks(kid: str, keys: list):
    """Resolve a key by kid from JWKS keys."""
    from jwt.algorithms import RSAAlgorithm

    for key in keys:
        if key.get("kid") == kid:
            # Reconstruct public key from JWK
            if key.get("kty") == "RSA":
                return RSAAlgorithm.from_jwk(json.dumps(key))
    raise jwt.exceptions.PyJWKClientError(
        f"Unable to find a signing key that matches: {kid}"
    )


def nest_error(code: str, message: str, request_id: str) -> dict:
    """Generate a Nest API error response."""
    return {
        "code": code,
        "message": message,
        "requestId": request_id,
        "docsUrl": f"https://docs.nest.penguintech.io/errors/{code}",
    }


def parse_token(token: str) -> Optional[Claims]:
    """Parse JWT token with OIDC JWKS validation.

    Falls back to dev mode (sub:tenant:tier format) if OIDC_JWKS_URL not configured.
    """
    if not token:
        return None

    jwks_url = os.getenv("OIDC_JWKS_URL", "").strip()

    # Development mode: fall back to simple format
    if not jwks_url:
        log.warning(
            "OIDC_JWKS_URL not configured — using dev token format sub:tenant:tier"
        )
        parts = token.split(":", 3)
        return Claims(
            sub=parts[0] if len(parts) > 0 else "",
            tenant=parts[1] if len(parts) > 1 else "",
            scopes=["nest:*:admin"],
            tier=parts[2] if len(parts) > 2 else "free",
        )

    # Production mode: validate JWT with JWKS
    try:
        issuer = os.getenv("OIDC_ISSUER", "").strip()
        audience = os.getenv("OIDC_AUDIENCE", "nest-api").strip()

        if not issuer:
            log.error("OIDC_ISSUER required when OIDC_JWKS_URL is configured")
            return None

        # Get JWKS keys (cached with 5-min TTL)
        keys = _get_jwks_keys()
        if not keys:
            log.error("Failed to fetch or retrieve cached JWKS keys")
            return None

        # Get the key ID from token header (without verification first)
        header = jwt.get_unverified_header(token)
        kid = header.get("kid")

        if not kid:
            log.warning("JWT token missing 'kid' in header")
            return None

        # Get the signing key
        key = _get_key_from_jwks(kid, keys)

        # Decode and validate JWT
        decoded = jwt.decode(
            token,
            key,
            algorithms=["RS256"],
            audience=audience,
            issuer=issuer,
        )

        # Extract required claims
        sub = decoded.get("sub", "")
        tenant = decoded.get("tenant", "")
        scope_str = decoded.get("scope", "")
        tier = decoded.get("https://nest.penguintech.io/tier", "free")

        # Parse scopes (space-separated string → list)
        scopes = scope_str.split() if scope_str else []

        return Claims(sub=sub, tenant=tenant, scopes=scopes, tier=tier)

    except jwt.ExpiredSignatureError:
        log.warning("JWT token expired")
        return None
    except jwt.InvalidTokenError as e:
        log.warning(f"Invalid JWT token: {e}")
        return None
    except Exception as e:
        log.error(f"Unexpected error parsing JWT: {e}")
        return None


async def tenant_middleware():
    """Middleware that enforces presence of tenant claim.

    Runs before all /api/v1/tenants/ routes.
    Stores claims in g context for per-request access.
    """
    auth = request.headers.get("Authorization", "")
    request_id = request.headers.get("X-Request-ID", str(uuid.uuid4()))

    if not auth.startswith("Bearer "):
        raise Unauthorized(
            response=nest_error(
                "nest.auth.missing_token",
                "Authorization header with Bearer token required",
                request_id,
            )
        )

    token = auth[7:]  # Remove "Bearer " prefix
    claims = parse_token(token)

    if not claims or not claims.tenant:
        raise Forbidden(
            response=nest_error(
                "nest.auth.missing_tenant",
                "JWT missing mandatory tenant claim",
                request_id,
            )
        )

    # Store in g for request scope
    g.tenant = claims.tenant
    g.claims = claims
    g.request_id = request_id


def get_tenant() -> str:
    """Extract the validated tenant from request context."""
    return getattr(g, "tenant", "")


def get_claims() -> Optional[Claims]:
    """Extract the validated claims from request context."""
    return getattr(g, "claims", None)


def require_scope(scope: str):
    """Decorator to require a specific scope on the request."""

    def decorator(f):
        @wraps(f)
        async def decorated_function(*args, **kwargs):
            claims = get_claims()
            request_id = getattr(g, "request_id", str(uuid.uuid4()))

            if not claims:
                raise Forbidden(
                    response=nest_error(
                        "nest.auth.scope_denied",
                        "Authentication required",
                        request_id,
                    )
                )

            # Check if token has the scope or admin scope
            has_scope = False
            for s in claims.scopes:
                if s == scope or s == "nest:*:admin":
                    has_scope = True
                    break

            if not has_scope:
                raise Forbidden(
                    response=nest_error(
                        "nest.auth.scope_denied",
                        f"Insufficient scope: {scope} required",
                        request_id,
                    )
                )

            return await f(*args, **kwargs)

        return decorated_function

    return decorator
