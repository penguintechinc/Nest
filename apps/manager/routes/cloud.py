"""Cloud provider and instance management routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role
from utils.crypto import encrypt_field as encrypt_value, decrypt_field as decrypt_value

logger = logging.getLogger(__name__)

cloud_bp = Blueprint("cloud_bp", __name__, url_prefix="/api/v1")


# ---------------------------------------------------------------------------
# Cloud Providers
# ---------------------------------------------------------------------------

@cloud_bp.route("/cloud/providers", methods=["GET"])
@require_auth
async def list_providers():
    """List all cloud providers."""
    def _query():
        db = get_db()
        rows = db(db.cloud_provider.id > 0).select(
            orderby=db.cloud_provider.name
        )
        results = []
        for r in rows:
            d = r.as_dict()
            d.pop("credentials_encrypted", None)
            results.append(d)
        return results

    providers = await asyncio.to_thread(_query)
    return jsonify({"data": providers}), 200


@cloud_bp.route("/cloud/providers", methods=["POST"])
@require_role("admin")
async def create_provider():
    """Create a cloud provider entry (credentials are encrypted at rest)."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    required = ["name", "provider_type"]
    missing = [f for f in required if not body.get(f)]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    raw_credentials = body.get("credentials", "")
    encrypted = encrypt_value(raw_credentials) if raw_credentials else ""

    def _insert():

        db = get_db()
        prov_id = db.cloud_provider.insert(
            name=body["name"],
            provider_type=body["provider_type"],
            region=body.get("region", ""),
            credentials_encrypted=encrypted,
            created_by=g.user_id,
        )
        db.commit()
        result = db.cloud_provider[prov_id].as_dict()
        result.pop("credentials_encrypted", None)
        return result

    provider = await asyncio.to_thread(_insert)
    return jsonify({"data": provider}), 201


@cloud_bp.route("/cloud/providers/<int:prov_id>", methods=["GET"])
@require_auth
async def get_provider(prov_id: int):
    """Get a cloud provider (credentials omitted from response)."""
    def _query():
        db = get_db()
        row = db.cloud_provider[prov_id]
        if not row:
            return None
        result = row.as_dict()
        result.pop("credentials_encrypted", None)
        return result

    provider = await asyncio.to_thread(_query)
    if provider is None:
        return jsonify({"error": "Provider not found"}), 404
    return jsonify({"data": provider}), 200


@cloud_bp.route("/cloud/providers/<int:prov_id>", methods=["PUT"])
@require_role("admin")
async def update_provider(prov_id: int):
    """Update a cloud provider."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    def _update():

        db = get_db()
        row = db.cloud_provider[prov_id]
        if not row:
            return None
        updatable = ["name", "provider_type", "region"]
        updates = {k: body[k] for k in updatable if k in body}
        if "credentials" in body:
            updates["credentials_encrypted"] = encrypt_value(body["credentials"])
        if updates:
            db(db.cloud_provider.id == prov_id).update(**updates)
            db.commit()
        result = db.cloud_provider[prov_id].as_dict()
        result.pop("credentials_encrypted", None)
        return result

    provider = await asyncio.to_thread(_update)
    if provider is None:
        return jsonify({"error": "Provider not found"}), 404
    return jsonify({"data": provider}), 200


@cloud_bp.route("/cloud/providers/<int:prov_id>", methods=["DELETE"])
@require_role("admin")
async def delete_provider(prov_id: int):
    """Delete a cloud provider."""
    def _delete():
        db = get_db()
        row = db.cloud_provider[prov_id]
        if not row:
            return False
        db(db.cloud_provider.id == prov_id).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "Provider not found"}), 404
    return jsonify({"message": "Provider deleted"}), 200


# ---------------------------------------------------------------------------
# Cloud Instances
# ---------------------------------------------------------------------------

@cloud_bp.route("/cloud/instances", methods=["GET"])
@require_auth
async def list_instances():
    """List all cloud instances."""
    def _query():
        db = get_db()
        rows = db(db.cloud_instance.id > 0).select(
            orderby=db.cloud_instance.id
        )
        return [r.as_dict() for r in rows]

    instances = await asyncio.to_thread(_query)
    return jsonify({"data": instances}), 200


@cloud_bp.route("/cloud/instances", methods=["POST"])
@require_role("admin")
async def create_instance():
    """Create a cloud instance record."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    required = ["provider_id", "instance_id", "instance_type"]
    missing = [f for f in required if not body.get(f)]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    def _insert():

        db = get_db()
        inst_id = db.cloud_instance.insert(
            provider_id=int(body["provider_id"]),
            instance_id=body["instance_id"],
            instance_type=body["instance_type"],
            region=body.get("region", ""),
            status=body.get("status", "unknown"),
            ip_address=body.get("ip_address", ""),
            created_by=g.user_id,
        )
        db.commit()
        return db.cloud_instance[inst_id].as_dict()

    instance = await asyncio.to_thread(_insert)
    return jsonify({"data": instance}), 201


@cloud_bp.route("/cloud/instances/<int:inst_id>", methods=["PUT"])
@require_auth
async def update_instance_status(inst_id: int):
    """Update cloud instance status."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    def _update():

        db = get_db()
        row = db.cloud_instance[inst_id]
        if not row:
            return None
        updatable = ["status", "ip_address", "instance_type", "region"]
        updates = {k: body[k] for k in updatable if k in body}
        if updates:
            db(db.cloud_instance.id == inst_id).update(**updates)
            db.commit()
        return db.cloud_instance[inst_id].as_dict()

    instance = await asyncio.to_thread(_update)
    if instance is None:
        return jsonify({"error": "Instance not found"}), 404
    return jsonify({"data": instance}), 200
