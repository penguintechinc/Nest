"""User profile management routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth
from utils.crypto import generate_api_key, encrypt_field as encrypt_value

logger = logging.getLogger(__name__)

profiles_bp = Blueprint("profiles_bp", __name__, url_prefix="/api/v1")


@profiles_bp.route("/users/<int:user_id>/profile", methods=["GET"])
@require_auth
async def get_profile(user_id: int):
    """Get user profile."""
    def _query():
        db = get_db()
        row = db(db.user_profile.user_id == user_id).select().first()
        return row.as_dict() if row else None

    profile = await asyncio.to_thread(_query)
    if profile is None:
        return jsonify({"error": "Profile not found"}), 404
    # Omit encrypted api_key from response
    profile.pop("api_key_encrypted", None)
    return jsonify({"data": profile}), 200


@profiles_bp.route("/users/<int:user_id>/profile", methods=["PUT"])
@require_auth
async def update_profile(user_id: int):
    """Update user profile (rate_limit, ip_whitelist)."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    # Non-admins may only update their own profile
    if g.user_role != "admin" and g.user_id != user_id:
        return jsonify({"error": "Insufficient permissions"}), 403

    updatable = ["rate_limit", "ip_whitelist", "display_name", "timezone"]
    updates = {k: body[k] for k in updatable if k in body}
    if not updates:
        return jsonify({"error": "No valid fields to update"}), 400

    def _update():

        db = get_db()
        row = db(db.user_profile.user_id == user_id).select().first()
        if not row:
            return None
        db(db.user_profile.user_id == user_id).update(**updates)
        db.commit()
        updated = db(db.user_profile.user_id == user_id).select().first()
        result = updated.as_dict()
        result.pop("api_key_encrypted", None)
        return result

    profile = await asyncio.to_thread(_update)
    if profile is None:
        return jsonify({"error": "Profile not found"}), 404
    return jsonify({"data": profile}), 200


@profiles_bp.route("/users/<int:user_id>/regenerate-api-key", methods=["POST"])
@require_auth
async def regenerate_api_key(user_id: int):
    """Regenerate API key for a user."""
    # Non-admins may only regenerate their own key
    if g.user_role != "admin" and g.user_id != user_id:
        return jsonify({"error": "Insufficient permissions"}), 403

    def _regenerate():

        db = get_db()
        row = db(db.user_profile.user_id == user_id).select().first()
        if not row:
            return None, None
        new_key = generate_api_key()
        encrypted = encrypt_value(new_key)
        db(db.user_profile.user_id == user_id).update(
            api_key_encrypted=encrypted
        )
        db.commit()
        return new_key, True

    new_key, ok = await asyncio.to_thread(_regenerate)
    if not ok:
        return jsonify({"error": "Profile not found"}), 404
    # Return the plaintext key once — it will not be retrievable again
    return jsonify({"api_key": new_key, "message": "Store this key securely; it will not be shown again"}), 200
