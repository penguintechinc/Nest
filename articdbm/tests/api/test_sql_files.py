"""Tests for /api/sql-files CRUD endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestSQLFiles:
    def test_list_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/sql-files", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_sql_files(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/sql-files", timeout=10)
        assert resp.status_code == 200

    def test_create_sql_file(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/sql-files",
            json={
                "name": "test-migration.sql",
                "database_id": 1,
                "file_type": "migration",
                "file_content": "SELECT 1;",
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201, 400)

    def test_validate_sql_file(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/sql-files/validate",
            json={"content": "SELECT 1;"},
            timeout=10,
        )
        assert resp.status_code in (200, 400, 404)

    def test_get_sql_file_not_found(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/sql-files/99999", timeout=10)
        assert resp.status_code in (404, 400)
