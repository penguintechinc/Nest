"""Tenant middleware for extracting tenant from JWT Authorization header."""

import json
import logging
import os
import time
from typing import Optional

import jwt
import requests
from quart import g, request
from werkzeug.exceptions import Unauthorized, Forbidden

log = logging.getLogger(__name__)

# JWKS cache: { "keys": [], "timestamp": float }
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


def parse_token(token: str) -> Optional[dict]:
    """Parse JWT token with OIDC JWKS validation.

    Returns a dict with sub, tenant, tier, scopes if valid, None otherwise.
    Raises Unauthorized if token is malformed or verification fails.
    """
    if not token:
        return None

    jwks_url = os.getenv("OIDC_JWKS_URL", "").strip()

    # OIDC required: fail closed if not configured
    if not jwks_url:
        log.error(
            "OIDC_JWKS_URL not configured — cannot validate JWT tokens"
        )
        raise Unauthorized(
            response={
                "error": "auth.oidc_not_configured",
                "message": "OIDC authentication not configured on this service",
            }
        )

    # Production mode: validate JWT with JWKS
    try:
        issuer = os.getenv("OIDC_ISSUER", "").strip()
        audience = os.getenv("OIDC_AUDIENCE", "nest-manager").strip()

        if not issuer:
            log.error("OIDC_ISSUER required when OIDC_JWKS_URL is configured")
            raise Unauthorized(
                response={
                    "error": "auth.issuer_not_configured",
                    "message": "OIDC issuer not configured",
                }
            )

        # Get JWKS keys (cached with 5-min TTL)
        keys = _get_jwks_keys()
        if not keys:
            log.error("Failed to fetch or retrieve cached JWKS keys")
            raise Unauthorized(
                response={
                    "error": "auth.jwks_unavailable",
                    "message": "OIDC keys unavailable",
                }
            )

        # Get the key ID from token header (without verification first)
        header = jwt.get_unverified_header(token)
        kid = header.get("kid")

        if not kid:
            log.warning("JWT token missing 'kid' in header")
            raise Unauthorized(
                response={
                    "error": "auth.invalid_token",
                    "message": "JWT token missing key ID",
                }
            )

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

        if not tenant:
            log.warning("JWT token missing tenant claim")
            raise Unauthorized(
                response={
                    "error": "auth.missing_tenant",
                    "message": "JWT missing mandatory tenant claim",
                }
            )

        # Parse scopes (space-separated string → list)
        scopes = scope_str.split() if scope_str else []

        return {
            "sub": sub,
            "tenant": tenant,
            "tier": tier,
            "scopes": scopes,
        }

    except jwt.ExpiredSignatureError:
        log.warning("JWT token expired")
        raise Unauthorized(
            response={
                "error": "auth.token_expired",
                "message": "JWT token has expired",
            }
        )
    except jwt.InvalidTokenError as e:
        log.warning(f"Invalid JWT token: {e}")
        raise Unauthorized(
            response={
                "error": "auth.invalid_token",
                "message": "JWT token is invalid or could not be verified",
            }
        )
    except Unauthorized:
        raise
    except Exception as e:
        log.error(f"Unexpected error parsing JWT: {e}")
        raise Unauthorized(
            response={
                "error": "auth.error",
                "message": "Authentication error",
            }
        )


async def tenant_middleware() -> None:
    """Middleware that enforces presence of tenant claim in JWT.

    Extracts Authorization Bearer token, validates with OIDC JWKS.
    Stores tenant and claims in g context for per-request access.

    Raises:
        Unauthorized: If token is missing, malformed, invalid, or JWT is not configured.
        Forbidden: If tenant claim is missing from valid token.
    """
    auth_header = request.headers.get("Authorization", "")

    if not auth_header.startswith("Bearer "):
        raise Unauthorized(
            response={
                "error": "auth.missing_token",
                "message": "Authorization header with Bearer token required",
            }
        )

    token = auth_header[7:]  # Remove "Bearer " prefix
    claims = parse_token(token)

    if not claims or not claims.get("tenant"):
        raise Forbidden(
            response={
                "error": "auth.missing_tenant",
                "message": "JWT missing mandatory tenant claim",
            }
        )

    # Store in g for request scope
    g.tenant = claims["tenant"]
    g.claims = claims
