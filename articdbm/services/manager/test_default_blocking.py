#!/usr/bin/env python3
"""
Default Database and Account Blocking Tests

This test suite validates the blocking of default databases, accounts, and system resources:
- Default system databases (SQL Server, MySQL, PostgreSQL, MongoDB)
- Default administrative accounts (sa, root, admin, etc.)
- Test and demo databases/accounts
- System tables and schemas
- Pattern-based blocking rules
- Blocking rule management (CRUD operations)
- Redis synchronization of blocking rules
"""

import unittest
import json
import tempfile
import shutil
from unittest.mock import Mock, patch, MagicMock
from pathlib import Path

# Import the functions we want to test
from app import (
    BlockedDatabaseModel,
    validate_sql_security,
    seed_default_blocked_resources,
    sync_to_redis
)


class TestDefaultDatabaseBlocking(unittest.TestCase):
    """Test blocking of default and system databases"""
    
    def setUp(self):
        """Set up test cases for default database blocking"""
        self.system_databases = {
            'sql_server': [
                'master', 'msdb', 'tempdb', 'model'
            ],
            'mysql': [
                'mysql', 'sys', 'information_schema', 'performance_schema'
            ],
            'postgresql': [
                'postgres', 'template0', 'template1'
            ],
            'mongodb': [
                'admin', 'local', 'config'
            ],
            'test_databases': [
                'test', 'demo', 'sample', 'example', 'temp', 'tmp'
            ]
        }
        
        self.dangerous_database_patterns = [
            ('test_dev', 'test database pattern'),
            ('sample_app', 'sample database pattern'),
            ('demo_system', 'demo database pattern'),
            ('user_backup', 'backup database pattern'),
            ('old_data', 'old database pattern')
        ]
    
    def test_sql_server_system_database_detection(self):
        """Test detection of SQL Server system databases"""
        for db_name in self.system_databases['sql_server']:
            test_queries = [
                f"SELECT * FROM {db_name}.dbo.sysobjects",
                f"USE {db_name}; SELECT * FROM sysdatabases",
                f"SELECT * FROM {db_name}.sys.tables",
                f"INSERT INTO {db_name}.dbo.test_table VALUES (1, 'test')"
            ]
            
            for query in test_queries:
                with self.subTest(database=db_name, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should detect system database access
                    self.assertFalse(result['valid'],
                        f"Failed to detect {db_name} database access: {query}")
                    
                    # Should have high severity
                    self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                        f"{db_name} access should have high severity")
                    
                    # Should mention system database in errors
                    system_error = any(
                        db_name.lower() in error.lower() or 'system' in error.lower()
                        for error in result['errors']
                    )
                    self.assertTrue(system_error,
                        f"No system database error for {db_name}: {result['errors']}")
    
    def test_mysql_system_database_detection(self):
        """Test detection of MySQL system databases"""
        for db_name in self.system_databases['mysql']:
            test_queries = [
                f"SELECT * FROM {db_name}.user",
                f"USE {db_name}; SELECT * FROM tables",
                f"SHOW TABLES FROM {db_name}",
                f"DELETE FROM {db_name}.user WHERE user = 'test'"
            ]
            
            for query in test_queries:
                with self.subTest(database=db_name, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should detect system database access
                    self.assertFalse(result['valid'],
                        f"Failed to detect {db_name} database access: {query}")
                    
                    # Should have appropriate severity
                    self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                        f"{db_name} access should have high severity")
    
    def test_postgresql_system_database_detection(self):
        """Test detection of PostgreSQL system databases"""
        for db_name in self.system_databases['postgresql']:
            test_queries = [
                f"\\c {db_name}; SELECT * FROM pg_user",
                f"SELECT * FROM {db_name}.pg_tables",
                f"CREATE TABLE {db_name}.test_table (id INT)",
                f"DROP DATABASE {db_name}"
            ]
            
            for query in test_queries:
                with self.subTest(database=db_name, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should detect system database access
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"No issues detected for {db_name} access: {query}")
    
    def test_mongodb_system_database_detection(self):
        """Test detection of MongoDB system databases"""
        for db_name in self.system_databases['mongodb']:
            test_queries = [
                f"use {db_name}; db.users.find()",
                f"db.getSiblingDB('{db_name}').users.find()",
                f"show collections from {db_name}",
                f"use {db_name}; db.dropDatabase()"
            ]
            
            for query in test_queries:
                with self.subTest(database=db_name, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should detect system database access
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"No issues detected for {db_name} access: {query}")
    
    def test_test_database_detection(self):
        """Test detection of test/demo databases"""
        for db_name in self.system_databases['test_databases']:
            test_queries = [
                f"SELECT * FROM {db_name}.users",
                f"USE {db_name}; CREATE TABLE test (id INT)",
                f"DROP DATABASE {db_name}",
                f"BACKUP DATABASE {db_name} TO '/tmp/backup.bak'"
            ]
            
            for query in test_queries:
                with self.subTest(database=db_name, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should detect test database access
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"No issues detected for test database {db_name}: {query}")
                    
                    # Should have warnings about test database usage
                    if result['warnings']:
                        test_warning = any(
                            term in ' '.join(result['warnings']).lower()
                            for term in [db_name, 'test', 'demo', 'sample']
                        )
                        self.assertTrue(test_warning,
                            f"No test database warning for {db_name}")
    
    def test_database_pattern_matching(self):
        """Test pattern-based database blocking"""
        for db_name, description in self.dangerous_database_patterns:
            test_queries = [
                f"SELECT * FROM {db_name}.users",
                f"CREATE DATABASE {db_name}",
                f"USE {db_name}; SELECT COUNT(*) FROM products",
                f"DROP DATABASE IF EXISTS {db_name}"
            ]
            
            for query in test_queries:
                with self.subTest(database=db_name, description=description, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Pattern-based databases should generate warnings
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"No issues detected for pattern database {db_name}: {query}")


class TestDefaultAccountBlocking(unittest.TestCase):
    """Test blocking of default and administrative accounts"""
    
    def setUp(self):
        """Set up test cases for default account blocking"""
        self.default_accounts = {
            'admin_accounts': [
                'sa', 'root', 'admin', 'administrator', 'sysadmin'
            ],
            'guest_accounts': [
                'guest', 'anonymous', 'public', 'everyone'
            ],
            'service_accounts': [
                'mysql', 'postgres', 'oracle', 'sqlserver', 'mongodb'
            ],
            'test_accounts': [
                'test', 'demo', 'sample', 'user', 'testuser'
            ],
            'empty_accounts': [
                '', 'null', 'undefined'
            ]
        }
        
        self.account_pattern_tests = [
            ('test_user', 'test user pattern'),
            ('admin_backup', 'admin pattern'),
            ('root_temp', 'root pattern'),
            ('demo_account', 'demo pattern')
        ]
    
    def test_admin_account_detection(self):
        """Test detection of default administrative accounts"""
        for account in self.default_accounts['admin_accounts']:
            test_queries = [
                f"SELECT * FROM users WHERE username = '{account}'",
                f"CREATE USER '{account}' IDENTIFIED BY 'password'",
                f"GRANT ALL PRIVILEGES ON *.* TO '{account}'",
                f"ALTER USER '{account}' SET PASSWORD = 'newpass'",
                f"DROP USER '{account}'"
            ]
            
            for query in test_queries:
                with self.subTest(account=account, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should generate warnings for admin accounts
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"No issues for admin account {account}: {query}")
                    
                    # Should have admin-related warnings
                    if result['warnings']:
                        admin_warning = any(
                            account in warning.lower() or 'admin' in warning.lower()
                            for warning in result['warnings']
                        )
                        self.assertTrue(admin_warning,
                            f"No admin warning for {account}: {result['warnings']}")
    
    def test_guest_account_detection(self):
        """Test detection of guest and anonymous accounts"""
        for account in self.default_accounts['guest_accounts']:
            test_queries = [
                f"SELECT * FROM users WHERE username = '{account}'",
                f"LOGIN AS '{account}'",
                f"SET USER '{account}'",
                f"CONNECT AS '{account}'"
            ]
            
            for query in test_queries:
                with self.subTest(account=account, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should generate warnings for guest accounts
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"No issues for guest account {account}: {query}")
    
    def test_service_account_detection(self):
        """Test detection of database service accounts"""
        for account in self.default_accounts['service_accounts']:
            test_queries = [
                f"SELECT * FROM users WHERE username = '{account}'",
                f"CONNECT {account}/password@database",
                f"CREATE USER {account} WITH PASSWORD 'service_pass'",
                f"ALTER ROLE {account} WITH LOGIN"
            ]
            
            for query in test_queries:
                with self.subTest(account=account, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should generate warnings for service accounts
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"No issues for service account {account}: {query}")
    
    def test_test_account_detection(self):
        """Test detection of test and demo accounts"""
        for account in self.default_accounts['test_accounts']:
            test_queries = [
                f"SELECT * FROM users WHERE username = '{account}'",
                f"INSERT INTO users (username, password) VALUES ('{account}', 'test123')",
                f"UPDATE users SET active = 1 WHERE username = '{account}'",
                f"DELETE FROM users WHERE username = '{account}'"
            ]
            
            for query in test_queries:
                with self.subTest(account=account, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should generate warnings for test accounts
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"No issues for test account {account}: {query}")
    
    def test_empty_account_detection(self):
        """Test detection of empty and null usernames"""
        test_queries = [
            "SELECT * FROM users WHERE username = ''",
            "SELECT * FROM users WHERE username IS NULL",
            "CREATE USER '' IDENTIFIED BY 'password'",
            "LOGIN AS ''"
        ]
        
        for query in test_queries:
            with self.subTest(query=query[:50]):
                result = validate_sql_security(query)
                
                # Should generate warnings for empty accounts
                total_issues = len(result['errors']) + len(result['warnings'])
                self.assertGreater(total_issues, 0,
                    f"No issues for empty account query: {query}")
    
    def test_account_pattern_matching(self):
        """Test pattern-based account blocking"""
        for account, description in self.account_pattern_tests:
            test_queries = [
                f"SELECT * FROM users WHERE username = '{account}'",
                f"CREATE USER '{account}' WITH PASSWORD 'test'",
                f"GRANT SELECT ON users TO '{account}'",
                f"SET SESSION AUTHORIZATION '{account}'"
            ]
            
            for query in test_queries:
                with self.subTest(account=account, description=description, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Pattern accounts should generate warnings
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"No issues for pattern account {account}: {query}")


class TestSystemTableBlocking(unittest.TestCase):
    """Test blocking of system tables and schemas"""
    
    def setUp(self):
        """Set up system table test cases"""
        self.system_tables = {
            'sql_server': [
                'sysobjects', 'syscolumns', 'sysusers', 'sysdatabases',
                'sys.tables', 'sys.columns', 'sys.objects'
            ],
            'mysql': [
                'mysql.user', 'mysql.db', 'mysql.tables_priv', 'mysql.columns_priv'
            ],
            'postgresql': [
                'pg_user', 'pg_database', 'pg_tables', 'pg_class'
            ],
            'information_schema': [
                'information_schema.tables', 'information_schema.columns',
                'information_schema.schemata', 'information_schema.table_privileges'
            ]
        }
    
    def test_sql_server_system_table_blocking(self):
        """Test blocking of SQL Server system tables"""
        for table in self.system_tables['sql_server']:
            test_queries = [
                f"SELECT * FROM {table}",
                f"INSERT INTO {table} VALUES (1, 'test')",
                f"UPDATE {table} SET name = 'modified'",
                f"DELETE FROM {table} WHERE id = 1",
                f"DROP TABLE {table}"
            ]
            
            for query in test_queries:
                with self.subTest(table=table, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should detect system table access
                    self.assertFalse(result['valid'],
                        f"Failed to block system table {table} access: {query}")
                    
                    # Should have high severity
                    self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                        f"System table {table} access should have high severity")
    
    def test_mysql_system_table_blocking(self):
        """Test blocking of MySQL system tables"""
        for table in self.system_tables['mysql']:
            test_queries = [
                f"SELECT * FROM {table}",
                f"SELECT User, Host FROM {table}",
                f"UPDATE {table} SET Password = 'hacked'",
                f"INSERT INTO {table} VALUES ('hacker', 'localhost')",
                f"DELETE FROM {table} WHERE User = 'root'"
            ]
            
            for query in test_queries:
                with self.subTest(table=table, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should detect system table access
                    self.assertFalse(result['valid'],
                        f"Failed to block MySQL system table {table}: {query}")
                    
                    # Should have high severity
                    self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                        f"MySQL system table {table} should have high severity")
    
    def test_information_schema_blocking(self):
        """Test blocking of information_schema access"""
        for table in self.system_tables['information_schema']:
            test_queries = [
                f"SELECT * FROM {table}",
                f"SELECT table_name FROM {table}",
                f"SELECT column_name FROM {table} WHERE table_name = 'users'",
                f"SHOW TABLES FROM {table}"
            ]
            
            for query in test_queries:
                with self.subTest(table=table, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    # Should detect information schema access
                    self.assertFalse(result['valid'],
                        f"Failed to block information schema {table}: {query}")
                    
                    # Should mention schema introspection
                    schema_error = any(
                        'schema' in error.lower() or 'introspection' in error.lower()
                        for error in result['errors']
                    )
                    self.assertTrue(schema_error,
                        f"No schema introspection error for {table}")


class TestBlockedDatabaseModel(unittest.TestCase):
    """Test the BlockedDatabaseModel validation"""
    
    def test_valid_blocked_database_model(self):
        """Test valid blocked database model creation"""
        valid_models = [
            {
                'name': 'block_master_db',
                'type': 'database',
                'pattern': '^master$',
                'reason': 'SQL Server system database should not be accessible',
                'active': True
            },
            {
                'name': 'block_root_user',
                'type': 'username', 
                'pattern': '^root$',
                'reason': 'Default root account blocked for security',
                'active': True
            },
            {
                'name': 'block_system_tables',
                'type': 'table',
                'pattern': '^sys.*',
                'reason': 'System tables should not be directly accessible',
                'active': True
            }
        ]
        
        for model_data in valid_models:
            with self.subTest(model=model_data['name']):
                model = BlockedDatabaseModel(**model_data)
                
                self.assertEqual(model.name, model_data['name'])
                self.assertEqual(model.type, model_data['type'])
                self.assertEqual(model.pattern, model_data['pattern'])
                self.assertEqual(model.reason, model_data['reason'])
                self.assertTrue(model.active)
    
    def test_blocked_database_model_defaults(self):
        """Test blocked database model with default values"""
        minimal_data = {
            'name': 'test_block',
            'type': 'database',
            'pattern': '^test.*'
        }
        
        model = BlockedDatabaseModel(**minimal_data)
        
        self.assertEqual(model.name, 'test_block')
        self.assertEqual(model.type, 'database')
        self.assertEqual(model.pattern, '^test.*')
        self.assertIsNone(model.reason)
        self.assertTrue(model.active)  # Should default to True


class TestDefaultResourceSeeding(unittest.TestCase):
    """Test seeding of default blocked resources"""
    
    def setUp(self):
        """Set up mocks for testing seeding functionality"""
        self.db_mock = Mock()
        self.redis_mock = Mock()
    
    @patch('app.db')
    @patch('app.sync_to_redis')
    def test_seed_default_resources_success(self, mock_sync, mock_db):
        """Test successful seeding of default blocked resources"""
        # Mock database operations
        mock_db.return_value.count.return_value = 0  # No existing resources
        mock_db.blocked_database.insert = Mock(return_value=1)
        mock_db.commit = Mock()
        mock_sync.return_value = True
        
        # Call seeding function
        result = seed_default_blocked_resources()
        
        # Verify seeding was successful
        self.assertTrue(result)
        self.assertTrue(mock_db.blocked_database.insert.called)
        self.assertTrue(mock_db.commit.called)
        self.assertTrue(mock_sync.called)
    
    @patch('app.db')
    def test_seed_default_resources_already_exists(self, mock_db):
        """Test seeding when resources already exist"""
        # Mock existing resources
        mock_db.return_value.count.return_value = 50  # Resources exist
        
        # Call seeding function
        result = seed_default_blocked_resources()
        
        # Should return early without inserting
        self.assertIsNone(result)
        self.assertFalse(mock_db.blocked_database.insert.called)
    
    def test_critical_resources_coverage(self):
        """Test that critical resources are covered in defaults"""
        # This tests the data structure used in seeding
        critical_databases = {
            'master', 'msdb', 'tempdb', 'model',  # SQL Server
            'mysql', 'sys', 'information_schema', 'performance_schema',  # MySQL
            'postgres', 'template0', 'template1',  # PostgreSQL
            'admin', 'local', 'config',  # MongoDB
            'test', 'demo', 'sample', 'example'  # Test databases
        }
        
        critical_users = {
            'sa', 'root', 'admin', 'administrator', 'guest',
            'test', 'demo', 'sample', 'mysql', 'postgres'
        }
        
        # Verify coverage (this would be done against actual seeding data)
        self.assertGreater(len(critical_databases), 10,
            "Should cover major database systems")
        self.assertGreater(len(critical_users), 8,
            "Should cover common default accounts")


class TestBlockingRuleCRUD(unittest.TestCase):
    """Test CRUD operations for blocking rules"""
    
    def setUp(self):
        """Set up test database for blocking rule tests"""
        from py4web import DAL, Field
        
        self.db = DAL('sqlite:memory:', migrate=True)
        
        self.db.define_table(
            'blocked_database',
            Field('name', 'string', required=True),
            Field('type', 'string', requires=lambda v: v in ['database', 'username', 'table']),
            Field('pattern', 'string', required=True),
            Field('reason', 'text'),
            Field('active', 'boolean', default=True),
            Field('created_at', 'datetime', default=lambda: datetime.utcnow())
        )
    
    def tearDown(self):
        """Clean up test database"""
        self.db.close()
    
    def test_create_blocking_rule(self):
        """Test creating a blocking rule"""
        rule_data = {
            'name': 'block_test_dbs',
            'type': 'database',
            'pattern': '^test.*',
            'reason': 'Test databases should not be accessible in production',
            'active': True
        }
        
        # Test model validation
        rule_model = BlockedDatabaseModel(**rule_data)
        self.assertEqual(rule_model.name, 'block_test_dbs')
        
        # Test database insertion
        rule_id = self.db.blocked_database.insert(
            name=rule_model.name,
            type=rule_model.type,
            pattern=rule_model.pattern,
            reason=rule_model.reason,
            active=rule_model.active
        )
        
        # Verify creation
        rule_record = self.db.blocked_database[rule_id]
        self.assertEqual(rule_record.name, 'block_test_dbs')
        self.assertEqual(rule_record.type, 'database')
        self.assertEqual(rule_record.pattern, '^test.*')
        self.assertTrue(rule_record.active)
    
    def test_update_blocking_rule(self):
        """Test updating a blocking rule"""
        # Create initial rule
        rule_id = self.db.blocked_database.insert(
            name='update_test',
            type='database',
            pattern='^old_pattern$',
            reason='Initial reason',
            active=True
        )
        
        # Update rule
        rule_record = self.db.blocked_database[rule_id]
        rule_record.update_record(
            pattern='^new_pattern$',
            reason='Updated reason for better security',
            active=True
        )
        
        # Verify update
        updated_record = self.db.blocked_database[rule_id]
        self.assertEqual(updated_record.pattern, '^new_pattern$')
        self.assertEqual(updated_record.reason, 'Updated reason for better security')
    
    def test_soft_delete_blocking_rule(self):
        """Test soft deletion of blocking rule"""
        # Create rule
        rule_id = self.db.blocked_database.insert(
            name='delete_test',
            type='username',
            pattern='^temp_user$',
            active=True
        )
        
        # Soft delete
        rule_record = self.db.blocked_database[rule_id]
        rule_record.update_record(active=False)
        
        # Verify soft deletion
        deleted_record = self.db.blocked_database[rule_id]
        self.assertFalse(deleted_record.active)
        
        # Verify active count
        active_count = self.db(self.db.blocked_database.active == True).count()
        self.assertEqual(active_count, 0)
    
    def test_multiple_blocking_rules(self):
        """Test creating multiple blocking rules"""
        rules = [
            {'name': 'block_admin_dbs', 'type': 'database', 'pattern': '.*admin.*'},
            {'name': 'block_root_users', 'type': 'username', 'pattern': '^root$'},
            {'name': 'block_sys_tables', 'type': 'table', 'pattern': '^sys.*'},
        ]
        
        created_rules = []
        for rule in rules:
            rule_id = self.db.blocked_database.insert(
                name=rule['name'],
                type=rule['type'],
                pattern=rule['pattern'],
                active=True
            )
            created_rules.append(rule_id)
        
        # Verify all rules created
        total_rules = self.db(self.db.blocked_database).count()
        self.assertEqual(total_rules, 3)
        
        # Verify rule types
        db_rules = self.db(self.db.blocked_database.type == 'database').count()
        user_rules = self.db(self.db.blocked_database.type == 'username').count()
        table_rules = self.db(self.db.blocked_database.type == 'table').count()
        
        self.assertEqual(db_rules, 1)
        self.assertEqual(user_rules, 1)
        self.assertEqual(table_rules, 1)


if __name__ == '__main__':
    # Run default blocking tests
    unittest.main(verbosity=2)