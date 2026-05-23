"""Tests for import/export handlers."""
import pytest
from datetime import datetime
from unittest.mock import AsyncMock, patch, MagicMock
from uuid import UUID

from app import create_app
from handlers.import_handler import (
    snapshot_data_resource,
    restore_data_resource,
    introspect_imported_resource,
    migrate_to_managed,
)
from models import DataResourceRecord
from store.store import MemoryStore


@pytest.fixture
async def shared_client():
    """Create a test client with a shared MemoryStore instance."""
    store = MemoryStore()
    app = create_app(store)

    async with app.test_client() as test_client:
        # Attach store to client so we can access it in tests
        test_client.store = store
        yield test_client


class TestSnapshotDataResource:
    """Tests for snapshot_data_resource handler."""

    @pytest.mark.asyncio
    async def test_snapshot_returns_202(self, client, bearer_token):
        """Test snapshot endpoint returns 202 Accepted."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/my-resource/snapshot",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        assert response.status_code == 202
        data = await response.get_json()
        assert data["status"] == "RUNNING"
        assert data["type"] == "snapshot"
        assert data["resource"] == "my-resource"
        assert data["tenant"] == "test-tenant"

    @pytest.mark.asyncio
    async def test_snapshot_operation_id_generated(self, client, bearer_token):
        """Test snapshot generates valid operationId UUID."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/test-dr/snapshot",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        data = await response.get_json()
        op_id = data["operationId"]

        # Verify it's a valid UUID
        try:
            UUID(op_id, version=4)
        except (ValueError, AttributeError):
            pytest.fail(f"operationId '{op_id}' is not a valid UUID")

    @pytest.mark.asyncio
    async def test_snapshot_has_location_header(self, client, bearer_token):
        """Test snapshot response includes Location header."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/my-dr/snapshot",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        assert "Location" in response.headers
        data = await response.get_json()
        op_id = data["operationId"]
        expected_location = f"/api/v1/tenants/test-tenant/operations/{op_id}"
        assert response.headers["Location"] == expected_location

    @pytest.mark.asyncio
    async def test_snapshot_has_started_at_timestamp(self, client, bearer_token):
        """Test snapshot includes startedAt timestamp."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/test/snapshot",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        data = await response.get_json()
        assert "startedAt" in data
        # Verify it's ISO format with Z
        assert data["startedAt"].endswith("Z")
        # Should be parseable as ISO datetime
        try:
            datetime.fromisoformat(data["startedAt"].replace("Z", "+00:00"))
        except ValueError:
            pytest.fail("startedAt is not valid ISO format")

    @pytest.mark.asyncio
    async def test_snapshot_requires_auth(self, client):
        """Test snapshot requires authorization."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/test/snapshot"
        )

        assert response.status_code == 401

    @pytest.mark.asyncio
    async def test_snapshot_multiple_calls_unique_ids(self, client, bearer_token):
        """Test multiple snapshot calls generate unique operationIds."""
        response1 = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr1/snapshot",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )
        response2 = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr2/snapshot",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        data1 = await response1.get_json()
        data2 = await response2.get_json()

        assert data1["operationId"] != data2["operationId"]


class TestRestoreDataResource:
    """Tests for restore_data_resource handler."""

    @pytest.mark.asyncio
    async def test_restore_returns_202(self, client, bearer_token):
        """Test restore endpoint returns 202 Accepted."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/my-resource/restore",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={"snapshotId": "snap-123"},
        )

        assert response.status_code == 202
        data = await response.get_json()
        assert data["status"] == "RUNNING"
        assert data["type"] == "restore"
        assert data["resource"] == "my-resource"
        assert data["tenant"] == "test-tenant"

    @pytest.mark.asyncio
    async def test_restore_with_snapshot_id(self, client, bearer_token):
        """Test restore with explicit snapshotId."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/restore",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={"snapshotId": "snap-12345"},
        )

        data = await response.get_json()
        # snapshotId is consumed but not echoed in response (stub behavior)
        assert data["type"] == "restore"

    @pytest.mark.asyncio
    async def test_restore_without_snapshot_id(self, client, bearer_token):
        """Test restore without snapshotId (empty request)."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/restore",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={},
        )

        assert response.status_code == 202
        data = await response.get_json()
        assert data["type"] == "restore"

    @pytest.mark.asyncio
    async def test_restore_with_mode_side_by_side(self, client, bearer_token):
        """Test restore with side-by-side mode."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/restore",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={"mode": "side-by-side"},
        )

        data = await response.get_json()
        assert data["mode"] == "side-by-side"

    @pytest.mark.asyncio
    async def test_restore_with_mode_replace(self, client, bearer_token):
        """Test restore with replace mode."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/restore",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={"mode": "replace"},
        )

        data = await response.get_json()
        assert data["mode"] == "replace"

    @pytest.mark.asyncio
    async def test_restore_empty_mode_defaults_to_side_by_side(
        self, client, bearer_token
    ):
        """Test restore with empty mode string defaults to side-by-side."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/restore",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={"mode": ""},
        )

        data = await response.get_json()
        assert data["mode"] == "side-by-side"

    @pytest.mark.asyncio
    async def test_restore_without_request_body(self, client, bearer_token):
        """Test restore with no JSON body."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/restore",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        assert response.status_code == 202
        data = await response.get_json()
        assert data["mode"] == "side-by-side"

    @pytest.mark.asyncio
    async def test_restore_has_location_header(self, client, bearer_token):
        """Test restore response includes Location header."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/restore",
            headers={"Authorization": f"Bearer {bearer_token}"},
            json={},
        )

        assert "Location" in response.headers
        data = await response.get_json()
        op_id = data["operationId"]
        expected_location = f"/api/v1/tenants/test-tenant/operations/{op_id}"
        assert response.headers["Location"] == expected_location

    @pytest.mark.asyncio
    async def test_restore_requires_auth(self, client):
        """Test restore requires authorization."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/restore"
        )

        assert response.status_code == 401


class TestIntrospectImportedResource:
    """Tests for introspect_imported_resource handler."""

    @pytest.mark.asyncio
    async def test_introspect_missing_resource_returns_404(
        self, shared_client, bearer_token
    ):
        """Test introspect returns 404 when DataResource not found."""
        response = await shared_client.post(
            "/api/v1/tenants/test-tenant/data-resources/nonexistent/introspect",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        assert response.status_code == 404
        data = await response.get_json()
        assert data["code"] == "nest.dataresource.not_found"
        assert data["message"] == "DataResource not found"

    @pytest.mark.asyncio
    async def test_introspect_no_connection_string_returns_400(
        self, shared_client, bearer_token
    ):
        """Test introspect returns 400 when no connection string configured."""
        # Create a DataResource without connection string
        dr = DataResourceRecord(
            id="dr-no-conn-id",
            name="dr-no-conn",
            tenant="test-tenant",
            resource_type="postgres",
            engine_type="postgres",
            storage_class="",
            driver_type="",
            origination="imported",
            phase="ready",
            created_at=datetime.utcnow().isoformat(),
            updated_at=datetime.utcnow().isoformat(),
            import_conn_str="",
            external_endpoint="",
        )
        await shared_client.store.create_data_resource(dr)

        response = await shared_client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr-no-conn/introspect",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        assert response.status_code == 400
        data = await response.get_json()
        assert data["code"] == "nest.dataresource.invalid"
        assert "No connection string or endpoint" in data["message"]

    @pytest.mark.asyncio
    async def test_introspect_with_tcp_error_and_empty_strings(
        self, shared_client, bearer_token
    ):
        """Test introspect returns 200 even when TCP fails."""
        # Create DR with string that extracts a host successfully
        dr = DataResourceRecord(
            id="dr-any-host-id",
            name="dr-any-host",
            tenant="test-tenant",
            resource_type="postgres",
            engine_type="postgres",
            storage_class="",
            driver_type="",
            origination="imported",
            phase="ready",
            created_at=datetime.utcnow().isoformat(),
            updated_at=datetime.utcnow().isoformat(),
            import_conn_str="somehost:5432",
            external_endpoint="",
        )
        await shared_client.store.create_data_resource(dr)

        with patch("handlers.import_handler.tcp_ping") as mock_ping:
            # Simulate a network error
            mock_ping.return_value = (False, 1500, "Name or service not known")

            response = await shared_client.post(
                "/api/v1/tenants/test-tenant/data-resources/dr-any-host/introspect",
                headers={"Authorization": f"Bearer {bearer_token}"},
            )

        assert response.status_code == 200
        data = await response.get_json()
        assert data["reachable"] is False
        assert "Name or service not known" in data["message"]

    @pytest.mark.asyncio
    async def test_introspect_successful_tcp_connection(
        self, shared_client, bearer_token
    ):
        """Test introspect with successful TCP connection."""
        # Create DR with valid postgresql URI
        dr = DataResourceRecord(
            id="dr-pg-id",
            name="dr-pg",
            tenant="test-tenant",
            resource_type="postgres",
            engine_type="postgres",
            storage_class="",
            driver_type="",
            origination="imported",
            phase="ready",
            created_at=datetime.utcnow().isoformat(),
            updated_at=datetime.utcnow().isoformat(),
            import_conn_str="postgresql://db.example.com:5432/mydb",
            external_endpoint="",
        )
        await shared_client.store.create_data_resource(dr)

        with patch("handlers.import_handler.tcp_ping") as mock_ping:
            mock_ping.return_value = (True, 42, "tcp connection successful")

            response = await shared_client.post(
                "/api/v1/tenants/test-tenant/data-resources/dr-pg/introspect",
                headers={"Authorization": f"Bearer {bearer_token}"},
            )

        assert response.status_code == 200
        data = await response.get_json()
        assert data["resource"] == "dr-pg"
        assert data["reachable"] is True
        assert data["latencyMs"] == 42
        assert data["message"] == "tcp connection successful"

    @pytest.mark.asyncio
    async def test_introspect_tcp_timeout(self, shared_client, bearer_token):
        """Test introspect when TCP connection times out."""
        dr = DataResourceRecord(
            id="dr-timeout-id",
            name="dr-timeout",
            tenant="test-tenant",
            resource_type="postgres",
            engine_type="postgres",
            storage_class="",
            driver_type="",
            origination="imported",
            phase="ready",
            created_at=datetime.utcnow().isoformat(),
            updated_at=datetime.utcnow().isoformat(),
            import_conn_str="postgresql://unreachable.example.com:5432/db",
            external_endpoint="",
        )
        await shared_client.store.create_data_resource(dr)

        with patch("handlers.import_handler.tcp_ping") as mock_ping:
            mock_ping.return_value = (False, 5000, "connection timeout after 5.0s")

            response = await shared_client.post(
                "/api/v1/tenants/test-tenant/data-resources/dr-timeout/introspect",
                headers={"Authorization": f"Bearer {bearer_token}"},
            )

        assert response.status_code == 200
        data = await response.get_json()
        assert data["reachable"] is False
        assert data["latencyMs"] == 5000

    @pytest.mark.asyncio
    async def test_introspect_tcp_refused(self, shared_client, bearer_token):
        """Test introspect when TCP connection is refused."""
        dr = DataResourceRecord(
            id="dr-refused-id",
            name="dr-refused",
            tenant="test-tenant",
            resource_type="postgres",
            engine_type="postgres",
            storage_class="",
            driver_type="",
            origination="imported",
            phase="ready",
            created_at=datetime.utcnow().isoformat(),
            updated_at=datetime.utcnow().isoformat(),
            import_conn_str="host:9999",
            external_endpoint="",
        )
        await shared_client.store.create_data_resource(dr)

        with patch("handlers.import_handler.tcp_ping") as mock_ping:
            mock_ping.return_value = (
                False,
                10,
                "Connection refused",
            )

            response = await shared_client.post(
                "/api/v1/tenants/test-tenant/data-resources/dr-refused/introspect",
                headers={"Authorization": f"Bearer {bearer_token}"},
            )

        assert response.status_code == 200
        data = await response.get_json()
        assert data["reachable"] is False

    @pytest.mark.asyncio
    async def test_introspect_uses_import_conn_str(
        self, shared_client, bearer_token
    ):
        """Test introspect prefers import_conn_str over external_endpoint."""
        dr = DataResourceRecord(
            id="dr-conn-preference-id",
            name="dr-conn-preference",
            tenant="test-tenant",
            resource_type="postgres",
            engine_type="postgres",
            storage_class="",
            driver_type="",
            origination="imported",
            phase="ready",
            created_at=datetime.utcnow().isoformat(),
            updated_at=datetime.utcnow().isoformat(),
            import_conn_str="postgresql://primary.db:5432/db",
            external_endpoint="postgresql://secondary.db:5432/db",
        )
        await shared_client.store.create_data_resource(dr)

        with patch("handlers.import_handler.tcp_ping") as mock_ping:
            mock_ping.return_value = (True, 10, "ok")

            response = await shared_client.post(
                "/api/v1/tenants/test-tenant/data-resources/dr-conn-preference/introspect",
                headers={"Authorization": f"Bearer {bearer_token}"},
            )

        # Verify tcp_ping was called with primary host
        mock_ping.assert_called_once()
        call_args = mock_ping.call_args[0]
        assert call_args[0] == "primary.db"
        assert call_args[1] == 5432

    @pytest.mark.asyncio
    async def test_introspect_uses_external_endpoint_fallback(
        self, shared_client, bearer_token
    ):
        """Test introspect uses external_endpoint when import_conn_str is None."""
        dr = DataResourceRecord(
            id="dr-endpoint-id",
            name="dr-endpoint",
            tenant="test-tenant",
            resource_type="postgres",
            engine_type="postgres",
            storage_class="",
            driver_type="",
            origination="imported",
            phase="ready",
            created_at=datetime.utcnow().isoformat(),
            updated_at=datetime.utcnow().isoformat(),
            import_conn_str="",
            external_endpoint="postgresql://fallback.db:5432/db",
        )
        await shared_client.store.create_data_resource(dr)

        with patch("handlers.import_handler.tcp_ping") as mock_ping:
            mock_ping.return_value = (True, 25, "ok")

            response = await shared_client.post(
                "/api/v1/tenants/test-tenant/data-resources/dr-endpoint/introspect",
                headers={"Authorization": f"Bearer {bearer_token}"},
            )

        mock_ping.assert_called_once()
        call_args = mock_ping.call_args[0]
        assert call_args[0] == "fallback.db"

    @pytest.mark.asyncio
    async def test_introspect_default_port(self, shared_client, bearer_token):
        """Test introspect uses default port when endpoint has no port."""
        dr = DataResourceRecord(
            id="dr-default-port-id",
            name="dr-default-port",
            tenant="test-tenant",
            resource_type="postgres",
            engine_type="postgres",
            storage_class="",
            driver_type="",
            origination="imported",
            phase="ready",
            created_at=datetime.utcnow().isoformat(),
            updated_at=datetime.utcnow().isoformat(),
            import_conn_str="myhost",
            external_endpoint="",
        )
        await shared_client.store.create_data_resource(dr)

        with patch("handlers.import_handler.tcp_ping") as mock_ping:
            mock_ping.return_value = (True, 5, "ok")

            response = await shared_client.post(
                "/api/v1/tenants/test-tenant/data-resources/dr-default-port/introspect",
                headers={"Authorization": f"Bearer {bearer_token}"},
            )

        mock_ping.assert_called_once()
        call_args = mock_ping.call_args[0]
        assert call_args[1] == 5432  # Default PostgreSQL port

    @pytest.mark.asyncio
    async def test_introspect_requires_auth(self, client):
        """Test introspect requires authorization."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/introspect"
        )

        assert response.status_code == 401


class TestMigrateToManaged:
    """Tests for migrate_to_managed handler."""

    @pytest.mark.asyncio
    async def test_migrate_returns_202(self, client, bearer_token):
        """Test migrate endpoint returns 202 Accepted."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/my-resource/migrate",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        assert response.status_code == 202
        data = await response.get_json()
        assert data["status"] == "RUNNING"
        assert data["type"] == "migrate"
        assert data["resource"] == "my-resource"
        assert data["tenant"] == "test-tenant"

    @pytest.mark.asyncio
    async def test_migrate_operation_id_generated(self, client, bearer_token):
        """Test migrate generates valid operationId UUID."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/migrate",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        data = await response.get_json()
        op_id = data["operationId"]

        try:
            UUID(op_id, version=4)
        except (ValueError, AttributeError):
            pytest.fail(f"operationId '{op_id}' is not a valid UUID")

    @pytest.mark.asyncio
    async def test_migrate_has_location_header(self, client, bearer_token):
        """Test migrate response includes Location header."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/migrate",
            headers={"Authorization": f"Bearer {bearer_token}"},
        )

        assert "Location" in response.headers
        data = await response.get_json()
        op_id = data["operationId"]
        expected_location = f"/api/v1/tenants/test-tenant/operations/{op_id}"
        assert response.headers["Location"] == expected_location

    @pytest.mark.asyncio
    async def test_migrate_requires_auth(self, client):
        """Test migrate requires authorization."""
        response = await client.post(
            "/api/v1/tenants/test-tenant/data-resources/dr/migrate"
        )

        assert response.status_code == 401
