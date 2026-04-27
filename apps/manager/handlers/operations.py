"""Handlers for public operation endpoints."""

from quart import g, jsonify, request

from models import OperationRecord
from store.store import OperationStore


async def list_operations(store: OperationStore, tid: str) -> tuple[dict, int]:
    """List all operations for a tenant with optional filters."""
    if g.tenant != tid:
        return jsonify({"error": "Tenant mismatch"}), 403

    phase = request.args.get("phase")
    operation_type = request.args.get("operationType")
    resource_type = request.args.get("resourceType")
    try:
        limit = int(request.args.get("limit", 0)) or None
        offset = int(request.args.get("offset", 0))
    except ValueError:
        return jsonify({"error": "limit and offset must be integers"}), 400

    try:
        operations = await store.list_by_tenant(tid)

        if phase:
            operations = [op for op in operations if op.phase == phase]
        if operation_type:
            operations = [op for op in operations if op.operation_type == operation_type]
        if resource_type:
            operations = [op for op in operations if op.resource_type == resource_type]

        if offset:
            operations = operations[offset:]
        if limit:
            operations = operations[:limit]

        return jsonify({
            "status": "success",
            "operations": [
                {
                    "id": op.id,
                    "tenant": op.tenant,
                    "resourceName": op.resource_name,
                    "resourceType": op.resource_type,
                    "operationType": op.operation_type,
                    "phase": op.phase,
                    "message": op.message,
                    "createdAt": op.created_at,
                    "updatedAt": op.updated_at,
                    "error": op.error,
                    "progress": op.progress,
                }
                for op in operations
            ],
        }), 200
    except Exception as e:
        return jsonify({"error": str(e)}), 500


async def get_operation(
    store: OperationStore,
    tid: str,
    op_id: str,
) -> tuple[dict, int]:
    """Get a single operation."""
    if g.tenant != tid:
        return jsonify({"error": "Tenant mismatch"}), 403

    try:
        operation = await store.get_operation(tid, op_id)
        return jsonify({
            "status": "success",
            "operation": {
                "id": operation.id,
                "tenant": operation.tenant,
                "resourceName": operation.resource_name,
                "resourceType": operation.resource_type,
                "operationType": operation.operation_type,
                "phase": operation.phase,
                "message": operation.message,
                "createdAt": operation.created_at,
                "updatedAt": operation.updated_at,
                "error": operation.error,
                "progress": operation.progress,
            },
        }), 200
    except ValueError:
        return jsonify({"error": "Operation not found"}), 404
    except Exception as e:
        return jsonify({"error": str(e)}), 500
