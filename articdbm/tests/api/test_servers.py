"""Tests for /api/servers CRUD endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestServers:
    def test_list_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/servers", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_servers(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/servers", timeout=10)
        assert resp.status_code == 200
        data = resp.json()
        assert isinstance(data, (list, dict))

    def test_create_server(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/servers",
            json={
                "name": "test-server-api",
                "type": "mysql",
                "host": "localhost",
                "port": 3306,
                "role": "both",
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201)

    def test_create_server_invalid(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/servers",
            json={"name": ""},
            timeout=10,
        )
        assert resp.status_code in (400, 422)

    def test_get_server_not_found(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/servers/99999", timeout=10)
        assert resp.status_code in (404, 400)

    def test_delete_server_not_found(self, api_session, api_url):
        resp = api_session.delete(f"{api_url}/servers/99999", timeout=10)
        assert resp.status_code in (404, 400)
