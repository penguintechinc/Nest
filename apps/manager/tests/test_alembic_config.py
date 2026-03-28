"""Tests that Alembic migration infrastructure is properly configured."""
from pathlib import Path

BASE = Path(__file__).parent.parent


def test_alembic_ini_exists():
    assert (BASE / "alembic.ini").exists(), "alembic.ini must exist in apps/manager/"


def test_alembic_migrations_dir_exists():
    assert (BASE / "migrations" / "versions").is_dir(), \
        "migrations/versions/ directory must exist"


def test_alembic_env_exists():
    assert (BASE / "migrations" / "env.py").exists(), \
        "migrations/env.py must exist"


def test_initial_migration_exists():
    versions = BASE / "migrations" / "versions"
    migration_files = [f for f in versions.glob("*.py") if not f.name.startswith("_")]
    assert len(migration_files) >= 1, "At least one migration file required"
