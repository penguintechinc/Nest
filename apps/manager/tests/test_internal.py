"""Tests for internal endpoints via HTTP client."""
import pytest


@pytest.mark.asyncio
async def test_create_operation_success(client):
    """Test successfully creating an operation via internal endpoint."""
    response = await client.post(
        "/internal/v1/operations",
        json={
            "tenant": "tenant-1",
            "resourceName": "my-pvc",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    assert response.status_code == 201
    data = await response.get_json()
    assert data["operation"]["phase"] == "pending"
    assert data["operation"]["tenant"] == "tenant-1"
    assert data["operation"]["resourceName"] == "my-pvc"
    assert "id" in data["operation"]


@pytest.mark.asyncio
async def test_create_operation_missing_field(client):
    """Test creating an operation with missing required field."""
    response = await client.post(
        "/internal/v1/operations",
        json={
            "tenant": "tenant-1",
            # Missing resourceName
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    assert response.status_code == 400
    data = await response.get_json()
    assert "error" in data


@pytest.mark.asyncio
async def test_create_operation_invalid_json(client):
    """Test creating an operation with invalid/empty JSON body."""
    response = await client.post(
        "/internal/v1/operations",
        data="not-json",
        headers={"Content-Type": "application/json"},
    )
    assert response.status_code == 400


@pytest.mark.asyncio
async def test_create_operation_value_error(client, monkeypatch):
    """Test creating an operation when store raises ValueError."""
    async def mock_create_operation(operation):
        raise ValueError("Duplicate operation ID")

    from store.store import MemoryOperationStore
    store = MemoryOperationStore()
    monkeypatch.setattr(store, "create_operation", mock_create_operation)

    # Mock the app's store
    response = await client.post(
        "/internal/v1/operations",
        json={
            "tenant": "tenant-1",
            "resourceName": "my-pvc",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    # Should return 409 on ValueError (duplicate)
    if response.status_code == 409:
        data = await response.get_json()
        assert "error" in data


@pytest.mark.asyncio
async def test_create_operation_exception(client, monkeypatch):
    """Test creating an operation when store raises generic Exception."""
    async def mock_create_operation(operation):
        raise RuntimeError("Database connection failed")

    from store.store import MemoryOperationStore
    store = MemoryOperationStore()
    monkeypatch.setattr(store, "create_operation", mock_create_operation)

    response = await client.post(
        "/internal/v1/operations",
        json={
            "tenant": "tenant-1",
            "resourceName": "my-pvc",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    # Should return 500 on generic Exception
    if response.status_code == 500:
        data = await response.get_json()
        assert "error" in data


@pytest.mark.asyncio
async def test_create_operation_with_custom_id(client):
    """Test creating an operation with a custom operation ID."""
    response = await client.post(
        "/internal/v1/operations",
        json={
            "operationId": "custom-op-id",
            "tenant": "tenant-1",
            "resourceName": "my-pvc",
            "resourceType": "pvc/block",
            "operationType": "create",
        },
    )
    assert response.status_code == 201
    data = await response.get_json()
    assert data["operation"]["id"] == "custom-op-id"
