#!/usr/bin/env python3
"""
Flask-Security-Too + SQLAlchemy Database Initialization Script

This script initializes the database for the Manager application using Flask-Security-Too
and SQLAlchemy. It should ONLY be used for initial database setup and creating the default
Global team. After initial setup, all database operations use PyDAL.

Usage:
    python db_init.py

Environment Variables:
    DB_HOST: PostgreSQL host (default: localhost)
    DB_PORT: PostgreSQL port (default: 5432)
    DB_NAME: Database name (default: manager)
    DB_USER: Database user (default: postgres)
    DB_PASSWORD: Database password (required)
"""

import os
import sys
import logging
from datetime import datetime

try:
    from flask import Flask
    from flask_sqlalchemy import SQLAlchemy
    from flask_security import Security, SQLAlchemyUserDatastore, hash_password
    from sqlalchemy import Column, Integer, String, Boolean, DateTime, ForeignKey, Enum as SQLEnum
    from sqlalchemy.orm import relationship
except ImportError as e:
    print(f"Error: Required packages not installed: {e}")
    print("Install required packages with:")
    print("  pip install flask flask-sqlalchemy flask-security-too")
    sys.exit(1)

# Import db_models so that Base.metadata.create_all() at startup detects
# all tables — existing schema plus all new ArticDBM tables.
# Alembic also imports this package (via migrations/env.py) for autogenerate.
# NOTE: Alembic migrations are run MANUALLY via ./scripts/migrate.sh — never
# call alembic upgrade head from application startup code.
try:
    import db_models  # noqa: F401 — import for side-effect (table registration)
    from db_models import Base as DeclarativeBase
except ImportError:
    # db_models may not yet exist during initial bootstrap — non-fatal
    DeclarativeBase = None  # type: ignore[assignment]

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class DatabaseConfig:
    """Database configuration from environment variables"""

    def __init__(self):
        self.host = os.getenv('DB_HOST', 'localhost')
        self.port = os.getenv('DB_PORT', '5432')
        self.name = os.getenv('DB_NAME', 'manager')
        self.user = os.getenv('DB_USER', 'postgres')
        self.password = os.getenv('DB_PASSWORD', '')

        if not self.password:
            raise ValueError("DB_PASSWORD environment variable is required")

    @property
    def connection_string(self):
        """Generate PostgreSQL connection string"""
        return (
            f"postgresql://{self.user}:{self.password}@"
            f"{self.host}:{self.port}/{self.name}"
        )


# Database configuration
db_config = DatabaseConfig()

# Flask app initialization
app = Flask(__name__)
app.config['SQLALCHEMY_DATABASE_URI'] = db_config.connection_string
app.config['SQLALCHEMY_TRACK_MODIFICATIONS'] = False
app.config['SECRET_KEY'] = os.getenv('SECRET_KEY', 'change-me-in-production')

db = SQLAlchemy(app)


# ============================================================================
# Database Models
# ============================================================================

class Role(db.Model):
    """User role model for Flask-Security-Too"""
    __tablename__ = 'role'

    id = Column(Integer, primary_key=True)
    name = Column(String(80), unique=True, nullable=False)
    description = Column(String(255))
    created_at = Column(DateTime, default=datetime.utcnow)

    # Relationships
    users = relationship('User', secondary='roles_users', backref='roles')

    def __repr__(self):
        return f'<Role {self.name}>'


class User(db.Model):
    """User model for Flask-Security-Too"""
    __tablename__ = 'user'

    id = Column(Integer, primary_key=True)
    email = Column(String(255), unique=True, nullable=False)
    username = Column(String(255), unique=True, nullable=False)
    password = Column(String(255), nullable=False)
    active = Column(Boolean, default=True, nullable=False)
    confirmed_at = Column(DateTime)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    # Relationships
    role_ids = relationship('Role', secondary='roles_users')

    def __repr__(self):
        return f'<User {self.email}>'


class RolesUsers(db.Model):
    """Association table for User-Role relationships"""
    __tablename__ = 'roles_users'

    id = Column(Integer, primary_key=True)
    user_id = Column(Integer, ForeignKey('user.id'), nullable=False)
    role_id = Column(Integer, ForeignKey('role.id'), nullable=False)


class Team(db.Model):
    """Team model"""
    __tablename__ = 'team'

    id = Column(Integer, primary_key=True)
    name = Column(String(255), nullable=False)
    description = Column(String(500))
    is_global = Column(Boolean, default=False, nullable=False)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    def __repr__(self):
        return f'<Team {self.name}>'


class ResourceType(db.Model):
    """Supported resource types"""
    __tablename__ = 'resource_type'

    id = Column(Integer, primary_key=True)
    category = Column(String(50), nullable=False)  # storage, database, bigdata
    type_name = Column(String(100), unique=True, nullable=False)
    description = Column(String(500))
    created_at = Column(DateTime, default=datetime.utcnow)

    def __repr__(self):
        return f'<ResourceType {self.type_name}>'


# ============================================================================
# Database Initialization Functions
# ============================================================================

def create_tables():
    """Create all database tables"""
    logger.info("Creating database tables...")
    try:
        db.create_all()
        logger.info("Database tables created successfully")
        return True
    except Exception as e:
        logger.error(f"Failed to create database tables: {e}")
        return False


def create_default_team():
    """Create the default 'Global' team"""
    logger.info("Creating default 'Global' team...")
    try:
        # Check if Global team already exists
        existing_team = Team.query.filter_by(name='Global', is_global=True).first()
        if existing_team:
            logger.info("Global team already exists (id=%d)", existing_team.id)
            return True

        # Create new Global team
        global_team = Team(
            name='Global',
            description='Global default team for all resources',
            is_global=True
        )
        db.session.add(global_team)
        db.session.commit()
        logger.info("Default 'Global' team created successfully (id=%d)", global_team.id)
        return True
    except Exception as e:
        logger.error(f"Failed to create default team: {e}")
        db.session.rollback()
        return False


def seed_resource_types():
    """Seed the resource_types table with all supported types"""
    logger.info("Seeding resource types...")

    resource_types = [
        # Storage types
        ('storage', 'storage-iscsi', 'iSCSI Storage'),
        ('storage', 'storage-nfs', 'NFS Network File System'),
        ('storage', 'storage-scsi', 'SCSI Storage'),
        ('storage', 'storage-sas', 'Serial Attached SCSI'),
        ('storage', 'storage-san', 'Storage Area Network'),
        ('storage', 'storage-ceph', 'Ceph Distributed Storage'),

        # Database types
        ('database', 'db-mariadb', 'MariaDB Database'),
        ('database', 'db-galera', 'Galera Cluster'),
        ('database', 'db-postgresql', 'PostgreSQL Database'),
        ('database', 'db-redis', 'Redis Cache'),
        ('database', 'db-valkey', 'Valkey Cache'),
        ('database', 'db-sqlite', 'SQLite Database'),

        # Big Data types
        ('bigdata', 'bigdata-hadoop', 'Apache Hadoop'),
        ('bigdata', 'bigdata-trino', 'Trino Query Engine'),
    ]

    try:
        for category, type_name, description in resource_types:
            # Check if resource type already exists
            existing_type = ResourceType.query.filter_by(type_name=type_name).first()
            if existing_type:
                logger.debug(f"Resource type '{type_name}' already exists (id={existing_type.id})")
                continue

            # Create new resource type
            resource_type = ResourceType(
                category=category,
                type_name=type_name,
                description=description
            )
            db.session.add(resource_type)

        db.session.commit()

        # Count total resource types
        total_count = ResourceType.query.count()
        logger.info(f"Resource types seeded successfully (total: {total_count})")
        return True
    except Exception as e:
        logger.error(f"Failed to seed resource types: {e}")
        db.session.rollback()
        return False


def verify_database():
    """Verify database setup by checking table counts"""
    logger.info("Verifying database setup...")
    try:
        roles_count = Role.query.count()
        users_count = User.query.count()
        teams_count = Team.query.count()
        resource_types_count = ResourceType.query.count()

        logger.info(f"Database verification:")
        logger.info(f"  - Roles: {roles_count}")
        logger.info(f"  - Users: {users_count}")
        logger.info(f"  - Teams: {teams_count}")
        logger.info(f"  - Resource Types: {resource_types_count}")

        # Verify Global team exists
        global_team = Team.query.filter_by(name='Global', is_global=True).first()
        if global_team:
            logger.info(f"  - Global team verified (id={global_team.id})")
        else:
            logger.warning("Warning: Global team not found!")

        return True
    except Exception as e:
        logger.error(f"Database verification failed: {e}")
        return False


def print_success_summary():
    """Print summary of successful initialization"""
    print("\n" + "="*70)
    print("DATABASE INITIALIZATION COMPLETED SUCCESSFULLY")
    print("="*70)
    print(f"\nDatabase Connection: {db_config.host}:{db_config.port}/{db_config.name}")
    print(f"Connection String: postgresql://{db_config.user}:***@{db_config.host}:{db_config.port}/{db_config.name}")
    print("\nInitialization Summary:")
    print(f"  ✓ Database tables created")
    print(f"  ✓ Default 'Global' team created")
    print(f"  ✓ Resource types seeded ({ResourceType.query.count()} types)")
    print(f"  ✓ Total teams: {Team.query.count()}")
    print(f"  ✓ Total users: {User.query.count()}")
    print(f"  ✓ Total roles: {Role.query.count()}")
    print("\nNext Steps:")
    print("  1. All future database operations should use PyDAL")
    print("  2. Do NOT use SQLAlchemy directly after initial setup")
    print("  3. Configure PyDAL models in your application")
    print("="*70 + "\n")


def main():
    """Main initialization function"""
    logger.info("Starting database initialization...")
    logger.info(f"Connecting to PostgreSQL: {db_config.host}:{db_config.port}/{db_config.name}")

    with app.app_context():
        try:
            # Step 1: Create all tables
            if not create_tables():
                logger.error("Failed to create database tables")
                return False

            # Step 2: Create default Global team
            if not create_default_team():
                logger.error("Failed to create default team")
                return False

            # Step 3: Seed resource types
            if not seed_resource_types():
                logger.error("Failed to seed resource types")
                return False

            # Step 4: Verify setup
            if not verify_database():
                logger.error("Database verification failed")
                return False

            # Step 5: Print success message
            print_success_summary()
            logger.info("Database initialization completed successfully")
            return True

        except Exception as e:
            logger.error(f"Unexpected error during initialization: {e}")
            return False


if __name__ == '__main__':
    try:
        success = main()
        sys.exit(0 if success else 1)
    except KeyboardInterrupt:
        logger.info("Initialization cancelled by user")
        sys.exit(1)
    except Exception as e:
        logger.error(f"Fatal error: {e}")
        sys.exit(1)
