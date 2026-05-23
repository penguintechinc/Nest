"""SQLAlchemy table definitions for the EXISTING manager schema.

These models mirror tables already in the database so that Alembic
autogenerate includes them in the baseline migration. They are NOT
used for runtime queries — all runtime access uses PyDAL (migrate=False).

Tables covered:
  teams, users, team_memberships, resource_types, resources,
  resource_users, resource_stats, backup_jobs, provisioning_jobs,
  certificate_authorities, certificates, audit_logs
"""

from datetime import datetime

from sqlalchemy import (
    Boolean,
    Column,
    DateTime,
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


class Team(Base):
    """Existing teams table."""

    __tablename__ = "teams"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    description = Column(Text, nullable=True)
    is_global = Column(Boolean, default=False, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class User(Base):
    """Existing users table."""

    __tablename__ = "users"

    id = Column(Integer, primary_key=True, autoincrement=True)
    email = Column(String(255), unique=True, nullable=False)
    username = Column(String(255), unique=True, nullable=False)
    password_hash = Column(String(255), nullable=False)
    active = Column(Boolean, default=True, nullable=False)
    role = Column(String(50), nullable=True)
    confirmed_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class TeamMembership(Base):
    """Existing team_memberships table."""

    __tablename__ = "team_memberships"

    id = Column(Integer, primary_key=True, autoincrement=True)
    team_id = Column(Integer, ForeignKey("teams.id"), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id"), nullable=False)
    role = Column(String(50), nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)


class ResourceType(Base):
    """Existing resource_types table."""

    __tablename__ = "resource_types"

    id = Column(Integer, primary_key=True, autoincrement=True)
    category = Column(String(50), nullable=False)
    type_name = Column(String(100), unique=True, nullable=False)
    description = Column(String(500), nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)


class Resource(Base):
    """Existing resources table."""

    __tablename__ = "resources"

    id = Column(Integer, primary_key=True, autoincrement=True)
    team_id = Column(Integer, ForeignKey("teams.id"), nullable=True)
    resource_type_id = Column(Integer, ForeignKey("resource_types.id"), nullable=True)
    name = Column(String(255), nullable=False)
    description = Column(Text, nullable=True)
    config = Column(JSON, nullable=True)
    active = Column(Boolean, default=True, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class ResourceUser(Base):
    """Existing resource_users table."""

    __tablename__ = "resource_users"

    id = Column(Integer, primary_key=True, autoincrement=True)
    resource_id = Column(Integer, ForeignKey("resources.id"), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id"), nullable=False)
    access_level = Column(String(50), nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)


class ResourceStat(Base):
    """Existing resource_stats table."""

    __tablename__ = "resource_stats"

    id = Column(Integer, primary_key=True, autoincrement=True)
    resource_id = Column(Integer, ForeignKey("resources.id"), nullable=False)
    metric_name = Column(String(100), nullable=False)
    metric_value = Column(String(255), nullable=True)
    recorded_at = Column(DateTime, default=datetime.utcnow)


class BackupJob(Base):
    """Existing backup_jobs table."""

    __tablename__ = "backup_jobs"

    id = Column(Integer, primary_key=True, autoincrement=True)
    resource_id = Column(Integer, ForeignKey("resources.id"), nullable=True)
    status = Column(String(50), nullable=True)
    started_at = Column(DateTime, nullable=True)
    completed_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)


class ProvisioningJob(Base):
    """Existing provisioning_jobs table."""

    __tablename__ = "provisioning_jobs"

    id = Column(Integer, primary_key=True, autoincrement=True)
    resource_id = Column(Integer, ForeignKey("resources.id"), nullable=True)
    job_type = Column(String(100), nullable=True)
    status = Column(String(50), nullable=True)
    payload = Column(JSON, nullable=True)
    started_at = Column(DateTime, nullable=True)
    completed_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)


class CertificateAuthority(Base):
    """Existing certificate_authorities table."""

    __tablename__ = "certificate_authorities"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    certificate = Column(Text, nullable=True)
    active = Column(Boolean, default=True, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class Certificate(Base):
    """Existing certificates table."""

    __tablename__ = "certificates"

    id = Column(Integer, primary_key=True, autoincrement=True)
    ca_id = Column(Integer, ForeignKey("certificate_authorities.id"), nullable=True)
    resource_id = Column(Integer, ForeignKey("resources.id"), nullable=True)
    common_name = Column(String(255), nullable=True)
    certificate = Column(Text, nullable=True)
    expires_at = Column(DateTime, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)


class AuditLog(Base):
    """Existing audit_logs table.

    NOTE: The articdbm migration adds nullable columns to this table:
      server_id, query_text, ip_address, result, user_agent
    Those columns are defined here so that Alembic's autogenerate sees them
    as part of the final target schema. The actual ALTER TABLE statements
    are emitted by the articdbm Alembic migration revision.
    """

    __tablename__ = "audit_logs"

    id = Column(Integer, primary_key=True, autoincrement=True)
    user_id = Column(Integer, ForeignKey("users.id"), nullable=True)
    action = Column(String(255), nullable=True)
    resource_type = Column(String(100), nullable=True)
    resource_id = Column(Integer, nullable=True)
    details = Column(JSON, nullable=True)
    created_at = Column(DateTime, default=datetime.utcnow)

    # ArticDBM extension columns — added via ALTER TABLE in articdbm migration.
    # Defined here so Alembic autogenerate sees the complete target state.
    server_id = Column(
        Integer,
        ForeignKey("database_server.id"),
        nullable=True,
        comment="ArticDBM: which database server this audit event relates to",
    )
    query_text = Column(
        Text,
        nullable=True,
        comment="ArticDBM: SQL query text if applicable",
    )
    ip_address = Column(
        String(45),
        nullable=True,
        comment="ArticDBM: client IP address (IPv4 or IPv6)",
    )
    result = Column(
        String(50),
        nullable=True,
        comment="ArticDBM: outcome of the audited action (e.g. success, denied)",
    )
    user_agent = Column(
        Text,
        nullable=True,
        comment="ArticDBM: HTTP user-agent string if applicable",
    )
