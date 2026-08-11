"""Add operations table for durable operation state storage.

Revision ID: 002_operations
Revises: 001_initial
Create Date: 2026-07-08
"""
import sqlalchemy as sa
from alembic import op

revision = "002_operations"
down_revision = "001_initial"
branch_labels = None
depends_on = None


def upgrade() -> None:
    """Create the operations table."""
    op.create_table(
        "operations",
        sa.Column("id", sa.String(36), nullable=False),
        sa.Column("tenant", sa.String(255), nullable=False),
        sa.Column("resource_name", sa.String(255), nullable=False),
        sa.Column("resource_type", sa.String(100), nullable=False),
        sa.Column("operation_type", sa.String(100), nullable=False),
        sa.Column("phase", sa.String(50), nullable=False, index=True),
        sa.Column("message", sa.Text(), nullable=False),
        sa.Column("error", sa.Text(), nullable=True),
        sa.Column("progress", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("created_at", sa.DateTime(), nullable=False, server_default=sa.func.now()),
        sa.Column("updated_at", sa.DateTime(), nullable=False, server_default=sa.func.now()),
        sa.PrimaryKeyConstraint("tenant", "id", name="pk_operations_tenant_id"),
        sa.Index("ix_operations_tenant", "tenant"),
    )


def downgrade() -> None:
    """Drop the operations table."""
    op.drop_table("operations")
