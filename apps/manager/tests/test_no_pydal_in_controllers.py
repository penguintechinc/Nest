"""Tests that no controller or route directly imports pydal."""
from pathlib import Path

MANAGER_DIR = Path(__file__).parent.parent


def _py_files_in(*dirs):
    for d in dirs:
        path = MANAGER_DIR / d
        if path.exists():
            yield from path.glob("**/*.py")


def test_no_pydal_imports_in_routes():
    """Routes should not import from pydal or models.db directly."""
    for f in _py_files_in("routes"):
        source = f.read_text()
        assert "from pydal import" not in source, \
            f"{f.relative_to(MANAGER_DIR)} still imports from pydal"
        assert "import pydal" not in source, \
            f"{f.relative_to(MANAGER_DIR)} still imports pydal"


def test_no_pydal_imports_in_controllers():
    """Controllers should not import from pydal or models.db directly."""
    for f in _py_files_in("controllers"):
        source = f.read_text()
        assert "from pydal import" not in source, \
            f"{f.relative_to(MANAGER_DIR)} still imports from pydal"
        assert "import pydal" not in source, \
            f"{f.relative_to(MANAGER_DIR)} still imports pydal"
