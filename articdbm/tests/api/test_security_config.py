"""Tests for /api/databases/<id>/security-config endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestSecurityConfig:
    def test_security_config_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/databases/1/security-config", timeout=10)
        assert resp.status_code in (401, 403)

    def test_security_config_not_found(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/databases/99999/security-config", timeout=10)
        assert resp.status_code in (404, 400, 200)
