"""Tenant middleware for extracting tenant from JWT Authorization header.

Implements JWT algorithm policy:
  1. ES256 (manager's own key)
  2. RS256 (OIDC_JWKS_URL)
  3. HS256 (symmetric, only if JWT_ALLOW_HS256=true)
"""

import json
import logging
import os
import time
from typing import Optional, Any
from dataclasses import dataclass

import jwt
import requests
from quart import g, request
from werkzeug.exceptions import Unauthorized, Forbidden

log = logging.getLogger(__name__)

# ============================================================================
# Configuration
# ============================================================================

JWT_ALLOW_HS256 = os.environ.get("JWT_ALLOW_HS256", "false").lower() in ("true", "1", "yes")
JWT_SECRET = os.environ.get("JWT_SECRET", "")

# Load manager's EC public key for ES256 validation
_MANAGER_PUBLIC_KEY = None


def _get_manager_public_key() -> Any:
    """Lazily load manager's EC public key (singleton)."""
    global _MANAGER_PUBLIC_KEY
    if _MANAGER_PUBLIC_KEY is None:
        try:
            from utils.ec_keys import get_manager_ec_keys
            _, _MANAGER_PUBLIC_KEY = get_manager_ec_keys()
        except Exception as e:
            log.error("Failed to load manager EC public key: %s", e)
            raise
    return _MANAGER_PUBLIC_KEY


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
# Token Parsing (Policy: ES256 → RS256 → HS256 with admin flag)
# ============================================================================

def parse_token(token: str) -> Optional[dict]:
    """Parse JWT token per algorithm policy.

    Policy order:
      1. ES256 (manager's own key) — try first
      2. RS256 (OIDC_JWKS_URL) — if ES256 fails and OIDC configured
      3. HS256 (symmetric) — only if JWT_ALLOW_HS256=true

    Returns:
        Dict with sub, tenant, tier, scopes if valid, None otherwise.

    Raises:
        Unauthorized: If token is malformed, verification fails, or policy rejects it.
    """
    if not token:
        return None

    try:
        # Get unverified header to inspect algorithm
        header = jwt.get_unverified_header(token)
        alg = header.get("alg", "unknown")
    except Exception as e:
        raise Unauthorized(
            response={
                "error": "auth.invalid_token",
                "message": f"Failed to parse token header: {e}",
            }
        ) from e

    issuer = os.getenv("OIDC_ISSUER", "").strip()
    audience = os.getenv("OIDC_AUDIENCE", "nest-manager").strip()
    jwks_url = os.getenv("OIDC_JWKS_URL", "").strip()

    # ========================================================================
    # 1. Try ES256 (manager's own key)
    # ========================================================================
    if alg == "ES256":
        try:
            manager_public_key = _get_manager_public_key()
            decoded = jwt.decode(
                token,
                manager_public_key,
                algorithms=["ES256"],
                options={"verify_aud": False},  # Manager tokens don't have aud
            )
            log.debug("Token verified with ES256 (manager key)")

            # Extract claims
            sub = decoded.get("sub", "")
            tenant = decoded.get("tenant", "")
            scope_str = decoded.get("scope", "")
            tier = decoded.get("tier", "free")

            if not tenant:
                log.warning("ES256 token missing tenant claim")
                raise Unauthorized(
                    response={
                        "error": "auth.missing_tenant",
                        "message": "JWT missing mandatory tenant claim",
                    }
                )

            scopes = scope_str.split() if scope_str else []
            return {
                "sub": sub,
                "tenant": tenant,
                "tier": tier,
                "scopes": scopes,
            }
        except jwt.ExpiredSignatureError:
            raise Unauthorized(
                response={
                    "error": "auth.token_expired",
                    "message": "JWT token has expired",
                }
            )
        except jwt.JWTError as e:
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

                        # Extract claims
                        sub = decoded.get("sub", "")
                        tenant = decoded.get("tenant", "")
                        scope_str = decoded.get("scope", "")
                        tier = decoded.get("https://nest.penguintech.io/tier", "free")

                        if not tenant:
                            log.warning("RS256 token missing tenant claim")
                            raise Unauthorized(
                                response={
                                    "error": "auth.missing_tenant",
                                    "message": "JWT missing mandatory tenant claim",
                                }
                            )

                        scopes = scope_str.split() if scope_str else []
                        return {
                            "sub": sub,
                            "tenant": tenant,
                            "tier": tier,
                            "scopes": scopes,
                        }
                    except jwt.exceptions.PyJWKClientError as e:
                        log.warning("Failed to resolve RS256 key: %s", e)
        except jwt.ExpiredSignatureError:
            raise Unauthorized(
                response={
                    "error": "auth.token_expired",
                    "message": "JWT token has expired",
                }
            )
        except jwt.JWTError as e:
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

            # Extract claims
            sub = decoded.get("sub", "")
            tenant = decoded.get("tenant", "")
            scope_str = decoded.get("scope", "")
            tier = decoded.get("tier", "free")

            if not tenant:
                log.warning("HS256 token missing tenant claim")
                raise Unauthorized(
                    response={
                        "error": "auth.missing_tenant",
                        "message": "JWT missing mandatory tenant claim",
                    }
                )

            scopes = scope_str.split() if scope_str else []
            return {
                "sub": sub,
                "tenant": tenant,
                "tier": tier,
                "scopes": scopes,
            }
        except jwt.ExpiredSignatureError:
            raise Unauthorized(
                response={
                    "error": "auth.token_expired",
                    "message": "JWT token has expired",
                }
            )
        except jwt.JWTError as e:
            log.debug("HS256 verification failed: %s", e)
            # Fall through to error

    # ========================================================================
    # Policy rejected the token
    # ========================================================================
    if alg == "HS256" and not JWT_ALLOW_HS256:
        raise Unauthorized(
            response={
                "error": "auth.algorithm_rejected",
                "message": (
                    "HS256 (symmetric) algorithm rejected by policy. "
                    "Enable with JWT_ALLOW_HS256=true (admin override only)."
                ),
            }
        )

    raise Unauthorized(
        response={
            "error": "auth.invalid_token",
            "message": f"JWT token verification failed with algorithm {alg}: not accepted by policy",
        }
    )


async def tenant_middleware() -> None:
    """Middleware that enforces presence of tenant claim in JWT.

    Extracts Authorization Bearer token, validates per JWT algorithm policy.
    Stores tenant and claims in g context for per-request access.

    Raises:
        Unauthorized: If token is missing, malformed, invalid, or policy rejects it.
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
