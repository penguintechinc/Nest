"""Smoke tests for Docker image builds and compose config validation."""
import os
import subprocess
import pytest

PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

skip_if_no_docker = pytest.mark.skipif(
    os.system("docker info > /dev/null 2>&1") != 0,
    reason="Docker not available",
)


@skip_if_no_docker
class TestDockerBuilds:
    """Verify Docker images build successfully."""

    @pytest.mark.timeout(300)
    def test_manager_image_builds(self):
        """Manager Dockerfile should build without errors."""
        result = subprocess.run(
            ["docker", "build", "-t", "articdbm-manager-test", "./services/manager"],
            capture_output=True,
            text=True,
            timeout=300,
            cwd=PROJECT_ROOT,
        )
        assert result.returncode == 0, f"Manager build failed:\n{result.stderr}"

    @pytest.mark.timeout(300)
    def test_proxy_image_builds(self):
        """Proxy Dockerfile should build without errors."""
        result = subprocess.run(
            ["docker", "build", "-t", "articdbm-proxy-test", "./services/proxy"],
            capture_output=True,
            text=True,
            timeout=300,
            cwd=PROJECT_ROOT,
        )
        assert result.returncode == 0, f"Proxy build failed:\n{result.stderr}"

    @pytest.mark.timeout(60)
    def test_docker_compose_config_valid(self):
        """docker-compose.dev.yml should be valid configuration."""
        result = subprocess.run(
            ["docker", "compose", "-f", "docker-compose.dev.yml", "config"],
            capture_output=True,
            text=True,
            timeout=60,
            cwd=PROJECT_ROOT,
        )
        assert result.returncode == 0, f"Compose config invalid:\n{result.stderr}"
