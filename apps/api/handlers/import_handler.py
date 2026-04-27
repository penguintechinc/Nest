"""Import, introspect, snapshot, and restore handlers."""
import uuid
from datetime import datetime

from quart import jsonify, request, g

from middleware import get_tenant
from handlers.probe import extract_host_port, tcp_ping
from store import Store


async def snapshot_data_resource(store: Store):
    """Snapshot a DataResource (LRO stub).

    POST /api/v1/tenants/:tenantId/data-resources/:name/snapshot
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    op_id = str(uuid.uuid4())
    response = jsonify(
        {
            "operationId": op_id,
            "type": "snapshot",
            "resource": name,
            "tenant": tenant,
            "status": "RUNNING",
            "startedAt": datetime.utcnow().isoformat() + "Z",
        }
    )
    response.headers["Location"] = f"/api/v1/tenants/{tenant}/operations/{op_id}"
    response.status_code = 202
    return response


async def restore_data_resource(store: Store):
    """Restore a DataResource from snapshot (LRO stub).

    POST /api/v1/tenants/:tenantId/data-resources/:name/restore
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        req = await request.get_json() or {}
    except Exception:
        req = {}

    snapshot_id = req.get("snapshotId", "")
    mode = req.get("mode", "side-by-side")
    if not mode:
        mode = "side-by-side"

    op_id = str(uuid.uuid4())
    response = jsonify(
        {
            "operationId": op_id,
            "type": "restore",
            "resource": name,
            "tenant": tenant,
            "mode": mode,
            "status": "RUNNING",
            "startedAt": datetime.utcnow().isoformat() + "Z",
        }
    )
    response.headers["Location"] = f"/api/v1/tenants/{tenant}/operations/{op_id}"
    response.status_code = 202
    return response


async def introspect_imported_resource(store: Store):
    """Introspect an imported resource via TCP probe.

    POST /api/v1/tenants/:tenantId/data-resources/:name/introspect
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        dr = await store.get_data_resource(tenant, name)
    except ValueError:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.not_found",
                    "message": "DataResource not found",
                    "requestId": request_id,
                }
            ),
            404,
        )

    # Extract host:port from connection string or external endpoint
    conn_str = dr.import_conn_str or dr.external_endpoint
    if not conn_str:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.invalid",
                    "message": "No connection string or endpoint configured",
                    "requestId": request_id,
                }
            ),
            400,
        )

    host_port = extract_host_port(conn_str, default_port=5432)
    if not host_port:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.invalid",
                    "message": f"Failed to parse endpoint: {conn_str}",
                    "requestId": request_id,
                }
            ),
            400,
        )

    host, port = host_port
    reachable, latency_ms, message = await tcp_ping(host, port, timeout=5.0)

    return jsonify(
        {
            "resource": name,
            "reachable": reachable,
            "latencyMs": latency_ms,
            "message": message,
        }
    )


async def migrate_to_managed(store: Store):
    """Migrate resource to managed (LRO stub).

    POST /api/v1/tenants/:tenantId/data-resources/:name/migrate
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    op_id = str(uuid.uuid4())
    response = jsonify(
        {
            "operationId": op_id,
            "type": "migrate",
            "resource": name,
            "tenant": tenant,
            "status": "RUNNING",
            "startedAt": datetime.utcnow().isoformat() + "Z",
        }
    )
    response.headers["Location"] = f"/api/v1/tenants/{tenant}/operations/{op_id}"
    response.status_code = 202
    return response
