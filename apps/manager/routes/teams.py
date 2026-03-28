"""
Teams routes — ported from apps/api/controllers/teams.go.

Port notes:
- Go used a `team_members` table; the Python manager schema uses `team_memberships`
  (see models/users.py define_team_memberships).
- Go scoped list/get by team membership for non-global-admins; here we replicate
  that using PyDAL joins.  Callers with role=="admin" are treated as global admins.
- Go's AddMemberRequest.Role accepted: team_admin | team_maintainer | team_viewer.
  We preserve those values unchanged.
- All PyDAL calls are wrapped in asyncio.to_thread() per Quart async-route rules.
- Soft-delete for teams is implemented via deleted_at timestamp; queries filter
  deleted_at IS NULL.
"""

import asyncio
import logging
from typing import Any

from quart import Blueprint, g, jsonify, request

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

teams_bp = Blueprint("teams", __name__, url_prefix="/api/v1")

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

VALID_MEMBER_ROLES = {"team_admin", "team_maintainer", "team_viewer"}


def _team_to_dict(team: Any, members: list | None = None) -> dict:


    db = get_db()
    """Serialize a PyDAL team Row."""
    result: dict = {
        "id": int(team.id),
        "name": team.name,
        "description": team.description or "",
        "is_global": bool(team.is_global),
        "created_at": team.created_at.isoformat() if team.created_at else None,
        "updated_at": team.updated_at.isoformat() if team.updated_at else None,
    }
    if members is not None:
        result["members"] = members
    return result


def _member_to_dict(membership: Any) -> dict:


    db = get_db()
    """Serialize a team_memberships row (with joined user data)."""
    return {
        "user_id": int(membership.user_id),
        "username": membership.users.username if hasattr(membership, "users") else "",
        "email": membership.users.email if hasattr(membership, "users") else "",
        "role": membership.role,
    }


def _is_global_admin() -> bool:


    db = get_db()
    return getattr(g, "user_role", "viewer") == "admin"


def _user_is_member(team_id: int, user_id: int) -> bool:


    db = get_db()
    """Synchronous helper — call inside asyncio.to_thread."""
    row = db(
        (db.team_memberships.team_id == team_id)
        & (db.team_memberships.user_id == user_id)
    ).select(limitby=(0, 1)).first()
    return row is not None


def _user_is_team_admin(team_id: int, user_id: int) -> bool:


    db = get_db()
    """Synchronous helper — call inside asyncio.to_thread."""
    row = db(
        (db.team_memberships.team_id == team_id)
        & (db.team_memberships.user_id == user_id)
        & (db.team_memberships.role == "team_admin")
    ).select(limitby=(0, 1)).first()
    return row is not None


# ---------------------------------------------------------------------------
# GET /api/v1/teams
# ---------------------------------------------------------------------------

@teams_bp.route("/teams", methods=["GET"])
@require_auth
async def list_teams() -> tuple:
    """List teams.  Non-admins see only teams they belong to."""
    current_user_id: int = g.user_id

    def _do_list() -> list:

        db = get_db()
        base_query = db.teams.deleted_at == None  # noqa: E711

        if _is_global_admin():
            teams = db(base_query).select(db.teams.ALL, orderby=db.teams.name)
        else:
            # Join to team_memberships to restrict to teams the user belongs to.
            teams = db(
                base_query
                & (db.team_memberships.team_id == db.teams.id)
                & (db.team_memberships.user_id == current_user_id)
            ).select(db.teams.ALL, orderby=db.teams.name, groupby=db.teams.id)

        return [_team_to_dict(t) for t in teams]

    result = await asyncio.to_thread(_do_list)
    return jsonify({"teams": result, "count": len(result)}), 200


# ---------------------------------------------------------------------------
# POST /api/v1/teams
# ---------------------------------------------------------------------------

@teams_bp.route("/teams", methods=["POST"])
@require_role("admin")
async def create_team() -> tuple:
    """Create a new team.  Requires admin role."""
    body = await request.get_json(silent=True)
    if not body:
        return jsonify({"error": "invalid_request", "message": "request body required"}), 400

    name: str = (body.get("name") or "").strip()
    description: str = (body.get("description") or "").strip()

    if not name or len(name) > 255:
        return jsonify({
            "error": "invalid_request",
            "message": "name is required and must be ≤255 characters",
        }), 400

    if len(description) > 1000:
        return jsonify({
            "error": "invalid_request",
            "message": "description must be ≤1000 characters",
        }), 400

    def _do_create() -> dict:

        db = get_db()
        existing = db(
            (db.teams.name == name) & (db.teams.deleted_at == None)  # noqa: E711
        ).select(limitby=(0, 1)).first()
        if existing is not None:
            return {"conflict": True}

        team_id = db.teams.insert(name=name, description=description, is_global=False)
        db.commit()
        team = db(db.teams.id == team_id).select(limitby=(0, 1)).first()
        return {"team": team}

    result = await asyncio.to_thread(_do_create)

    if result.get("conflict"):
        return jsonify({"error": "duplicate_name", "message": "Team name already exists"}), 409

    logger.info("Team created: id=%d name=%s by user id=%d", int(result["team"].id), name, g.user_id)
    return jsonify(_team_to_dict(result["team"])), 201


# ---------------------------------------------------------------------------
# GET /api/v1/teams/<team_id>
# ---------------------------------------------------------------------------

@teams_bp.route("/teams/<int:team_id>", methods=["GET"])
@require_auth
async def get_team(team_id: int) -> tuple:
    """Retrieve a single team with its members."""
    current_user_id: int = g.user_id

    def _do_get() -> dict | None:

        db = get_db()
        team = db(
            (db.teams.id == team_id) & (db.teams.deleted_at == None)  # noqa: E711
        ).select(limitby=(0, 1)).first()
        if team is None:
            return None

        if not _is_global_admin() and not _user_is_member(team_id, current_user_id):
            return {"forbidden": True}

        memberships = db(db.team_memberships.team_id == team_id).select(
            db.team_memberships.ALL,
            db.users.id,
            db.users.username,
            db.users.email,
            left=db.users.on(db.team_memberships.user_id == db.users.id),
        )
        members = [_member_to_dict(m) for m in memberships]
        return {"team": _team_to_dict(team, members=members)}

    result = await asyncio.to_thread(_do_get)

    if result is None:
        return jsonify({"error": "not_found", "message": "Team not found"}), 404
    if result.get("forbidden"):
        return jsonify({"error": "insufficient_permissions", "message": "Access denied"}), 403

    return jsonify(result["team"]), 200


# ---------------------------------------------------------------------------
# PUT /api/v1/teams/<team_id>
# ---------------------------------------------------------------------------

@teams_bp.route("/teams/<int:team_id>", methods=["PUT"])
@require_role("maintainer")
async def update_team(team_id: int) -> tuple:
    """Update team name/description.  Requires maintainer or admin role."""
    current_user_id: int = g.user_id

    body = await request.get_json(silent=True)
    if not body:
        return jsonify({"error": "invalid_request", "message": "request body required"}), 400

    name: str = (body.get("name") or "").strip()
    description: str = (body.get("description") or "").strip()

    if not name or len(name) > 255:
        return jsonify({
            "error": "invalid_request",
            "message": "name is required and must be ≤255 characters",
        }), 400

    if len(description) > 1000:
        return jsonify({
            "error": "invalid_request",
            "message": "description must be ≤1000 characters",
        }), 400

    def _do_update() -> dict:

        db = get_db()
        team = db(
            (db.teams.id == team_id) & (db.teams.deleted_at == None)  # noqa: E711
        ).select(limitby=(0, 1)).first()
        if team is None:
            return {"not_found": True}

        # Non-admins must be team_admin in this team.
        if not _is_global_admin() and not _user_is_team_admin(team_id, current_user_id):
            return {"forbidden": True}

        # Check name uniqueness (excluding current team).
        if name != team.name:
            duplicate = db(
                (db.teams.name == name)
                & (db.teams.id != team_id)
                & (db.teams.deleted_at == None)  # noqa: E711
            ).select(limitby=(0, 1)).first()
            if duplicate is not None:
                return {"conflict": True}

        db(db.teams.id == team_id).update(name=name, description=description)
        db.commit()
        updated = db(db.teams.id == team_id).select(limitby=(0, 1)).first()
        return {"team": updated}

    result = await asyncio.to_thread(_do_update)

    if result.get("not_found"):
        return jsonify({"error": "not_found", "message": "Team not found"}), 404
    if result.get("forbidden"):
        return jsonify({"error": "insufficient_permissions", "message": "Access denied"}), 403
    if result.get("conflict"):
        return jsonify({"error": "duplicate_name", "message": "Team name already exists"}), 409

    logger.info("Team updated: id=%d name=%s by user id=%d", team_id, name, g.user_id)
    return jsonify(_team_to_dict(result["team"])), 200


# ---------------------------------------------------------------------------
# DELETE /api/v1/teams/<team_id>
# ---------------------------------------------------------------------------

@teams_bp.route("/teams/<int:team_id>", methods=["DELETE"])
@require_role("admin")
async def delete_team(team_id: int) -> tuple:
    """Soft-delete a team (sets deleted_at).  Requires admin role.
    Global teams cannot be deleted."""

    def _do_delete() -> dict:

        db = get_db()
        team = db(
            (db.teams.id == team_id) & (db.teams.deleted_at == None)  # noqa: E711
        ).select(limitby=(0, 1)).first()
        if team is None:
            return {"not_found": True}

        if bool(team.is_global):
            return {"global": True}

        from datetime import datetime, timezone
        db(db.teams.id == team_id).update(deleted_at=datetime.now(timezone.utc))
        db.commit()
        return {"ok": True}

    result = await asyncio.to_thread(_do_delete)

    if result.get("not_found"):
        return jsonify({"error": "not_found", "message": "Team not found"}), 404
    if result.get("global"):
        return jsonify({"error": "cannot_delete_global", "message": "Cannot delete the global team"}), 400

    logger.info("Team deleted: id=%d by user id=%d", team_id, g.user_id)
    return jsonify({"message": "Team deleted successfully"}), 200


# ---------------------------------------------------------------------------
# GET /api/v1/teams/<team_id>/members
# ---------------------------------------------------------------------------

@teams_bp.route("/teams/<int:team_id>/members", methods=["GET"])
@require_auth
async def list_team_members(team_id: int) -> tuple:
    """List members of a team."""
    current_user_id: int = g.user_id

    def _do_list() -> dict:

        db = get_db()
        team = db(
            (db.teams.id == team_id) & (db.teams.deleted_at == None)  # noqa: E711
        ).select(limitby=(0, 1)).first()
        if team is None:
            return {"not_found": True}

        if not _is_global_admin() and not _user_is_member(team_id, current_user_id):
            return {"forbidden": True}

        memberships = db(db.team_memberships.team_id == team_id).select(
            db.team_memberships.ALL,
            db.users.id,
            db.users.username,
            db.users.email,
            left=db.users.on(db.team_memberships.user_id == db.users.id),
        )
        members = [_member_to_dict(m) for m in memberships]
        return {"members": members}

    result = await asyncio.to_thread(_do_list)

    if result.get("not_found"):
        return jsonify({"error": "not_found", "message": "Team not found"}), 404
    if result.get("forbidden"):
        return jsonify({"error": "insufficient_permissions", "message": "Access denied"}), 403

    members = result["members"]
    return jsonify({"members": members, "count": len(members)}), 200


# ---------------------------------------------------------------------------
# POST /api/v1/teams/<team_id>/members
# ---------------------------------------------------------------------------

@teams_bp.route("/teams/<int:team_id>/members", methods=["POST"])
@require_role("admin")
async def add_team_member(team_id: int) -> tuple:
    """Add a user to a team.  Requires admin role.

    Accepts JSON: {"user_id": <int>, "role": "team_admin|team_maintainer|team_viewer"}
    """
    body = await request.get_json(silent=True)
    if not body:
        return jsonify({"error": "invalid_request", "message": "request body required"}), 400

    try:
        user_id = int(body.get("user_id", 0))
    except (TypeError, ValueError):
        return jsonify({"error": "invalid_request", "message": "user_id must be an integer"}), 400

    role: str = (body.get("role") or "").strip()

    if not user_id:
        return jsonify({"error": "invalid_request", "message": "user_id is required"}), 400

    if role not in VALID_MEMBER_ROLES:
        return jsonify({
            "error": "invalid_request",
            "message": f"role must be one of {sorted(VALID_MEMBER_ROLES)}",
        }), 400

    def _do_add() -> dict:

        db = get_db()
        team = db(
            (db.teams.id == team_id) & (db.teams.deleted_at == None)  # noqa: E711
        ).select(limitby=(0, 1)).first()
        if team is None:
            return {"team_not_found": True}

        user = db(db.users.id == user_id).select(limitby=(0, 1)).first()
        if user is None:
            return {"user_not_found": True}

        existing = db(
            (db.team_memberships.team_id == team_id)
            & (db.team_memberships.user_id == user_id)
        ).select(limitby=(0, 1)).first()
        if existing is not None:
            return {"already_member": True}

        db.team_memberships.insert(team_id=team_id, user_id=user_id, role=role)
        db.commit()

        membership = db(
            (db.team_memberships.team_id == team_id)
            & (db.team_memberships.user_id == user_id)
        ).select(
            db.team_memberships.ALL,
            db.users.id,
            db.users.username,
            db.users.email,
            left=db.users.on(db.team_memberships.user_id == db.users.id),
        ).first()
        return {"membership": membership}

    result = await asyncio.to_thread(_do_add)

    if result.get("team_not_found"):
        return jsonify({"error": "not_found", "message": "Team not found"}), 404
    if result.get("user_not_found"):
        return jsonify({"error": "not_found", "message": "User not found"}), 404
    if result.get("already_member"):
        return jsonify({"error": "already_member", "message": "User is already a member of this team"}), 409

    logger.info("Member added: user_id=%d team_id=%d role=%s by user id=%d", user_id, team_id, role, g.user_id)
    return jsonify(_member_to_dict(result["membership"])), 201


# ---------------------------------------------------------------------------
# DELETE /api/v1/teams/<team_id>/members/<user_id>
# ---------------------------------------------------------------------------

@teams_bp.route("/teams/<int:team_id>/members/<int:user_id>", methods=["DELETE"])
@require_role("admin")
async def remove_team_member(team_id: int, user_id: int) -> tuple:
    """Remove a user from a team.  Requires admin role."""

    def _do_remove() -> dict:

        db = get_db()
        team = db(
            (db.teams.id == team_id) & (db.teams.deleted_at == None)  # noqa: E711
        ).select(limitby=(0, 1)).first()
        if team is None:
            return {"team_not_found": True}

        membership = db(
            (db.team_memberships.team_id == team_id)
            & (db.team_memberships.user_id == user_id)
        ).select(limitby=(0, 1)).first()
        if membership is None:
            return {"not_found": True}

        db(
            (db.team_memberships.team_id == team_id)
            & (db.team_memberships.user_id == user_id)
        ).delete()
        db.commit()
        return {"ok": True}

    result = await asyncio.to_thread(_do_remove)

    if result.get("team_not_found"):
        return jsonify({"error": "not_found", "message": "Team not found"}), 404
    if result.get("not_found"):
        return jsonify({"error": "not_found", "message": "Team member not found"}), 404

    logger.info("Member removed: user_id=%d team_id=%d by user id=%d", user_id, team_id, g.user_id)
    return jsonify({"message": "Team member removed successfully"}), 200
