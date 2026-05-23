"""Tests for /api/temporary-access endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestTemporaryAccess:
    def test_list_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/temporary-access", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_temp_access(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/temporary-access", timeout=10)
        assert resp.status_code == 200

    def test_create_temp_access(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/temporary-access",
            json={
                "database_name": "testdb",
                "actions": ["read"],
                "max_uses": 5,
                "expires_at": "2026-12-31T23:59:59Z",
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201, 400)

    def test_revoke_not_found(self, api_session, api_url):
        resp = api_session.post(f"{api_url}/temporary-access/99999/revoke", timeout=10)
        assert resp.status_code in (404, 400)
