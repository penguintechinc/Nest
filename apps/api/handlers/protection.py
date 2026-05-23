"""Protection and snapshot handlers for VolumeSnapshot and DataProtectionPolicy CRs."""
import asyncio
import uuid
from datetime import datetime, timezone

from quart import jsonify, request, g
from werkzeug.exceptions import BadRequest, NotFound

from middleware import get_tenant, get_claims, emit_audit, AuditEvent
from store import Store


async def list_snapshots(store: Store):
    """List all VolumeSnapshots for a tenant.

    GET /api/v1/tenants/:tenantId/snapshots
    """
    tenant = get_tenant()
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        # For P1, return mock snapshots from store
        # In production, would query K8s API
        snapshots = await store.list_snapshots(tenant)

        return jsonify({
            "snapshots": [s.to_dict() for s in snapshots],
            "meta": {"count": len(snapshots), "version": 1}
        }), 200
    except Exception as e:
        return jsonify({
            "code": "nest.internal",
            "message": str(e),
            "requestId": request_id,
        }), 500


async def create_snapshot(store: Store):
    """Create a new VolumeSnapshot CR.

    POST /api/v1/tenants/:tenantId/snapshots
    """
    tenant = get_tenant()
    claims = get_claims()
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        req = await request.get_json() or {}
    except Exception as e:
        return jsonify({
            "code": "nest.api.invalid_request",
            "message": str(e),
            "requestId": request_id,
        }), 400

    # Validate required fields
    name = req.get("name", "").strip()
    source_pvc = req.get("sourcePVC", "").strip()
    snapshot_class = req.get("snapshotClass", "nest-rbd-snapshot").strip()

    if not name or not source_pvc:
        return jsonify({
            "code": "nest.api.validation_error",
            "message": "name and sourcePVC are required",
            "requestId": request_id,
        }), 400

    try:
        snapshot = await store.create_snapshot(
            tenant=tenant,
            name=name,
            source_pvc=source_pvc,
            snapshot_class=snapshot_class,
        )

        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="snapshot.created",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="VolumeSnapshot",
                resource_name=name,
                action="create",
                outcome="success",
                request_id=request_id,
            ))
        )

        op_id = str(uuid.uuid4())
        response = jsonify(snapshot.to_dict())
        response.headers["X-Operation-ID"] = op_id
        response.status_code = 202
        return response
    except ValueError as e:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="snapshot.created",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="VolumeSnapshot",
                resource_name=name,
                action="create",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.snapshot.already_exists",
            "message": str(e),
            "requestId": request_id,
        }), 409
    except Exception as e:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="snapshot.created",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="VolumeSnapshot",
                resource_name=name,
                action="create",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.internal",
            "message": str(e),
            "requestId": request_id,
        }), 500


async def delete_snapshot(store: Store):
    """Delete a VolumeSnapshot CR.

    DELETE /api/v1/tenants/:tenantId/snapshots/:name
    """
    tenant = get_tenant()
    claims = get_claims()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        await store.delete_snapshot(tenant, name)
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="snapshot.deleted",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="VolumeSnapshot",
                resource_name=name,
                action="delete",
                outcome="success",
                request_id=request_id,
            ))
        )
        return "", 204
    except ValueError:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="snapshot.deleted",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="VolumeSnapshot",
                resource_name=name,
                action="delete",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.snapshot.not_found",
            "message": "VolumeSnapshot not found",
            "requestId": request_id,
        }), 404
    except Exception as e:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="snapshot.deleted",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="VolumeSnapshot",
                resource_name=name,
                action="delete",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.internal",
            "message": str(e),
            "requestId": request_id,
        }), 500


async def list_protection_policies(store: Store):
    """List all DataProtectionPolicy CRs for a tenant.

    GET /api/v1/tenants/:tenantId/protection-policies
    """
    tenant = get_tenant()
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        policies = await store.list_protection_policies(tenant)

        return jsonify({
            "policies": [p.to_dict() for p in policies],
            "meta": {"count": len(policies), "version": 1}
        }), 200
    except Exception as e:
        return jsonify({
            "code": "nest.internal",
            "message": str(e),
            "requestId": request_id,
        }), 500


async def create_protection_policy(store: Store):
    """Create a new DataProtectionPolicy CR.

    POST /api/v1/tenants/:tenantId/protection-policies
    """
    tenant = get_tenant()
    claims = get_claims()
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        req = await request.get_json() or {}
    except Exception as e:
        return jsonify({
            "code": "nest.api.invalid_request",
            "message": str(e),
            "requestId": request_id,
        }), 400

    # Validate required fields
    name = req.get("name", "").strip()
    snapshot_schedule = req.get("snapshotSchedule", "").strip()
    backup_schedule = req.get("backupSchedule", "").strip()
    destination = req.get("destination", "").strip()

    if not name:
        return jsonify({
            "code": "nest.api.validation_error",
            "message": "name is required",
            "requestId": request_id,
        }), 400

    try:
        policy = await store.create_protection_policy(
            tenant=tenant,
            name=name,
            snapshot_schedule=snapshot_schedule,
            backup_schedule=backup_schedule,
            destination=destination,
        )

        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="policy.created",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="DataProtectionPolicy",
                resource_name=name,
                action="create",
                outcome="success",
                request_id=request_id,
            ))
        )

        op_id = str(uuid.uuid4())
        response = jsonify(policy.to_dict())
        response.headers["X-Operation-ID"] = op_id
        response.status_code = 202
        return response
    except ValueError as e:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="policy.created",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="DataProtectionPolicy",
                resource_name=name,
                action="create",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.policy.already_exists",
            "message": str(e),
            "requestId": request_id,
        }), 409
    except Exception as e:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="policy.created",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="DataProtectionPolicy",
                resource_name=name,
                action="create",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.internal",
            "message": str(e),
            "requestId": request_id,
        }), 500


async def delete_protection_policy(store: Store):
    """Delete a DataProtectionPolicy CR.

    DELETE /api/v1/tenants/:tenantId/protection-policies/:name
    """
    tenant = get_tenant()
    claims = get_claims()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        await store.delete_protection_policy(tenant, name)
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="policy.deleted",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="DataProtectionPolicy",
                resource_name=name,
                action="delete",
                outcome="success",
                request_id=request_id,
            ))
        )
        return "", 204
    except ValueError:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="policy.deleted",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="DataProtectionPolicy",
                resource_name=name,
                action="delete",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.policy.not_found",
            "message": "DataProtectionPolicy not found",
            "requestId": request_id,
        }), 404
    except Exception as e:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="policy.deleted",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="DataProtectionPolicy",
                resource_name=name,
                action="delete",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.internal",
            "message": str(e),
            "requestId": request_id,
        }), 500
