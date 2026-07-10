"""Sync, blocking config, and seed routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role
from utils.redis_sync import sync_to_redis
from clients.db_proxy_grpc import get_db_proxy_client

logger = logging.getLogger(__name__)

sync_bp = Blueprint("sync_bp", __name__, url_prefix="/api/v1")


@sync_bp.route("/sync", methods=["POST"])
@require_auth
async def sync_servers():
    """Sync database_server rows to Redis and trigger DB Proxy reload."""
    def _sync():
        db = get_db()
        return sync_to_redis(db)

    try:
        result = await asyncio.to_thread(_sync)
    except Exception as exc:
        logger.error("Redis sync failed: %s", exc)
        return jsonify({"error": "Sync to Redis failed", "detail": str(exc)}), 500

    try:
        db_proxy = get_db_proxy_client()
        success = await db_proxy.reload()
        if not success:
            logger.warning("DB Proxy reload did not return success status")
    except Exception as exc:
        logger.error("DB Proxy reload failed: %s", exc)
        return jsonify({
            "message": "Redis sync succeeded but DB Proxy reload failed",
            "sync_result": result,
            "db_proxy_error": str(exc),
        }), 207

    return jsonify({"message": "Sync complete", "result": result}), 200


@sync_bp.route("/blocking-config", methods=["GET"])
@require_auth
async def get_blocking_config():
    """Get current blocking config from DB Proxy."""
    try:
        db_proxy = get_db_proxy_client()
        config = await db_proxy.get_blocking_config()
    except Exception as exc:
        logger.error("Failed to get blocking config: %s", exc)
        return jsonify({"error": "Failed to retrieve config", "detail": str(exc)}), 500

    return jsonify({"data": config}), 200


@sync_bp.route("/blocking-config", methods=["PUT"])
@require_role("admin")
async def update_blocking_config():
    """Update blocking config on DB Proxy."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    try:
        db_proxy = get_db_proxy_client()
        success = await db_proxy.set_blocking_config(body)
        if not success:
            return jsonify({"error": "DB Proxy rejected config update"}), 500
    except Exception as exc:
        logger.error("Failed to update blocking config: %s", exc)
        return jsonify({"error": "Failed to update config", "detail": str(exc)}), 500

    return jsonify({"message": "Blocking config updated"}), 200


@sync_bp.route("/seed-blocked-resources", methods=["POST"])
@require_role("admin")
async def seed_blocked_resources():
    """Seed default blocked resources from articdbm defaults."""
    DEFAULT_BLOCKED = [
        "information_schema",
        "performance_schema",
        "mysql",
        "sys",
        "pg_catalog",
        "pg_toast",
    ]

    def _seed():

        db = get_db()
        inserted = 0
        for db_name in DEFAULT_BLOCKED:
            existing = db(db.blocked_database.db_name == db_name).count()
            if existing == 0:
                db.blocked_database.insert(
                    db_name=db_name,
                    reason="Default articdbm blocked resource",
                    blocked_by=g.user_id,
                )
                inserted += 1
        if inserted > 0:
            db.commit()
        return inserted

    count = await asyncio.to_thread(_seed)
    return jsonify({"message": f"Seeded {count} blocked resources"}), 200
