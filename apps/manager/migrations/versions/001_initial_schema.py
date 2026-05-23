"""Initial schema — all tables for apps/manager.

Covers the baseline manager schema (teams, users, team_memberships,
resource_types, resources, resource_users, resource_stats, backup_jobs,
provisioning_jobs, certificate_authorities, certificates, audit_logs) plus
all ArticDBM feature tables (database_server, user_permission, user_profile,
temporary_access, security_rule, managed_database, sql_file,
blocked_database, database_schema, threat_intel_feed,
threat_intel_indicator, database_security_config, threat_intel_match,
license_info, cloud_provider, cloud_database_instance, scaling_policy,
scaling_event).

The audit_logs table includes ArticDBM extension columns (server_id,
query_text, ip_address, result, user_agent) so that the initial schema is
fully consistent with the SQLAlchemy model in db_models/existing.py.

Revision ID: 001_initial
Revises:
Create Date: 2026-03-27
"""
import sqlalchemy as sa
from alembic import op

revision = "001_initial"
down_revision = None
branch_labels = None
depends_on = None


def upgrade() -> None:
    """Create all tables in FK-dependency order."""

    # -------------------------------------------------------------------------
    # teams — no FK dependencies
    # -------------------------------------------------------------------------
    op.create_table(
        "teams",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("description", sa.Text(), nullable=True),
        sa.Column("is_global", sa.Boolean(), nullable=False, server_default="false"),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # users — no FK dependencies
    # -------------------------------------------------------------------------
    op.create_table(
        "users",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("email", sa.String(255), nullable=False),
        sa.Column("username", sa.String(255), nullable=False),
        sa.Column("password_hash", sa.String(255), nullable=False),
        sa.Column("active", sa.Boolean(), nullable=False, server_default="true"),
        sa.Column("role", sa.String(50), nullable=True),
        sa.Column("confirmed_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
        sa.UniqueConstraint("email", name="uq_users_email"),
        sa.UniqueConstraint("username", name="uq_users_username"),
    )

    # -------------------------------------------------------------------------
    # team_memberships — depends on teams, users
    # -------------------------------------------------------------------------
    op.create_table(
        "team_memberships",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("team_id", sa.Integer(), sa.ForeignKey("teams.id"), nullable=False),
        sa.Column("user_id", sa.Integer(), sa.ForeignKey("users.id"), nullable=False),
        sa.Column("role", sa.String(50), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # resource_types — no FK dependencies
    # -------------------------------------------------------------------------
    op.create_table(
        "resource_types",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("category", sa.String(50), nullable=False),
        sa.Column("type_name", sa.String(100), nullable=False),
        sa.Column("description", sa.String(500), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.UniqueConstraint("type_name", name="uq_resource_types_type_name"),
    )

    # -------------------------------------------------------------------------
    # resources — depends on teams, resource_types
    # -------------------------------------------------------------------------
    op.create_table(
        "resources",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("team_id", sa.Integer(), sa.ForeignKey("teams.id"), nullable=True),
        sa.Column("resource_type_id", sa.Integer(), sa.ForeignKey("resource_types.id"), nullable=True),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("description", sa.Text(), nullable=True),
        sa.Column("config", sa.JSON(), nullable=True),
        sa.Column("active", sa.Boolean(), nullable=False, server_default="true"),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # resource_users — depends on resources, users
    # -------------------------------------------------------------------------
    op.create_table(
        "resource_users",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("resource_id", sa.Integer(), sa.ForeignKey("resources.id"), nullable=False),
        sa.Column("user_id", sa.Integer(), sa.ForeignKey("users.id"), nullable=False),
        sa.Column("access_level", sa.String(50), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # resource_stats — depends on resources
    # -------------------------------------------------------------------------
    op.create_table(
        "resource_stats",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("resource_id", sa.Integer(), sa.ForeignKey("resources.id"), nullable=False),
        sa.Column("metric_name", sa.String(100), nullable=False),
        sa.Column("metric_value", sa.String(255), nullable=True),
        sa.Column("recorded_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # backup_jobs — depends on resources
    # -------------------------------------------------------------------------
    op.create_table(
        "backup_jobs",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("resource_id", sa.Integer(), sa.ForeignKey("resources.id"), nullable=True),
        sa.Column("status", sa.String(50), nullable=True),
        sa.Column("started_at", sa.DateTime(), nullable=True),
        sa.Column("completed_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # provisioning_jobs — depends on resources
    # -------------------------------------------------------------------------
    op.create_table(
        "provisioning_jobs",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("resource_id", sa.Integer(), sa.ForeignKey("resources.id"), nullable=True),
        sa.Column("job_type", sa.String(100), nullable=True),
        sa.Column("status", sa.String(50), nullable=True),
        sa.Column("payload", sa.JSON(), nullable=True),
        sa.Column("started_at", sa.DateTime(), nullable=True),
        sa.Column("completed_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # certificate_authorities — no FK dependencies
    # -------------------------------------------------------------------------
    op.create_table(
        "certificate_authorities",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("certificate", sa.Text(), nullable=True),
        sa.Column("active", sa.Boolean(), nullable=False, server_default="true"),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # certificates — depends on certificate_authorities, resources
    # -------------------------------------------------------------------------
    op.create_table(
        "certificates",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("ca_id", sa.Integer(), sa.ForeignKey("certificate_authorities.id"), nullable=True),
        sa.Column("resource_id", sa.Integer(), sa.ForeignKey("resources.id"), nullable=True),
        sa.Column("common_name", sa.String(255), nullable=True),
        sa.Column("certificate", sa.Text(), nullable=True),
        sa.Column("expires_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # database_server — ArticDBM root table, no FK dependencies on other
    # articdbm tables. Must be created BEFORE audit_logs references it.
    # -------------------------------------------------------------------------
    op.create_table(
        "database_server",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("host", sa.String(255), nullable=False),
        sa.Column("port", sa.Integer(), nullable=False),
        sa.Column("db_type", sa.String(50), nullable=False),
        sa.Column("ssl_enabled", sa.Boolean(), nullable=False, server_default="false"),
        sa.Column("active", sa.Boolean(), nullable=False, server_default="true"),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # audit_logs — depends on users, database_server (ArticDBM extension cols)
    # -------------------------------------------------------------------------
    op.create_table(
        "audit_logs",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("user_id", sa.Integer(), sa.ForeignKey("users.id"), nullable=True),
        sa.Column("action", sa.String(255), nullable=True),
        sa.Column("resource_type", sa.String(100), nullable=True),
        sa.Column("resource_id", sa.Integer(), nullable=True),
        sa.Column("details", sa.JSON(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        # ArticDBM extension columns
        sa.Column(
            "server_id",
            sa.Integer(),
            sa.ForeignKey("database_server.id"),
            nullable=True,
            comment="ArticDBM: which database server this audit event relates to",
        ),
        sa.Column(
            "query_text",
            sa.Text(),
            nullable=True,
            comment="ArticDBM: SQL query text if applicable",
        ),
        sa.Column(
            "ip_address",
            sa.String(45),
            nullable=True,
            comment="ArticDBM: client IP address (IPv4 or IPv6)",
        ),
        sa.Column(
            "result",
            sa.String(50),
            nullable=True,
            comment="ArticDBM: outcome of the audited action (e.g. success, denied)",
        ),
        sa.Column(
            "user_agent",
            sa.Text(),
            nullable=True,
            comment="ArticDBM: HTTP user-agent string if applicable",
        ),
    )

    # -------------------------------------------------------------------------
    # user_permission — depends on users, database_server
    # -------------------------------------------------------------------------
    op.create_table(
        "user_permission",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("user_id", sa.Integer(), sa.ForeignKey("users.id"), nullable=False),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=False),
        sa.Column("permission_type", sa.String(50), nullable=False),
        sa.Column("database_name", sa.String(255), nullable=True),
        sa.Column("table_name", sa.String(255), nullable=True),
        sa.Column("column_name", sa.String(255), nullable=True),
        sa.Column("expires_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # user_profile — depends on users
    # -------------------------------------------------------------------------
    op.create_table(
        "user_profile",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("user_id", sa.Integer(), sa.ForeignKey("users.id"), nullable=False),
        sa.Column("api_key", sa.String(512), nullable=True, comment="Encrypted API key"),
        sa.Column("rate_limit_per_hour", sa.Integer(), nullable=False, server_default="1000"),
        sa.Column("two_factor_secret", sa.Text(), nullable=True, comment="Encrypted TOTP secret"),
        sa.Column("ip_whitelist", sa.JSON(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
        sa.UniqueConstraint("user_id", name="uq_user_profile_user_id"),
    )

    # -------------------------------------------------------------------------
    # temporary_access — depends on database_server, users
    # -------------------------------------------------------------------------
    op.create_table(
        "temporary_access",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("token", sa.String(512), nullable=False),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=False),
        sa.Column("permissions", sa.JSON(), nullable=False),
        sa.Column("expires_at", sa.DateTime(), nullable=False),
        sa.Column("used_at", sa.DateTime(), nullable=True),
        sa.Column("created_by", sa.Integer(), sa.ForeignKey("users.id"), nullable=False),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.UniqueConstraint("token", name="uq_temporary_access_token"),
    )

    # -------------------------------------------------------------------------
    # security_rule — depends on database_server (nullable)
    # -------------------------------------------------------------------------
    op.create_table(
        "security_rule",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("rule_type", sa.String(50), nullable=False),
        sa.Column("rule_config", sa.JSON(), nullable=False),
        sa.Column("priority", sa.Integer(), nullable=False, server_default="100"),
        sa.Column(
            "applies_to_server_id",
            sa.Integer(),
            sa.ForeignKey("database_server.id"),
            nullable=True,
        ),
        sa.Column("active", sa.Boolean(), nullable=False, server_default="true"),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # managed_database — depends on database_server
    # -------------------------------------------------------------------------
    op.create_table(
        "managed_database",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=False),
        sa.Column("database_name", sa.String(255), nullable=False),
        sa.Column("backup_enabled", sa.Boolean(), nullable=False, server_default="false"),
        sa.Column("max_connections", sa.Integer(), nullable=False, server_default="100"),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # sql_file — depends on database_server (nullable), users (nullable)
    # -------------------------------------------------------------------------
    op.create_table(
        "sql_file",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("file_content", sa.Text(), nullable=False),
        sa.Column("file_hash", sa.String(128), nullable=False),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=True),
        sa.Column("allowed_databases", sa.JSON(), nullable=True),
        sa.Column("created_by", sa.Integer(), sa.ForeignKey("users.id"), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # blocked_database — depends on database_server
    # -------------------------------------------------------------------------
    op.create_table(
        "blocked_database",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=False),
        sa.Column("database_name", sa.String(255), nullable=False),
        sa.Column("block_type", sa.String(50), nullable=False),
        sa.Column("reason", sa.Text(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # database_schema — depends on database_server
    # -------------------------------------------------------------------------
    op.create_table(
        "database_schema",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=False),
        sa.Column("database_name", sa.String(255), nullable=False),
        sa.Column("schema_json", sa.JSON(), nullable=False),
        sa.Column("last_synced_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # threat_intel_feed — no FK dependencies
    # -------------------------------------------------------------------------
    op.create_table(
        "threat_intel_feed",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("feed_type", sa.String(50), nullable=False),
        sa.Column("url", sa.String(1024), nullable=False),
        sa.Column("poll_interval", sa.Integer(), nullable=False, server_default="3600"),
        sa.Column("last_polled_at", sa.DateTime(), nullable=True),
        sa.Column("active", sa.Boolean(), nullable=False, server_default="true"),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # threat_intel_indicator — depends on threat_intel_feed
    # -------------------------------------------------------------------------
    op.create_table(
        "threat_intel_indicator",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("feed_id", sa.Integer(), sa.ForeignKey("threat_intel_feed.id"), nullable=False),
        sa.Column("indicator_type", sa.String(50), nullable=False),
        sa.Column("value", sa.Text(), nullable=False),
        sa.Column("confidence", sa.Integer(), nullable=False, server_default="50"),
        sa.Column("severity", sa.String(20), nullable=False, server_default="'medium'"),
        sa.Column("expires_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # database_security_config — depends on database_server (unique)
    # -------------------------------------------------------------------------
    op.create_table(
        "database_security_config",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column(
            "server_id",
            sa.Integer(),
            sa.ForeignKey("database_server.id"),
            nullable=False,
        ),
        sa.Column("sql_injection_detection", sa.Boolean(), nullable=False, server_default="true"),
        sa.Column("allowed_ips", sa.JSON(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
        sa.UniqueConstraint("server_id", name="uq_database_security_config_server_id"),
    )

    # -------------------------------------------------------------------------
    # threat_intel_match — depends on threat_intel_indicator, database_server
    # -------------------------------------------------------------------------
    op.create_table(
        "threat_intel_match",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column(
            "indicator_id",
            sa.Integer(),
            sa.ForeignKey("threat_intel_indicator.id"),
            nullable=False,
        ),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=False),
        sa.Column("matched_value", sa.Text(), nullable=False),
        sa.Column("action_taken", sa.String(50), nullable=False),
        sa.Column("matched_at", sa.DateTime(), nullable=False),
    )

    # -------------------------------------------------------------------------
    # license_info — no FK dependencies
    # -------------------------------------------------------------------------
    op.create_table(
        "license_info",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("license_key", sa.String(512), nullable=False),
        sa.Column("features", sa.JSON(), nullable=False),
        sa.Column("expires_at", sa.DateTime(), nullable=True),
        sa.Column("last_validated_at", sa.DateTime(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # cloud_provider — no FK dependencies on articdbm tables
    # -------------------------------------------------------------------------
    op.create_table(
        "cloud_provider",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(255), nullable=False),
        sa.Column("provider_type", sa.String(50), nullable=False),
        sa.Column("credentials", sa.JSON(), nullable=True, comment="Encrypted cloud credentials"),
        sa.Column("region", sa.String(100), nullable=True),
        sa.Column("active", sa.Boolean(), nullable=False, server_default="true"),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # cloud_database_instance — depends on cloud_provider, database_server
    # -------------------------------------------------------------------------
    op.create_table(
        "cloud_database_instance",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("provider_id", sa.Integer(), sa.ForeignKey("cloud_provider.id"), nullable=False),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=True),
        sa.Column("instance_id", sa.String(255), nullable=False),
        sa.Column("status", sa.String(50), nullable=False, server_default="'unknown'"),
        sa.Column("endpoint", sa.String(512), nullable=True),
        sa.Column("storage_gb", sa.Integer(), nullable=True),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # scaling_policy — depends on database_server
    # -------------------------------------------------------------------------
    op.create_table(
        "scaling_policy",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=False),
        sa.Column("policy_type", sa.String(50), nullable=False),
        sa.Column("trigger_metric", sa.String(100), nullable=False),
        sa.Column("scale_up_threshold", sa.Float(), nullable=True),
        sa.Column("scale_down_threshold", sa.Float(), nullable=True),
        sa.Column("active", sa.Boolean(), nullable=False, server_default="true"),
        sa.Column("created_at", sa.DateTime(), nullable=True),
        sa.Column("updated_at", sa.DateTime(), nullable=True),
    )

    # -------------------------------------------------------------------------
    # scaling_event — depends on scaling_policy, database_server
    # -------------------------------------------------------------------------
    op.create_table(
        "scaling_event",
        sa.Column("id", sa.Integer(), primary_key=True, autoincrement=True),
        sa.Column("policy_id", sa.Integer(), sa.ForeignKey("scaling_policy.id"), nullable=False),
        sa.Column("server_id", sa.Integer(), sa.ForeignKey("database_server.id"), nullable=False),
        sa.Column("event_type", sa.String(50), nullable=False),
        sa.Column("status", sa.String(50), nullable=False, server_default="'pending'"),
        sa.Column("triggered_at", sa.DateTime(), nullable=False),
        sa.Column("completed_at", sa.DateTime(), nullable=True),
    )


def downgrade() -> None:
    """Drop all tables in reverse FK-dependency order."""
    # ArticDBM tables (reverse order of creation)
    op.drop_table("scaling_event")
    op.drop_table("scaling_policy")
    op.drop_table("cloud_database_instance")
    op.drop_table("cloud_provider")
    op.drop_table("license_info")
    op.drop_table("threat_intel_match")
    op.drop_table("database_security_config")
    op.drop_table("threat_intel_indicator")
    op.drop_table("threat_intel_feed")
    op.drop_table("database_schema")
    op.drop_table("blocked_database")
    op.drop_table("sql_file")
    op.drop_table("managed_database")
    op.drop_table("security_rule")
    op.drop_table("temporary_access")
    op.drop_table("user_profile")
    op.drop_table("user_permission")
    op.drop_table("audit_logs")
    op.drop_table("database_server")

    # Existing tables (reverse order of creation)
    op.drop_table("certificates")
    op.drop_table("certificate_authorities")
    op.drop_table("provisioning_jobs")
    op.drop_table("backup_jobs")
    op.drop_table("resource_stats")
    op.drop_table("resource_users")
    op.drop_table("resources")
    op.drop_table("resource_types")
    op.drop_table("team_memberships")
    op.drop_table("users")
    op.drop_table("teams")
