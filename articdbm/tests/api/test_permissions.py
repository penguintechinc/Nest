"""Tests for /api/permissions CRUD endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestPermissions:
    def test_list_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/permissions", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_permissions(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/permissions", timeout=10)
        assert resp.status_code == 200

    def test_create_permission(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/permissions",
            json={
                "user_id": 1,
                "database_name": "testdb",
                "table_name": "*",
                "actions": ["read"],
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201, 400)

    def test_delete_permission_not_found(self, api_session, api_url):
        resp = api_session.delete(f"{api_url}/permissions/99999", timeout=10)
        assert resp.status_code in (404, 400)
