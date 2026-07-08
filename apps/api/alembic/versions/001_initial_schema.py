"""Initial schema creation.

Revision ID: 001
Revises:
Create Date: 2025-01-01 00:00:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

# revision identifiers, used by Alembic.
revision: str = "001"
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Create initial tables."""
    # DataResource table
    op.create_table(
        "data_resources",
        sa.Column("id", sa.String(255), nullable=False),
        sa.Column("tenant", sa.String(255), nullable=False),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("resource_type", sa.String(100), nullable=False),
        sa.Column("engine_type", sa.String(100), nullable=False),
        sa.Column("storage_class", sa.String(255), nullable=False),
        sa.Column("driver_type", sa.String(100), nullable=False),
        sa.Column("origination", sa.String(50), nullable=False),
        sa.Column("phase", sa.String(50), nullable=False),
        sa.Column("created_at", sa.String(50), nullable=False),
        sa.Column("updated_at", sa.String(50), nullable=False),
        sa.Column("namespace", sa.String(255), server_default=""),
        sa.Column("size_gi", sa.Integer, server_default="0"),
        sa.Column("import_conn_str", sa.String(1024), server_default=""),
        sa.Column("import_db_name", sa.String(255), server_default=""),
        sa.Column("external_provider", sa.String(100), server_default=""),
        sa.Column("external_resource_id", sa.String(255), server_default=""),
        sa.Column("external_endpoint", sa.String(1024), server_default=""),
        sa.Column("external_region", sa.String(100), server_default=""),
        sa.Column("health_state", sa.String(50), server_default=""),
        sa.Column("health_message", sa.Text, server_default=""),
        sa.Column("health_last_check", sa.String(50), server_default=""),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_data_resources_tenant", "data_resources", ["tenant"])
    op.create_index("ix_data_resources_name", "data_resources", ["name"])

    # Operation table
    op.create_table(
        "operations",
        sa.Column("id", sa.String(255), nullable=False),
        sa.Column("tenant", sa.String(255), nullable=False),
        sa.Column("op_type", sa.String(100), nullable=False),
        sa.Column("resource", sa.String(255), nullable=False),
        sa.Column("phase", sa.String(50), nullable=False),
        sa.Column("started_at", sa.String(50), nullable=False),
        sa.Column("completed_at", sa.String(50), server_default=None),
        sa.Column("error", sa.Text, server_default=None),
        sa.Column("result", sa.Text, server_default=None),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index("ix_operations_tenant", "operations", ["tenant"])

    # VolumeSnapshot table
    op.create_table(
        "volume_snapshots",
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("tenant", sa.String(255), nullable=False),
        sa.Column("source_pvc", sa.String(255), nullable=False),
        sa.Column("snapshot_class", sa.String(255), nullable=False),
        sa.Column("ready_to_use", sa.Integer, server_default="0"),
        sa.Column("creation_time", sa.String(50), server_default=""),
        sa.Column("size_bytes", sa.Integer, server_default="0"),
        sa.PrimaryKeyConstraint("name"),
    )
    op.create_index("ix_volume_snapshots_tenant", "volume_snapshots", ["tenant"])

    # DataProtectionPolicy table
    op.create_table(
        "data_protection_policies",
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("tenant", sa.String(255), nullable=False),
        sa.Column("snapshot_schedule", sa.String(255), server_default=""),
        sa.Column("backup_schedule", sa.String(255), server_default=""),
        sa.Column("destination", sa.String(255), server_default=""),
        sa.Column("last_snapshot", sa.String(50), server_default=""),
        sa.Column("last_backup", sa.String(50), server_default=""),
        sa.Column("created_at", sa.String(50), server_default=""),
        sa.PrimaryKeyConstraint("name"),
    )
    op.create_index(
        "ix_data_protection_policies_tenant",
        "data_protection_policies",
        ["tenant"],
    )

    # SearchPool table
    op.create_table(
        "search_pools",
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("phase", sa.String(50), server_default="Pending"),
        sa.Column("endpoint", sa.String(1024), server_default=""),
        sa.Column("tenant_count", sa.Integer, server_default="0"),
        sa.Column("replicas", sa.Integer, server_default="1"),
        sa.Column("version", sa.String(100), server_default=""),
        sa.Column("created_at", sa.String(50), server_default=""),
        sa.PrimaryKeyConstraint("name"),
    )


def downgrade() -> None:
    """Drop all tables."""
    op.drop_table("search_pools")
    op.drop_table("data_protection_policies")
    op.drop_table("volume_snapshots")
    op.drop_table("operations")
    op.drop_table("data_resources")
