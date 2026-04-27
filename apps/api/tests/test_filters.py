"""Tests for API query parameter filters and pagination."""
import pytest


@pytest.mark.asyncio
async def test_list_data_resources_pagination_limit(client, bearer_token):
    """Test pagination: ?limit=1 returns 1 item."""
    # Create 3 data resources
    for i in range(3):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"dr-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?limit=1",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["items"]) == 1


@pytest.mark.asyncio
async def test_list_data_resources_pagination_limit_2(client, bearer_token):
    """Test pagination: ?limit=2 returns 2 items."""
    # Create 3 data resources
    for i in range(3):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"dr-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?limit=2",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["items"]) == 2


@pytest.mark.asyncio
async def test_list_data_resources_pagination_offset(client, bearer_token):
    """Test pagination: ?offset=1 skips first item."""
    # Create 3 data resources
    for i in range(3):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"dr-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )

    # Get all first
    response_all = await client.get(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    data_all = await response_all.get_json()

    # Get with offset=1
    response_offset = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?offset=1",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response_offset.status_code == 200
    data_offset = await response_offset.get_json()
    assert len(data_offset["items"]) == 2

    # First item from all should not be in offset results
    first_name_all = data_all["items"][0]["name"]
    offset_names = [dr["name"] for dr in data_offset["items"]]
    assert first_name_all not in offset_names


@pytest.mark.asyncio
async def test_list_data_resources_filter_by_type(client, bearer_token):
    """Test type filter: ?type=pvc/block returns only pvc/block resources."""
    # Create 2 pvc/block and 1 pvc/filesystem
    for i in range(2):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"block-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )

    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "fs-1",
            "type": "pvc/filesystem",
            "class": "nest-fs",
        },
    )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?type=pvc/block",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["items"]) == 2
    for item in data["items"]:
        assert item["resourceType"] == "pvc/block"


@pytest.mark.asyncio
async def test_list_data_resources_filter_by_origination(client, bearer_token):
    """Test origination filter: ?origination=managed returns only managed resources."""
    # Create managed resource
    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "managed-dr",
            "type": "pvc/block",
            "class": "nest-block",
            "origination": "managed",
        },
    )

    # Create imported resource
    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "imported-dr",
            "type": "postgres",
            "class": "",
            "origination": "imported",
            "connectionString": "postgres://user:pass@host:5432/db",
        },
    )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?origination=managed",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["items"]) == 1
    assert data["items"][0]["name"] == "managed-dr"


@pytest.mark.asyncio
async def test_list_data_resources_combined_filters(client, bearer_token):
    """Test combined filters: ?type=pvc/block&origination=managed."""
    # Create various combinations
    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "managed-block",
            "type": "pvc/block",
            "class": "nest-block",
            "origination": "managed",
        },
    )

    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "imported-block",
            "type": "pvc/block",
            "class": "",
            "origination": "imported",
            "connectionString": "postgres://test",
        },
    )

    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "managed-fs",
            "type": "pvc/filesystem",
            "class": "nest-fs",
            "origination": "managed",
        },
    )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?type=pvc/block&origination=managed",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["items"]) == 1
    assert data["items"][0]["name"] == "managed-block"


@pytest.mark.asyncio
async def test_list_data_resources_empty_filter_result(client, bearer_token):
    """Test empty result: filter that matches nothing returns empty list."""
    # Create a resource
    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "dr-1",
            "type": "pvc/block",
            "class": "nest-block",
        },
    )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?type=postgres",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert data["items"] == []


@pytest.mark.asyncio
async def test_list_data_resources_invalid_filter_value(client, bearer_token):
    """Test invalid filter value: returns empty or 400."""
    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "dr-1",
            "type": "pvc/block",
            "class": "nest-block",
        },
    )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?type=invalid/type",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    # Should either return empty or 400
    assert response.status_code in [200, 400]
    if response.status_code == 200:
        data = await response.get_json()
        assert data["items"] == []


@pytest.mark.asyncio
async def test_list_data_resources_large_offset(client, bearer_token):
    """Test pagination: offset > total returns empty list."""
    # Create 3 resources
    for i in range(3):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"dr-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?offset=100",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert data["items"] == []


@pytest.mark.asyncio
async def test_list_data_resources_limit_zero(client, bearer_token):
    """Test pagination boundary: ?limit=0 behavior."""
    await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "dr-1",
            "type": "pvc/block",
            "class": "nest-block",
        },
    )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?limit=0",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    # Should either ignore limit=0, treat as no limit, or return empty
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_list_data_resources_limit_very_large(client, bearer_token):
    """Test pagination boundary: ?limit=1000 with fewer items."""
    # Create 2 resources
    for i in range(2):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"dr-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?limit=1000",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["items"]) == 2


@pytest.mark.asyncio
async def test_list_with_multiple_resources(client, bearer_token):
    """Test listing: create 3 resources, list returns all 3."""
    # Create 3 resources
    for i in range(3):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"dr-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["items"]) == 3
    names = {dr["name"] for dr in data["items"]}
    assert names == {"dr-0", "dr-1", "dr-2"}


@pytest.mark.asyncio
async def test_data_resource_name_uniqueness(client, bearer_token):
    """Test name uniqueness: two creates with different names both succeed."""
    response1 = await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "unique-dr-1",
            "type": "pvc/block",
            "class": "nest-block",
        },
    )
    assert response1.status_code == 202
    data1 = await response1.get_json()
    name1 = data1["name"]

    response2 = await client.post(
        "/api/v1/tenants/test-tenant/data-resources",
        headers={"Authorization": f"Bearer {bearer_token}"},
        json={
            "name": "unique-dr-2",
            "type": "pvc/block",
            "class": "nest-block",
        },
    )
    assert response2.status_code == 202
    data2 = await response2.get_json()
    name2 = data2["name"]

    # Names should be different
    assert name1 != name2
    assert name1 and name2


@pytest.mark.asyncio
async def test_filter_pagination_combined(client, bearer_token):
    """Test filter + pagination together: ?type=pvc/block&limit=1."""
    # Create 3 pvc/block and 2 pvc/filesystem
    for i in range(3):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"block-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )

    for i in range(2):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"fs-{i}",
                "type": "pvc/filesystem",
                "class": "nest-fs",
            },
        )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?type=pvc/block&limit=1",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    # Should return 1 filtered item (block type only)
    assert len(data["items"]) == 1
    assert data["items"][0]["resourceType"] == "pvc/block"


@pytest.mark.asyncio
async def test_filter_pagination_offset_combined(client, bearer_token):
    """Test filter + pagination + offset: ?type=pvc/block&limit=1&offset=1."""
    # Create 3 pvc/block resources
    for i in range(3):
        await client.post(
            "/api/v1/tenants/test-tenant/data-resources",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={
                "name": f"block-{i}",
                "type": "pvc/block",
                "class": "nest-block",
            },
        )

    response = await client.get(
        "/api/v1/tenants/test-tenant/data-resources?type=pvc/block&limit=1&offset=1",
        headers={"Authorization": f"Bearer {bearer_token}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    # Should return 1 item (2nd item of filtered results)
    assert len(data["items"]) == 1
    assert data["items"][0]["resourceType"] == "pvc/block"
