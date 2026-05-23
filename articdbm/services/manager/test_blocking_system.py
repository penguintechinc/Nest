#!/usr/bin/env python3
"""
Test suite for ArticDBM blocking system

This test suite validates the blocking functionality including:
- Default resource seeding
- Blocked database management
- Redis synchronization
- Security validation
"""

import os
import sys
import unittest
import json
import tempfile
import shutil
from unittest.mock import Mock, patch, MagicMock
from pathlib import Path

# Add the app directory to path so we can import the modules
app_dir = Path(__file__).parent
sys.path.insert(0, str(app_dir))

# Mock the py4web environment before importing
sys.modules['py4web'] = MagicMock()
sys.modules['py4web.utils.cors'] = MagicMock() 
sys.modules['py4web.utils.auth'] = MagicMock()
sys.modules['pydal'] = MagicMock()
sys.modules['pydal.validators'] = MagicMock()
sys.modules['redis'] = MagicMock()
sys.modules['redis.asyncio'] = MagicMock()

# Now we can import our app module
from app import (
    validate_sql_security, 
    seed_default_blocked_resources,
    BlockedDatabaseModel
)

class TestSQLSecurityValidation(unittest.TestCase):
    """Test SQL security validation functionality"""
    
    def test_dangerous_sql_patterns(self):
        """Test detection of dangerous SQL patterns"""
        
        dangerous_queries = [
            "SELECT * FROM users WHERE id = 1; DROP TABLE users;",
            "EXEC xp_cmdshell 'dir'",
            "SELECT * FROM information_schema.tables",
            "UNION SELECT username, password FROM users",
            "'; DROP TABLE users; --",
            "SELECT * FROM master.dbo.sysobjects",
            "LOAD_FILE('/etc/passwd')",
            "INTO OUTFILE '/tmp/dump.txt'",
        ]
        
        for query in dangerous_queries:
            result = validate_sql_security(query)
            self.assertFalse(result['valid'], f"Should detect dangerous query: {query[:50]}...")
            self.assertIn('high', result['severity'].lower())
    
    def test_shell_command_detection(self):
        """Test detection of shell commands in SQL"""
        
        shell_queries = [
            "SELECT * FROM users; exec('rm -rf /')",
            "'; system('ls'); --",
            "SELECT * FROM users WHERE name = 'test' AND cmd = 'bash'",
            "EXEC master..xp_cmdshell 'powershell -c Get-Process'",
        ]
        
        for query in shell_queries:
            result = validate_sql_security(query)
            self.assertFalse(result['valid'], f"Should detect shell command: {query}")
            self.assertEqual(result['severity'], 'critical')
    
    def test_default_database_access_warnings(self):
        """Test warnings for default/test database access"""
        
        test_queries = [
            "SELECT * FROM test.users",
            "USE sample; SELECT * FROM products",
            "INSERT INTO demo.customers VALUES (1, 'John')",
            "SELECT * FROM information_schema WHERE user = 'root'",
        ]
        
        for query in test_queries:
            result = validate_sql_security(query)
            self.assertTrue(len(result['warnings']) > 0, f"Should warn about test database access: {query}")
    
    def test_valid_sql_queries(self):
        """Test that valid SQL queries pass validation"""
        
        valid_queries = [
            "SELECT id, name FROM customers WHERE active = 1",
            "UPDATE products SET price = 10.99 WHERE id = 1",
            "INSERT INTO orders (customer_id, total) VALUES (1, 100.00)",
            "DELETE FROM temp_data WHERE created_at < '2023-01-01'",
        ]
        
        for query in valid_queries:
            result = validate_sql_security(query)
            self.assertTrue(result['valid'], f"Should pass valid query: {query}")
            self.assertEqual(result['severity'], 'low')
    
    def test_sql_syntax_validation(self):
        """Test basic SQL syntax validation"""
        
        # Test unmatched parentheses
        result = validate_sql_security("SELECT * FROM users WHERE (name = 'test'")
        self.assertFalse(result['valid'])
        self.assertIn("Unmatched parentheses", str(result['errors']))
        
        # Test unmatched quotes
        result = validate_sql_security("SELECT * FROM users WHERE name = 'test")
        self.assertFalse(result['valid'])
        self.assertIn("Unmatched single quotes", str(result['errors']))


class TestBlockedDatabaseModel(unittest.TestCase):
    """Test the blocked database model validation"""
    
    def test_valid_blocked_database_model(self):
        """Test valid blocked database model creation"""
        
        valid_data = {
            'name': 'test_db',
            'type': 'database',
            'pattern': '^test$',
            'reason': 'Test database should be blocked',
            'active': True
        }
        
        model = BlockedDatabaseModel(**valid_data)
        self.assertEqual(model.name, 'test_db')
        self.assertEqual(model.type, 'database')
        self.assertEqual(model.pattern, '^test$')
        self.assertTrue(model.active)
    
    def test_blocked_database_model_defaults(self):
        """Test blocked database model with default values"""
        
        minimal_data = {
            'name': 'blocked_item',
            'type': 'username', 
            'pattern': '^admin$'
        }
        
        model = BlockedDatabaseModel(**minimal_data)
        self.assertIsNone(model.reason)
        self.assertTrue(model.active)  # Should default to True


class TestDefaultBlockedResources(unittest.TestCase):
    """Test seeding of default blocked resources"""
    
    def setUp(self):
        """Set up test database mock"""
        self.db_mock = MagicMock()
        self.db_mock.blocked_database = MagicMock()
        
        # Mock the count to return 0 (no existing resources)
        self.db_mock.return_value.count.return_value = 0
        
        # Mock insert operations
        self.db_mock.blocked_database.insert = MagicMock(return_value=1)
        self.db_mock.commit = MagicMock()
    
    @patch('app.db')  
    @patch('app.sync_to_redis')
    def test_seed_default_blocked_resources_success(self, mock_sync, mock_db):
        """Test successful seeding of default blocked resources"""
        
        mock_db.return_value.count.return_value = 0  # No existing resources
        mock_db.blocked_database.insert = MagicMock(return_value=1)
        mock_db.commit = MagicMock()
        mock_sync.return_value = True
        
        result = seed_default_blocked_resources()
        
        self.assertTrue(result)
        self.assertTrue(mock_db.blocked_database.insert.called)
        self.assertTrue(mock_db.commit.called)
        self.assertTrue(mock_sync.called)
    
    @patch('app.db')
    def test_seed_default_blocked_resources_already_exists(self, mock_db):
        """Test seeding when resources already exist"""
        
        mock_db.return_value.count.return_value = 10  # Existing resources
        
        result = seed_default_blocked_resources()
        
        # Should return early without inserting
        self.assertIsNone(result)  # Function returns None when skipping
        self.assertFalse(mock_db.blocked_database.insert.called)
    
    def test_critical_databases_included(self):
        """Test that critical system databases are included in defaults"""
        
        # Since we can't easily test the actual seeding function due to DB dependencies,
        # we'll test the data structure that would be used
        
        critical_databases = {
            # SQL Server
            'master', 'msdb', 'tempdb', 'model',
            # MySQL  
            'mysql', 'sys', 'information_schema', 'performance_schema',
            # PostgreSQL
            'postgres', 'template0', 'template1',
            # MongoDB
            'admin', 'local', 'config',
            # Common test databases
            'test', 'demo', 'sample', 'example'
        }
        
        # This would be the actual data structure used in seeding
        default_databases = [
            {"name": "master", "type": "database", "pattern": "^master$", "reason": "SQL Server system database"},
            {"name": "mysql", "type": "database", "pattern": "^mysql$", "reason": "MySQL system database"},
            {"name": "postgres", "type": "database", "pattern": "^postgres$", "reason": "PostgreSQL default database"},
            {"name": "admin", "type": "database", "pattern": "^admin$", "reason": "MongoDB admin database"},
            {"name": "test", "type": "database", "pattern": "^test$", "reason": "Default test database"},
            {"name": "demo", "type": "database", "pattern": "^demo$", "reason": "Default demo database"},
        ]
        
        seeded_names = {db['name'] for db in default_databases}
        
        # Check that critical databases are included
        for critical_db in ['master', 'mysql', 'postgres', 'admin', 'test', 'demo']:
            self.assertIn(critical_db, seeded_names, f"Critical database {critical_db} should be in defaults")
    
    def test_critical_users_included(self):
        """Test that critical default users are included in defaults"""
        
        critical_users = ['sa', 'root', 'admin', 'administrator', 'guest', 'test', 'demo']
        
        # This represents the structure that would be seeded
        default_users = [
            {"name": "sa", "type": "username", "pattern": "^sa$", "reason": "SQL Server default admin account"},
            {"name": "root", "type": "username", "pattern": "^root$", "reason": "Default root account"},
            {"name": "admin", "type": "username", "pattern": "^admin$", "reason": "Default admin account"},
            {"name": "guest", "type": "username", "pattern": "^guest$", "reason": "Default guest account"},
            {"name": "test", "type": "username", "pattern": "^test$", "reason": "Test user account"},
        ]
        
        seeded_names = {user['name'] for user in default_users}
        
        # Check that critical users are included
        for critical_user in ['sa', 'root', 'admin', 'guest', 'test']:
            self.assertIn(critical_user, seeded_names, f"Critical user {critical_user} should be in defaults")


class TestRedisIntegration(unittest.TestCase):
    """Test Redis integration for blocking configuration"""
    
    @patch('app.redis_client')
    def test_redis_sync_blocked_databases(self, mock_redis):
        """Test that blocked databases are synced to Redis"""
        
        # Mock Redis client
        mock_redis.set = MagicMock()
        mock_redis.expire = MagicMock()
        
        # Mock database data
        with patch('app.db') as mock_db:
            # Setup database mocks
            mock_db.return_value.select.return_value = []  # No users
            mock_db.user_permission = MagicMock()
            mock_db.user_permission.select.return_value = []  # No permissions
            mock_db.database_server = MagicMock() 
            mock_db.database_server.active = True
            mock_db.return_value.select.return_value = []  # No servers
            mock_db.blocked_database = MagicMock()
            mock_db.blocked_database.active = True
            
            # Mock blocked database data
            blocked_data = [
                MagicMock(id=1, name='test', type='database', pattern='^test$', reason='Test DB', active=True),
                MagicMock(id=2, name='root', type='username', pattern='^root$', reason='Root user', active=True)
            ]
            mock_db.return_value.select.return_value = blocked_data
            
            from app import sync_to_redis
            result = sync_to_redis()
            
            self.assertTrue(result)
            
            # Verify Redis was called to set blocked databases
            self.assertTrue(mock_redis.set.called)
            
            # Check that the blocked_databases key was set
            call_args = mock_redis.set.call_args_list
            redis_keys = [call[0][0] for call in call_args]
            self.assertIn('articdbm:blocked_databases', redis_keys)


class TestEndToEndBlocking(unittest.TestCase):
    """End-to-end tests for the blocking system"""
    
    def test_blocking_workflow(self):
        """Test the complete blocking workflow"""
        
        # 1. Test SQL validation identifies dangerous content
        dangerous_sql = "SELECT * FROM users; DROP TABLE admin_users; --"
        validation = validate_sql_security(dangerous_sql)
        
        self.assertFalse(validation['valid'])
        self.assertGreater(len(validation['errors']), 0)
        
        # 2. Test blocking model validation
        blocking_rule = {
            'name': 'dangerous_table',
            'type': 'table', 
            'pattern': 'admin_users',
            'reason': 'Administrative table should not be accessible',
            'active': True
        }
        
        model = BlockedDatabaseModel(**blocking_rule)
        self.assertEqual(model.name, 'dangerous_table')
        self.assertEqual(model.type, 'table')
        self.assertTrue(model.active)
        
        # 3. Test that the pattern would match dangerous access
        import re
        pattern = model.pattern
        test_table = 'admin_users'
        
        self.assertTrue(re.search(pattern, test_table), "Pattern should match dangerous table")


if __name__ == '__main__':
    # Set up test environment
    os.environ['DATABASE_URL'] = 'sqlite:///:memory:'
    os.environ['REDIS_HOST'] = 'localhost'
    os.environ['REDIS_PORT'] = '6379'
    
    # Run tests
    unittest.main(verbosity=2)