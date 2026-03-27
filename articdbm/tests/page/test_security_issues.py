"""Tests for security_issues page load and content verification."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_manager = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1 to run page tests (requires running services)",
)


def _get_auth_session():
    """Get an authenticated session for page tests."""
    session = requests.Session()
    try:
        session.post(
            f"{MANAGER_URL}/auth/api/register",
            json={
                "email": "pagetest@articdbm.test",
                "password": "TestPass123!",
                "first_name": "Page",
                "last_name": "Tester",
            },
            timeout=10,
        )
    except requests.RequestException:
        pass
    try:
        resp = session.post(
            f"{MANAGER_URL}/auth/api/login",
            json={
                "email": "pagetest@articdbm.test",
                "password": "TestPass123!",
            },
            timeout=10,
        )
        if resp.status_code != 200:
            return None
    except requests.RequestException:
        return None
    return session


@skip_if_no_manager
class TestSecurityIssuesPage:
    """Tests for the /security_issues page."""

    @pytest.fixture(autouse=True, scope="class")
    def setup_session(self, request):
        """Set up authenticated session for all tests in this class."""
        session = _get_auth_session()
        if session is None:
            pytest.skip("Cannot authenticate for page tests")
        request.cls.session = session
        request.cls.base_url = MANAGER_URL

    def _get_page(self):
        """Fetch the security_issues page."""
        resp = self.session.get(
            f"{self.base_url}/security_issues",
            timeout=15,
        )
        return resp

    def test_requires_auth(self):
        """Unauthenticated GET should redirect or return login."""
        anon = requests.Session()
        resp = anon.get(
            f"{self.base_url}/security_issues",
            timeout=15,
            allow_redirects=False,
        )
        # Should redirect to login or return 303/401/403
        assert resp.status_code in (301, 302, 303, 401, 403), (
            f"Expected auth redirect, got {resp.status_code}"
        )

    def test_returns_html(self):
        """Authenticated GET should return 200 with text/html."""
        resp = self._get_page()
        assert resp.status_code == 200
        assert "text/html" in resp.headers.get("Content-Type", "")

    def test_contains_bulma_css(self):
        """Page should include Bulma CSS framework."""
        resp = self._get_page()
        assert "bulma" in resp.text.lower()

    def test_contains_summary_section(self):
        """Page should have security score summary section."""
        resp = self._get_page()
        assert "security-score" in resp.text

    def test_contains_server_cards(self):
        """Page should have server card elements."""
        resp = self._get_page()
        assert "server-card" in resp.text

    def test_contains_fix_modal(self):
        """Page should contain the fix suggestion modal."""
        resp = self._get_page()
        assert 'id="fixModal"' in resp.text or "fixModal" in resp.text

    def test_js_functions_present(self):
        """Page should contain all required JavaScript functions."""
        resp = self._get_page()
        expected_functions = [
            "toggleServerDetails",
            "runSecurityScan",
            "getFixSuggestions",
            "showFixModal",
            "closeFixModal",
            "exportReport",
            "viewSchedulerStatus",
            "scheduleScans",
        ]
        for func_name in expected_functions:
            assert func_name in resp.text, (
                f"Missing JS function: {func_name}"
            )

    def test_action_buttons_present(self):
        """Page should have Run Scan, Export Report, Schedule buttons."""
        resp = self._get_page()
        text = resp.text.lower()
        assert "run" in text and "scan" in text, "Missing Run Scan button"
        assert "export" in text and "report" in text, "Missing Export Report button"
        assert "schedule" in text, "Missing Schedule button"

    def test_contains_font_awesome(self):
        """Page should include Font Awesome icons."""
        resp = self._get_page()
        assert "font-awesome" in resp.text.lower() or "fontawesome" in resp.text.lower()

    def test_loading_overlay_present(self):
        """Page should have loading overlay element."""
        resp = self._get_page()
        assert "loadingOverlay" in resp.text
