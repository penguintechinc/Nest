"""Managed database entry routes."""
import asyncio
import logging
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

databases_bp = Blueprint("databases_bp", __name__, url_prefix="/api/v1")


@databases_bp.route("/databases", methods=["GET"])
@require_auth
async def list_databases():
    """List all managed database entries."""
    def _query():
        db = get_db()
        rows = db(db.managed_database.id > 0).select(
            orderby=db.managed_database.id
        )
        return [r.as_dict() for r in rows]

    dbs = await asyncio.to_thread(_query)
    return jsonify({"data": dbs}), 200


@databases_bp.route("/databases", methods=["POST"])
@require_auth
async def create_database():
    """Create a managed database entry."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    required = ["server_id", "db_name"]
    missing = [f for f in required if not body.get(f)]
    if missing:
        return jsonify({"error": f"Missing required fields: {missing}"}), 400

    def _insert():

        db = get_db()
        db_id = db.managed_database.insert(
            server_id=int(body["server_id"]),
            db_name=body["db_name"],
            description=body.get("description", ""),
            db_type=body.get("db_type", ""),
            size_bytes=body.get("size_bytes", 0),
            created_by=g.user_id,
        )
        db.commit()
        return db.managed_database[db_id].as_dict()

    entry = await asyncio.to_thread(_insert)
    return jsonify({"data": entry}), 201


@databases_bp.route("/databases/<int:db_id>", methods=["GET"])
@require_auth
async def get_database(db_id: int):
    """Get a managed database entry."""
    def _query():
        db = get_db()
        row = db.managed_database[db_id]
        return row.as_dict() if row else None

    entry = await asyncio.to_thread(_query)
    if entry is None:
        return jsonify({"error": "Database not found"}), 404
    return jsonify({"data": entry}), 200


@databases_bp.route("/databases/<int:db_id>", methods=["PUT"])
@require_auth
async def update_database(db_id: int):
    """Update a managed database entry."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    def _update():

        db = get_db()
        row = db.managed_database[db_id]
        if not row:
            return None
        updatable = ["db_name", "description", "db_type", "size_bytes"]
        updates = {k: body[k] for k in updatable if k in body}
        if updates:
            db(db.managed_database.id == db_id).update(**updates)
            db.commit()
        return db.managed_database[db_id].as_dict()

    entry = await asyncio.to_thread(_update)
    if entry is None:
        return jsonify({"error": "Database not found"}), 404
    return jsonify({"data": entry}), 200


@databases_bp.route("/databases/<int:db_id>", methods=["DELETE"])
@require_role("admin")
async def delete_database(db_id: int):
    """Delete a managed database entry."""
    def _delete():
        db = get_db()
        row = db.managed_database[db_id]
        if not row:
            return False
        db(db.managed_database.id == db_id).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "Database not found"}), 404
    return jsonify({"message": "Database entry deleted"}), 200


@databases_bp.route("/databases/<int:db_id>/schema", methods=["GET"])
@require_auth
async def get_database_schema(db_id: int):
    """Get schema information for a managed database."""
    def _query():
        db = get_db()
        db_row = db.managed_database[db_id]
        if not db_row:
            return None, None
        rows = db(db.database_schema.managed_db_id == db_id).select(
            orderby=db.database_schema.table_name
        )
        return db_row.as_dict(), [r.as_dict() for r in rows]

    db_entry, schema = await asyncio.to_thread(_query)
    if db_entry is None:
        return jsonify({"error": "Database not found"}), 404
    return jsonify({"data": {"database": db_entry, "schema": schema}}), 200


@databases_bp.route("/databases/<int:db_id>/schema", methods=["POST"])
@require_auth
async def refresh_database_schema(db_id: int):
    """Trigger a schema sync job for a managed database."""
    def _check():
        db = get_db()
        row = db.managed_database[db_id]
        return row is not None

    exists = await asyncio.to_thread(_check)
    if not exists:
        return jsonify({"error": "Database not found"}), 404

    # Schema refresh is handled asynchronously by the background worker.
    # Mark the database as pending schema sync.
    def _mark_pending():
        db = get_db()
        db(db.managed_database.id == db_id).update(schema_sync_pending=True)
        db.commit()

    await asyncio.to_thread(_mark_pending)
    return jsonify({"message": "Schema refresh queued", "db_id": db_id}), 202
