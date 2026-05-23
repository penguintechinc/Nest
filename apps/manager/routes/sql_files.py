"""SQL file management routes."""
import asyncio
import hashlib
import logging
from concurrent.futures import ProcessPoolExecutor
from quart import Blueprint, jsonify, request, g

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

sql_files_bp = Blueprint("sql_files_bp", __name__, url_prefix="/api/v1")

# ProcessPoolExecutor is initialized in app.py and injected via app context.
# Routes access it via current_app.
from quart import current_app


def _validate_sql_security(content: str) -> dict:


    db = get_db()
    """CPU-bound SQL security validation — runs in a separate process."""
    from utils.sql_validator import validate_sql_security  # type: ignore
    return validate_sql_security(content)


@sql_files_bp.route("/sql-files", methods=["GET"])
@require_auth
async def list_sql_files():
    """List all SQL files."""
    def _query():
        db = get_db()
        rows = db(db.sql_file.id > 0).select(
            db.sql_file.id,
            db.sql_file.filename,
            db.sql_file.sha256_hash,
            db.sql_file.file_size,
            db.sql_file.created_at,
            db.sql_file.created_by,
            db.sql_file.is_valid,
            orderby=~db.sql_file.created_at,
        )
        return [r.as_dict() for r in rows]

    files = await asyncio.to_thread(_query)
    return jsonify({"data": files}), 200


@sql_files_bp.route("/sql-files", methods=["POST"])
@require_auth
async def upload_sql_file():
    """Upload a SQL file (content provided as JSON)."""
    body = await request.get_json()
    if not body:
        return jsonify({"error": "Request body required"}), 400

    filename = body.get("filename", "").strip()
    content = body.get("content", "")
    if not filename:
        return jsonify({"error": "filename is required"}), 400
    if not content:
        return jsonify({"error": "content is required"}), 400

    sha256_hash = hashlib.sha256(content.encode()).hexdigest()
    file_size = len(content.encode())

    def _insert():

        db = get_db()
        file_id = db.sql_file.insert(
            filename=filename,
            content=content,
            sha256_hash=sha256_hash,
            file_size=file_size,
            created_by=g.user_id,
            is_valid=None,
        )
        db.commit()
        row = db.sql_file[file_id]
        result = row.as_dict()
        result.pop("content", None)
        return result

    file_record = await asyncio.to_thread(_insert)
    return jsonify({"data": file_record}), 201


@sql_files_bp.route("/sql-files/<int:file_id>", methods=["GET"])
@require_auth
async def get_sql_file(file_id: int):
    """Get SQL file metadata and content."""
    def _query():
        db = get_db()
        row = db.sql_file[file_id]
        return row.as_dict() if row else None

    file_record = await asyncio.to_thread(_query)
    if file_record is None:
        return jsonify({"error": "SQL file not found"}), 404
    return jsonify({"data": file_record}), 200


@sql_files_bp.route("/sql-files/<int:file_id>", methods=["DELETE"])
@require_role("admin")
async def delete_sql_file(file_id: int):
    """Delete a SQL file."""
    def _delete():
        db = get_db()
        row = db.sql_file[file_id]
        if not row:
            return False
        db(db.sql_file.id == file_id).delete()
        db.commit()
        return True

    deleted = await asyncio.to_thread(_delete)
    if not deleted:
        return jsonify({"error": "SQL file not found"}), 404
    return jsonify({"message": "SQL file deleted"}), 200


@sql_files_bp.route("/sql-files/<int:file_id>/validate", methods=["POST"])
@require_auth
async def validate_sql_file(file_id: int):
    """Validate SQL file security using a subprocess pool."""
    def _get_content():
        db = get_db()
        row = db.sql_file[file_id]
        if not row:
            return None, None
        return row.content, row.id

    content, row_id = await asyncio.to_thread(_get_content)
    if row_id is None:
        return jsonify({"error": "SQL file not found"}), 404

    cpu_pool: ProcessPoolExecutor = current_app.cpu_pool
    loop = asyncio.get_event_loop()
    try:
        result = await loop.run_in_executor(cpu_pool, _validate_sql_security, content)
    except Exception as exc:
        logger.error("SQL validation error for file %d: %s", file_id, exc)
        return jsonify({"error": "Validation failed", "detail": str(exc)}), 500

    is_valid = result.get("valid", False)

    def _store_result():

        db = get_db()
        db(db.sql_file.id == file_id).update(is_valid=is_valid, validation_result=str(result))
        db.commit()

    await asyncio.to_thread(_store_result)
    return jsonify({"file_id": file_id, "valid": is_valid, "result": result}), 200


@sql_files_bp.route("/sql-files/<int:file_id>/execute", methods=["POST"])
@require_role("admin")
async def execute_sql_file(file_id: int):
    """Execute a SQL file — not yet implemented."""
    return jsonify({"error": "SQL file execution is not yet implemented"}), 501
