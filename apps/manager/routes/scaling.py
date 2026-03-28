"""Scaling policy and event management routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

scaling_bp = Blueprint("scaling_bp", __name__, url_prefix="/api/v1")


# ---------------------------------------------------------------------------
# Scaling Policies
# ---------------------------------------------------------------------------

@scaling_bp.route("/scaling/policies", methods=["GET"])
@require_auth
async def list_policies():
    """List all scaling policies."""
    def _query():
        db = get_db()
        rows = db(db.scaling_policy.id > 0).select(
            orderby=db.scaling_policy.name
        )
        return [r.as_dict() for r in rows]

    policies = await asyncio.to_thread(_query)
    return jsonify({"data": policies}), 200


@scaling_bp.route("/scaling/policies", methods=["POST"])
@require_role("admin")
async def create_policy():
    """Create a new scaling policy."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    required = ["name", "server_id", "metric", "threshold"]
    missing = [f for f in required if body.get(f) is None]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    def _insert():

        db = get_db()
        pol_id = db.scaling_policy.insert(
            name=body["name"],
            server_id=int(body["server_id"]),
            metric=body["metric"],
            threshold=float(body["threshold"]),
            scale_up_by=int(body.get("scale_up_by", 1)),
            scale_down_by=int(body.get("scale_down_by", 1)),
            cooldown_seconds=int(body.get("cooldown_seconds", 300)),
            enabled=body.get("enabled", True),
            created_by=g.user_id,
        )
        db.commit()
        return db.scaling_policy[pol_id].as_dict()

    policy = await asyncio.to_thread(_insert)
    return jsonify({"data": policy}), 201


@scaling_bp.route("/scaling/policies/<int:pol_id>", methods=["GET"])
@require_auth
async def get_policy(pol_id: int):
    """Get a single scaling policy."""
    def _query():
        db = get_db()
        row = db.scaling_policy[pol_id]
        return row.as_dict() if row else None

    policy = await asyncio.to_thread(_query)
    if policy is None:
        return jsonify({"error": "Policy not found"}), 404
    return jsonify({"data": policy}), 200


@scaling_bp.route("/scaling/policies/<int:pol_id>", methods=["PUT"])
@require_role("admin")
async def update_policy(pol_id: int):
    """Update a scaling policy."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    def _update():

        db = get_db()
        row = db.scaling_policy[pol_id]
        if not row:
            return None
        updatable = [
            "name", "metric", "threshold", "scale_up_by",
            "scale_down_by", "cooldown_seconds", "enabled",
        ]
        updates = {k: body[k] for k in updatable if k in body}
        if updates:
            db(db.scaling_policy.id == pol_id).update(**updates)
            db.commit()
        return db.scaling_policy[pol_id].as_dict()

    policy = await asyncio.to_thread(_update)
    if policy is None:
        return jsonify({"error": "Policy not found"}), 404
    return jsonify({"data": policy}), 200


@scaling_bp.route("/scaling/policies/<int:pol_id>", methods=["DELETE"])
@require_role("admin")
async def delete_policy(pol_id: int):
    """Delete a scaling policy."""
    def _delete():
        db = get_db()
        row = db.scaling_policy[pol_id]
        if not row:
            return False
        db(db.scaling_policy.id == pol_id).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "Policy not found"}), 404
    return jsonify({"message": "Policy deleted"}), 200


# ---------------------------------------------------------------------------
# Scaling Events
# ---------------------------------------------------------------------------

@scaling_bp.route("/scaling/events", methods=["GET"])
@require_auth
async def list_events():
    """List scaling events, optionally filtered by server_id or status."""
    server_id = request.args.get("server_id", type=int)
    status = request.args.get("status")

    def _query():

        db = get_db()
        query = db.scaling_event.id > 0
        if server_id is not None:
            query &= db.scaling_event.server_id == server_id
        if status:
            query &= db.scaling_event.status == status
        rows = db(query).select(
            orderby=~db.scaling_event.triggered_at,
            limitby=(0, 200),
        )
        return [r.as_dict() for r in rows]

    events = await asyncio.to_thread(_query)
    return jsonify({"data": events}), 200
