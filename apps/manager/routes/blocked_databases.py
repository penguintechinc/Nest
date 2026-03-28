"""Blocked database management routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

blocked_bp = Blueprint("blocked_bp", __name__, url_prefix="/api/v1")


@blocked_bp.route("/blocked-databases", methods=["GET"])
@require_auth
async def list_blocked_databases():
    """List all blocked database entries."""
    def _query():
        db = get_db()
        rows = db(db.blocked_database.id > 0).select(
            orderby=db.blocked_database.id
        )
        return [r.as_dict() for r in rows]

    blocks = await asyncio.to_thread(_query)
    return jsonify({"data": blocks}), 200


@blocked_bp.route("/blocked-databases", methods=["POST"])
@require_role("admin")
async def add_blocked_database():
    """Add a database to the blocked list."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    required = ["db_name"]
    missing = [f for f in required if not body.get(f)]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    def _insert():

        db = get_db()
        block_id = db.blocked_database.insert(
            db_name=body["db_name"],
            server_id=body.get("server_id"),
            reason=body.get("reason", ""),
            blocked_by=g.user_id,
        )
        db.commit()
        return db.blocked_database[block_id].as_dict()

    block = await asyncio.to_thread(_insert)
    return jsonify({"data": block}), 201


@blocked_bp.route("/blocked-databases/<int:block_id>", methods=["GET"])
@require_auth
async def get_blocked_database(block_id: int):
    """Get a blocked database entry."""
    def _query():
        db = get_db()
        row = db.blocked_database[block_id]
        return row.as_dict() if row else None

    block = await asyncio.to_thread(_query)
    if block is None:
        return jsonify({"error": "Block entry not found"}), 404
    return jsonify({"data": block}), 200


@blocked_bp.route("/blocked-databases/<int:block_id>", methods=["DELETE"])
@require_role("admin")
async def remove_blocked_database(block_id: int):
    """Remove a database block entry."""
    def _delete():
        db = get_db()
        row = db.blocked_database[block_id]
        if not row:
            return False
        db(db.blocked_database.id == block_id).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "Block entry not found"}), 404
    return jsonify({"message": "Block entry removed"}), 200
