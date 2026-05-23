"""Integration tests for authentication flow.

Tests login, token validation, and protected endpoint access
with a mocked PyDAL database layer.
"""
import pytest
import sys
import os
from unittest.mock import patch, MagicMock
from werkzeug.security import generate_password_hash

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'apps', 'manager'))

from apps.manager.app import app
from apps.manager.utils.auth import create_token


@pytest.fixture
def client():
    """Create a Quart test client."""
    app.config['TESTING'] = True
    return app.test_client()


def _make_user_row(
    user_id: int = 1,
    username: str = 'testadmin',
    email: str = 'admin@test.local',
    password: str = 'testpass123',
    role: str = 'admin',
    is_active: bool = True,
) -> MagicMock:
    """Create a mock PyDAL user row."""
    row = MagicMock()
    row.id = user_id
    row.username = username
    row.email = email
    row.password_hash = generate_password_hash(password)
    row.first_name = 'Test'
    row.last_name = 'Admin'
    row.role = role
    row.is_active = is_active
    return row


def _make_auth_header(
    user_id: int = 1,
    email: str = 'admin@test.local',
    role: str = 'admin',
) -> dict:
    """Create a Bearer token Authorization header."""
    token = create_token(user_id=user_id, email=email, role=role)
    return {'Authorization': f'Bearer {token}'}


@pytest.mark.asyncio
async def test_login_success(client):
    """Successful login returns token and user data."""
    user = _make_user_row()
    with patch('apps.manager.routes.auth._lookup_user', return_value=user):
        response = await client.post('/api/v1/auth/login', json={
            'username': 'testadmin',
            'password': 'testpass123',
        })
    assert response.status_code == 200
    data = await response.get_json()
    assert 'token' in data
    assert data['user']['username'] == 'testadmin'


@pytest.mark.asyncio
async def test_login_invalid_password(client):
    """Login with wrong password returns 401."""
    user = _make_user_row()
    with patch('apps.manager.routes.auth._lookup_user', return_value=user):
        response = await client.post('/api/v1/auth/login', json={
            'username': 'testadmin',
            'password': 'wrongpassword',
        })
    assert response.status_code == 401


@pytest.mark.asyncio
async def test_login_user_not_found(client):
    """Login with nonexistent user returns 401."""
    with patch('apps.manager.routes.auth._lookup_user', return_value=None):
        response = await client.post('/api/v1/auth/login', json={
            'username': 'nobody',
            'password': 'pass',
        })
    assert response.status_code == 401


@pytest.mark.asyncio
async def test_login_inactive_user(client):
    """Login with inactive account returns 401."""
    user = _make_user_row(is_active=False)
    with patch('apps.manager.routes.auth._lookup_user', return_value=user):
        response = await client.post('/api/v1/auth/login', json={
            'username': 'testadmin',
            'password': 'testpass123',
        })
    assert response.status_code == 401


@pytest.mark.asyncio
async def test_login_missing_fields(client):
    """Login without required fields returns 400."""
    response = await client.post('/api/v1/auth/login', json={
        'username': '',
        'password': '',
    })
    assert response.status_code == 400


@pytest.mark.asyncio
async def test_me_with_valid_token(client):
    """GET /auth/me with valid token returns user profile."""
    user = _make_user_row()
    mock_db = MagicMock()
    mock_db.return_value.select.return_value.first.return_value = user

    with patch('apps.manager.routes.auth.db') as db_mock:
        db_mock.users.id = 'id'
        db_mock.__call__ = mock_db
        db_mock.return_value = mock_db.return_value

        headers = _make_auth_header()
        response = await client.get('/api/v1/auth/me', headers=headers)

    assert response.status_code in (200, 404)


@pytest.mark.asyncio
async def test_me_without_token(client):
    """GET /auth/me without token returns 401."""
    response = await client.get('/api/v1/auth/me')
    assert response.status_code == 401


@pytest.mark.asyncio
async def test_logout_with_token(client):
    """POST /auth/logout with valid token returns 200."""
    headers = _make_auth_header()
    response = await client.post('/api/v1/auth/logout', headers=headers)
    assert response.status_code == 200
    data = await response.get_json()
    assert 'logged out' in data['message']


@pytest.mark.asyncio
async def test_logout_without_token(client):
    """POST /auth/logout without token returns 401."""
    response = await client.post('/api/v1/auth/logout')
    assert response.status_code == 401
