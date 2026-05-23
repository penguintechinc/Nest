"""Tests for /api/blocked-databases endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestBlockedDatabases:
    def test_list_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/blocked-databases", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_blocked(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/blocked-databases", timeout=10)
        assert resp.status_code == 200

    def test_create_blocked(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/blocked-databases",
            json={
                "name": "test-blocked",
                "type": "database",
                "pattern": "test_blocked_*",
                "reason": "API test",
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201)

    def test_delete_blocked_not_found(self, api_session, api_url):
        resp = api_session.delete(f"{api_url}/blocked-databases/99999", timeout=10)
        assert resp.status_code in (404, 400)
