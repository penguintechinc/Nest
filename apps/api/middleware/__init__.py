"""Middleware package."""
from .tenant import Claims, tenant_middleware, get_claims, get_tenant, require_scope

TenantMiddleware = tenant_middleware  # alias for backwards compat

__all__ = ["TenantMiddleware", "tenant_middleware", "get_tenant", "get_claims", "require_scope", "Claims"]
