"""Shared test fixtures for ArticDBM test suite."""
import os
import pytest
import requests
import subprocess
import time


# === Base URL Configuration (env-var driven for portability) ===

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")
PROXY_METRICS_URL = os.getenv("PROXY_METRICS_URL", "http://localhost:9091")

# Database connection info
DB_CONFIG = {
    "redis_dev": {
        "host": os.getenv("REDIS_DEV_HOST", "localhost"),
        "port": int(os.getenv("REDIS_DEV_PORT", "6379")),
    },
    "postgres_dev": {
        "host": os.getenv("POSTGRES_DEV_HOST", "localhost"),
        "port": int(os.getenv("POSTGRES_DEV_PORT", "5433")),
        "user": os.getenv("POSTGRES_DEV_USER", "articdbm"),
        "password": os.getenv("POSTGRES_DEV_PASSWORD", "devpass123"),
        "dbname": os.getenv("POSTGRES_DEV_DB", "articdbm"),
    },
    "mysql_test": {
        "host": os.getenv("MYSQL_TEST_HOST", "localhost"),
        "port": int(os.getenv("MYSQL_TEST_PORT", "3307")),
        "user": os.getenv("MYSQL_TEST_USER", "testuser"),
        "password": os.getenv("MYSQL_TEST_PASSWORD", "testpass123"),
        "dbname": os.getenv("MYSQL_TEST_DB", "testdb"),
    },
    "postgres_test": {
        "host": os.getenv("POSTGRES_TEST_HOST", "localhost"),
        "port": int(os.getenv("POSTGRES_TEST_PORT", "5434")),
        "user": os.getenv("POSTGRES_TEST_USER", "testuser"),
        "password": os.getenv("POSTGRES_TEST_PASSWORD", "testpass123"),
        "dbname": os.getenv("POSTGRES_TEST_DB", "testdb"),
    },
    "mongo_test": {
        "host": os.getenv("MONGO_TEST_HOST", "localhost"),
        "port": int(os.getenv("MONGO_TEST_PORT", "27017")),
        "user": os.getenv("MONGO_TEST_USER", "testuser"),
        "password": os.getenv("MONGO_TEST_PASSWORD", "testpass123"),
        "dbname": os.getenv("MONGO_TEST_DB", "testdb"),
    },
    "redis_test": {
        "host": os.getenv("REDIS_TEST_HOST", "localhost"),
        "port": int(os.getenv("REDIS_TEST_PORT", "6380")),
        "password": os.getenv("REDIS_TEST_PASSWORD", "testpass123"),
    },
}


@pytest.fixture(scope="session")
def manager_url():
    """Base URL for the ArticDBM manager service."""
    return MANAGER_URL


@pytest.fixture(scope="session")
def proxy_metrics_url():
    """Base URL for the ArticDBM proxy metrics endpoint."""
    return PROXY_METRICS_URL


@pytest.fixture(scope="session")
def db_config():
    """Database connection configuration dict."""
    return DB_CONFIG


@pytest.fixture(scope="session")
def http_client():
    """Reusable requests session."""
    session = requests.Session()
    session.headers.update({"Accept": "application/json"})
    yield session
    session.close()


@pytest.fixture(scope="session")
def auth_session(manager_url):
    """Authenticated requests session with py4web login.

    Attempts to register a test user, then login.
    Returns session with auth cookie set.
    """
    session = requests.Session()

    # Try to register test user (may already exist)
    try:
        session.post(
            f"{manager_url}/auth/api/register",
            json={
                "email": "testrunner@articdbm.test",
                "password": "TestPass123!",
                "first_name": "Test",
                "last_name": "Runner",
            },
            timeout=10,
        )
    except requests.RequestException:
        pass

    # Login
    try:
        resp = session.post(
            f"{manager_url}/auth/api/login",
            json={
                "email": "testrunner@articdbm.test",
                "password": "TestPass123!",
            },
            timeout=10,
        )
        if resp.status_code != 200:
            pytest.skip("Could not authenticate test user")
    except requests.RequestException:
        pytest.skip("Manager service not available for auth")

    yield session
    session.close()


def _is_docker_available():
    """Check if Docker is available."""
    try:
        result = subprocess.run(
            ["docker", "info"],
            capture_output=True,
            timeout=10,
        )
        return result.returncode == 0
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return False


skip_if_no_docker = pytest.mark.skipif(
    not _is_docker_available(),
    reason="Docker not available",
)


def _is_service_up(url, timeout=5):
    """Check if an HTTP service is responding."""
    try:
        resp = requests.get(url, timeout=timeout)
        return resp.status_code < 500
    except requests.RequestException:
        return False


skip_if_no_manager = pytest.mark.skipif(
    not _is_service_up(f"{MANAGER_URL}/api/health"),
    reason="Manager service not available",
)

skip_if_no_proxy = pytest.mark.skipif(
    not _is_service_up(f"{PROXY_METRICS_URL}/health"),
    reason="Proxy service not available",
)


def wait_for_service(url, timeout=60, interval=2):
    """Wait for an HTTP service to become available."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        if _is_service_up(url, timeout=5):
            return True
        time.sleep(interval)
    return False
