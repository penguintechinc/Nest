"""Error injection tests for SQLOperationStore exception handlers."""

from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from models.operations import OperationRecord
from store.sql_store import SQLOperationStore


class TestSQLOperationStoreErrorHandling:
    """Test suite for SQLOperationStore error handling paths."""

    async def test_create_operation_database_error(self) -> None:
        """Test create_operation wraps database exceptions as ValueError."""
        mock_db = AsyncMock()
        mock_db.operations = MagicMock()
        mock_db.operations.async_insert = AsyncMock(
            side_effect=RuntimeError("Database connection failed")
        )
        mock_db.side_effect = RuntimeError("Query failed")

        store = SQLOperationStore(mock_db)
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-001",
            tenant="tenant-a",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="pending",
            message="Starting backup",
            created_at=now,
            updated_at=now,
        )

        with pytest.raises(ValueError, match="Failed to create operation"):
            await store.create_operation(operation)

    async def test_get_operation_database_error(self) -> None:
        """Test get_operation wraps database exceptions as ValueError."""
        mock_db = AsyncMock()
        mock_query_set = AsyncMock()
        mock_query_set.select = AsyncMock(
            side_effect=RuntimeError("Connection reset by peer")
        )
        mock_db.return_value = mock_query_set

        store = SQLOperationStore(mock_db)

        with pytest.raises(ValueError, match="Failed to get operation"):
            await store.get_operation("tenant-a", "op-001")

    async def test_list_by_tenant_database_error(self) -> None:
        """Test list_by_tenant wraps database exceptions as ValueError."""
        mock_db = AsyncMock()
        mock_query_set = AsyncMock()
        mock_query_set.select = AsyncMock(
            side_effect=TimeoutError("Query timeout")
        )
        mock_db.return_value = mock_query_set

        store = SQLOperationStore(mock_db)

        with pytest.raises(ValueError, match="Failed to list operations for tenant"):
            await store.list_by_tenant("tenant-a")

    async def test_update_operation_database_error(self) -> None:
        """Test update_operation wraps database exceptions as ValueError."""
        mock_db = AsyncMock()
        mock_query_set = AsyncMock()
        # First call (exists check) succeeds
        mock_query_set.exists = AsyncMock(return_value=True)
        # Second call (update) fails
        mock_query_set.update = AsyncMock(
            side_effect=RuntimeError("Constraint violation")
        )
        mock_db.return_value = mock_query_set

        store = SQLOperationStore(mock_db)
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-001",
            tenant="tenant-a",
            resource_name="resource-1",
            resource_type="database",
            operation_type="backup",
            phase="running",
            message="In progress",
            created_at=now,
            updated_at=now,
        )

        with pytest.raises(ValueError, match="Failed to update operation"):
            await store.update_operation(operation)

    async def test_list_pending_or_running_database_error(self) -> None:
        """Test list_pending_or_running wraps database exceptions as ValueError."""
        mock_db = AsyncMock()
        mock_query_set = AsyncMock()
        mock_query_set.select = AsyncMock(
            side_effect=RuntimeError("Database connection lost")
        )
        mock_db.return_value = mock_query_set

        store = SQLOperationStore(mock_db)

        with pytest.raises(ValueError, match="Failed to list pending/running"):
            await store.list_pending_or_running()

    async def test_create_operation_exists_check_error(self) -> None:
        """Test create_operation when exists() check fails."""
        mock_db = AsyncMock()
        mock_query_set = AsyncMock()
        mock_query_set.exists = AsyncMock(
            side_effect=RuntimeError("Check failed")
        )
        mock_db.return_value = mock_query_set

        store = SQLOperationStore(mock_db)
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-err",
            tenant="tenant-err",
            resource_name="res",
            resource_type="db",
            operation_type="bak",
            phase="pending",
            message="msg",
            created_at=now,
            updated_at=now,
        )

        with pytest.raises(ValueError, match="Failed to create operation"):
            await store.create_operation(operation)

    async def test_update_operation_exists_check_error(self) -> None:
        """Test update_operation when exists() check fails."""
        mock_db = AsyncMock()
        mock_query_set = AsyncMock()
        mock_query_set.exists = AsyncMock(
            side_effect=RuntimeError("Existence check failed")
        )
        mock_db.return_value = mock_query_set

        store = SQLOperationStore(mock_db)
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-update-err",
            tenant="tenant-update-err",
            resource_name="res",
            resource_type="db",
            operation_type="upd",
            phase="running",
            message="updating",
            created_at=now,
            updated_at=now,
        )

        with pytest.raises(ValueError, match="Failed to update operation"):
            await store.update_operation(operation)

    async def test_update_operation_row_count_mismatch(self) -> None:
        """Test update_operation when update affects wrong number of rows.

        This tests the defensive check for database inconsistency where
        exists() returns True but update() affects 0 rows (race condition).
        """
        mock_db = MagicMock()
        mock_query_obj = MagicMock()

        # Set up the chain: db(...) returns a query object with exists() and update()
        mock_db.return_value = mock_query_obj
        # Also set db.operations for the attribute access
        mock_db.operations = MagicMock()

        # First call: exists() returns True
        # Second call: update() returns 0 (indicating race condition)
        mock_query_obj.exists = AsyncMock(return_value=True)
        mock_query_obj.update = AsyncMock(return_value=0)

        store = SQLOperationStore(mock_db)
        now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        operation = OperationRecord(
            id="op-race",
            tenant="tenant-race",
            resource_name="res",
            resource_type="db",
            operation_type="upd",
            phase="running",
            message="updating",
            created_at=now,
            updated_at=now,
        )

        with pytest.raises(ValueError, match="Update affected 0 rows, expected 1"):
            await store.update_operation(operation)

    async def test_parse_iso_datetime_valid_formats(self) -> None:
        """Test _parse_iso_datetime handles both with/without Z suffix."""
        # With Z suffix
        dt1 = SQLOperationStore._parse_iso_datetime("2026-07-08T22:45:20.238476Z")
        assert dt1.year == 2026
        assert dt1.month == 7
        assert dt1.day == 8

        # Without Z suffix
        dt2 = SQLOperationStore._parse_iso_datetime("2026-07-08T22:45:20.238476")
        assert dt2.year == 2026
        assert dt2.month == 7
        assert dt2.day == 8

        # Both should be equal
        assert dt1 == dt2
