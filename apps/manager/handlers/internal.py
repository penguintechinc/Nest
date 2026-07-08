"""Handlers for internal (service-to-service authenticated) endpoints."""

from datetime import datetime, timezone
from uuid import uuid4

from quart import jsonify, request, g

from models import OperationRecord
from store.store import OperationStore


async def create_operation(store: OperationStore) -> tuple[dict, int]:
    """Create a new operation from internal request (service-to-service authenticated).

    Requires valid JWT in Authorization header. Tenant is extracted from JWT claims,
    not trusted from request body.

    Expected body:
    {
        "operationId": str (optional, UUID generated if not provided),
        "resourceName": str,
        "resourceType": str,
        "operationType": str
    }
    """
    # Get authenticated service claims from middleware
    service_claims = getattr(g, "service_claims", None)
    if not service_claims:
        return jsonify({"error": "Service authentication required"}), 401

    try:
        body = await request.get_json()
    except Exception as e:
        return jsonify({"error": f"Invalid JSON: {e}"}), 400

    # Tenant comes from authenticated JWT, not from request body
    tenant = service_claims.get("tenant")
    if not tenant:
        return jsonify({"error": "Service token missing tenant claim"}), 401

    required_fields = ["resourceName", "resourceType", "operationType"]
    for field in required_fields:
        if field not in body:
            return jsonify({"error": f"Missing required field: {field}"}), 400

    operation_id = body.get("operationId", str(uuid4()))
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

    operation = OperationRecord(
        id=operation_id,
        tenant=tenant,
        resource_name=body["resourceName"],
        resource_type=body["resourceType"],
        operation_type=body["operationType"],
        phase=body.get("phase", "pending"),
        message="Operation created",
        created_at=now,
        updated_at=now,
    )

    try:
        await store.create_operation(operation)
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
        }), 201
    except ValueError as e:
        return jsonify({"error": str(e)}), 409
    except Exception as e:
        return jsonify({"error": str(e)}), 500


async def cancel_operation(store: OperationStore, op_id: str) -> tuple[dict, int]:
    """Cancel an operation (stub implementation for P1)."""
    try:
        from quart import g
        # Get all operations and find the one with this ID
        # This is a stub - proper implementation would require tenant context
        return jsonify({"status": "accepted"}), 202
    except Exception as e:
        return jsonify({"error": str(e)}), 500
