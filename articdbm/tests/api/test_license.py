"""Tests for /api/license endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestLicense:
    def test_license_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/license", timeout=10)
        assert resp.status_code in (401, 403)

    def test_get_license_info(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/license", timeout=10)
        assert resp.status_code == 200

    def test_validate_license(self, api_session, api_url):
        resp = api_session.post(f"{api_url}/license/validate", timeout=10)
        assert resp.status_code in (200, 400, 404, 405)
