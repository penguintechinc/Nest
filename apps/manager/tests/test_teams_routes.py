"""Teams routes tests — comprehensive coverage for routes/teams.py."""
import os
import sys
import types
import pytest
import pytest_asyncio
from unittest.mock import AsyncMock, MagicMock, patch

# Ensure the manager app directory is on the path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

# Required env vars before any import
os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("DB_HOST", "localhost")
os.environ.setdefault("DB_NAME", "test_nest")
os.environ.setdefault("DB_USER", "test")
os.environ.setdefault("FIELD_ENCRYPTION_KEY", "Fernet_key_placeholder_32bytes==")


def _install_fake_modules():
    """Inject fake heavy-weight modules so app.py imports cleanly."""
    fake_quart_ext = types.ModuleType("penguin_dal.quart_ext")
    fake_quart_ext.init_dal = MagicMock(return_value=None)
    fake_quart_ext.get_db = MagicMock()
    sys.modules["penguin_dal.quart_ext"] = fake_quart_ext

    fake_dblb = types.ModuleType("clients.dblb_grpc")
    fake_dblb.get_dblb_client = MagicMock(return_value=MagicMock())
    fake_dblb.init_dblb_client = MagicMock(return_value=None)
    fake_dblb.DblbGrpcClient = MagicMock()
    sys.modules["clients.dblb_grpc"] = fake_dblb

    for mod_name, fn_names in [
        ("workers.threat_intel_poller", ["threat_intel_poller_loop"]),
        ("workers.db_health_checker", ["db_health_checker_loop"]),
        ("workers.scaling_evaluator", ["scaling_evaluator_loop"]),
    ]:
        fake_w = types.ModuleType(mod_name)
        for fn in fn_names:
            setattr(fake_w, fn, AsyncMock())
        sys.modules.setdefault(mod_name, fake_w)


_install_fake_modules()

sys.modules.pop("app", None)
import app as _app_module  # noqa: E402

_application = _app_module.app
_application.config["TESTING"] = True


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_db() -> MagicMock:
    """Build a fresh MagicMock DB for teams routes."""
    db = MagicMock()
    db.commit = MagicMock()

    # Default query chain: db(...).select(...).first() -> None
    select_result = MagicMock()
    select_result.first.return_value = None
    select_result.__iter__ = lambda self: iter([])
    query_result = MagicMock()
    query_result.select.return_value = select_result
    db.return_value = query_result

    db.teams = MagicMock()
    db.teams.ALL = MagicMock()
    db.teams.id = MagicMock()
    db.teams.name = MagicMock()
    db.teams.deleted_at = MagicMock()
    db.teams.insert = MagicMock(return_value=1)

    db.team_memberships = MagicMock()
    db.team_memberships.ALL = MagicMock()
    db.team_memberships.team_id = MagicMock()
    db.team_memberships.user_id = MagicMock()
    db.team_memberships.role = MagicMock()
    db.team_memberships.insert = MagicMock()

    db.users = MagicMock()
    db.users.id = MagicMock()
    db.users.username = MagicMock()
    db.users.email = MagicMock()
    db.users.on = MagicMock()

    return db


def _make_team(team_id: int = 1, name: str = "Alpha", is_global: bool = False) -> MagicMock:
    from datetime import datetime
    t = MagicMock()
    t.id = team_id
    t.name = name
    t.description = "Test team"
    t.is_global = is_global
    t.deleted_at = None
    _dt = datetime(2025, 1, 1)
    t.created_at = _dt
    t.updated_at = _dt
    # Ensure isoformat() returns a real string, not a MagicMock
    t.created_at = _dt
    t.updated_at = _dt
    return t


def _make_membership(team_id: int = 1, user_id: int = 1, role: str = "team_viewer") -> MagicMock:
    m = MagicMock()
    m.team_id = team_id
    m.user_id = user_id
    m.role = role
    m.users = MagicMock()
    m.users.username = "testuser"
    m.users.email = "test@example.com"
    return m


def _make_user_mock(user_id: int = 1) -> MagicMock:
    u = MagicMock()
    u.id = user_id
    u.username = "testuser"
    u.email = "test@example.com"
    return u


def _make_token(role: str = "admin") -> str:
    from utils.auth import create_token
    return create_token(user_id=1, email="test@example.com", role=role)


def _auth_headers(role: str = "admin") -> dict:
    return {"Authorization": f"Bearer {_make_token(role)}"}


# The routes module imports get_db from penguin_dal.quart_ext at module level.
# We must patch the name as used inside the routes.teams module.
_TEAMS_GET_DB = "routes.teams.get_db"


# ---------------------------------------------------------------------------
# GET /api/v1/teams — list_teams
# ---------------------------------------------------------------------------

class TestListTeams:
    @pytest.mark.asyncio
    async def test_list_teams_admin_returns_all(self):
        """Admin sees all teams."""
        team = _make_team()
        db = _make_db()

        select_result = MagicMock()
        select_result.__iter__ = lambda self: iter([team])
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 200
        data = await resp.get_json()
        assert "teams" in data
        assert "count" in data

    @pytest.mark.asyncio
    async def test_list_teams_viewer_sees_own(self):
        """Non-admin sees only their teams."""
        team = _make_team()
        db = _make_db()

        select_result = MagicMock()
        select_result.__iter__ = lambda self: iter([team])
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams",
                    headers=_auth_headers("viewer"),
                )
        assert resp.status_code == 200

    @pytest.mark.asyncio
    async def test_list_teams_no_auth_returns_401(self):
        """No auth token → 401."""
        async with _application.test_client() as client:
            resp = await client.get("/api/v1/teams")
        assert resp.status_code == 401

    @pytest.mark.asyncio
    async def test_list_teams_empty_list(self):
        """Returns empty list when no teams exist."""
        db = _make_db()

        select_result = MagicMock()
        select_result.__iter__ = lambda self: iter([])
        query_result = MagicMock()
        query_result.select.return_value = select_result
        db.return_value = query_result

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 200
        data = await resp.get_json()
        assert data["count"] == 0


# ---------------------------------------------------------------------------
# POST /api/v1/teams — create_team
# ---------------------------------------------------------------------------

class TestCreateTeam:
    @pytest.mark.asyncio
    async def test_create_team_success(self):
        """Admin creates a team successfully."""
        db = _make_db()
        new_team = _make_team(team_id=5, name="Beta")

        # No existing team
        no_result = MagicMock()
        no_result.first.return_value = None
        # Team after insert
        found_result = MagicMock()
        found_result.first.return_value = new_team

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            r.select.return_value = no_result if call_count["n"] == 0 else found_result
            call_count["n"] += 1
            return r

        db.side_effect = side_effect
        db.teams.insert.return_value = 5

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.post(
                    "/api/v1/teams",
                    json={"name": "Beta", "description": "Beta team"},
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 201

    @pytest.mark.asyncio
    async def test_create_team_no_body_returns_400(self):
        """Missing body → 400."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams",
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_create_team_missing_name_returns_400(self):
        """Empty name → 400."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams",
                json={"name": "", "description": "desc"},
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_create_team_name_too_long_returns_400(self):
        """Name > 255 chars → 400."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams",
                json={"name": "x" * 256},
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_create_team_description_too_long_returns_400(self):
        """Description > 1000 chars → 400."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams",
                json={"name": "Valid", "description": "x" * 1001},
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_create_team_duplicate_name_returns_409(self):
        """Duplicate name → 409."""
        db = _make_db()
        existing_team = _make_team(name="Existing")

        result = MagicMock()
        result.first.return_value = existing_team
        query_result = MagicMock()
        query_result.select.return_value = result
        db.return_value = query_result

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.post(
                    "/api/v1/teams",
                    json={"name": "Existing"},
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 409

    @pytest.mark.asyncio
    async def test_create_team_requires_admin(self):
        """Viewer cannot create teams."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams",
                json={"name": "New"},
                headers=_auth_headers("viewer"),
            )
        assert resp.status_code in (401, 403)


# ---------------------------------------------------------------------------
# GET /api/v1/teams/<team_id> — get_team
# ---------------------------------------------------------------------------

class TestGetTeam:
    @pytest.mark.asyncio
    async def test_get_team_admin_success(self):
        """Admin can get any team."""
        team = _make_team()
        db = _make_db()

        # First call: find team; second call: list memberships
        team_result = MagicMock()
        team_result.first.return_value = team
        member_result = MagicMock()
        member_result.__iter__ = lambda self: iter([])
        member_result.first.return_value = None

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = team_result
            else:
                r.select.return_value = member_result
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams/1",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 200

    @pytest.mark.asyncio
    async def test_get_team_not_found_returns_404(self):
        """Non-existent team → 404."""
        db = _make_db()

        not_found = MagicMock()
        not_found.first.return_value = None
        qr = MagicMock()
        qr.select.return_value = not_found
        db.return_value = qr

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams/999",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_get_team_forbidden_for_non_member(self):
        """Non-member viewer gets 403."""
        team = _make_team()
        db = _make_db()

        # First call: team found
        team_result = MagicMock()
        team_result.first.return_value = team
        # Second call: membership check (not a member)
        no_member = MagicMock()
        no_member.first.return_value = None

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = team_result
            else:
                r.select.return_value = no_member
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams/1",
                    headers=_auth_headers("viewer"),
                )
        assert resp.status_code == 403

    @pytest.mark.asyncio
    async def test_get_team_member_can_see_own_team(self):
        """A team member (viewer role) can see their own team."""
        team = _make_team()
        membership = _make_membership(user_id=1)
        db = _make_db()

        team_result = MagicMock()
        team_result.first.return_value = team
        member_check = MagicMock()
        member_check.first.return_value = membership
        member_list = MagicMock()
        member_list.__iter__ = lambda self: iter([membership])

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = team_result
            elif call_count["n"] == 1:
                r.select.return_value = member_check
            else:
                r.select.return_value = member_list
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams/1",
                    headers=_auth_headers("viewer"),
                )
        assert resp.status_code == 200


# ---------------------------------------------------------------------------
# PUT /api/v1/teams/<team_id> — update_team
# ---------------------------------------------------------------------------

class TestUpdateTeam:
    @pytest.mark.asyncio
    async def test_update_team_admin_success(self):
        """Admin can update any team."""
        team = _make_team(name="OldName")
        updated_team = _make_team(name="NewName")
        db = _make_db()

        # Calls: find team, find duplicate name check (none), get updated team
        call_count = {"n": 0}
        results = []

        found = MagicMock()
        found.first.return_value = team
        no_dup = MagicMock()
        no_dup.first.return_value = None
        updated = MagicMock()
        updated.first.return_value = updated_team

        def side_effect(*args, **kwargs):
            r = MagicMock()
            n = call_count["n"]
            call_count["n"] += 1
            if n == 0:
                r.select.return_value = found
            elif n == 1:
                r.select.return_value = no_dup
            else:
                r.select.return_value = updated
            r.update = MagicMock()
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.put(
                    "/api/v1/teams/1",
                    json={"name": "NewName", "description": "Updated"},
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 200

    @pytest.mark.asyncio
    async def test_update_team_no_body_returns_400(self):
        """Missing body → 400."""
        async with _application.test_client() as client:
            resp = await client.put(
                "/api/v1/teams/1",
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_update_team_empty_name_returns_400(self):
        """Empty name → 400."""
        async with _application.test_client() as client:
            resp = await client.put(
                "/api/v1/teams/1",
                json={"name": ""},
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_update_team_not_found_returns_404(self):
        """Updating non-existent team → 404."""
        db = _make_db()

        not_found = MagicMock()
        not_found.first.return_value = None
        qr = MagicMock()
        qr.select.return_value = not_found
        db.return_value = qr

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.put(
                    "/api/v1/teams/999",
                    json={"name": "NewName"},
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_update_team_duplicate_name_returns_409(self):
        """Duplicate name on update → 409."""
        team = _make_team(name="OldName")
        other_team = _make_team(team_id=2, name="TakenName")
        db = _make_db()

        call_count = {"n": 0}

        found = MagicMock()
        found.first.return_value = team
        dup = MagicMock()
        dup.first.return_value = other_team

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = found
            else:
                r.select.return_value = dup
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.put(
                    "/api/v1/teams/1",
                    json={"name": "TakenName"},
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 409

    @pytest.mark.asyncio
    async def test_update_team_forbidden_non_team_admin(self):
        """Viewer who is not team_admin cannot update team."""
        team = _make_team()
        db = _make_db()

        # team found, but user is not team_admin
        found = MagicMock()
        found.first.return_value = team
        no_admin = MagicMock()
        no_admin.first.return_value = None

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = found
            else:
                r.select.return_value = no_admin
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.put(
                    "/api/v1/teams/1",
                    json={"name": "NewName"},
                    headers=_auth_headers("viewer"),
                )
        assert resp.status_code == 403

    @pytest.mark.asyncio
    async def test_update_team_requires_auth(self):
        """No token → 401."""
        async with _application.test_client() as client:
            resp = await client.put(
                "/api/v1/teams/1",
                json={"name": "X"},
            )
        assert resp.status_code == 401


# ---------------------------------------------------------------------------
# DELETE /api/v1/teams/<team_id> — delete_team
# ---------------------------------------------------------------------------

class TestDeleteTeam:
    @pytest.mark.asyncio
    async def test_delete_team_success(self):
        """Admin soft-deletes a team."""
        team = _make_team(is_global=False)
        db = _make_db()

        found = MagicMock()
        found.first.return_value = team
        qr = MagicMock()
        qr.select.return_value = found
        qr.update = MagicMock()
        db.return_value = qr

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.delete(
                    "/api/v1/teams/1",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 200

    @pytest.mark.asyncio
    async def test_delete_global_team_returns_400(self):
        """Cannot delete a global team."""
        global_team = _make_team(is_global=True)
        db = _make_db()

        found = MagicMock()
        found.first.return_value = global_team
        qr = MagicMock()
        qr.select.return_value = found
        db.return_value = qr

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.delete(
                    "/api/v1/teams/1",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_delete_team_not_found_returns_404(self):
        """Deleting non-existent team → 404."""
        db = _make_db()

        not_found = MagicMock()
        not_found.first.return_value = None
        qr = MagicMock()
        qr.select.return_value = not_found
        db.return_value = qr

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.delete(
                    "/api/v1/teams/999",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_delete_team_requires_admin(self):
        """Non-admin cannot delete teams."""
        async with _application.test_client() as client:
            resp = await client.delete(
                "/api/v1/teams/1",
                headers=_auth_headers("viewer"),
            )
        assert resp.status_code in (401, 403)


# ---------------------------------------------------------------------------
# GET /api/v1/teams/<team_id>/members — list_team_members
# ---------------------------------------------------------------------------

class TestListTeamMembers:
    @pytest.mark.asyncio
    async def test_list_members_admin_success(self):
        """Admin can list members of any team."""
        team = _make_team()
        membership = _make_membership()
        db = _make_db()

        team_result = MagicMock()
        team_result.first.return_value = team
        member_list = MagicMock()
        member_list.__iter__ = lambda self: iter([membership])

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = team_result
            else:
                r.select.return_value = member_list
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams/1/members",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 200
        data = await resp.get_json()
        assert "members" in data
        assert "count" in data

    @pytest.mark.asyncio
    async def test_list_members_team_not_found_returns_404(self):
        """Team not found → 404."""
        db = _make_db()

        not_found = MagicMock()
        not_found.first.return_value = None
        qr = MagicMock()
        qr.select.return_value = not_found
        db.return_value = qr

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams/999/members",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_list_members_forbidden_non_member(self):
        """Non-member viewer gets 403."""
        team = _make_team()
        db = _make_db()

        team_result = MagicMock()
        team_result.first.return_value = team
        no_member = MagicMock()
        no_member.first.return_value = None

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = team_result
            else:
                r.select.return_value = no_member
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.get(
                    "/api/v1/teams/1/members",
                    headers=_auth_headers("viewer"),
                )
        assert resp.status_code == 403


# ---------------------------------------------------------------------------
# POST /api/v1/teams/<team_id>/members — add_team_member
# ---------------------------------------------------------------------------

class TestAddTeamMember:
    @pytest.mark.asyncio
    async def test_add_member_success(self):
        """Admin adds a member to a team."""
        team = _make_team()
        user = _make_user_mock(user_id=2)
        membership = _make_membership(user_id=2, role="team_viewer")
        db = _make_db()

        team_result = MagicMock()
        team_result.first.return_value = team
        user_result = MagicMock()
        user_result.first.return_value = user
        no_existing = MagicMock()
        no_existing.first.return_value = None
        new_membership = MagicMock()
        new_membership.first.return_value = membership

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            n = call_count["n"]
            call_count["n"] += 1
            if n == 0:
                r.select.return_value = team_result
            elif n == 1:
                r.select.return_value = user_result
            elif n == 2:
                r.select.return_value = no_existing
            else:
                r.select.return_value = new_membership
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.post(
                    "/api/v1/teams/1/members",
                    json={"user_id": 2, "role": "team_viewer"},
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 201

    @pytest.mark.asyncio
    async def test_add_member_no_body_returns_400(self):
        """Missing body → 400."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams/1/members",
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_add_member_invalid_user_id_returns_400(self):
        """Non-integer user_id → 400."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams/1/members",
                json={"user_id": "abc", "role": "team_viewer"},
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_add_member_missing_user_id_returns_400(self):
        """Missing user_id → 400."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams/1/members",
                json={"role": "team_viewer"},
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_add_member_invalid_role_returns_400(self):
        """Invalid role → 400."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams/1/members",
                json={"user_id": 2, "role": "superuser"},
                headers=_auth_headers("admin"),
            )
        assert resp.status_code == 400

    @pytest.mark.asyncio
    async def test_add_member_team_not_found_returns_404(self):
        """Team not found → 404."""
        db = _make_db()

        not_found = MagicMock()
        not_found.first.return_value = None
        qr = MagicMock()
        qr.select.return_value = not_found
        db.return_value = qr

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.post(
                    "/api/v1/teams/999/members",
                    json={"user_id": 2, "role": "team_viewer"},
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_add_member_user_not_found_returns_404(self):
        """User not found → 404."""
        team = _make_team()
        db = _make_db()

        team_result = MagicMock()
        team_result.first.return_value = team
        no_user = MagicMock()
        no_user.first.return_value = None

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = team_result
            else:
                r.select.return_value = no_user
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.post(
                    "/api/v1/teams/1/members",
                    json={"user_id": 999, "role": "team_viewer"},
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_add_member_already_member_returns_409(self):
        """Already a member → 409."""
        team = _make_team()
        user = _make_user_mock()
        existing = _make_membership()
        db = _make_db()

        team_result = MagicMock()
        team_result.first.return_value = team
        user_result = MagicMock()
        user_result.first.return_value = user
        existing_result = MagicMock()
        existing_result.first.return_value = existing

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            n = call_count["n"]
            call_count["n"] += 1
            if n == 0:
                r.select.return_value = team_result
            elif n == 1:
                r.select.return_value = user_result
            else:
                r.select.return_value = existing_result
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.post(
                    "/api/v1/teams/1/members",
                    json={"user_id": 1, "role": "team_viewer"},
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 409

    @pytest.mark.asyncio
    async def test_add_member_all_valid_roles(self):
        """All three valid roles are accepted at validation layer."""
        for role in ("team_admin", "team_maintainer", "team_viewer"):
            db = _make_db()
            not_found = MagicMock()
            not_found.first.return_value = None
            qr = MagicMock()
            qr.select.return_value = not_found
            db.return_value = qr

            with patch(_TEAMS_GET_DB, return_value=db):
                async with _application.test_client() as client:
                    resp = await client.post(
                        "/api/v1/teams/1/members",
                        json={"user_id": 2, "role": role},
                        headers=_auth_headers("admin"),
                    )
            # 404 means it passed role validation (team not found is ok here)
            assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_add_member_requires_admin(self):
        """Viewer cannot add members."""
        async with _application.test_client() as client:
            resp = await client.post(
                "/api/v1/teams/1/members",
                json={"user_id": 2, "role": "team_viewer"},
                headers=_auth_headers("viewer"),
            )
        assert resp.status_code in (401, 403)


# ---------------------------------------------------------------------------
# DELETE /api/v1/teams/<team_id>/members/<user_id> — remove_team_member
# ---------------------------------------------------------------------------

class TestRemoveTeamMember:
    @pytest.mark.asyncio
    async def test_remove_member_success(self):
        """Admin removes a team member."""
        team = _make_team()
        membership = _make_membership()
        db = _make_db()

        team_result = MagicMock()
        team_result.first.return_value = team
        member_result = MagicMock()
        member_result.first.return_value = membership

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = team_result
            else:
                r.select.return_value = member_result
            r.delete = MagicMock()
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.delete(
                    "/api/v1/teams/1/members/1",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 200

    @pytest.mark.asyncio
    async def test_remove_member_team_not_found_returns_404(self):
        """Team not found → 404."""
        db = _make_db()

        not_found = MagicMock()
        not_found.first.return_value = None
        qr = MagicMock()
        qr.select.return_value = not_found
        db.return_value = qr

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.delete(
                    "/api/v1/teams/999/members/1",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_remove_member_not_found_returns_404(self):
        """Member not found → 404."""
        team = _make_team()
        db = _make_db()

        team_result = MagicMock()
        team_result.first.return_value = team
        no_member = MagicMock()
        no_member.first.return_value = None

        call_count = {"n": 0}

        def side_effect(*args, **kwargs):
            r = MagicMock()
            if call_count["n"] == 0:
                r.select.return_value = team_result
            else:
                r.select.return_value = no_member
            call_count["n"] += 1
            return r

        db.side_effect = side_effect

        with patch(_TEAMS_GET_DB, return_value=db):
            async with _application.test_client() as client:
                resp = await client.delete(
                    "/api/v1/teams/1/members/999",
                    headers=_auth_headers("admin"),
                )
        assert resp.status_code == 404

    @pytest.mark.asyncio
    async def test_remove_member_requires_admin(self):
        """Viewer cannot remove members."""
        async with _application.test_client() as client:
            resp = await client.delete(
                "/api/v1/teams/1/members/1",
                headers=_auth_headers("viewer"),
            )
        assert resp.status_code in (401, 403)
