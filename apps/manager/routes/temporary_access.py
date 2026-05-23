"""Temporary access token management routes."""
import asyncio
import logging
import secrets
from datetime import datetime, timezone, timedelta
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

temp_access_bp = Blueprint("temp_access_bp", __name__, url_prefix="/api/v1")

_DEFAULT_TTL_HOURS = 24


@temp_access_bp.route("/temporary-access", methods=["GET"])
@require_role("admin")
async def list_temp_tokens():
    """List all temporary access tokens."""
    def _query():
        db = get_db()
        rows = db(db.temporary_access_token.id > 0).select(
            orderby=~db.temporary_access_token.created_at
        )
        return [r.as_dict() for r in rows]

    tokens = await asyncio.to_thread(_query)
    return jsonify({"data": tokens}), 200


@temp_access_bp.route("/temporary-access", methods=["POST"])
@require_role("admin")
async def create_temp_token():
    """Create a new temporary access token."""
    body = await request.get_json() or {}

    ttl_hours = int(body.get("ttl_hours", _DEFAULT_TTL_HOURS))
    if ttl_hours < 1 or ttl_hours > 720:
        return jsonify({"error": "ttl_hours must be between 1 and 720"}), 400

    required = ["user_id", "server_id"]
    missing = [f for f in required if body.get(f) is None]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    def _insert():

        db = get_db()
        token = secrets.token_urlsafe(32)
        expires_at = datetime.now(timezone.utc) + timedelta(hours=ttl_hours)
        tok_id = db.temporary_access_token.insert(
            user_id=int(body["user_id"]),
            server_id=int(body["server_id"]),
            token=token,
            expires_at=expires_at,
            created_by=g.user_id,
        )
        db.commit()
        result = db.temporary_access_token[tok_id].as_dict()
        return result

    token_record = await asyncio.to_thread(_insert)
    return jsonify({"data": token_record}), 201


@temp_access_bp.route("/temporary-access/<int:token_id>", methods=["GET"])
@require_auth
async def get_temp_token(token_id: int):
    """Get temporary access token details."""
    def _query():
        db = get_db()
        row = db.temporary_access_token[token_id]
        return row.as_dict() if row else None

    token = await asyncio.to_thread(_query)
    if token is None:
        return jsonify({"error": "Token not found"}), 404
    return jsonify({"data": token}), 200


@temp_access_bp.route("/temporary-access/<int:token_id>", methods=["DELETE"])
@require_role("admin")
async def delete_temp_token(token_id: int):
    """Delete a temporary access token."""
    def _delete():
        db = get_db()
        row = db.temporary_access_token[token_id]
        if not row:
            return False
        db(db.temporary_access_token.id == token_id).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "Token not found"}), 404
    return jsonify({"message": "Token deleted"}), 200


@temp_access_bp.route("/temporary-access/<int:token_id>/revoke", methods=["POST"])
@require_role("admin")
async def revoke_temp_token(token_id: int):
    """Revoke a temporary access token by setting used_at to now."""
    def _revoke():
        db = get_db()
        row = db.temporary_access_token[token_id]
        if not row:
            return None
        used_at = datetime.now(timezone.utc)
        db(db.temporary_access_token.id == token_id).update(used_at=used_at)
        db.commit()
        return db.temporary_access_token[token_id].as_dict()

    token = await asyncio.to_thread(_revoke)
    if token is None:
        return jsonify({"error": "Token not found"}), 404
    return jsonify({"data": token, "message": "Token revoked"}), 200
