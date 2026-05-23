"""Tests for API query parameter filters and pagination."""
import pytest

BEARER = "sub:tenant-1:pro"
OTHER_BEARER = "sub:tenant-2:pro"


@pytest.mark.asyncio
async def test_list_operations_pagination_limit(client):
    """Test pagination: ?limit=1 returns 1 item."""
    # Create 3 operations
    for i in range(3):
        await client.post(
            "/internal/v1/operations",
            json={
                "operationId": f"op-{i}",
                "tenant": "tenant-1",
                "resourceName": f"pvc-{i}",
                "resourceType": "pvc/block",
                "operationType": "create",
            },
        )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?limit=1",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 1


@pytest.mark.asyncio
async def test_list_operations_pagination_limit_2(client):
    """Test pagination: ?limit=2 returns 2 items."""
    # Create 3 operations
    for i in range(3):
        await client.post(
            "/internal/v1/operations",
            json={
                "operationId": f"op-{i}",
                "tenant": "tenant-1",
                "resourceName": f"pvc-{i}",
                "resourceType": "pvc/block",
                "operationType": "create",
            },
        )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?limit=2",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 2


@pytest.mark.asyncio
async def test_list_operations_pagination_offset(client):
    """Test pagination: ?offset=1 skips first item."""
    # Create 3 operations
    for i in range(3):
        await client.post(
            "/internal/v1/operations",
            json={
                "operationId": f"op-{i}",
                "tenant": "tenant-1",
                "resourceName": f"pvc-{i}",
                "resourceType": "pvc/block",
                "operationType": "create",
            },
        )

    # Get all first
    response_all = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    data_all = await response_all.get_json()

    # Get with offset=1
    response_offset = await client.get(
        "/api/v1/tenants/tenant-1/operations?offset=1",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response_offset.status_code == 200
    data_offset = await response_offset.get_json()
    assert len(data_offset["operations"]) == 2

    # First op from all should not be in offset results
    first_id_all = data_all["operations"][0]["id"]
    offset_ids = [op["id"] for op in data_offset["operations"]]
    assert first_id_all not in offset_ids


@pytest.mark.asyncio
async def test_list_operations_filter_by_phase(client):
    """Test status filter: ?phase=pending returns only pending ops."""
    # Create 2 pending ops and 1 completed
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-pending-1",
            "tenant": "tenant-1",
            "resourceName": "pvc-1",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-pending-2",
            "tenant": "tenant-1",
            "resourceName": "pvc-2",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    # Create a completed operation
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-completed",
            "tenant": "tenant-1",
            "resourceName": "pvc-3",
            "resourceType": "pvc/block",
            "operationType": "create",
            "phase": "completed",
        },
    )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?phase=pending",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 2
    for op in data["operations"]:
        assert op["phase"] == "pending"


@pytest.mark.asyncio
async def test_list_operations_filter_by_operation_type(client):
    """Test type filter: ?operationType=create returns only create ops."""
    # Create 2 create ops and 1 delete
    for i in range(2):
        await client.post(
            "/internal/v1/operations",
            json={
                "operationId": f"op-create-{i}",
                "tenant": "tenant-1",
                "resourceName": f"pvc-{i}",
                "resourceType": "pvc/block",
                "operationType": "create",
            },
        )

    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-delete",
            "tenant": "tenant-1",
            "resourceName": "pvc-delete",
            "resourceType": "pvc/block",
            "operationType": "delete",
        },
    )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?operationType=create",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 2
    for op in data["operations"]:
        assert op["operationType"] == "create"


@pytest.mark.asyncio
async def test_list_operations_filter_by_resource_type(client):
    """Test resource type filter: ?resourceType=pvc/block filters correctly."""
    # Create 2 pvc/block ops and 1 pvc/filesystem
    for i in range(2):
        await client.post(
            "/internal/v1/operations",
            json={
                "operationId": f"op-block-{i}",
                "tenant": "tenant-1",
                "resourceName": f"block-{i}",
                "resourceType": "pvc/block",
                "operationType": "create",
            },
        )

    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-fs",
            "tenant": "tenant-1",
            "resourceName": "fs-1",
            "resourceType": "pvc/filesystem",
            "operationType": "create",
        },
    )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?resourceType=pvc/block",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 2
    for op in data["operations"]:
        assert op["resourceType"] == "pvc/block"


@pytest.mark.asyncio
async def test_list_operations_combined_filters(client):
    """Test combined filters: ?phase=pending&operationType=create."""
    # Create mix of ops
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-1",
            "tenant": "tenant-1",
            "resourceName": "pvc-1",
            "resourceType": "pvc/block",
            "operationType": "create",
            "phase": "pending",
        },
    )
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-2",
            "tenant": "tenant-1",
            "resourceName": "pvc-2",
            "resourceType": "pvc/block",
            "operationType": "delete",
            "phase": "pending",
        },
    )
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-3",
            "tenant": "tenant-1",
            "resourceName": "pvc-3",
            "resourceType": "pvc/block",
            "operationType": "create",
            "phase": "completed",
        },
    )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?phase=pending&operationType=create",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 1
    assert data["operations"][0]["id"] == "op-1"
    assert data["operations"][0]["phase"] == "pending"
    assert data["operations"][0]["operationType"] == "create"


@pytest.mark.asyncio
async def test_list_operations_empty_filter_result(client):
    """Test empty result: filter that matches nothing returns empty list."""
    # Create ops
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-1",
            "tenant": "tenant-1",
            "resourceName": "pvc-1",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?operationType=delete",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert data["operations"] == []


@pytest.mark.asyncio
async def test_list_operations_invalid_phase_filter(client):
    """Test invalid filter value: invalid phase returns empty or 400."""
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-1",
            "tenant": "tenant-1",
            "resourceName": "pvc-1",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?phase=invalid-phase",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    # Should either return empty list or 400
    assert response.status_code in [200, 400]
    if response.status_code == 200:
        data = await response.get_json()
        assert data["operations"] == []


@pytest.mark.asyncio
async def test_list_operations_large_offset(client):
    """Test pagination: offset > total returns empty list."""
    # Create 3 operations
    for i in range(3):
        await client.post(
            "/internal/v1/operations",
            json={
                "operationId": f"op-{i}",
                "tenant": "tenant-1",
                "resourceName": f"pvc-{i}",
                "resourceType": "pvc/block",
                "operationType": "create",
            },
        )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?offset=100",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert data["operations"] == []


@pytest.mark.asyncio
async def test_get_operation_tenant_isolation(client):
    """Test tenant isolation: tenant-2 cannot get tenant-1's specific operation."""
    # Create op for tenant-1
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-tenant-1",
            "tenant": "tenant-1",
            "resourceName": "pvc-1",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )

    # tenant-2 tries to get it
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations/op-tenant-1",
        headers={"Authorization": f"Bearer {OTHER_BEARER}"},
    )
    assert response.status_code == 403


@pytest.mark.asyncio
async def test_list_with_multiple_ops(client):
    """Test listing: create 3 ops for same tenant, list returns all 3."""
    # Create 3 operations
    for i in range(3):
        await client.post(
            "/internal/v1/operations",
            json={
                "operationId": f"op-{i}",
                "tenant": "tenant-1",
                "resourceName": f"pvc-{i}",
                "resourceType": "pvc/block",
                "operationType": "create",
            },
        )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 3
    ids = {op["id"] for op in data["operations"]}
    assert ids == {"op-0", "op-1", "op-2"}


@pytest.mark.asyncio
async def test_operation_id_uniqueness(client):
    """Test operation ID uniqueness: two creates without explicit ID get different IDs."""
    # Create 2 operations without explicit IDs
    response1 = await client.post(
        "/internal/v1/operations",
        json={
            "tenant": "tenant-1",
            "resourceName": "pvc-1",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    assert response1.status_code == 201
    data1 = await response1.get_json()
    id1 = data1["operation"]["id"]

    response2 = await client.post(
        "/internal/v1/operations",
        json={
            "tenant": "tenant-1",
            "resourceName": "pvc-2",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    assert response2.status_code == 201
    data2 = await response2.get_json()
    id2 = data2["operation"]["id"]

    # IDs should be different
    assert id1 != id2
    assert id1 and id2  # Both should be non-empty


@pytest.mark.asyncio
async def test_limit_zero(client):
    """Test pagination boundary: ?limit=0 behavior."""
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-1",
            "tenant": "tenant-1",
            "resourceName": "pvc-1",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?limit=0",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    # Should either ignore limit=0, treat as no limit, or return empty
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_limit_very_large(client):
    """Test pagination boundary: ?limit=1000 with fewer items."""
    # Create 2 operations
    for i in range(2):
        await client.post(
            "/internal/v1/operations",
            json={
                "operationId": f"op-{i}",
                "tenant": "tenant-1",
                "resourceName": f"pvc-{i}",
                "resourceType": "pvc/block",
                "operationType": "create",
            },
        )

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?limit=1000",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 2
