"""Tests that the pydal → penguin-dal migration is complete."""
from pathlib import Path

MANAGER_DIR = Path(__file__).parent.parent


def test_pydal_not_imported_in_models():
    """No model file should import directly from pydal."""
    for py_file in (MANAGER_DIR / "models").glob("*.py"):
        source = py_file.read_text()
        assert "from pydal import" not in source, \
            f"{py_file.name} still imports from pydal"
        assert "pydal.validators" not in source, \
            f"{py_file.name} still uses pydal.validators"


def test_penguin_dal_used_in_models_init():
    """models/__init__.py must import from penguin_dal."""
    source = (MANAGER_DIR / "models" / "__init__.py").read_text()
    assert "from penguin_dal import" in source or "import penguin_dal" in source, \
        "models/__init__.py must import penguin_dal"


def test_no_migrate_true_in_models():
    """No model file should have migrate=True."""
    for py_file in (MANAGER_DIR / "models").glob("*.py"):
        assert "migrate=True" not in py_file.read_text(), \
            f"{py_file.name} still has migrate=True"


def test_db_instance_is_penguin_dal():
    """The db object in models/__init__.py must be a penguin_dal DB instance."""
    import sys
    sys.path.insert(0, str(MANAGER_DIR.parent.parent))
    # Just check the source has DB (not DAL) as the class name
    source = (MANAGER_DIR / "models" / "__init__.py").read_text()
    assert "= DB(" in source or "= AsyncDB(" in source, \
        "models/__init__.py must instantiate DB or AsyncDB, not DAL"
