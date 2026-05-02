"""Middleware package."""
from .tenant import Claims, tenant_middleware, get_claims, get_tenant, require_scope
from .ratelimit import check_rate_limit
from .audit import AuditEvent, emit_audit

TenantMiddleware = tenant_middleware  # alias for backwards compat

__all__ = [
    "TenantMiddleware",
    "tenant_middleware",
    "get_tenant",
    "get_claims",
    "require_scope",
    "Claims",
    "check_rate_limit",
    "AuditEvent",
    "emit_audit",
]
