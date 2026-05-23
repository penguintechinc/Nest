"""Threat intelligence feed and indicator routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

threat_intel_bp = Blueprint("threat_intel_bp", __name__, url_prefix="/api/v1")


# ---------------------------------------------------------------------------
# Feeds
# ---------------------------------------------------------------------------

@threat_intel_bp.route("/threat-intel/feeds", methods=["GET"])
@require_auth
async def list_feeds():
    """List all threat intelligence feeds."""
    def _query():
        db = get_db()
        rows = db(db.threat_intel_feed.id > 0).select(
            orderby=db.threat_intel_feed.name
        )
        return [r.as_dict() for r in rows]

    feeds = await asyncio.to_thread(_query)
    return jsonify({"data": feeds}), 200


@threat_intel_bp.route("/threat-intel/feeds", methods=["POST"])
@require_role("admin")
async def create_feed():
    """Create a new threat intel feed."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    required = ["name", "url", "feed_type"]
    missing = [f for f in required if not body.get(f)]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    def _insert():

        db = get_db()
        feed_id = db.threat_intel_feed.insert(
            name=body["name"],
            url=body["url"],
            feed_type=body["feed_type"],
            poll_interval_minutes=int(body.get("poll_interval_minutes", 60)),
            enabled=body.get("enabled", True),
            created_by=g.user_id,
        )
        db.commit()
        return db.threat_intel_feed[feed_id].as_dict()

    feed = await asyncio.to_thread(_insert)
    return jsonify({"data": feed}), 201


@threat_intel_bp.route("/threat-intel/feeds/<int:feed_id>", methods=["GET"])
@require_auth
async def get_feed(feed_id: int):
    """Get a single threat intel feed."""
    def _query():
        db = get_db()
        row = db.threat_intel_feed[feed_id]
        return row.as_dict() if row else None

    feed = await asyncio.to_thread(_query)
    if feed is None:
        return jsonify({"error": "Feed not found"}), 404
    return jsonify({"data": feed}), 200


@threat_intel_bp.route("/threat-intel/feeds/<int:feed_id>", methods=["PUT"])
@require_role("admin")
async def update_feed(feed_id: int):
    """Update a threat intel feed."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    def _update():

        db = get_db()
        row = db.threat_intel_feed[feed_id]
        if not row:
            return None
        updatable = ["name", "url", "feed_type", "poll_interval_minutes", "enabled"]
        updates = {k: body[k] for k in updatable if k in body}
        if updates:
            db(db.threat_intel_feed.id == feed_id).update(**updates)
            db.commit()
        return db.threat_intel_feed[feed_id].as_dict()

    feed = await asyncio.to_thread(_update)
    if feed is None:
        return jsonify({"error": "Feed not found"}), 404
    return jsonify({"data": feed}), 200


@threat_intel_bp.route("/threat-intel/feeds/<int:feed_id>", methods=["DELETE"])
@require_role("admin")
async def delete_feed(feed_id: int):
    """Delete a threat intel feed."""
    def _delete():
        db = get_db()
        row = db.threat_intel_feed[feed_id]
        if not row:
            return False
        db(db.threat_intel_feed.id == feed_id).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "Feed not found"}), 404
    return jsonify({"message": "Feed deleted"}), 200


@threat_intel_bp.route("/threat-intel/feeds/<int:feed_id>/poll", methods=["POST"])
@require_role("admin")
async def poll_feed(feed_id: int):
    """Trigger an immediate poll of a threat intel feed."""
    def _check_and_mark():
        db = get_db()
        row = db.threat_intel_feed[feed_id]
        if not row:
            return False
        db(db.threat_intel_feed.id == feed_id).update(poll_requested=True)
        db.commit()
        return True

    ok = await asyncio.to_thread(_check_and_mark)
    if not ok:
        return jsonify({"error": "Feed not found"}), 404
    return jsonify({"message": "Poll triggered", "feed_id": feed_id}), 202


# ---------------------------------------------------------------------------
# Indicators
# ---------------------------------------------------------------------------

@threat_intel_bp.route("/threat-intel/indicators", methods=["GET"])
@require_auth
async def list_indicators():
    """List threat indicators, optionally filtered by type or feed_id."""
    indicator_type = request.args.get("type")
    feed_id = request.args.get("feed_id", type=int)

    def _query():

        db = get_db()
        query = db.threat_indicator.id > 0
        if indicator_type:
            query &= db.threat_indicator.indicator_type == indicator_type
        if feed_id is not None:
            query &= db.threat_indicator.feed_id == feed_id
        rows = db(query).select(
            orderby=~db.threat_indicator.created_at,
            limitby=(0, 500),
        )
        return [r.as_dict() for r in rows]

    indicators = await asyncio.to_thread(_query)
    return jsonify({"data": indicators}), 200


# ---------------------------------------------------------------------------
# Matches
# ---------------------------------------------------------------------------

@threat_intel_bp.route("/threat-intel/matches", methods=["GET"])
@require_auth
async def list_matches():
    """List threat matches, optionally filtered by server_id."""
    server_id = request.args.get("server_id", type=int)

    def _query():

        db = get_db()
        query = db.threat_match.id > 0
        if server_id is not None:
            query &= db.threat_match.server_id == server_id
        rows = db(query).select(
            orderby=~db.threat_match.matched_at,
            limitby=(0, 500),
        )
        return [r.as_dict() for r in rows]

    matches = await asyncio.to_thread(_query)
    return jsonify({"data": matches}), 200
