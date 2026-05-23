"""SearchPool handlers for SearchPool CRs in nest-search namespace."""
import asyncio
import uuid
from datetime import datetime, timezone

from quart import jsonify, request, g
from werkzeug.exceptions import BadRequest, NotFound

from middleware import get_tenant, get_claims, emit_audit, AuditEvent
from store import Store


async def list_search_pools(store: Store):
    """List all SearchPool CRs (admin operation, not tenant-specific).

    GET /api/v1/tenants/:tenantId/search-pools
    """
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        # SearchPool CRs live in nest-search namespace, not tenant namespace
        # For P1, return mock search pools from store
        search_pools = await store.list_search_pools()

        return jsonify({
            "searchPools": [sp.to_dict() for sp in search_pools],
            "meta": {"count": len(search_pools), "version": 1}
        }), 200
    except Exception as e:
        return jsonify({
            "code": "nest.internal",
            "message": str(e),
            "requestId": request_id,
        }), 500


async def create_search_pool(store: Store):
    """Create a new SearchPool CR in nest-search namespace.

    POST /api/v1/tenants/:tenantId/search-pools
    """
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
    replicas = req.get("replicas", 1)

    if not name:
        return jsonify({
            "code": "nest.api.validation_error",
            "message": "name is required",
            "requestId": request_id,
        }), 400

    try:
        search_pool = await store.create_search_pool(
            name=name,
            replicas=replicas,
        )

        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="searchpool.created",
                tenant="admin",
                subject=claims.sub if claims else "",
                resource="SearchPool",
                resource_name=name,
                action="create",
                outcome="success",
                request_id=request_id,
            ))
        )

        op_id = str(uuid.uuid4())
        response = jsonify(search_pool.to_dict())
        response.headers["X-Operation-ID"] = op_id
        response.status_code = 202
        return response
    except ValueError as e:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="searchpool.created",
                tenant="admin",
                subject=claims.sub if claims else "",
                resource="SearchPool",
                resource_name=name,
                action="create",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.searchpool.already_exists",
            "message": str(e),
            "requestId": request_id,
        }), 409
    except Exception as e:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="searchpool.created",
                tenant="admin",
                subject=claims.sub if claims else "",
                resource="SearchPool",
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


async def get_search_pool(store: Store):
    """Get a single SearchPool CR.

    GET /api/v1/tenants/:tenantId/search-pools/:name
    """
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        search_pool = await store.get_search_pool(name)
        return jsonify(search_pool.to_dict()), 200
    except ValueError:
        return jsonify({
            "code": "nest.searchpool.not_found",
            "message": "SearchPool not found",
            "requestId": request_id,
        }), 404
    except Exception as e:
        return jsonify({
            "code": "nest.internal",
            "message": str(e),
            "requestId": request_id,
        }), 500


async def delete_search_pool(store: Store):
    """Delete a SearchPool CR.

    DELETE /api/v1/tenants/:tenantId/search-pools/:name
    """
    claims = get_claims()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        await store.delete_search_pool(name)
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="searchpool.deleted",
                tenant="admin",
                subject=claims.sub if claims else "",
                resource="SearchPool",
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
                event_type="searchpool.deleted",
                tenant="admin",
                subject=claims.sub if claims else "",
                resource="SearchPool",
                resource_name=name,
                action="delete",
                outcome="failure",
                request_id=request_id,
            ))
        )
        return jsonify({
            "code": "nest.searchpool.not_found",
            "message": "SearchPool not found",
            "requestId": request_id,
        }), 404
    except Exception as e:
        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="searchpool.deleted",
                tenant="admin",
                subject=claims.sub if claims else "",
                resource="SearchPool",
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
