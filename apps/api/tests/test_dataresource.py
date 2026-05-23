"""Tests for DataResource CRUD handlers."""
import pytest


@pytest.mark.asyncio
async def test_list_empty(client, bearer_token):
    """Test listing DataResources when empty."""
    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert data["items"] == []
    assert data["meta"]["count"] == 0


@pytest.mark.asyncio
async def test_create_data_resource(client, bearer_token):
    """Test creating a DataResource."""
    response = await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "my-block-storage",
            "type": "pvc/block",
            "class": "nest-block",
            "origination": "managed",
            "sizeGi": 100,
        },
    )
    assert response.status_code == 202
    data = await response.get_json()
    assert data["name"] == "my-block-storage"
    assert data["resourceType"] == "pvc/block"
    assert "operationId" in data


@pytest.mark.asyncio
async def test_create_missing_fields(client, bearer_token):
    """Test creating DataResource with missing required fields."""
    response = await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={"name": "incomplete"},
    )
    assert response.status_code == 400
    data = await response.get_json()
    assert data["code"] == "nest.dataresource.invalid"


@pytest.mark.asyncio
async def test_create_invalid_type(client, bearer_token):
    """Test creating DataResource with invalid type."""
    response = await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "invalid-type",
            "type": "invalid/type",
            "class": "some-class",
        },
    )
    assert response.status_code == 400
    data = await response.get_json()
    assert data["code"] == "nest.dataresource.invalid_type"


@pytest.mark.asyncio
async def test_create_imported_without_conn_str(client, bearer_token):
    """Test creating imported DataResource without connection string."""
    response = await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "imported-dr",
            "type": "postgres",
            "class": "",
            "origination": "imported",
        },
    )
    assert response.status_code == 400
    data = await response.get_json()
    assert data["code"] == "nest.dataresource.invalid"
    assert "connectionString" in data["message"]


@pytest.mark.asyncio
async def test_create_external_without_provider(client, bearer_token):
    """Test creating external DataResource without provider."""
    response = await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "external-dr",
            "type": "object",
            "class": "",
            "origination": "external",
        },
    )
    assert response.status_code == 400
    data = await response.get_json()
    assert data["code"] == "nest.dataresource.invalid"
    assert "provider" in data["message"]


@pytest.mark.asyncio
async def test_create_external_invalid_provider(client, bearer_token):
    """Test creating external DataResource with invalid provider."""
    response = await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "external-dr",
            "type": "object",
            "class": "",
            "origination": "external",
            "external": {"provider": "invalid-provider"},
        },
    )
    assert response.status_code == 400
    data = await response.get_json()
    assert data["code"] == "nest.dataresource.invalid"


@pytest.mark.asyncio
async def test_free_tier_limit(client, free_tier_token):
    """Test free tier limit (5 DataResources)."""
    # Create 5 DataResources
    for i in range(5):
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {free_tier_token}"},
            json={
                "name": f"dr-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )
        assert response.status_code == 202

    # 6th should fail
    response = await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {free_tier_token}"},
        json={
            "name": "dr-6",
            "type": "pvc/block",
            "class": "nest-block",
        },
    )
    assert response.status_code == 402
    data = await response.get_json()
    assert data["code"] == "nest.quota.data_resource_limit"


@pytest.mark.asyncio
async def test_get_data_resource(client, bearer_token):
    """Test getting a single DataResource."""
    # Create one first
    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "my-storage",
            "type": "pvc/block",
            "class": "nest-block",
        },
    )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources/my-storage",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert data["name"] == "my-storage"
    assert data["resourceType"] == "pvc/block"


@pytest.mark.asyncio
async def test_get_nonexistent(client, bearer_token):
    """Test getting a nonexistent DataResource."""
    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources/nonexistent",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 404
    data = await response.get_json()
    assert data["code"] == "nest.dataresource.not_found"


@pytest.mark.asyncio
async def test_delete_data_resource(client, bearer_token):
    """Test deleting a DataResource."""
    # Create one first
    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "to-delete",
            "type": "pvc/block",
            "class": "nest-block",
        },
    )

    response = await client.delete(
        "/api/v1/tenants/test-tenant/data-resources/to-delete",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 204

    # Verify it's gone
    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources/to-delete",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 404


@pytest.mark.asyncio
async def test_delete_nonexistent(client, bearer_token):
    """Test deleting a nonexistent DataResource."""
    response = await client.delete(
        "/api/v1/tenants/test-tenant/data-resources/nonexistent",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 404
