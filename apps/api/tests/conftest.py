"""Pytest fixtures for Nest API tests."""

import sys
from pathlib import Path

# Add parent directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent))

import json  # noqa: E402
import os  # noqa: E402
from datetime import datetime, timedelta, timezone  # noqa: E402
from typing import Any, cast  # noqa: E402
from unittest.mock import MagicMock  # noqa: E402
from unittest.mock import patch  # noqa: E402

import jwt  # noqa: E402
import pytest  # noqa: E402
from cryptography.hazmat.backends import default_backend  # noqa: E402
from cryptography.hazmat.primitives.asymmetric import rsa  # noqa: E402
from cryptography.hazmat.primitives.asymmetric.rsa import (  # noqa: E402
    RSAPrivateKey,
    RSAPublicKey,
)

from app import create_app  # noqa: E402
from store.store import MemoryStore  # noqa: E402

# Test RSA keypair (generated once per session)
_TEST_PRIVATE_KEY = None
_TEST_PUBLIC_KEY = None
_TEST_KID = "test-key-id"


def _generate_test_keypair() -> tuple[RSAPrivateKey, RSAPublicKey]:
    """Generate a test RSA keypair."""
    global _TEST_PRIVATE_KEY, _TEST_PUBLIC_KEY

    if _TEST_PRIVATE_KEY is None:
        private_key = rsa.generate_private_key(
            public_exponent=65537,
            key_size=2048,
            backend=default_backend(),
        )
        _TEST_PRIVATE_KEY = private_key
        _TEST_PUBLIC_KEY = private_key.public_key()

    return _TEST_PRIVATE_KEY, _TEST_PUBLIC_KEY


def _get_test_jwk() -> dict[str, Any]:
    """Get the test public key as a JWK dict."""
    _, public_key = _generate_test_keypair()

    # Convert public key to JWK using PyJWT's RSAAlgorithm
    from jwt.algorithms import RSAAlgorithm

    # RSAAlgorithm.to_jwk() returns a JSON string
    jwk_str = RSAAlgorithm.to_jwk(public_key)
    # Parse the JSON string to a dict
    jwk = cast(dict[str, Any], json.loads(jwk_str))
    # Add kid
    jwk["kid"] = _TEST_KID
    return jwk


def _mock_get_jwks_keys() -> list[dict[str, Any]]:
    """Mock implementation of _get_jwks_keys that returns test key."""
    return [_get_test_jwk()]


def _mint_token(
    sub: str, tenant: str, tier: str, scopes: list[str] | None = None
) -> str:
    """Mint a valid RS256 JWT token with required claims.

    Args:
        sub: Subject claim
        tenant: Tenant claim (mandatory)
        tier: Tier claim (pro/free)
        scopes: List of scope strings (will be joined with space)

    Returns:
        Bearer token string (no "Bearer " prefix)
    """
    if scopes is None:
        scopes = ["nest:*:admin"]  # Default: admin scope

    private_key, _ = _generate_test_keypair()

    now = datetime.now(timezone.utc)
    exp = now + timedelta(hours=1)

    payload = {
        "sub": sub,
        "tenant": tenant,
        "scope": " ".join(scopes),
        "iss": "https://nest.penguintech.io",
        "aud": "nest-api",
        "iat": int(now.timestamp()),
        "exp": int(exp.timestamp()),
        "https://nest.penguintech.io/tier": tier,
    }

    token = jwt.encode(
        payload,
        private_key,
        algorithm="RS256",
        headers={"kid": _TEST_KID},
    )

    return token


@pytest.fixture(scope="session", autouse=True)
def setup_oidc_env() -> None:
    """Set up OIDC environment variables for tests."""
    os.environ["OIDC_JWKS_URL"] = "https://nest.penguintech.io/.well-known/jwks.json"
    os.environ["OIDC_ISSUER"] = "https://nest.penguintech.io"
    os.environ["OIDC_AUDIENCE"] = "nest-api"


@pytest.fixture(scope="session", autouse=True)
def patch_jwks_resolution():  # type: ignore[no-untyped-def]
    """Patch JWKS resolution to use test keypair for all tests."""
    with patch("middleware.tenant._get_jwks_keys", side_effect=_mock_get_jwks_keys):
        yield


@pytest.fixture(scope="session", autouse=True)
def patch_kubernetes_client():  # type: ignore[no-untyped-def]
    """Patch Kubernetes client initialization for all tests.

    This prevents the restore handler from trying to load in-cluster config,
    which fails in test environments. All K8s API calls are mocked.
    """

    def mock_load_incluster_config():  # type: ignore[no-untyped-def]
        """Mock load_incluster_config to do nothing instead of raising."""
        pass

    def mock_custom_objects_api(api_client):  # type: ignore[no-untyped-def]
        """Return a mock CustomObjectsApi."""
        mock_api = MagicMock()
        # Mock list_namespaced_custom_object to return a sample backup
        # so the handler can proceed with restore
        mock_api.list_namespaced_custom_object.return_value = {
            "items": [
                {
                    "metadata": {
                        "name": "backup-001",
                        "creationTimestamp": "2024-01-01T00:00:00Z",
                    },
                    "spec": {"includedNamespaces": ["default"]},
                }
            ]
        }
        # Mock create_namespaced_custom_object to succeed
        mock_api.create_namespaced_custom_object.return_value = {
            "metadata": {"name": "restore-001"},
        }
        return mock_api

    def mock_core_v1_api(api_client):  # type: ignore[no-untyped-def]
        """Return a mock CoreV1Api."""
        mock_api = MagicMock()
        # Mock create_namespaced_persistent_volume_claim to succeed
        mock_api.create_namespaced_persistent_volume_claim.return_value = {
            "metadata": {"name": "pvc-restored"},
        }
        return mock_api

    def mock_api_client():  # type: ignore[no-untyped-def]
        """Return a mock ApiClient."""
        return MagicMock()

    with (
        patch(
            "kubernetes.config.load_incluster_config",
            side_effect=mock_load_incluster_config,
        ),
        patch("kubernetes.client.ApiClient", side_effect=mock_api_client),
        patch(
            "kubernetes.client.CustomObjectsApi",
            side_effect=mock_custom_objects_api,
        ),
        patch(
            "kubernetes.client.CoreV1Api", side_effect=mock_core_v1_api
        ),
    ):
        yield


@pytest.fixture
async def client():  # type: ignore[no-untyped-def]
    """Create a test app with fresh MemoryStore."""
    store = MemoryStore()
    app = create_app(store)

    async with app.test_client() as test_client:
        yield test_client


@pytest.fixture
async def store():  # type: ignore[no-untyped-def]
    """Create a fresh MemoryStore for each test."""
    return MemoryStore()


@pytest.fixture
def bearer_token() -> str:
    """Generate a valid RS256 bearer token for testing.

    Returns a pro-tier token with admin scope.
    """
    return _mint_token(sub="test-sub", tenant="test-tenant", tier="pro")


@pytest.fixture
def free_tier_token() -> str:
    """Generate a free-tier RS256 bearer token for testing."""
    return _mint_token(sub="test-sub", tenant="test-tenant", tier="free")
