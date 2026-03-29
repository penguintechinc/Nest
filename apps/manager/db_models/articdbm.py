"""SQLAlchemy table definitions for the new ArticDBM feature tables.

These 19 tables are introduced by the ArticDBM integration. They are defined
here in FK-dependency order so that SQLAlchemy metadata resolution and Alembic
autogenerate work correctly.

IMPORTANT: These models are ONLY for schema management (Alembic / create_all).
All runtime queries use PyDAL with migrate=False — never SQLAlchemy at runtime.

Table list (FK order):
  1.  database_server
  2.  user_permission
  3.  user_profile
  4.  temporary_access
  5.  security_rule
  6.  managed_database
  7.  sql_file
  8.  blocked_database
  9.  database_schema
  10. threat_intel_feed
  11. threat_intel_indicator
  12. database_security_config
  13. threat_intel_match
  14. license_info
  15. cloud_provider
  16. cloud_database_instance
  17. scaling_policy
  18. scaling_event

  Note: audit_logs extension columns (server_id, query_text, ip_address,
  result, user_agent) are defined in existing.py on the AuditLog model so
  Alembic autogenerate sees the complete target schema in one place.
"""

from datetime import datetime

from sqlalchemy import (
    Boolean,
    Column,
    DateTime,
    Float,
    ForeignKey,
    Integer,
    String,
    Text,
)

try:
    from sqlalchemy import JSON
except ImportError:
    from sqlalchemy import Text as JSON  # type: ignore[assignment]

from . import Base


# ---------------------------------------------------------------------------
# 1. database_server — root table, no FK dependencies on articdbm tables
# ---------------------------------------------------------------------------

class DatabaseServer(Base):
    """Registered database servers managed by ArticDBM."""

    __tablename__ = "database_server"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    host = Column(String(255), nullable=False)
    port = Column(Integer, nullable=False)
    db_type = Column(String(50), nullable=False)
    ssl_enabled = Column(Boolean, default=False, nullable=False)
    active = Column(Boolean, default=True, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 2. user_permission — depends on users, database_server
# ---------------------------------------------------------------------------

class UserPermission(Base):
    """Fine-grained per-user database permissions."""

    __tablename__ = "user_permission"

    id = Column(Integer, primary_key=True, autoincrement=True)
    user_id = Column(Integer, ForeignKey("users.id"), nullable=False)
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=False)
    permission_type = Column(String(50), nullable=False)
    database_name = Column(String(255), nullable=True)
    table_name = Column(String(255), nullable=True)
    column_name = Column(String(255), nullable=True)
    expires_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)


# ---------------------------------------------------------------------------
# 3. user_profile — depends on users
# ---------------------------------------------------------------------------

class UserProfile(Base):
    """Extended user profile for ArticDBM (API keys, rate limits, 2FA).

    Columns marked 'encrypted' store ciphertext; encryption/decryption is
    handled by the application layer, not the database.
    """

    __tablename__ = "user_profile"

    id = Column(Integer, primary_key=True, autoincrement=True)
    user_id = Column(Integer, ForeignKey("users.id"), unique=True, nullable=False)
    api_key = Column(String(512), nullable=True, comment="Encrypted API key")
    rate_limit_per_hour = Column(Integer, default=1000, nullable=False)
    two_factor_secret = Column(Text, nullable=True, comment="Encrypted TOTP secret")
    ip_whitelist = Column(JSON, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 4. temporary_access — depends on database_server, users
# ---------------------------------------------------------------------------

class TemporaryAccess(Base):
    """Short-lived, token-based access grants to a database server."""

    __tablename__ = "temporary_access"

    id = Column(Integer, primary_key=True, autoincrement=True)
    token = Column(String(512), unique=True, nullable=False)
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=False)
    permissions = Column(JSON, nullable=False)
    expires_at = Column(DateTime, nullable=False)
    used_at = Column(DateTime, nullable=True)
    created_by = Column(Integer, ForeignKey("users.id"), nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)


# ---------------------------------------------------------------------------
# 5. security_rule — depends on database_server (nullable)
# ---------------------------------------------------------------------------

class SecurityRule(Base):
    """Named security rules applied to queries or connections."""

    __tablename__ = "security_rule"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    rule_type = Column(String(50), nullable=False)
    rule_config = Column(JSON, nullable=False)
    priority = Column(Integer, default=100, nullable=False)
    applies_to_server_id = Column(
        Integer, ForeignKey("database_server.id"), nullable=True
    )
    active = Column(Boolean, default=True, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 6. managed_database — depends on database_server
# ---------------------------------------------------------------------------

class ManagedDatabase(Base):
    """A specific database within a managed server."""

    __tablename__ = "managed_database"

    id = Column(Integer, primary_key=True, autoincrement=True)
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=False)
    database_name = Column(String(255), nullable=False)
    backup_enabled = Column(Boolean, default=False, nullable=False)
    max_connections = Column(Integer, default=100, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 7. sql_file — depends on database_server (nullable), users (nullable)
# ---------------------------------------------------------------------------

class SqlFile(Base):
    """Stored SQL files that can be executed against managed databases."""

    __tablename__ = "sql_file"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    file_content = Column(Text, nullable=False)
    file_hash = Column(String(128), nullable=False)
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=True)
    allowed_databases = Column(JSON, nullable=True)
    created_by = Column(Integer, ForeignKey("users.id"), nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 8. blocked_database — depends on database_server
# ---------------------------------------------------------------------------

class BlockedDatabase(Base):
    """Databases explicitly blocked from access."""

    __tablename__ = "blocked_database"

    id = Column(Integer, primary_key=True, autoincrement=True)
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=False)
    database_name = Column(String(255), nullable=False)
    block_type = Column(String(50), nullable=False)
    reason = Column(Text, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)


# ---------------------------------------------------------------------------
# 9. database_schema — depends on database_server
# ---------------------------------------------------------------------------

class DatabaseSchema(Base):
    """Cached schema snapshot for a database on a server."""

    __tablename__ = "database_schema"

    id = Column(Integer, primary_key=True, autoincrement=True)
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=False)
    database_name = Column(String(255), nullable=False)
    schema_json = Column(JSON, nullable=False)
    last_synced_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)


# ---------------------------------------------------------------------------
# 10. threat_intel_feed — no FK dependencies on articdbm tables
# ---------------------------------------------------------------------------

class ThreatIntelFeed(Base):
    """External threat intelligence feed source."""

    __tablename__ = "threat_intel_feed"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    feed_type = Column(String(50), nullable=False)
    url = Column(String(1024), nullable=False)
    poll_interval = Column(Integer, default=3600, nullable=False)
    last_polled_at = Column(DateTime, nullable=True)
    active = Column(Boolean, default=True, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 11. threat_intel_indicator — depends on threat_intel_feed
# ---------------------------------------------------------------------------

class ThreatIntelIndicator(Base):
    """Individual IOC (indicator of compromise) from a threat intel feed."""

    __tablename__ = "threat_intel_indicator"

    id = Column(Integer, primary_key=True, autoincrement=True)
    feed_id = Column(Integer, ForeignKey("threat_intel_feed.id"), nullable=False)
    indicator_type = Column(String(50), nullable=False)
    value = Column(Text, nullable=False)
    confidence = Column(Integer, default=50, nullable=False)
    severity = Column(String(20), default="medium", nullable=False)
    expires_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)


# ---------------------------------------------------------------------------
# 12. database_security_config — depends on database_server (unique)
# ---------------------------------------------------------------------------

class DatabaseSecurityConfig(Base):
    """Per-server security configuration (one row per server)."""

    __tablename__ = "database_security_config"

    id = Column(Integer, primary_key=True, autoincrement=True)
    server_id = Column(
        Integer, ForeignKey("database_server.id"), unique=True, nullable=False
    )
    sql_injection_detection = Column(Boolean, default=True, nullable=False)
    allowed_ips = Column(JSON, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 13. threat_intel_match — depends on threat_intel_indicator, database_server
# ---------------------------------------------------------------------------

class ThreatIntelMatch(Base):
    """Record of a threat intel indicator matched against server activity."""

    __tablename__ = "threat_intel_match"

    id = Column(Integer, primary_key=True, autoincrement=True)
    indicator_id = Column(
        Integer, ForeignKey("threat_intel_indicator.id"), nullable=False
    )
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=False)
    matched_value = Column(Text, nullable=False)
    action_taken = Column(String(50), nullable=False)
    matched_at = Column(DateTime, nullable=False, default=datetime.utcnow)


# ---------------------------------------------------------------------------
# 14. license_info — no FK dependencies
# ---------------------------------------------------------------------------

class LicenseInfo(Base):
    """ArticDBM license key and feature entitlement cache."""

    __tablename__ = "license_info"

    id = Column(Integer, primary_key=True, autoincrement=True)
    license_key = Column(String(512), nullable=False)
    features = Column(JSON, nullable=False)
    expires_at = Column(DateTime, nullable=True)
    last_validated_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 15. cloud_provider — no FK dependencies on articdbm tables
# ---------------------------------------------------------------------------

class CloudProvider(Base):
    """Cloud provider account configuration.

    The credentials column stores encrypted JSON — plaintext credentials are
    never stored; encryption/decryption is handled by the application layer.
    """

    __tablename__ = "cloud_provider"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    provider_type = Column(String(50), nullable=False)
    credentials = Column(JSON, nullable=True, comment="Encrypted cloud credentials")
    region = Column(String(100), nullable=True)
    active = Column(Boolean, default=True, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 16. cloud_database_instance — depends on cloud_provider, database_server
# ---------------------------------------------------------------------------

class CloudDatabaseInstance(Base):
    """A cloud-managed database instance linked to a provider and server."""

    __tablename__ = "cloud_database_instance"

    id = Column(Integer, primary_key=True, autoincrement=True)
    provider_id = Column(Integer, ForeignKey("cloud_provider.id"), nullable=False)
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=True)
    instance_id = Column(String(255), nullable=False)
    status = Column(String(50), default="unknown", nullable=False)
    endpoint = Column(String(512), nullable=True)
    storage_gb = Column(Integer, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 17. scaling_policy — depends on database_server
# ---------------------------------------------------------------------------

class ScalingPolicy(Base):
    """Auto-scaling policy for a managed database server."""

    __tablename__ = "scaling_policy"

    id = Column(Integer, primary_key=True, autoincrement=True)
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=False)
    policy_type = Column(String(50), nullable=False)
    trigger_metric = Column(String(100), nullable=False)
    scale_up_threshold = Column(Float, nullable=True)
    scale_down_threshold = Column(Float, nullable=True)
    active = Column(Boolean, default=True, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


# ---------------------------------------------------------------------------
# 18. scaling_event — depends on scaling_policy, database_server
# ---------------------------------------------------------------------------

class ScalingEvent(Base):
    """Record of a scaling action triggered by a scaling policy."""

    __tablename__ = "scaling_event"

    id = Column(Integer, primary_key=True, autoincrement=True)
    policy_id = Column(Integer, ForeignKey("scaling_policy.id"), nullable=False)
    server_id = Column(Integer, ForeignKey("database_server.id"), nullable=False)
    event_type = Column(String(50), nullable=False)
    status = Column(String(50), default="pending", nullable=False)
    triggered_at = Column(DateTime, nullable=False, default=datetime.utcnow)
    completed_at = Column(DateTime, nullable=True)
