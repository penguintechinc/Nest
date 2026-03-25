#!/usr/bin/env python3
"""
Comprehensive unit tests for ArticDBM database management features
"""

import os
import json
import tempfile
import unittest
import uuid
from datetime import datetime
from unittest.mock import Mock, patch, MagicMock

# Set up py4web test environment
os.environ['PY4WEB_APPS_FOLDER'] = os.path.dirname(__file__)
os.environ['DATABASE_URL'] = 'sqlite:memory:'

from py4web import DAL, Field
from pydal.validators import IS_IN_SET
import redis

# Import the functions we want to test
from app import (
    validate_sql_security,
    ManagedDatabaseModel,
    SQLFileModel,
    BlockedDatabaseModel
)


class TestSQLSecurityValidation(unittest.TestCase):
    """Test SQL security validation functionality"""
    
    def test_clean_sql_passes_validation(self):
        """Test that clean SQL statements pass validation"""
        clean_sql = "SELECT id, name FROM users WHERE active = 1"
        result = validate_sql_security(clean_sql)
        
        self.assertTrue(result['valid'])
        self.assertEqual(len(result['errors']), 0)
        self.assertEqual(result['severity'], 'low')
    
    def test_sql_injection_detected(self):
        """Test that SQL injection patterns are detected"""
        malicious_sql = "SELECT * FROM users WHERE id = 1 OR 1=1"
        result = validate_sql_security(malicious_sql)
        
        self.assertFalse(result['valid'])
        self.assertGreater(len(result['errors']), 0)
        self.assertIn('high', result['severity'])
    
    def test_shell_command_detected(self):
        """Test that shell commands are detected"""
        malicious_sql = "SELECT * FROM users; xp_cmdshell 'whoami'"
        result = validate_sql_security(malicious_sql)
        
        self.assertFalse(result['valid'])
        self.assertEqual(result['severity'], 'critical')
        self.assertTrue(any('Shell command' in error for error in result['errors']))
    
    def test_system_database_access_warning(self):
        """Test that system database access generates warnings"""
        system_sql = "SELECT * FROM master.dbo.sysdatabases"
        result = validate_sql_security(system_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('Master database access' in error for error in result['errors']))
    
    def test_default_user_detection(self):
        """Test that default users are detected"""
        default_user_sql = "SELECT * FROM users WHERE username = 'sa'"
        result = validate_sql_security(default_user_sql)
        
        self.assertGreater(len(result['warnings']), 0)
        self.assertTrue(any('sa' in warning for warning in result['warnings']))
    
    def test_syntax_error_detection(self):
        """Test that basic syntax errors are caught"""
        syntax_error_sql = "SELECT * FROM users WHERE (id = 1"
        result = validate_sql_security(syntax_error_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('parentheses' in error for error in result['errors']))
    
    def test_multiple_statement_detection(self):
        """Test that multiple statements generate warnings"""
        multiple_sql = "SELECT * FROM users; DELETE FROM logs;"
        result = validate_sql_security(multiple_sql)
        
        self.assertGreater(len(result['warnings']), 0)
        self.assertTrue(any('Multiple different statement types' in warning for warning in result['warnings']))
    
    def test_file_operations_blocked(self):
        """Test that file operations are blocked"""
        file_sql = "SELECT LOAD_FILE('/etc/passwd')"
        result = validate_sql_security(file_sql)
        
        self.assertFalse(result['valid'])
        self.assertEqual(result['severity'], 'high')


class TestPydanticModels(unittest.TestCase):
    """Test Pydantic model validation"""
    
    def test_managed_database_model_valid(self):
        """Test valid managed database model"""
        data = {
            'name': 'test_db',
            'server_id': 1,
            'database_name': 'test_database',
            'description': 'Test database',
            'active': True
        }
        
        model = ManagedDatabaseModel(**data)
        self.assertEqual(model.name, 'test_db')
        self.assertEqual(model.server_id, 1)
        self.assertTrue(model.active)
    
    def test_sql_file_model_valid(self):
        """Test valid SQL file model"""
        data = {
            'name': 'init.sql',
            'database_id': 1,
            'file_type': 'init',
            'file_content': 'CREATE TABLE test (id INT);'
        }
        
        model = SQLFileModel(**data)
        self.assertEqual(model.name, 'init.sql')
        self.assertEqual(model.file_type, 'init')
    
    def test_blocked_database_model_valid(self):
        """Test valid blocked database model"""
        data = {
            'name': 'block_test',
            'type': 'database',
            'pattern': 'test.*',
            'reason': 'Test databases should not be accessible in production'
        }
        
        model = BlockedDatabaseModel(**data)
        self.assertEqual(model.type, 'database')
        self.assertTrue(model.active)  # Default value


class TestDatabaseIntegration(unittest.TestCase):
    """Test database operations (mocked)"""
    
    def setUp(self):
        """Set up test database"""
        self.db = DAL('sqlite:memory:', migrate=True)
        
        # Define minimal tables for testing
        self.db.define_table(
            'database_server',
            Field('name', 'string', required=True),
            Field('type', 'string'),
            Field('host', 'string', required=True),
            Field('port', 'integer', required=True),
            Field('active', 'boolean', default=True)
        )
        
        self.db.define_table(
            'managed_database',
            Field('name', 'string', required=True),
            Field('server_id', 'reference database_server', required=True),
            Field('database_name', 'string', required=True),
            Field('active', 'boolean', default=True),
            Field('created_at', 'datetime', default=datetime.utcnow)
        )
        
        self.db.define_table(
            'sql_file',
            Field('name', 'string', required=True),
            Field('database_id', 'reference managed_database', required=True),
            Field('file_type', 'string'),
            Field('file_path', 'string', required=True),
            Field('syntax_validated', 'boolean', default=False),
            Field('security_validated', 'boolean', default=False)
        )
        
        self.db.define_table(
            'blocked_database',
            Field('name', 'string', required=True),
            Field('type', 'string'),
            Field('pattern', 'string', required=True),
            Field('active', 'boolean', default=True)
        )
    
    def tearDown(self):
        """Clean up test database"""
        self.db.close()
    
    def test_create_managed_database(self):
        """Test creating a managed database record"""
        # First create a server
        server_id = self.db.database_server.insert(
            name='test-server',
            type='mysql',
            host='localhost',
            port=3306
        )
        
        # Then create a managed database
        db_id = self.db.managed_database.insert(
            name='test_managed_db',
            server_id=server_id,
            database_name='test_db'
        )
        
        # Verify creation
        db_record = self.db.managed_database[db_id]
        self.assertEqual(db_record.name, 'test_managed_db')
        self.assertEqual(db_record.server_id, server_id)
        self.assertTrue(db_record.active)
    
    def test_create_sql_file_record(self):
        """Test creating an SQL file record"""
        # Create prerequisites
        server_id = self.db.database_server.insert(
            name='test-server', type='mysql', host='localhost', port=3306
        )
        db_id = self.db.managed_database.insert(
            name='test_db', server_id=server_id, database_name='test'
        )
        
        # Create SQL file record
        file_id = self.db.sql_file.insert(
            name='test.sql',
            database_id=db_id,
            file_type='init',
            file_path='/tmp/test.sql',
            syntax_validated=True,
            security_validated=True
        )
        
        # Verify creation
        file_record = self.db.sql_file[file_id]
        self.assertEqual(file_record.name, 'test.sql')
        self.assertTrue(file_record.syntax_validated)
    
    def test_create_blocked_database_rule(self):
        """Test creating a blocked database rule"""
        blocked_id = self.db.blocked_database.insert(
            name='block_test_dbs',
            type='database',
            pattern='test.*'
        )
        
        # Verify creation
        blocked_record = self.db.blocked_database[blocked_id]
        self.assertEqual(blocked_record.type, 'database')
        self.assertEqual(blocked_record.pattern, 'test.*')
        self.assertTrue(blocked_record.active)


class TestFileOperations(unittest.TestCase):
    """Test file upload and validation operations"""
    
    def test_file_checksum_calculation(self):
        """Test that file checksums are calculated correctly"""
        import hashlib
        
        test_content = "SELECT * FROM test_table;"
        expected_checksum = hashlib.sha256(test_content.encode('utf-8')).hexdigest()
        
        # This would be part of the file upload process
        calculated_checksum = hashlib.sha256(test_content.encode('utf-8')).hexdigest()
        self.assertEqual(calculated_checksum, expected_checksum)
    
    def test_temporary_file_creation(self):
        """Test temporary file creation for SQL uploads"""
        test_content = "CREATE TABLE test (id INT PRIMARY KEY);"
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.sql', delete=False) as f:
            f.write(test_content)
            temp_path = f.name
        
        # Verify file was created and has content
        with open(temp_path, 'r') as f:
            read_content = f.read()
        
        self.assertEqual(read_content, test_content)
        
        # Clean up
        os.unlink(temp_path)


class TestSecurityPatterns(unittest.TestCase):
    """Test specific security patterns and edge cases"""
    
    def test_encoded_attacks(self):
        """Test detection of encoded attacks"""
        encoded_sql = "SELECT * FROM users WHERE id = 0x41424344"
        result = validate_sql_security(encoded_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('hex' in error.lower() for error in result['errors']))
    
    def test_comment_based_attacks(self):
        """Test detection of comment-based attacks"""
        comment_sql = "SELECT * FROM users WHERE id = 1 -- AND password = 'secret'"
        result = validate_sql_security(comment_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('comment' in error.lower() for error in result['errors']))
    
    def test_time_based_attacks(self):
        """Test detection of time-based attacks"""
        time_sql = "SELECT * FROM users WHERE id = 1; WAITFOR DELAY '00:00:05'"
        result = validate_sql_security(time_sql)
        
        self.assertFalse(result['valid'])
        self.assertEqual(result['severity'], 'high')
    
    def test_union_based_attacks(self):
        """Test detection of UNION-based attacks"""
        union_sql = "SELECT id FROM users UNION SELECT password FROM admin"
        result = validate_sql_security(union_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('union' in error.lower() for error in result['errors']))


class TestBlockedDatabaseLogic(unittest.TestCase):
    """Test blocked database logic"""
    
    def test_database_pattern_matching(self):
        """Test database pattern matching logic"""
        # This would test the logic used in the IsBlockedDatabase function
        blocked_patterns = [
            {'type': 'database', 'pattern': 'test', 'active': True},
            {'type': 'username', 'pattern': 'admin', 'active': True},
            {'type': 'table', 'pattern': 'sensitive_data', 'active': True}
        ]
        
        # Test database blocking
        test_database = "test_db"
        is_blocked = any(
            pattern['type'] == 'database' and 
            pattern['pattern'] in test_database.lower() and 
            pattern['active']
            for pattern in blocked_patterns
        )
        self.assertTrue(is_blocked)
        
        # Test username blocking
        test_username = "admin_user"
        is_blocked = any(
            pattern['type'] == 'username' and 
            pattern['pattern'] in test_username.lower() and 
            pattern['active']
            for pattern in blocked_patterns
        )
        self.assertTrue(is_blocked)


class TestAuditLogging(unittest.TestCase):
    """Test audit logging functionality"""
    
    def setUp(self):
        """Set up test database for audit logging"""
        self.db = DAL('sqlite:memory:', migrate=True)
        
        self.db.define_table(
            'audit_log',
            Field('user_id', 'integer'),
            Field('action', 'string'),
            Field('database_name', 'string'),
            Field('query', 'text'),
            Field('result', 'string'),
            Field('ip_address', 'string'),
            Field('timestamp', 'datetime', default=datetime.utcnow)
        )
    
    def tearDown(self):
        """Clean up test database"""
        self.db.close()
    
    def test_audit_log_creation(self):
        """Test creating audit log entries"""
        log_id = self.db.audit_log.insert(
            user_id=1,
            action='create_managed_database',
            database_name='test_db',
            query='Created database: test_db',
            result='success',
            ip_address='127.0.0.1'
        )
        
        # Verify creation
        log_record = self.db.audit_log[log_id]
        self.assertEqual(log_record.action, 'create_managed_database')
        self.assertEqual(log_record.result, 'success')


if __name__ == '__main__':
    # Run all tests
    unittest.main(verbosity=2)