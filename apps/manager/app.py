"""Quart application factory and route configuration."""

import asyncio
from typing import Any
from werkzeug.exceptions import Unauthorized, Forbidden

from prometheus_client import Counter, Histogram, generate_latest
from quart import Quart, g, jsonify, request

import grpc_server
import worker
from handlers import internal, operations
from middleware.tenant import tenant_middleware, parse_token
from store import create_operation_store, OperationStore

# Prometheus metrics
operations_total = Counter(
    "nest_manager_operations_total",
    "Total operations processed",
    ["type", "phase"],
)
operation_duration = Histogram(
    "nest_manager_operation_duration_seconds",
    "Operation duration in seconds",
)


def create_app(store: OperationStore | None = None) -> Quart:
    """Create and configure the Quart application.

    Args:
        store: Optional OperationStore instance. If None, creates one based on
               DB_TYPE environment variable (defaults to MemoryOperationStore).

    Returns:
        Configured Quart application.
    """
    app = Quart(__name__)

    # Store will be initialized in startup handler
    app.config["_store"] = store

    # Background tasks
    worker_task = None
    grpc_task = None

    @app.before_serving
    async def startup() -> None:
        """Start background tasks on app startup."""
        nonlocal worker_task, grpc_task

        # Create store if not provided
        if app.config.get("_store") is None:
            app.config["_store"] = await create_operation_store()

        store = app.config["_store"]
        worker_task = asyncio.create_task(worker.run(store))
        grpc_task = asyncio.create_task(grpc_server.serve(50052))

    @app.after_serving
    async def shutdown() -> None:
        """Clean up background tasks on app shutdown."""
        nonlocal worker_task, grpc_task
        if worker_task:
            worker_task.cancel()
            try:
                await worker_task
            except asyncio.CancelledError:
                pass
        if grpc_task:
            grpc_task.cancel()
            try:
                await grpc_task
            except asyncio.CancelledError:
                pass

    # Health check endpoints
    @app.route("/health", methods=["GET"])
    async def health() -> tuple[dict, int]:
        """Health check endpoint."""
        return jsonify({"status": "ok"}), 200

    @app.route("/ready", methods=["GET"])
    async def ready() -> tuple[dict, int]:
        """Readiness check endpoint."""
        return jsonify({"status": "ok"}), 200

    # Metrics endpoint
    @app.route("/metrics", methods=["GET"])
    async def metrics() -> tuple[str, int, dict]:
        """Prometheus metrics endpoint."""
        return generate_latest(), 200, {"Content-Type": "text/plain; charset=utf-8"}

    async def require_service_auth() -> None:
        """Require service-to-service JWT authentication on /internal/* endpoints.

        Internal endpoints must authenticate with a valid JWT token.
        Fail closed: if OIDC_JWKS_URL not configured, reject all internal calls.
        Do not accept service credentials from request body.
        """
        auth_header = request.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            raise Unauthorized(
                response={
                    "error": "auth.missing_service_token",
                    "message": "Authorization header with Bearer token required for internal endpoints",
                }
            )

        token = auth_header[7:]
        claims = parse_token(token)

        if not claims:
            raise Unauthorized(
                response={
                    "error": "auth.invalid_service_token",
                    "message": "Invalid or expired service authentication token",
                }
            )

        # Store validated claims in g for internal handlers
        g.service_claims = claims

    # Internal endpoints (service-to-service auth required)
    @app.route("/internal/v1/operations", methods=["POST"])
    async def create_op() -> tuple[dict, int]:
        """Create a new operation from internal request (requires service authentication)."""
        await require_service_auth()
        store = app.config["_store"]
        return await internal.create_operation(store)

    @app.route("/internal/v1/operations/<op_id>/cancel", methods=["POST"])
    async def cancel_op(op_id: str) -> tuple[dict, int]:
        """Cancel an operation (requires service authentication)."""
        await require_service_auth()
        store = app.config["_store"]
        return await internal.cancel_operation(store, op_id)

    # Public endpoints (require tenant auth)
    @app.before_request
    async def check_auth() -> None:
        """Validate tenant for public endpoints."""
        # Skip auth for health/ready/metrics (internal/* have their own service auth)
        if request.path in ["/health", "/ready", "/metrics"]:
            return

        # Internal endpoints have their own service-to-service auth (enforce in route handlers)
        if request.path.startswith("/internal/"):
            return

        # Public endpoints require valid tenant JWT
        try:
            await tenant_middleware()
        except (ValueError, Unauthorized, Forbidden) as e:
            return jsonify({"error": str(e)}), 401

    @app.route("/api/v1/tenants/<tid>/operations", methods=["GET"])
    async def list_ops(tid: str) -> tuple[dict, int]:
        """List operations for a tenant."""
        store = app.config["_store"]
        return await operations.list_operations(store, tid)

    @app.route("/api/v1/tenants/<tid>/operations/<op_id>", methods=["GET"])
    async def get_op(tid: str, op_id: str) -> tuple[dict, int]:
        """Get a single operation."""
        store = app.config["_store"]
        return await operations.get_operation(store, tid, op_id)

    # Error handlers
    @app.errorhandler(404)
    async def not_found(e) -> tuple[dict, int]:  # type: ignore
        """Handle 404 errors."""
        return jsonify({"error": "Not found"}), 404

    @app.errorhandler(500)
    async def internal_error(e) -> tuple[dict, int]:  # type: ignore
        """Handle 500 errors."""
        return jsonify({"error": "Internal server error"}), 500

    return app
