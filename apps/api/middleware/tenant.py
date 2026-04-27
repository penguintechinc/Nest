"""Tenant middleware for authentication."""
import uuid
from dataclasses import dataclass
from functools import wraps
from typing import Optional

from quart import g, request
from werkzeug.exceptions import Unauthorized, Forbidden


@dataclass(slots=True)
class Claims:
    """JWT claims subset Nest cares about."""

    sub: str
    tenant: str
    scopes: list[str]
    tier: str


TENANT_KEY = "nest_tenant"
CLAIMS_KEY = "nest_claims"


def nest_error(code: str, message: str, request_id: str) -> dict:
    """Generate a Nest API error response."""
    return {
        "code": code,
        "message": message,
        "requestId": request_id,
        "docsUrl": f"https://docs.nest.penguintech.io/errors/{code}",
    }


def parse_token(token: str) -> Optional[Claims]:
    """Parse a simple Bearer token format: sub:tenant:tier.

    P1 stub; P2+ will use OIDC JWKS validation.
    """
    if not token:
        return None
    parts = token.split(":", 3)
    claims = Claims(
        sub=parts[0] if len(parts) > 0 else "",
        tenant=parts[1] if len(parts) > 1 else "",
        scopes=["nest:*:admin"],
        tier=parts[2] if len(parts) > 2 else "free",
    )
    return claims


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
