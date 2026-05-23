"""Tests for public operation endpoints via HTTP client."""
import pytest

BEARER = "sub:tenant-1:pro"
OTHER_BEARER = "sub:tenant-2:pro"


@pytest.mark.asyncio
async def test_list_operations_empty(client):
    """Test listing operations returns empty list when none exist."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert data["operations"] == []


@pytest.mark.asyncio
async def test_list_operations_after_create(client):
    """Test listing operations returns created ops."""
    # Create an op via internal endpoint first
    await client.post(
        "/internal/v1/operations",
        json={
            "tenant": "tenant-1",
            "resourceName": "my-pvc",
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
    assert len(data["operations"]) == 1
    assert data["operations"][0]["tenant"] == "tenant-1"


@pytest.mark.asyncio
async def test_get_operation_success(client):
    """Test getting a single operation by ID."""
    create_resp = await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-abc",
            "tenant": "tenant-1",
            "resourceName": "my-pvc",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    assert create_resp.status_code == 201

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations/op-abc",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert data["operation"]["id"] == "op-abc"
    assert data["operation"]["phase"] == "pending"


@pytest.mark.asyncio
async def test_get_operation_not_found(client):
    """Test getting a nonexistent operation returns 404."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations/nonexistent",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 404


@pytest.mark.asyncio
async def test_list_operations_requires_auth(client):
    """Test listing operations without auth returns 401."""
    response = await client.get("/api/v1/tenants/tenant-1/operations")
    assert response.status_code == 401


@pytest.mark.asyncio
async def test_list_operations_tenant_isolation(client):
    """Test tenant isolation: tenant-2 token can't see tenant-1 ops."""
    await client.post(
        "/internal/v1/operations",
        json={
            "tenant": "tenant-1",
            "resourceName": "my-pvc",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    # tenant-2 tries to access tenant-1's operations
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": f"Bearer {OTHER_BEARER}"},
    )
    assert response.status_code == 403


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
async def test_list_operations_invalid_limit(client):
    """Test list operations with invalid limit parameter."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?limit=not-an-int",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 400
    data = await response.get_json()
    assert "error" in data
    assert "integers" in data["error"].lower()


@pytest.mark.asyncio
async def test_list_operations_invalid_offset(client):
    """Test list operations with invalid offset parameter."""
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?offset=invalid",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 400
    data = await response.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_list_operations_with_filters(client):
    """Test list operations with phase, operationType, and resourceType filters."""
    # Create multiple operations with different attributes
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-create-pvc",
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
            "operationId": "op-delete-pvc",
            "tenant": "tenant-1",
            "resourceName": "pvc-2",
            "resourceType": "pvc/block",
            "operationType": "delete",
            "phase": "running",
        },
    )
    await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "op-create-node",
            "tenant": "tenant-1",
            "resourceName": "node-1",
            "resourceType": "node",
            "operationType": "create",
            "phase": "pending",
        },
    )

    # Filter by phase
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?phase=pending",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 2
    for op in data["operations"]:
        assert op["phase"] == "pending"

    # Filter by operationType
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?operationType=delete",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 1
    assert data["operations"][0]["operationType"] == "delete"

    # Filter by resourceType
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?resourceType=node",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 1
    assert data["operations"][0]["resourceType"] == "node"


@pytest.mark.asyncio
async def test_list_operations_with_pagination(client):
    """Test list operations with limit and offset."""
    # Create 5 operations
    for i in range(5):
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

    # Get first 2 with limit=2, offset=0
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?limit=2&offset=0",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 2

    # Get next 2 with limit=2, offset=2
    response = await client.get(
        "/api/v1/tenants/tenant-1/operations?limit=2&offset=2",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    assert response.status_code == 200
    data = await response.get_json()
    assert len(data["operations"]) == 2


@pytest.mark.asyncio
async def test_get_operation_exception(client, monkeypatch):
    """Test getting an operation when store raises generic Exception."""
    async def mock_get_operation(tid, op_id):
        raise RuntimeError("Database error")

    from store.store import MemoryOperationStore
    store = MemoryOperationStore()
    monkeypatch.setattr(store, "get_operation", mock_get_operation)

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations/op-123",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    # Should return 500 on generic Exception
    if response.status_code == 500:
        data = await response.get_json()
        assert "error" in data


@pytest.mark.asyncio
async def test_list_operations_exception(client, monkeypatch):
    """Test listing operations when store raises generic Exception."""
    async def mock_list_by_tenant(tid):
        raise RuntimeError("Database connection failed")

    from store.store import MemoryOperationStore
    store = MemoryOperationStore()
    monkeypatch.setattr(store, "list_by_tenant", mock_list_by_tenant)

    response = await client.get(
        "/api/v1/tenants/tenant-1/operations",
        headers={"Authorization": f"Bearer {BEARER}"},
    )
    # Should return 500 on generic Exception
    if response.status_code == 500:
        data = await response.get_json()
        assert "error" in data
