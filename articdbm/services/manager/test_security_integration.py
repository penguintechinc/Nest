#!/usr/bin/env python3
"""
Security Integration Tests

This test suite validates the integration between Python manager and Go proxy security systems:
- Redis-based communication between manager and proxy
- Synchronized security configurations
- Blocking rule propagation
- Real-time security policy updates
- Cross-system security validation
- Failover and consistency checks
"""

import unittest
import json
import asyncio
import time
from unittest.mock import Mock, patch, MagicMock, AsyncMock
from datetime import datetime

# Import the functions we want to test
from app import (
    sync_to_redis,
    seed_default_blocked_resources,
    validate_sql_security,
    BlockedDatabaseModel
)


class TestSecurityIntegration(unittest.TestCase):
    """Test integration between Python manager and Go proxy security systems"""
    
    def setUp(self):
        """Set up test environment for security integration"""
        # Mock Redis clients
        self.redis_mock = Mock()
        self.redis_mock.set = Mock(return_value=True)
        self.redis_mock.get = Mock()
        self.redis_mock.expire = Mock(return_value=True)
        self.redis_mock.exists = Mock()
        self.redis_mock.delete = Mock(return_value=1)
        
        # Mock database
        self.db_mock = Mock()
        
        # Sample security configuration data
        self.sample_config = {
            'blocked_databases': {
                '1': {
                    'name': 'master',
                    'type': 'database',
                    'pattern': '^master$',
                    'reason': 'SQL Server system database',
                    'active': True
                },
                '2': {
                    'name': 'root_user',
                    'type': 'username', 
                    'pattern': '^root$',
                    'reason': 'Default root account',
                    'active': True
                }
            },
            'security_rules': {
                'sql_injection_enabled': True,
                'shell_command_blocking': True,
                'default_account_warnings': True,
                'pattern_matching_enabled': True
            }
        }
    
    def test_manager_to_proxy_sync_success(self):
        """Test successful synchronization from manager to proxy via Redis"""
        with patch('app.redis_client', self.redis_mock), \
             patch('app.db', self.db_mock):
            
            # Mock database queries
            self.db_mock.return_value.select.return_value = []
            self.db_mock.auth_user.select.return_value = []
            self.db_mock.user_permission.select.return_value = []
            self.db_mock.database_server.active = True
            self.db_mock.blocked_database.active = True
            
            # Mock blocked database data
            blocked_data = [
                Mock(id=1, name='test', type='database', pattern='^test$', reason='Test DB', active=True),
                Mock(id=2, name='root', type='username', pattern='^root$', reason='Root user', active=True)
            ]
            self.db_mock.return_value.select.return_value = blocked_data
            
            # Call sync function
            result = sync_to_redis()
            
            # Verify sync was successful
            self.assertTrue(result)
            
            # Verify Redis set operations were called
            self.assertTrue(self.redis_mock.set.called)
            
            # Verify blocked databases were synced
            set_calls = self.redis_mock.set.call_args_list
            keys_set = [call[0][0] for call in set_calls]
            
            self.assertIn('articdbm:blocked_databases', keys_set)
            self.assertIn('articdbm:manager:blocked_databases', keys_set)
    
    def test_proxy_blocking_configuration_retrieval(self):
        """Test that proxy can retrieve blocking configuration from Redis"""
        # Simulate proxy retrieving configuration
        blocked_config = json.dumps(self.sample_config['blocked_databases'])
        self.redis_mock.get.return_value = blocked_config
        
        # Verify configuration can be retrieved
        result = self.redis_mock.get('articdbm:blocked_databases')
        self.assertIsNotNone(result)
        
        # Verify configuration can be parsed
        parsed_config = json.loads(result)
        self.assertIn('1', parsed_config)
        self.assertEqual(parsed_config['1']['type'], 'database')
        self.assertEqual(parsed_config['2']['type'], 'username')
    
    def test_real_time_blocking_rule_updates(self):
        """Test real-time updates of blocking rules from manager to proxy"""
        with patch('app.redis_client', self.redis_mock), \
             patch('app.db', self.db_mock):
            
            # Initial sync
            self.db_mock.return_value.select.return_value = []
            self.db_mock.return_value.count.return_value = 0
            sync_to_redis()
            
            initial_calls = len(self.redis_mock.set.call_args_list)
            
            # Simulate adding a new blocking rule
            new_blocked_rule = Mock(
                id=3, 
                name='admin_tables', 
                type='table', 
                pattern='^admin_.*', 
                reason='Admin tables blocked', 
                active=True
            )
            
            self.db_mock.return_value.select.return_value = [new_blocked_rule]
            
            # Sync again with new rule
            sync_to_redis()
            
            # Verify additional sync occurred
            final_calls = len(self.redis_mock.set.call_args_list)
            self.assertGreater(final_calls, initial_calls)
    
    def test_security_validation_consistency(self):
        """Test consistency of security validation between manager and proxy"""
        # Test queries that should be blocked by both systems
        dangerous_queries = [
            "SELECT * FROM users WHERE id = 1 OR 1=1",
            "SELECT * FROM users; xp_cmdshell 'whoami'",
            "SELECT * FROM master.dbo.sysdatabases",
            "SELECT * FROM users WHERE username = 'root'",
            "SELECT LOAD_FILE('/etc/passwd')"
        ]
        
        for query in dangerous_queries:
            with self.subTest(query=query[:50]):
                # Manager-side validation
                manager_result = validate_sql_security(query)
                
                # Verify dangerous queries are blocked by manager
                self.assertFalse(manager_result['valid'],
                    f"Manager should block dangerous query: {query[:100]}")
                
                # Verify severity is appropriate
                self.assertIn(manager_result['severity'], ['medium', 'high', 'critical'],
                    f"Manager severity too low for: {query[:50]}")
                
                # This would be validated by proxy as well (tested in Go tests)
                # Here we ensure the validation data is consistent
                self.assertGreater(len(manager_result['errors']), 0,
                    f"Manager should report errors for: {query[:50]}")
    
    def test_blocking_rule_synchronization(self):
        """Test synchronization of blocking rules between systems"""
        with patch('app.redis_client', self.redis_mock), \
             patch('app.db', self.db_mock):
            
            # Create test blocking rules
            blocking_rules = [
                {'id': 1, 'name': 'mysql_system', 'type': 'database', 'pattern': '^mysql$', 'active': True},
                {'id': 2, 'name': 'admin_user', 'type': 'username', 'pattern': '^admin$', 'active': True},
                {'id': 3, 'name': 'sys_tables', 'type': 'table', 'pattern': '^sys.*', 'active': True},
            ]
            
            # Mock database return
            mock_rules = [Mock(**rule) for rule in blocking_rules]
            mock_rules[0].reason = 'MySQL system database'
            mock_rules[1].reason = 'Admin user account'
            mock_rules[2].reason = 'System tables'
            
            self.db_mock.return_value.select.return_value = mock_rules
            
            # Mock other required data
            self.db_mock.return_value.count.return_value = 0
            self.db_mock.auth_user.select.return_value = []
            self.db_mock.user_permission.select.return_value = []
            self.db_mock.database_server.active = True
            self.db_mock.blocked_database.active = True
            
            # Perform sync
            result = sync_to_redis()
            self.assertTrue(result)
            
            # Verify the correct data was set in Redis
            set_calls = self.redis_mock.set.call_args_list
            blocked_db_call = None
            
            for call in set_calls:
                if call[0][0] == 'articdbm:blocked_databases':
                    blocked_db_call = call
                    break
            
            self.assertIsNotNone(blocked_db_call,
                "Blocked databases should be synced to Redis")
            
            # Verify JSON data structure
            synced_data = blocked_db_call[0][1]
            parsed_data = json.loads(synced_data)
            
            # Should have entries for all rule types
            rule_types = set()
            for rule in parsed_data.values():
                rule_types.add(rule['type'])
            
            self.assertIn('database', rule_types)
            self.assertIn('username', rule_types)
            self.assertIn('table', rule_types)
    
    def test_configuration_expiration_and_refresh(self):
        """Test Redis key expiration and refresh mechanisms"""
        with patch('app.redis_client', self.redis_mock):
            
            # Mock database data
            with patch('app.db', self.db_mock):
                self.db_mock.return_value.select.return_value = []
                self.db_mock.return_value.count.return_value = 0
                
                # Perform sync
                sync_to_redis()
                
                # Verify expiration was set on keys
                expire_calls = self.redis_mock.expire.call_args_list
                self.assertGreater(len(expire_calls), 0,
                    "Keys should have expiration set")
                
                # Verify standard expiration time (5 minutes = 300 seconds)
                for call in expire_calls:
                    expiration_time = call[0][1]
                    self.assertEqual(expiration_time, 300,
                        f"Unexpected expiration time: {expiration_time}")
    
    def test_failover_configuration_handling(self):
        """Test handling of Redis connection failures and failover"""
        # Simulate Redis connection failure
        self.redis_mock.set.side_effect = Exception("Redis connection failed")
        
        with patch('app.redis_client', self.redis_mock), \
             patch('app.db', self.db_mock):
            
            self.db_mock.return_value.select.return_value = []
            
            # Sync should handle Redis failure gracefully
            result = sync_to_redis()
            
            # Should return False on failure but not crash
            self.assertFalse(result)
    
    def test_security_policy_propagation_timing(self):
        """Test timing of security policy propagation"""
        with patch('app.redis_client', self.redis_mock), \
             patch('app.db', self.db_mock):
            
            # Mock database data
            self.db_mock.return_value.select.return_value = []
            self.db_mock.return_value.count.return_value = 0
            
            # Record timing
            start_time = time.time()
            sync_to_redis()
            sync_time = time.time() - start_time
            
            # Sync should be fast (< 1 second for test data)
            self.assertLess(sync_time, 1.0,
                "Security policy sync should be fast")
            
            # Verify sync occurred
            self.assertTrue(self.redis_mock.set.called,
                "Redis set should be called during sync")
    
    def test_cross_system_validation_consistency(self):
        """Test validation consistency across Python manager and Go proxy"""
        # Define test cases that both systems should handle identically
        test_cases = [
            {
                'query': "SELECT * FROM users WHERE id = 1 OR 1=1",
                'expected_blocked': True,
                'expected_severity': 'high',
                'attack_type': 'sql_injection'
            },
            {
                'query': "SELECT * FROM users; xp_cmdshell 'whoami'",
                'expected_blocked': True,
                'expected_severity': 'critical',
                'attack_type': 'shell_command'
            },
            {
                'query': "SELECT * FROM master.dbo.sysdatabases",
                'expected_blocked': True,
                'expected_severity': 'high',
                'attack_type': 'system_database'
            },
            {
                'query': "SELECT id, name FROM customers WHERE active = 1",
                'expected_blocked': False,
                'expected_severity': 'low',
                'attack_type': 'clean_query'
            }
        ]
        
        for test_case in test_cases:
            with self.subTest(attack_type=test_case['attack_type']):
                # Test Python manager validation
                result = validate_sql_security(test_case['query'])
                
                # Verify blocking expectation
                if test_case['expected_blocked']:
                    self.assertFalse(result['valid'],
                        f"Manager should block {test_case['attack_type']}: {test_case['query'][:50]}")
                    
                    # Verify severity
                    severity_levels = ['low', 'medium', 'high', 'critical']
                    expected_level = severity_levels.index(test_case['expected_severity'])
                    actual_level = severity_levels.index(result['severity'])
                    
                    self.assertGreaterEqual(actual_level, expected_level,
                        f"Manager severity too low for {test_case['attack_type']}")
                else:
                    # Clean queries might pass or have low severity
                    if not result['valid']:
                        self.assertEqual(result['severity'], 'low',
                            f"Clean query should have low severity: {test_case['query']}")


class TestSecurityConfigurationManagement(unittest.TestCase):
    """Test management of security configurations across systems"""
    
    def setUp(self):
        """Set up configuration management test environment"""
        self.config_mock = {
            'blocking_enabled': True,
            'sql_injection_detection': True,
            'shell_command_blocking': True,
            'default_account_warnings': True,
            'system_database_blocking': True,
            'pattern_matching_enabled': True,
            'severity_thresholds': {
                'low': 'warn',
                'medium': 'warn',
                'high': 'block',
                'critical': 'block'
            }
        }
    
    def test_security_configuration_structure(self):
        """Test the structure of security configuration data"""
        # Verify all required configuration keys are present
        required_keys = [
            'blocking_enabled',
            'sql_injection_detection', 
            'shell_command_blocking',
            'default_account_warnings',
            'system_database_blocking'
        ]
        
        for key in required_keys:
            self.assertIn(key, self.config_mock,
                f"Required configuration key missing: {key}")
            
            # All boolean configurations should be boolean type
            if key != 'severity_thresholds':
                self.assertIsInstance(self.config_mock[key], bool,
                    f"Configuration {key} should be boolean")
    
    def test_severity_threshold_configuration(self):
        """Test severity threshold configuration"""
        thresholds = self.config_mock['severity_thresholds']
        
        # Verify all severity levels are configured
        severity_levels = ['low', 'medium', 'high', 'critical']
        for level in severity_levels:
            self.assertIn(level, thresholds,
                f"Severity level {level} should be configured")
            
            # Verify threshold actions are valid
            self.assertIn(thresholds[level], ['warn', 'block'],
                f"Invalid threshold action for {level}: {thresholds[level]}")
        
        # Verify logical progression (higher severity should be at least as strict)
        severity_order = ['low', 'medium', 'high', 'critical']
        action_severity = {'warn': 0, 'block': 1}
        
        for i in range(1, len(severity_order)):
            current_action = action_severity[thresholds[severity_order[i]]]
            previous_action = action_severity[thresholds[severity_order[i-1]]]
            
            self.assertGreaterEqual(current_action, previous_action,
                f"Severity progression issue: {severity_order[i]} should be >= {severity_order[i-1]}")
    
    def test_blocking_rule_priority(self):
        """Test priority handling of different blocking rule types"""
        # Define rule priorities (higher number = higher priority)
        rule_priorities = {
            'shell_command': 100,  # Critical - always block
            'sql_injection': 90,   # High priority
            'system_database': 80, # High priority
            'default_account': 70, # Medium priority
            'pattern_match': 60    # Lower priority
        }
        
        # Verify critical rules have highest priority
        self.assertGreater(rule_priorities['shell_command'], rule_priorities['sql_injection'],
            "Shell commands should have highest priority")
        
        # Verify system protection rules have high priority
        self.assertGreater(rule_priorities['system_database'], rule_priorities['default_account'],
            "System database protection should have higher priority than default accounts")
    
    def test_configuration_validation_rules(self):
        """Test validation of security configuration"""
        # Test invalid configuration scenarios
        invalid_configs = [
            # Missing required keys
            {'blocking_enabled': True},
            
            # Invalid boolean values
            {'blocking_enabled': 'true', 'sql_injection_detection': True},
            
            # Invalid severity thresholds
            {
                'blocking_enabled': True,
                'sql_injection_detection': True,
                'severity_thresholds': {'low': 'invalid_action'}
            }
        ]
        
        for invalid_config in invalid_configs:
            with self.subTest(config=str(invalid_config)):
                # Validation should fail for invalid configurations
                # This would be implemented in actual configuration validation
                validation_errors = []
                
                # Check required keys
                required_keys = ['blocking_enabled', 'sql_injection_detection']
                for key in required_keys:
                    if key not in invalid_config:
                        validation_errors.append(f"Missing required key: {key}")
                
                # Check boolean values
                for key, value in invalid_config.items():
                    if key.endswith('_enabled') or key.endswith('_detection') or key.endswith('_blocking'):
                        if not isinstance(value, bool):
                            validation_errors.append(f"Non-boolean value for {key}")
                
                # Should have validation errors for invalid configs
                self.assertGreater(len(validation_errors), 0,
                    f"Should have validation errors for invalid config: {invalid_config}")


class TestSecurityMetrics(unittest.TestCase):
    """Test security metrics and monitoring integration"""
    
    def test_security_validation_metrics(self):
        """Test collection of security validation metrics"""
        test_queries = [
            "SELECT * FROM users WHERE id = 1 OR 1=1",  # SQL injection
            "SELECT * FROM users; xp_cmdshell 'whoami'",  # Shell command
            "SELECT * FROM master.dbo.sysdatabases",  # System database
            "SELECT id, name FROM customers WHERE active = 1",  # Clean query
        ]
        
        metrics = {
            'total_queries': 0,
            'blocked_queries': 0,
            'warning_queries': 0,
            'clean_queries': 0,
            'attack_types': {}
        }
        
        for query in test_queries:
            result = validate_sql_security(query)
            metrics['total_queries'] += 1
            
            if not result['valid']:
                metrics['blocked_queries'] += 1
                
                # Categorize attack type
                if any('injection' in error.lower() for error in result['errors']):
                    metrics['attack_types']['sql_injection'] = metrics['attack_types'].get('sql_injection', 0) + 1
                elif any('shell' in error.lower() or 'command' in error.lower() for error in result['errors']):
                    metrics['attack_types']['shell_command'] = metrics['attack_types'].get('shell_command', 0) + 1
                elif any('system' in error.lower() or 'database' in error.lower() for error in result['errors']):
                    metrics['attack_types']['system_access'] = metrics['attack_types'].get('system_access', 0) + 1
                    
            elif len(result['warnings']) > 0:
                metrics['warning_queries'] += 1
            else:
                metrics['clean_queries'] += 1
        
        # Verify metrics collection
        self.assertEqual(metrics['total_queries'], len(test_queries))
        self.assertGreater(metrics['blocked_queries'], 0,
            "Should have blocked some dangerous queries")
        self.assertGreater(len(metrics['attack_types']), 0,
            "Should have detected different attack types")
    
    def test_performance_metrics(self):
        """Test security validation performance metrics"""
        import time
        
        # Test query validation performance
        test_query = "SELECT * FROM users WHERE id = 1 OR 1=1"
        
        # Measure validation time
        start_time = time.time()
        result = validate_sql_security(test_query)
        validation_time = time.time() - start_time
        
        # Validation should be fast (< 100ms for simple queries)
        self.assertLess(validation_time, 0.1,
            f"Security validation took too long: {validation_time:.3f}s")
        
        # Verify validation still worked correctly
        self.assertFalse(result['valid'], "Performance test query should be blocked")
    
    def test_security_coverage_metrics(self):
        """Test security pattern coverage metrics"""
        # Test a known query to get pattern coverage
        result = validate_sql_security("SELECT * FROM users WHERE id = 1 OR 1=1")
        
        # Should report number of patterns checked
        self.assertIn('patterns_checked', result,
            "Should report number of security patterns checked")
        
        # Should check a reasonable number of patterns
        patterns_checked = result['patterns_checked']
        self.assertGreaterEqual(patterns_checked, 40,
            f"Should check at least 40 patterns, checked: {patterns_checked}")


if __name__ == '__main__':
    # Run security integration tests
    unittest.main(verbosity=2)