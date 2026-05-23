"""Security rule management routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

security_rules_bp = Blueprint("security_rules_bp", __name__, url_prefix="/api/v1")


@security_rules_bp.route("/security-rules", methods=["GET"])
@require_auth
async def list_security_rules():
    """List all security rules, sorted by priority."""
    def _query():
        db = get_db()
        rows = db(db.security_rule.id > 0).select(
            orderby=db.security_rule.priority
        )
        return [r.as_dict() for r in rows]

    rules = await asyncio.to_thread(_query)
    return jsonify({"data": rules}), 200


@security_rules_bp.route("/security-rules", methods=["POST"])
@require_role("admin")
async def create_security_rule():
    """Create a new security rule."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    required = ["name", "rule_type", "action", "priority"]
    missing = [f for f in required if body.get(f) is None]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    def _insert():

        db = get_db()
        rule_id = db.security_rule.insert(
            name=body["name"],
            rule_type=body["rule_type"],
            action=body["action"],
            priority=int(body["priority"]),
            pattern=body.get("pattern", ""),
            description=body.get("description", ""),
            enabled=body.get("enabled", True),
            created_by=g.user_id,
        )
        db.commit()
        return db.security_rule[rule_id].as_dict()

    rule = await asyncio.to_thread(_insert)
    return jsonify({"data": rule}), 201


@security_rules_bp.route("/security-rules/<int:rule_id>", methods=["GET"])
@require_auth
async def get_security_rule(rule_id: int):
    """Get a single security rule."""
    def _query():
        db = get_db()
        row = db.security_rule[rule_id]
        return row.as_dict() if row else None

    rule = await asyncio.to_thread(_query)
    if rule is None:
        return jsonify({"error": "Security rule not found"}), 404
    return jsonify({"data": rule}), 200


@security_rules_bp.route("/security-rules/<int:rule_id>", methods=["PUT"])
@require_role("admin")
async def update_security_rule(rule_id: int):
    """Update a security rule."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    def _update():

        db = get_db()
        row = db.security_rule[rule_id]
        if not row:
            return None
        updatable = ["name", "rule_type", "action", "priority", "pattern", "description", "enabled"]
        updates = {k: body[k] for k in updatable if k in body}
        if updates:
            db(db.security_rule.id == rule_id).update(**updates)
            db.commit()
        return db.security_rule[rule_id].as_dict()

    rule = await asyncio.to_thread(_update)
    if rule is None:
        return jsonify({"error": "Security rule not found"}), 404
    return jsonify({"data": rule}), 200


@security_rules_bp.route("/security-rules/<int:rule_id>", methods=["DELETE"])
@require_role("admin")
async def delete_security_rule(rule_id: int):
    """Delete a security rule."""
    def _delete():
        db = get_db()
        row = db.security_rule[rule_id]
        if not row:
            return False
        db(db.security_rule.id == rule_id).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "Security rule not found"}), 404
    return jsonify({"message": "Security rule deleted"}), 200
