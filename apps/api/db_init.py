"""Database initialization utilities."""

import os

from sqlalchemy import create_engine

from db_models import Base


def init_db() -> None:
    """Initialize database schema using SQLAlchemy.

    This is idempotent - it only creates tables that don't exist.
    """
    db_type = os.getenv("DB_TYPE", "sqlite").lower()
    db_host = os.getenv("DB_HOST", "localhost")
    db_port = os.getenv("DB_PORT", "")
    db_name = os.getenv("DB_NAME", "nest_db")
    db_user = os.getenv("DB_USER", "")
    db_pass = os.getenv("DB_PASS", "")

    if db_type == "postgresql":
        port = db_port or "5432"
        uri = f"postgresql://{db_user}:{db_pass}@{db_host}:{port}/{db_name}"
    elif db_type == "mysql":
        port = db_port or "3306"
        uri = f"mysql+pymysql://{db_user}:{db_pass}@{db_host}:{port}/{db_name}"
    elif db_type == "sqlite":
        # SQLite uses local file path or :memory:
        db_file = os.getenv("DB_FILE", "nest.db")
        uri = f"sqlite:///{db_file}"
    else:
        raise ValueError(f"Unsupported DB_TYPE: {db_type}")

    engine = create_engine(uri, echo=False)
    Base.metadata.create_all(engine)
    engine.dispose()
