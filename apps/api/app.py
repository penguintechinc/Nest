"""Quart application factory and route registration."""
import os
import uuid
from datetime import datetime, timezone

from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
from quart import Quart, jsonify, request, g

from catalog import CATALOG
from middleware import tenant_middleware, get_tenant, get_claims
from handlers import (
    list_data_resources,
    create_data_resource,
    get_data_resource,
    delete_data_resource,
)
from handlers.import_handler import (
    snapshot_data_resource,
    restore_data_resource,
    introspect_imported_resource,
    migrate_to_managed,
)
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
        """Per-request setup: generate request ID, start timer."""
        g.request_id = request.headers.get("X-Request-ID", str(uuid.uuid4()))
        g.start_time = datetime.now(timezone.utc)

    @app.after_request
    async def after_request(response):
        """Record metrics after response."""
        method = request.method
        path = request.path or "/"
        status = response.status_code or 200
        requests_total.labels(method=method, path=path, status=status).inc()

        if hasattr(g, "start_time"):
            duration = (datetime.now(timezone.utc) - g.start_time).total_seconds()
            request_duration.labels(method=method, path=path).observe(duration)

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
        await tenant_middleware()
        return jsonify({"types": CATALOG, "meta": {"version": 1}})

    @app.route("/api/v1/versions", methods=["GET"])
    async def versions():
        """API version discovery."""
        await tenant_middleware()
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
        await tenant_middleware()
        return await list_data_resources(store)

    @app.route("/api/v1/tenants/<tenant_id>/data-resources", methods=["POST"])
    async def create_dr(tenant_id: str):
        """Create a DataResource."""
        await tenant_middleware()
        return await create_data_resource(store)

    @app.route("/api/v1/tenants/<tenant_id>/data-resources/<name>", methods=["GET"])
    async def get_dr(tenant_id: str, name: str):
        """Get a DataResource."""
        await tenant_middleware()
        return await get_data_resource(store)

    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>", methods=["DELETE"]
    )
    async def delete_dr(tenant_id: str, name: str):
        """Delete a DataResource."""
        await tenant_middleware()
        return await delete_data_resource(store)

    # LRO operations
    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>/snapshot", methods=["POST"]
    )
    async def snapshot_dr(tenant_id: str, name: str):
        """Snapshot a DataResource."""
        await tenant_middleware()
        return await snapshot_data_resource(store)

    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>/restore", methods=["POST"]
    )
    async def restore_dr(tenant_id: str, name: str):
        """Restore a DataResource."""
        await tenant_middleware()
        return await restore_data_resource(store)

    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>/introspect",
        methods=["POST"],
    )
    async def introspect_dr(tenant_id: str, name: str):
        """Introspect an imported DataResource."""
        await tenant_middleware()
        return await introspect_imported_resource(store)

    @app.route(
        "/api/v1/tenants/<tenant_id>/data-resources/<name>/migrate",
        methods=["POST"],
    )
    async def migrate_dr(tenant_id: str, name: str):
        """Migrate a DataResource to managed."""
        await tenant_middleware()
        return await migrate_to_managed(store)

    # Operations (LRO status stub)
    @app.route("/api/v1/tenants/<tenant_id>/operations/<op_id>", methods=["GET"])
    async def get_operation(tenant_id: str, op_id: str):
        """Get operation status."""
        await tenant_middleware()
        return jsonify(
            {
                "id": op_id,
                "phase": "Running",
                "tenant": tenant_id,
            }
        )

    return app
