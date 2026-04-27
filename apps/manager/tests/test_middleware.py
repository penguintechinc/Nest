"""Tests for tenant middleware."""
import pytest


@pytest.mark.asyncio
async def test_tenant_middleware_missing_header(client):
    """Test tenant middleware with missing Authorization header."""
    response = await client.get("/api/v1/tenants/tenant-1/operations")
    assert response.status_code == 401
    data = await response.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_tenant_middleware_malformed_bearer(client):
    """Test tenant middleware with malformed Bearer token (wrong format)."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": "NotBearer sub:tenant-1:pro"},
    )
    assert response.status_code == 401
    data = await response.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_tenant_middleware_missing_bearer_word(client):
    """Test tenant middleware with missing 'Bearer' keyword."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": "sub:tenant-1:pro"},
    )
    assert response.status_code == 401
    data = await response.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_tenant_middleware_too_many_parts(client):
    """Test tenant middleware with too many Authorization header parts."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": "Bearer token extra parts"},
    )
    assert response.status_code == 401
    data = await response.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_tenant_middleware_wrong_token_parts_count(client):
    """Test tenant middleware with wrong number of token parts (not 3)."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": "Bearer sub:tenant-1"},  # Missing tier
    )
    assert response.status_code == 401
    data = await response.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_tenant_middleware_too_many_token_parts(client):
    """Test tenant middleware with too many token parts."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": "Bearer sub:tenant:tier:extra"},
    )
    assert response.status_code == 401
    data = await response.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_tenant_middleware_empty_tenant(client):
    """Test tenant middleware with empty tenant claim."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": "Bearer sub::pro"},  # Empty tenant between colons
    )
    assert response.status_code == 401
    data = await response.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_tenant_middleware_valid_token(client):
    """Test tenant middleware with valid Bearer token format."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": "Bearer sub:tenant-1:pro"},
    )
    # Should pass middleware and return 200 (empty list)
    assert response.status_code == 200
    data = await response.get_json()
    assert "operations" in data
    assert data["operations"] == []
