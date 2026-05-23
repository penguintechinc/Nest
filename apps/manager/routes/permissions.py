"""User permission management routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

permissions_bp = Blueprint("permissions_bp", __name__, url_prefix="/api/v1")


@permissions_bp.route("/permissions", methods=["GET"])
@require_auth
async def list_permissions():
    """List permissions, optionally filtered by user_id or server_id."""
    user_id = request.args.get("user_id", type=int)
    server_id = request.args.get("server_id", type=int)

    def _query():
        db = get_db()
        query = db.user_permission.id > 0
        if user_id is not None:
            query &= db.user_permission.user_id == user_id
        if server_id is not None:
            query &= db.user_permission.server_id == server_id
        rows = db(query).select(orderby=db.user_permission.id)
        return [r.as_dict() for r in rows]

    perms = await asyncio.to_thread(_query)
    return jsonify({"data": perms}), 200


@permissions_bp.route("/permissions", methods=["POST"])
@require_role("admin")
async def create_permission():
    """Create a new user permission."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    required = ["user_id", "server_id", "permission_level"]
    missing = [f for f in required if body.get(f) is None]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    def _insert():
        db = get_db()
        perm_id = db.user_permission.insert(
            user_id=int(body["user_id"]),
            server_id=int(body["server_id"]),
            permission_level=body["permission_level"],
            granted_by=g.user_id,
        )
        db.commit()
        return db.user_permission[perm_id].as_dict()

    perm = await asyncio.to_thread(_insert)
    return jsonify({"data": perm}), 201


@permissions_bp.route("/permissions/<int:perm_id>", methods=["GET"])
@require_auth
async def get_permission(perm_id: int):
    """Get a single permission entry."""
    def _query():
        db = get_db()
        row = db.user_permission[perm_id]
        return row.as_dict() if row else None

    perm = await asyncio.to_thread(_query)
    if perm is None:
        return jsonify({"error": "Permission not found"}), 404
    return jsonify({"data": perm}), 200


@permissions_bp.route("/permissions/<int:perm_id>", methods=["DELETE"])
@require_role("admin")
async def delete_permission(perm_id: int):
    """Delete a permission entry."""
    def _delete():
        db = get_db()
        row = db.user_permission[perm_id]
        if not row:
            return False
        db(db.user_permission.id == perm_id).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "Permission not found"}), 404
    return jsonify({"message": "Permission deleted"}), 200


@permissions_bp.route("/users/<int:user_id>/permissions", methods=["GET"])
@require_auth
async def get_user_permissions(user_id: int):
    """Get all permissions for a specific user."""
    def _query():
        db = get_db()
        rows = db(db.user_permission.user_id == user_id).select(
            orderby=db.user_permission.id
        )
        return [r.as_dict() for r in rows]

    perms = await asyncio.to_thread(_query)
    return jsonify({"data": perms}), 200
