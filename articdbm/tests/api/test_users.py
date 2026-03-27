"""Tests for /api/users endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestUsers:
    def test_enhanced_users_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/users/enhanced", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_enhanced_users(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/users/enhanced", timeout=10)
        assert resp.status_code == 200

    def test_regenerate_api_key_requires_auth(self, anon_session):
        resp = anon_session.post(f"{MANAGER_URL}/api/users/1/regenerate-api-key", timeout=10)
        assert resp.status_code in (401, 403)

    def test_rate_limit_requires_auth(self, anon_session):
        resp = anon_session.put(f"{MANAGER_URL}/api/users/1/rate-limit", timeout=10)
        assert resp.status_code in (401, 403, 405)
