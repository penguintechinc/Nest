"""API test fixtures."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")


@pytest.fixture(scope="session")
def api_url():
    """Base API URL."""
    return f"{MANAGER_URL}/api"


@pytest.fixture(scope="session")
def api_session():
    """Authenticated API session."""
    session = requests.Session()
    session.headers.update({"Accept": "application/json"})

    # Register test user
    try:
        session.post(
            f"{MANAGER_URL}/auth/api/register",
            json={
                "email": "apitest@articdbm.test",
                "password": "ApiTest123!",
                "first_name": "API",
                "last_name": "Tester",
            },
            timeout=10,
        )
    except requests.RequestException:
        pass

    # Login
    try:
        resp = session.post(
            f"{MANAGER_URL}/auth/api/login",
            json={
                "email": "apitest@articdbm.test",
                "password": "ApiTest123!",
            },
            timeout=10,
        )
        if resp.status_code != 200:
            pytest.skip("Cannot authenticate for API tests")
    except requests.RequestException:
        pytest.skip("Manager not available for API tests")

    yield session
    session.close()


@pytest.fixture(scope="session")
def anon_session():
    """Unauthenticated session for testing auth enforcement."""
    session = requests.Session()
    session.headers.update({"Accept": "application/json"})
    yield session
    session.close()
