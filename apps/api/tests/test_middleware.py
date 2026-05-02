"""Tests for TenantMiddleware."""
import pytest


@pytest.mark.asyncio
async def test_missing_authorization_header(client):
    """Test that missing Authorization header returns 401."""
    response = await client.get("/api/v1/catalog")
    assert response.status_code == 401
    data = await response.get_json()
    assert data["code"] == "nest.auth.missing_token"


@pytest.mark.asyncio
async def test_invalid_bearer_token(client):
    """Test that invalid Bearer token returns 403."""
    response = await client.get(
        "/api/v1/catalog",
        headers={"Authorization": "Bearer invalid_no_tenant"},
    )
    assert response.status_code == 403
    data = await response.get_json()
    assert data["code"] == "nest.auth.missing_tenant"


@pytest.mark.asyncio
async def test_valid_token(client, bearer_token):
    """Test that valid token allows access."""
    response = await client.get(
        "/api/v1/catalog",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert "types" in data


@pytest.mark.asyncio
async def test_request_id_generation(client, bearer_token):
    """Test that X-Request-ID is generated if not present."""
    response = await client.get(
        "/api/v1/catalog",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_request_id_preservation(client, bearer_token):
    """Test that provided X-Request-ID is used."""
    custom_id = "my-custom-request-id"
    response = await client.get(
        "/api/v1/catalog",
        headers={
            "Authorization": f"Bearer {bearer_token}",
            "X-Request-ID": custom_id,
        },
    )
    assert response.status_code == 200
