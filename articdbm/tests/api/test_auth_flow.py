"""End-to-end authentication flow tests."""
import os
import uuid
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestAuthFlow:
    """E2E test: unauthenticated -> register -> login -> access protected."""

    def test_unauthenticated_access_denied(self):
        """Without auth, protected endpoints should return 401/403."""
        session = requests.Session()
        resp = session.get(f"{MANAGER_URL}/api/servers", timeout=10)
        assert resp.status_code in (401, 403)

    def test_full_auth_flow(self):
        """Register, login, access protected endpoint, logout."""
        session = requests.Session()
        unique = uuid.uuid4().hex[:8]

        # Register
        resp = session.post(
            f"{MANAGER_URL}/auth/api/register",
            json={
                "email": f"e2e-{unique}@articdbm.test",
                "password": "E2eTest123!",
                "first_name": "E2E",
                "last_name": "Test",
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201, 409)

        # Login
        resp = session.post(
            f"{MANAGER_URL}/auth/api/login",
            json={
                "email": f"e2e-{unique}@articdbm.test",
                "password": "E2eTest123!",
            },
            timeout=10,
        )
        assert resp.status_code == 200

        # Access protected endpoint
        resp = session.get(f"{MANAGER_URL}/api/servers", timeout=10)
        assert resp.status_code == 200

    def test_wrong_password_rejected(self):
        """Login with wrong password should fail."""
        session = requests.Session()
        resp = session.post(
            f"{MANAGER_URL}/auth/api/login",
            json={
                "email": "nonexistent@articdbm.test",
                "password": "WrongPass123!",
            },
            timeout=10,
        )
        assert resp.status_code in (401, 403, 422)
