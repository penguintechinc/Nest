"""DataResource CRUD handlers."""
import uuid
from datetime import datetime, timezone
from typing import Optional

from quart import jsonify, request, g
from werkzeug.exceptions import BadRequest, NotFound, Forbidden

from middleware import get_tenant, get_claims
from models import DataResourceRecord
from store import Store


async def list_data_resources(store: Store):
    """List all DataResources for a tenant.

    GET /api/v1/tenants/:tenantId/data-resources
    """
    tenant = get_tenant()
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    resource_type = request.args.get("type")
    origination = request.args.get("origination")
    phase = request.args.get("phase")
    try:
        limit = int(request.args.get("limit", 0)) or None
        offset = int(request.args.get("offset", 0))
    except ValueError:
        return jsonify({"code": "nest.dataresource.invalid", "message": "limit and offset must be integers"}), 400

    try:
        resources = await store.list_data_resources(tenant)

        if resource_type:
            resources = [r for r in resources if r.resource_type == resource_type]
        if origination:
            resources = [r for r in resources if r.origination == origination]
        if phase:
            resources = [r for r in resources if r.phase == phase]

        if offset:
            resources = resources[offset:]
        if limit:
            resources = resources[:limit]

        return jsonify(
            {
                "items": [dr.to_dict() for dr in resources],
                "meta": {"count": len(resources), "version": 1},
            }
        )
    except Exception as e:
        return (
            jsonify(
                {
                    "code": "nest.internal",
                    "message": str(e),
                    "requestId": request_id,
                }
            ),
            500,
        )


async def create_data_resource(store: Store):
    """Create a new DataResource.

    POST /api/v1/tenants/:tenantId/data-resources
    """
    tenant = get_tenant()
    claims = get_claims()
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        req = await request.get_json() or {}
    except Exception as e:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.invalid",
                    "message": str(e),
                    "requestId": request_id,
                }
            ),
            400,
        )

    # Validate required fields
    name = req.get("name", "").strip()
    resource_type = req.get("type", "").strip()
    storage_class = req.get("class", "").strip()
    origination = req.get("origination", "managed").strip()

    if not name or not resource_type:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.invalid",
                    "message": "name and type are required",
                    "requestId": request_id,
                }
            ),
            400,
        )

    valid_types = {
        "pvc/block",
        "pvc/file",
        "object",
        "nfs",
        "iscsi",
        "postgres",
        "keyvalue",
    }
    if resource_type not in valid_types:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.invalid_type",
                    "message": f"unknown resource type: {resource_type}",
                    "requestId": request_id,
                }
            ),
            400,
        )

    # Check free-tier limit (5 DataResources)
    if claims and claims.tier == "free":
        count = await store.count_data_resources(tenant)
        if count >= 5:
            return (
                jsonify(
                    {
                        "code": "nest.quota.data_resource_limit",
                        "message": "Free tier DataResource limit reached (5/5). Upgrade to Pro to provision more.",
                        "requestId": request_id,
                    }
                ),
                402,
            )

    # Validate origination-specific fields
    import_conn_str = ""
    import_db_name = ""
    external_provider = ""
    external_resource_id = ""
    external_endpoint = ""
    external_region = ""

    if origination == "imported":
        import_data = req.get("import") or {}
        import_conn_str = import_data.get("connectionString", "").strip()
        import_db_name = import_data.get("dbName", "").strip()
        if not import_conn_str:
            return (
                jsonify(
                    {
                        "code": "nest.dataresource.invalid",
                        "message": "import.connectionString is required when origination=imported",
                        "requestId": request_id,
                    }
                ),
                400,
            )
    elif origination == "external":
        external_data = req.get("external") or {}
        external_provider = external_data.get("provider", "").strip()
        external_resource_id = external_data.get("resourceId", "").strip()
        external_endpoint = external_data.get("endpoint", "").strip()
        external_region = external_data.get("region", "").strip()
        if not external_provider:
            return (
                jsonify(
                    {
                        "code": "nest.dataresource.invalid",
                        "message": "external.provider is required when origination=external",
                        "requestId": request_id,
                    }
                ),
                400,
            )
        valid_providers = {"aws", "gcp", "azure", "cloudflare", "vultr"}
        if external_provider not in valid_providers:
            return (
                jsonify(
                    {
                        "code": "nest.dataresource.invalid",
                        "message": f"unknown external provider: {external_provider}",
                        "requestId": request_id,
                    }
                ),
                400,
            )

    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    dr = DataResourceRecord(
        id=str(uuid.uuid4()),
        name=name,
        tenant=tenant,
        resource_type=resource_type,
        engine_type=_engine_type_for(resource_type),
        storage_class=storage_class,
        driver_type=_driver_type_for(resource_type),
        origination=origination,
        phase="pending",
        created_at=now,
        updated_at=now,
        namespace=req.get("namespace", "default"),
        size_gi=req.get("sizeGi", 0),
        import_conn_str=import_conn_str,
        import_db_name=import_db_name,
        external_provider=external_provider,
        external_resource_id=external_resource_id,
        external_endpoint=external_endpoint,
        external_region=external_region,
    )

    try:
        await store.create_data_resource(dr)
    except ValueError as e:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.already_exists",
                    "message": str(e),
                    "requestId": request_id,
                }
            ),
            409,
        )
    except Exception as e:
        return (
            jsonify(
                {
                    "code": "nest.internal",
                    "message": str(e),
                    "requestId": request_id,
                }
            ),
            500,
        )

    op_id = str(uuid.uuid4())
    response = jsonify(
        {
            "name": dr.name,
            "id": dr.id,
            "tenant": tenant,
            "resourceType": resource_type,
            "storageClass": storage_class,
            "phase": dr.phase,
            "operationId": op_id,
        }
    )
    response.headers["Location"] = f"/api/v1/tenants/{tenant}/operations/{op_id}"
    response.headers["X-Operation-ID"] = op_id
    response.status_code = 202
    return response


async def get_data_resource(store: Store):
    """Get a single DataResource.

    GET /api/v1/tenants/:tenantId/data-resources/:name
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        dr = await store.get_data_resource(tenant, name)
        return jsonify(dr.to_dict())
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
    except Exception as e:
        return (
            jsonify(
                {
                    "code": "nest.internal",
                    "message": str(e),
                    "requestId": request_id,
                }
            ),
            500,
        )


async def delete_data_resource(store: Store):
    """Delete a DataResource.

    DELETE /api/v1/tenants/:tenantId/data-resources/:name
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        await store.delete_data_resource(tenant, name)
        return "", 204
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
    except Exception as e:
        return (
            jsonify(
                {
                    "code": "nest.internal",
                    "message": str(e),
                    "requestId": request_id,
                }
            ),
            500,
        )


def _engine_type_for(resource_type: str) -> str:
    """Map resource type to engine type."""
    mapping = {
        "pvc/block": "block",
        "pvc/file": "filesystem",
        "object": "object",
        "nfs": "nfs",
        "iscsi": "iscsi",
        "postgres": "postgres",
        "keyvalue": "redis",
    }
    return mapping.get(resource_type, "")


def _driver_type_for(resource_type: str) -> str:
    """Map resource type to driver type."""
    mapping = {
        "pvc/block": "csi",
        "pvc/file": "csi",
        "object": "s3",
        "nfs": "nfs",
        "iscsi": "iscsi",
        "postgres": "cnpg",
        "keyvalue": "valkey",
    }
    return mapping.get(resource_type, "")
