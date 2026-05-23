"""Status and aggregate stats routes."""
import asyncio
import logging
import os
from quart import Blueprint, jsonify

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth

logger = logging.getLogger(__name__)

stats_bp = Blueprint("stats_bp", __name__, url_prefix="/api/v1")

# Read VERSION from .version file at module load time if it exists.
_VERSION = "1.0.0"
_version_file = os.path.join(
    os.path.dirname(__file__), "..", "..", "..", ".version"
)
try:
    with open(_version_file) as _f:
        _VERSION = _f.read().strip().lstrip("v")
except OSError:
    pass


@stats_bp.route("/status", methods=["GET"])
async def status():
    """Service status endpoint (used by AppConsoleVersion). No auth required."""
    return jsonify({
        "version": _VERSION,
        "build_epoch": 0,
        "service": "nest-manager",
        "status": "ok",
    }), 200


@stats_bp.route("/stats", methods=["GET"])
@require_auth
async def aggregate_stats():
    """Return aggregate statistics for the manager service."""
    def _query():
        db = get_db()
        server_total = db(db.database_server.id > 0).count()
        server_active = db(db.database_server.active == True).count()
        db_total = db(db.managed_database.id > 0).count()
        perm_total = db(db.user_permission.id > 0).count()
        indicator_total = db(db.threat_indicator.id > 0).count()
        feed_total = db(db.threat_intel_feed.id > 0).count()
        rule_total = db(db.security_rule.id > 0).count()
        blocked_total = db(db.blocked_database.id > 0).count()
        return {
            "servers": {"total": server_total, "active": server_active},
            "managed_databases": db_total,
            "permissions": perm_total,
            "threat_indicators": indicator_total,
            "threat_feeds": feed_total,
            "security_rules": rule_total,
            "blocked_databases": blocked_total,
        }

    stats = await asyncio.to_thread(_query)
    return jsonify({"data": stats}), 200
