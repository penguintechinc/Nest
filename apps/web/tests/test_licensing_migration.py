"""Tests that apps/web uses penguin_licensing, not shared.licensing."""
from pathlib import Path

APP_PY = Path(__file__).parent.parent / "app.py"


def test_no_shared_licensing_import():
    source = APP_PY.read_text()
    assert "shared.licensing" not in source, \
        "apps/web/app.py must not import from shared.licensing"
    assert "shared/licensing" not in source, \
        "apps/web/app.py must not reference shared/licensing"


def test_penguin_licensing_imported():
    source = APP_PY.read_text()
    assert "penguin_licensing" in source, \
        "apps/web/app.py must import from penguin_licensing"
