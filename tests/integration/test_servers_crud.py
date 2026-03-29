"""Integration tests for database servers CRUD operations.

Tests the /api/v1/servers endpoints with mocked PyDAL database.
"""
import pytest
import sys
import os
from unittest.mock import patch, MagicMock

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'apps', 'manager'))

from apps.manager.app import app
from apps.manager.utils.auth import create_token


@pytest.fixture
def client():
    """Create a Quart test client."""
    app.config['TESTING'] = True
    return app.test_client()


def _auth_header(role: str = 'admin') -> dict:
    """Create a Bearer token Authorization header."""
    token = create_token(user_id=1, email='admin@test.local', role=role)
    return {'Authorization': f'Bearer {token}'}


def _mock_server_row(server_id: int = 1) -> MagicMock:
    """Create a mock PyDAL database_server row."""
    row = MagicMock()
    row.id = server_id
    row.as_dict.return_value = {
        'id': server_id,
        'name': 'test-pg',
        'host': 'db.example.com',
        'port': 5432,
        'db_type': 'postgresql',
        'username': 'dbuser',
        'region': 'us-east-1',
        'tags': 'prod',
        'active': True,
        'created_by': 1,
    }
    return row


@pytest.mark.asyncio
async def test_list_servers(client):
    """GET /servers returns paginated list."""
    row = _mock_server_row()
    with patch('apps.manager.routes.database_servers.db') as db_mock:
        query_mock = MagicMock()
        query_mock.select.return_value = [row]
        query_mock.count.return_value = 1
        db_mock.return_value = query_mock
        db_mock.database_server.active = True
        db_mock.database_server.id = 'id'

        response = await client.get('/api/v1/servers')

    assert response.status_code == 200
    data = await response.get_json()
    assert 'data' in data
    assert 'meta' in data


@pytest.mark.asyncio
async def test_create_server(client):
    """POST /servers creates a new server entry."""
    row = _mock_server_row()
    with patch('apps.manager.routes.database_servers.db') as db_mock:
        db_mock.database_server.insert.return_value = 1
        db_mock.database_server.__getitem__ = MagicMock(return_value=row)

        response = await client.post(
            '/api/v1/servers',
            json={
                'name': 'test-pg',
                'host': 'db.example.com',
                'port': 5432,
                'db_type': 'postgresql',
            },
            headers=_auth_header(),
        )

    assert response.status_code == 201
    data = await response.get_json()
    assert data['data']['name'] == 'test-pg'


@pytest.mark.asyncio
async def test_create_server_missing_fields(client):
    """POST /servers with missing required fields returns 400."""
    response = await client.post(
        '/api/v1/servers',
        json={'name': 'incomplete'},
        headers=_auth_header(),
    )
    assert response.status_code == 400
    data = await response.get_json()
    assert 'Missing required fields' in data['error']


@pytest.mark.asyncio
async def test_create_server_no_auth(client):
    """POST /servers without auth returns 401."""
    response = await client.post(
        '/api/v1/servers',
        json={'name': 'test', 'host': 'h', 'port': 5432, 'db_type': 'pg'},
    )
    assert response.status_code == 401


@pytest.mark.asyncio
async def test_get_server(client):
    """GET /servers/<id> returns a single server."""
    row = _mock_server_row(server_id=42)
    with patch('apps.manager.routes.database_servers.db') as db_mock:
        db_mock.database_server.__getitem__ = MagicMock(return_value=row)

        response = await client.get(
            '/api/v1/servers/42',
            headers=_auth_header(),
        )

    assert response.status_code == 200
    data = await response.get_json()
    assert data['data']['id'] == 42


@pytest.mark.asyncio
async def test_get_server_not_found(client):
    """GET /servers/<id> for nonexistent server returns 404."""
    with patch('apps.manager.routes.database_servers.db') as db_mock:
        db_mock.database_server.__getitem__ = MagicMock(return_value=None)

        response = await client.get(
            '/api/v1/servers/999',
            headers=_auth_header(),
        )

    assert response.status_code == 404


@pytest.mark.asyncio
async def test_update_server(client):
    """PUT /servers/<id> updates server fields."""
    row = _mock_server_row()
    updated_dict = row.as_dict()
    updated_dict['name'] = 'renamed-pg'
    updated_row = MagicMock()
    updated_row.as_dict.return_value = updated_dict

    with patch('apps.manager.routes.database_servers.db') as db_mock:
        db_mock.database_server.__getitem__ = MagicMock(
            side_effect=[row, updated_row]
        )
        db_mock.return_value.update = MagicMock()

        response = await client.put(
            '/api/v1/servers/1',
            json={'name': 'renamed-pg'},
            headers=_auth_header(),
        )

    assert response.status_code == 200


@pytest.mark.asyncio
async def test_delete_server_as_admin(client):
    """DELETE /servers/<id> as admin soft-deletes the server."""
    row = _mock_server_row()
    with patch('apps.manager.routes.database_servers.db') as db_mock:
        db_mock.database_server.__getitem__ = MagicMock(return_value=row)
        db_mock.return_value.update = MagicMock()

        response = await client.delete(
            '/api/v1/servers/1',
            headers=_auth_header(role='admin'),
        )

    assert response.status_code == 200
    data = await response.get_json()
    assert 'deactivated' in data['message'].lower()


@pytest.mark.asyncio
async def test_delete_server_as_viewer(client):
    """DELETE /servers/<id> as viewer returns 403."""
    response = await client.delete(
        '/api/v1/servers/1',
        headers=_auth_header(role='viewer'),
    )
    assert response.status_code == 403
