"""Alembic environment configuration.

IMPORTANT: Alembic migrations are run MANUALLY only — never from application startup.
Run via: ./scripts/migrate.sh [command]

This file reads DB_URI from the environment so no credentials are hardcoded.
"""

import os
from logging.config import fileConfig

from sqlalchemy import engine_from_config, pool
from alembic import context

# Alembic Config object — provides access to values in alembic.ini
config = context.config

# Configure Python logging from alembic.ini [loggers] section
if config.config_file_name is not None:
    fileConfig(config.config_file_name)

# Import all SQLAlchemy models so Alembic can detect schema changes.
# db_models defines Base and imports all table models (existing + articdbm).
from db_models import Base  # noqa: E402

target_metadata = Base.metadata


def get_url() -> str:
    """Return the database URL from the environment.

    Never hardcodes credentials — always reads from DB_URI env var.
    Falls back to the local development default only.
    """
    return os.environ.get("DB_URI", "postgresql://nest:nest@localhost:5432/nest")


def run_migrations_offline() -> None:
    """Run migrations in 'offline' mode.

    Configures the context with just a URL (no Engine). Calls to
    context.execute() emit SQL to the script output.
    """
    url = get_url()
    context.configure(
        url=url,
        target_metadata=target_metadata,
        literal_binds=True,
        dialect_opts={"paramstyle": "named"},
    )
    with context.begin_transaction():
        context.run_migrations()


def run_migrations_online() -> None:
    """Run migrations in 'online' mode.

    Creates an Engine and associates a connection with the context.
    Uses NullPool so connections are not pooled during migration runs.
    """
    configuration = config.get_section(config.config_ini_section, {})
    configuration["sqlalchemy.url"] = get_url()
    connectable = engine_from_config(
        configuration,
        prefix="sqlalchemy.",
        poolclass=pool.NullPool,
    )
    with connectable.connect() as connection:
        context.configure(
            connection=connection,
            target_metadata=target_metadata,
        )
        with context.begin_transaction():
            context.run_migrations()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()
