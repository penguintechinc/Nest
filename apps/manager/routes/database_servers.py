"""Database server management routes."""
import asyncio
import logging
import socket
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

servers_bp = Blueprint("servers_bp", __name__, url_prefix="/api/v1")


@servers_bp.route("/servers", methods=["GET"])
async def list_servers():
    """List all active database servers (paginated)."""
    page = int(request.args.get("page", 1))
    per_page = int(request.args.get("per_page", 20))
    if page < 1:
        page = 1
    if per_page < 1 or per_page > 100:
        per_page = 20

    def _query():

        db = get_db()
        offset = (page - 1) * per_page
        rows = db(db.database_server.active == True).select(
            orderby=db.database_server.id,
            limitby=(offset, offset + per_page),
        )
        total = db(db.database_server.active == True).count()
        return [r.as_dict() for r in rows], total

    servers, total = await asyncio.to_thread(_query)
    return jsonify({
        "data": servers,
        "meta": {"page": page, "per_page": per_page, "total": total},
    }), 200


@servers_bp.route("/servers", methods=["POST"])
@require_auth
async def create_server():
    """Create a new database server entry."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    required = ["name", "host", "port", "db_type"]
    missing = [f for f in required if not body.get(f)]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    def _insert():

        db = get_db()
        server_id = db.database_server.insert(
            name=body["name"],
            host=body["host"],
            port=int(body["port"]),
            db_type=body["db_type"],
            username=body.get("username", ""),
            region=body.get("region", ""),
            tags=body.get("tags", ""),
            active=True,
            created_by=g.user_id,
        )
        db.commit()
        return db.database_server[server_id].as_dict()

    server = await asyncio.to_thread(_insert)
    return jsonify({"data": server}), 201


@servers_bp.route("/servers/<int:server_id>", methods=["GET"])
@require_auth
async def get_server(server_id: int):
    """Get a single database server."""
    def _query():
        db = get_db()
        row = db.database_server[server_id]
        return row.as_dict() if row else None

    server = await asyncio.to_thread(_query)
    if server is None:
        return jsonify({"error": "Server not found"}), 404
    return jsonify({"data": server}), 200


@servers_bp.route("/servers/<int:server_id>", methods=["PUT"])
@require_auth
async def update_server(server_id: int):
    """Update a database server entry."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    def _update():

        db = get_db()
        row = db.database_server[server_id]
        if not row:
            return None
        updatable = ["name", "host", "port", "db_type", "username", "region", "tags"]
        updates = {k: body[k] for k in updatable if k in body}
        if updates:
            db(db.database_server.id == server_id).update(**updates)
            db.commit()
        return db.database_server[server_id].as_dict()

    server = await asyncio.to_thread(_update)
    if server is None:
        return jsonify({"error": "Server not found"}), 404
    return jsonify({"data": server}), 200


@servers_bp.route("/servers/<int:server_id>", methods=["DELETE"])
@require_role("admin")
async def delete_server(server_id: int):
    """Soft-delete a database server (sets active=False)."""
    def _soft_delete():
        db = get_db()
        row = db.database_server[server_id]
        if not row:
            return False
        db(db.database_server.id == server_id).update(active=False)
        db.commit()
        return True

    deleted = await asyncio.to_thread(_soft_delete)
    if not deleted:
        return jsonify({"error": "Server not found"}), 404
    return jsonify({"message": "Server deactivated"}), 200


@servers_bp.route("/servers/<int:server_id>/test", methods=["POST"])
@require_auth
async def test_server_connectivity(server_id: int):
    """Test TCP connectivity to a database server."""
    def _get_server():
        db = get_db()
        row = db.database_server[server_id]
        return row.as_dict() if row else None

    server = await asyncio.to_thread(_get_server)
    if server is None:
        return jsonify({"error": "Server not found"}), 404

    def _tcp_test():

        db = get_db()
        try:
            with socket.create_connection((server["host"], server["port"]), timeout=5):
                return True, None
        except (socket.timeout, ConnectionRefusedError, OSError) as exc:
            return False, str(exc)

    reachable, error = await asyncio.to_thread(_tcp_test)
    return jsonify({
        "server_id": server_id,
        "host": server["host"],
        "port": server["port"],
        "reachable": reachable,
        "error": error,
    }), 200
