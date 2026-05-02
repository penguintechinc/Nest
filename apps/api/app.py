"""Quart application factory and route registration."""
import logging
import os
import uuid
from datetime import datetime, timezone

from opentelemetry import trace
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
from quart import Quart, jsonify, request, g, current_app

_api_logger = logging.getLogger("nest.api")
_tracer = trace.get_tracer("nest-api")

from catalog import CATALOG
from middleware import tenant_middleware, get_tenant, get_claims, check_rate_limit, require_scope
from handlers import (
    list_data_resources,
    create_data_resource,
    get_data_resource,
    delete_data_resource,
    list_snapshots,
    create_snapshot,
    delete_snapshot,
    list_protection_policies,
    create_protection_policy,
    delete_protection_policy,
    list_search_pools,
    create_search_pool,
    get_search_pool,
    delete_search_pool,
)
from handlers.import_handler import (
    snapshot_data_resource,
    restore_data_resource,
    introspect_imported_resource,
    migrate_to_managed,
)
from handlers.cost import get_cost_report, get_cost_summary
from handlers.anomaly import list_anomalies
from store import MemoryStore, Store


# Prometheus metrics
requests_total = Counter(
    "nest_api_requests_total",
    "Total Nest API requests",
    ["method", "path", "status"],
)

request_duration = Histogram(
    "nest_api_request_duration_seconds",
    "Nest API request duration",
    ["method", "path"],
)


def create_app(store: Store | None = None) -> Quart:
    """Create and configure the Quart application."""
    app = Quart(__name__)

    if store is None:
        store = MemoryStore()

    # Store instance accessible in handlers
    app.config["STORE"] = store

    @app.before_request
    async def before_request():
        """Per-request setup: generate request ID, start timer, log start."""
        g.request_id = request.headers.get("X-Request-ID", str(uuid.uuid4()))
        g.start_time = datetime.now(timezone.utc)

        _api_logger.debug(
            "request.start",
            extra={
                "method": request.method,
                "path": request.path,
                "request_id": g.request_id,
            },
        )

        # Attach tenant to current span when available
        tenant = getattr(g, "tenant", None)
        if tenant:
            span = trace.get_current_span()
            span.set_attribute("tenant", tenant)

    async def auth_middleware(required_scope: str | None = None) -> None:
        """Combined auth + rate-limit middleware called per authenticated route.

        Args:
            required_scope: Optional scope to validate. If provided, raises Forbidden if token lacks it.
        """
        await tenant_middleware()
        await check_rate_limit()

        if required_scope:
            claims = get_claims()
            request_id = getattr(g, "request_id", str(uuid.uuid4()))

            if not claims:
                from werkzeug.exceptions import Forbidden
                raise Forbidden(
                    response={
                        "code": "nest.auth.scope_denied",
                        "message": "Authentication required",
                        "requestId": request_id,
                        "docsUrl": "https://docs.nest.penguintech.io/errors/nest.auth.scope_denied",
                    }
                )

            # Check if token has the scope or admin scope
            has_scope = False
            for s in claims.scopes:
                if s == required_scope or s == "nest:*:admin":
                    has_scope = True
                    break

            if not has_scope:
                from werkzeug.exceptions import Forbidden
                raise Forbidden(
                    response={
                        "code": "nest.auth.scope_denied",
                        "message": f"Insufficient scope: {required_scope} required",
                        "requestId": request_id,
                        "docsUrl": "https://docs.nest.penguintech.io/errors/nest.auth.scope_denied",
                    }
                )

    @app.after_request
    async def after_request(response):
        """Record metrics, log request end, and attach rate-limit headers."""
        method = request.method
        path = request.path or "/"
        status = response.status_code or 200
        requests_total.labels(method=method, path=path, status=status).inc()

        duration_ms = 0.0
        if hasattr(g, "start_time"):
            duration = (datetime.now(timezone.utc) - g.start_time).total_seconds()
            request_duration.labels(method=method, path=path).observe(duration)
            duration_ms = round(duration * 1000, 2)

        _api_logger.info(
            "request.end",
            extra={
                "status": status,
                "duration_ms": duration_ms,
                "request_id": getattr(g, "request_id", None),
            },
        )

        if hasattr(g, "rl_limit"):
            response.headers["X-RateLimit-Limit"] = str(g.rl_limit)
            response.headers["X-RateLimit-Remaining"] = str(g.rl_remaining)
            response.headers["X-RateLimit-Reset"] = str(g.rl_reset)

        return response

    @app.errorhandler(401)
    async def handle_unauthorized(e):
        """Handle 401 Unauthorized."""
        request_id = getattr(g, "request_id", str(uuid.uuid4()))
        if hasattr(e, "response") and isinstance(e.response, dict):
            return jsonify(e.response), 401
        return (
            jsonify(
                {
                    "code": "nest.auth.unauthorized",
                    "message": "Unauthorized",
                    "requestId": request_id,
                    "docsUrl": "https://docs.nest.penguintech.io/errors/nest.auth.unauthorized",
                }
            ),
            401,
        )

    @app.errorhandler(403)
    async def handle_forbidden(e):
        """Handle 403 Forbidden."""
        request_id = getattr(g, "request_id", str(uuid.uuid4()))
        if hasattr(e, "response") and isinstance(e.response, dict):
            return jsonify(e.response), 403
        return (
            jsonify(
                {
                    "code": "nest.auth.forbidden",
                    "message": "Forbidden",
                    "requestId": request_id,
                    "docsUrl": "https://docs.nest.penguintech.io/errors/nest.auth.forbidden",
                }
            ),
            403,
        )

    @app.errorhandler(404)
    async def handle_not_found(e):
        """Handle 404 Not Found."""
        request_id = getattr(g, "request_id", str(uuid.uuid4()))
        return (
            jsonify(
                {
                    "code": "nest.api.not_found",
                    "message": "Not found",
                    "requestId": request_id,
                    "docsUrl": "https://docs.nest.penguintech.io/errors/nest.api.not_found",
                }
            ),
            404,
        )

    # Health checks (no auth required)
    @app.route("/health", methods=["GET"])
    async def health():
        """Health check endpoint."""
        return jsonify({"status": "ok"})

    @app.route("/ready", methods=["GET"])
    async def ready():
        """Readiness check endpoint."""
        return jsonify({"status": "ok"})

    @app.route("/metrics", methods=["GET"])
    async def metrics():
        """Prometheus metrics endpoint."""
        return generate_latest(), 200, {"Content-Type": CONTENT_TYPE_LATEST}

    # Authenticated API endpoints
    @app.route("/api/v1/catalog", methods=["GET"])
    async def catalog():
        """List available resource types."""
        await auth_middleware(required_scope="nest:catalog:read")
        return jsonify({"types": CATALOG, "meta": {"version": 1}})

    @app.route("/api/v1/versions", methods=["GET"])
    async def versions():
        """API version discovery."""
        await auth_middleware(required_scope="nest:catalog:read")
        return jsonify(
            {
                "versions": [
                    {
                        "version": "v1",
                        "status": "stable",
                        "specUrl": "/api/v1/openapi.json",
                    }
                ]
            }
        )

    # DataResource CRUD
    @app.route("/api/v1/tenants/<tenant_id>/data-resources", methods=["GET"])
    async def list_dr(tenant_id: str):
        """List DataResources for a tenant."""
        await auth_middleware(required_scope="nest:dataresource:read")
        return await list_data_resources(store)

    @app.route("/api/v1/tenants/<tenant_id>/data-resources", methods=["POST"])
    async def create_dr(tenant_id: str):
        """Create a DataResource."""
        await auth_middleware(required_scope="nest:dataresource:write")
        return await create_data_resource(store)

    @app.route("/api/v1/tenants/<tenant_id>/data-resources/<name>", methods=["GET"])
    async def get_dr(tenant_id: str, name: str):
        """Get a DataResource."""
        await auth_middleware(required_scope="nest:dataresource:read")
        return await get_data_resource(store)

    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>", methods=["DELETE"]
    )
    async def delete_dr(tenant_id: str, name: str):
        """Delete a DataResource."""
        await auth_middleware(required_scope="nest:dataresource:delete")
        return await delete_data_resource(store)

    # LRO operations
    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>/snapshot", methods=["POST"]
    )
    async def snapshot_dr(tenant_id: str, name: str):
        """Snapshot a DataResource."""
        await auth_middleware(required_scope="nest:dataresource:write")
        return await snapshot_data_resource(store)

    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>/restore", methods=["POST"]
    )
    async def restore_dr(tenant_id: str, name: str):
        """Restore a DataResource."""
        await auth_middleware(required_scope="nest:dataresource:write")
        return await restore_data_resource(store)

    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>/introspect",
        methods=["POST"],
    )
    async def introspect_dr(tenant_id: str, name: str):
        """Introspect an imported DataResource."""
        await auth_middleware(required_scope="nest:dataresource:read")
        return await introspect_imported_resource(store)

    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>/migrate",
        methods=["POST"],
    )
    async def migrate_dr(tenant_id: str, name: str):
        """Migrate a DataResource to managed."""
        await auth_middleware(required_scope="nest:dataresource:write")
        return await migrate_to_managed(store)

    # Operations (LRO status)
    @app.route("/api/v1/tenants/<tenant_id>/operations/<op_id>", methods=["GET"])
    async def get_operation(tenant_id: str, op_id: str):
        """Get operation status."""
        await auth_middleware(required_scope="nest:operations:read")
        request_id = getattr(g, "request_id", str(uuid.uuid4()))
        store = current_app.config["STORE"]
        try:
            op = await store.get_operation(tenant_id, op_id)
            return jsonify(op.to_dict()), 200
        except ValueError:
            return (
                jsonify(
                    {
                        "code": "nest.operation.not_found",
                        "message": "Operation not found",
                        "requestId": request_id,
                        "docsUrl": "https://docs.nest.penguintech.io/errors/nest.operation.not_found",
                    }
                ),
                404,
            )

    # VolumeSnapshot routes
    @app.route("/api/v1/tenants/<tenant_id>/snapshots", methods=["GET"])
    async def list_snapshots_route(tenant_id: str):
        """List VolumeSnapshots for a tenant."""
        await auth_middleware(required_scope="nest:snapshot:read")
        return await list_snapshots(store)

    @app.route("/api/v1/tenants/<tenant_id>/snapshots", methods=["POST"])
    async def create_snapshot_route(tenant_id: str):
        """Create a VolumeSnapshot."""
        await auth_middleware(required_scope="nest:snapshot:write")
        return await create_snapshot(store)

    @app.route("/api/v1/tenants/<tenant_id>/snapshots/<name>", methods=["DELETE"])
    async def delete_snapshot_route(tenant_id: str, name: str):
        """Delete a VolumeSnapshot."""
        await auth_middleware(required_scope="nest:snapshot:delete")
        return await delete_snapshot(store)

    # DataProtectionPolicy routes
    @app.route("/api/v1/tenants/<tenant_id>/protection-policies", methods=["GET"])
    async def list_policies_route(tenant_id: str):
        """List DataProtectionPolicies for a tenant."""
        await auth_middleware(required_scope="nest:policy:read")
        return await list_protection_policies(store)

    @app.route("/api/v1/tenants/<tenant_id>/protection-policies", methods=["POST"])
    async def create_policy_route(tenant_id: str):
        """Create a DataProtectionPolicy."""
        await auth_middleware(required_scope="nest:policy:write")
        return await create_protection_policy(store)

    @app.route("/api/v1/tenants/<tenant_id>/protection-policies/<name>", methods=["DELETE"])
    async def delete_policy_route(tenant_id: str, name: str):
        """Delete a DataProtectionPolicy."""
        await auth_middleware(required_scope="nest:policy:delete")
        return await delete_protection_policy(store)

    # SearchPool routes
    @app.route("/api/v1/tenants/<tenant_id>/search-pools", methods=["GET"])
    async def list_pools_route(tenant_id: str):
        """List SearchPools."""
        await auth_middleware(required_scope="nest:searchpool:read")
        return await list_search_pools(store)

    @app.route("/api/v1/tenants/<tenant_id>/search-pools", methods=["POST"])
    async def create_pool_route(tenant_id: str):
        """Create a SearchPool."""
        await auth_middleware(required_scope="nest:searchpool:write")
        return await create_search_pool(store)

    @app.route("/api/v1/tenants/<tenant_id>/search-pools/<name>", methods=["GET"])
    async def get_pool_route(tenant_id: str, name: str):
        """Get a SearchPool."""
        await auth_middleware(required_scope="nest:searchpool:read")
        return await get_search_pool(store)

    @app.route("/api/v1/tenants/<tenant_id>/search-pools/<name>", methods=["DELETE"])
    async def delete_pool_route(tenant_id: str, name: str):
        """Delete a SearchPool."""
        await auth_middleware(required_scope="nest:searchpool:delete")
        return await delete_search_pool(store)

    # Cost report routes (proxies to nest-cost-calculator)
    @app.route("/api/v1/tenants/<tenant_id>/cost-report", methods=["GET"])
    async def cost_report(tenant_id: str):
        """Return billing history for a tenant."""
        await auth_middleware(required_scope="nest:cost:read")
        return await get_cost_report(tenant_id)

    @app.route("/api/v1/tenants/<tenant_id>/cost-report/summary", methods=["GET"])
    async def cost_report_summary(tenant_id: str):
        """Return aggregate billing summary for a tenant."""
        await auth_middleware(required_scope="nest:cost:read")
        return await get_cost_summary(tenant_id)

    # Anomaly detection routes (proxies to nest-anomaly-detector; enterprise/WaddleAI gated)
    @app.route("/api/v1/tenants/<tenant_id>/anomalies", methods=["GET"])
    async def anomalies(tenant_id: str):
        """Return current anomalies for a tenant's DataResources."""
        await auth_middleware(required_scope="nest:anomaly:read")
        return await list_anomalies(tenant_id)

    from telemetry import configure_telemetry
    configure_telemetry(app)

    return app
