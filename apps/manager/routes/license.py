"""License management routes."""
import asyncio
import logging
import os
import requests
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

license_bp = Blueprint("license_bp", __name__, url_prefix="/api/v1")

LICENSE_SERVER_URL = os.environ.get("LICENSE_SERVER_URL", "https://license.penguintech.io")
PRODUCT_NAME = os.environ.get("PRODUCT_NAME", "nest")


def _validate_with_server(license_key: str) -> dict:


    db = get_db()
    """Validate a license key against the PenguinTech license server (blocking)."""
    try:
        resp = requests.post(
            f"{LICENSE_SERVER_URL}/api/v2/validate",
            json={"license_key": license_key, "product": PRODUCT_NAME},
            timeout=10,
        )
        resp.raise_for_status()
        return resp.json()
    except requests.RequestException as exc:
        logger.error("License server request failed: %s", exc)
        return {"valid": False, "error": str(exc)}


@license_bp.route("/license", methods=["GET"])
@require_auth
async def get_license():
    """Get current license information."""
    def _query():
        db = get_db()
        row = db(db.license_info.id > 0).select().first()
        if not row:
            return None
        result = row.as_dict()
        # Never expose the raw license key — mask it
        key = result.get("license_key", "")
        if key:
            result["license_key"] = key[:8] + "****" + key[-4:] if len(key) > 12 else "****"
        return result

    license_data = await asyncio.to_thread(_query)
    if license_data is None:
        return jsonify({"data": None, "message": "No license configured"}), 200
    return jsonify({"data": license_data}), 200


@license_bp.route("/license", methods=["POST"])
@require_role("admin")
async def set_license():
    """Validate and store a license key."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    license_key = body.get("license_key", "").strip()
    if not license_key:
        return jsonify({"error": "license_key is required"}), 400

    # Validate against license server
    validation = await asyncio.to_thread(_validate_with_server, license_key)
    if not validation.get("valid"):
        return jsonify({
            "error": "License validation failed",
            "detail": validation.get("error", "Invalid license key"),
        }), 422

    def _upsert():

        db = get_db()
        existing = db(db.license_info.id > 0).select().first()
        if existing:
            db(db.license_info.id == existing.id).update(
                license_key=license_key,
                validation_response=str(validation),
                updated_by=g.user_id,
            )
        else:
            db.license_info.insert(
                license_key=license_key,
                validation_response=str(validation),
                created_by=g.user_id,
            )
        db.commit()

    await asyncio.to_thread(_upsert)
    return jsonify({"message": "License stored successfully", "valid": True}), 200


@license_bp.route("/license", methods=["DELETE"])
@require_role("admin")
async def remove_license():
    """Remove license information."""
    def _delete():
        db = get_db()
        count = db(db.license_info.id > 0).count()
        if count == 0:
            return False
        db(db.license_info.id > 0).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "No license to remove"}), 404
    return jsonify({"message": "License removed"}), 200
