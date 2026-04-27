"""Tenant middleware for extracting tenant from Authorization header."""

from quart import g, request


async def tenant_middleware() -> None:
    """Extract tenant from Authorization header and validate.

    Expected format: Authorization: Bearer sub:tenant:tier

    Raises:
        ValueError: If header is missing or malformed.
    """
    auth_header = request.headers.get("Authorization", "")

    if not auth_header:
        raise ValueError("Missing Authorization header")

    parts = auth_header.split()
    if len(parts) != 2 or parts[0] != "Bearer":
        raise ValueError("Malformed Authorization header")

    token_parts = parts[1].split(":")
    if len(token_parts) != 3:
        raise ValueError("Malformed Bearer token format")

    sub, tenant, tier = token_parts

    if not tenant:
        raise ValueError("Missing tenant in Bearer token")

    g.tenant = tenant
    g.claims = {
        "sub": sub,
        "tenant": tenant,
        "tier": tier,
    }
