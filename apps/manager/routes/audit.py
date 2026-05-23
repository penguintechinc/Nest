"""Audit log routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth

logger = logging.getLogger(__name__)

audit_bp = Blueprint("audit_bp", __name__, url_prefix="/api/v1")


@audit_bp.route("/audit-log", methods=["GET"])
@require_auth
async def list_audit_log():
    """Paginated audit log with optional filters."""
    page = int(request.args.get("page", 1))
    per_page = int(request.args.get("per_page", 50))
    user_id = request.args.get("user_id", type=int)
    server_id = request.args.get("server_id", type=int)
    action = request.args.get("action")
    from_ts = request.args.get("from")
    to_ts = request.args.get("to")

    if page < 1:
        page = 1
    if per_page < 1 or per_page > 200:
        per_page = 50

    def _query():

        db = get_db()
        query = db.audit_log.id > 0
        if user_id is not None:
            query &= db.audit_log.user_id == user_id
        if server_id is not None:
            query &= db.audit_log.server_id == server_id
        if action:
            query &= db.audit_log.action == action
        if from_ts:
            query &= db.audit_log.created_at >= from_ts
        if to_ts:
            query &= db.audit_log.created_at <= to_ts

        offset = (page - 1) * per_page
        rows = db(query).select(
            orderby=~db.audit_log.created_at,
            limitby=(offset, offset + per_page),
        )
        total = db(query).count()
        return [r.as_dict() for r in rows], total

    entries, total = await asyncio.to_thread(_query)
    return jsonify({
        "data": entries,
        "meta": {"page": page, "per_page": per_page, "total": total},
    }), 200
