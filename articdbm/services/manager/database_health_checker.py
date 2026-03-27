"""
Database Health Checker for ArticDBM Manager

This module provides comprehensive health checking for downstream databases,
identifying common security and configuration issues including:
- TLS configuration problems
- Weak or insecure ciphers
- End-of-Life database versions
- Default databases, users, and passwords
- Misconfigured security settings
"""

import asyncio
import logging
import re
import socket
import ssl
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Tuple, Any
import pymysql
import psycopg2
import redis
import pymongo
from sqlalchemy import create_engine, text
import asyncpg
import aiomysql
import aioredis
from motor.motor_asyncio import AsyncIOMotorClient

logger = logging.getLogger(__name__)

class DatabaseHealthChecker:
    """Main health checker class for all database types"""

    def __init__(self, config: Dict[str, Any]):
        self.config = config
        self.checkers = {
            'mysql': MySQLHealthChecker(),
            'postgresql': PostgreSQLHealthChecker(),
            'redis': RedisHealthChecker(),
            'mongodb': MongoDBHealthChecker(),
            'mssql': MSSQLHealthChecker()
        }
        self.eol_versions = self._load_eol_versions()
        self.default_credentials = self._load_default_credentials()
        self.weak_ciphers = self._load_weak_ciphers()

    def _load_eol_versions(self) -> Dict[str, Dict[str, str]]:
        """Load End-of-Life version information for each database type"""
        return {
            'mysql': {
                '5.6': '2021-02-05',  # EOL
                '5.7': '2023-10-31',  # EOL
                '8.0.0-8.0.16': 'deprecated',  # Specific vulnerable versions
            },
            'postgresql': {
                '9.6': '2021-11-11',  # EOL
                '10': '2022-11-10',   # EOL
                '11': '2023-11-09',   # EOL
                '12': '2024-11-14',   # EOL soon
            },
            'redis': {
                '4.0': '2021-07-01',  # EOL
                '5.0': '2022-04-01',  # EOL
                '6.0.0-6.0.8': 'critical_vulnerabilities',
            },
            'mongodb': {
                '3.6': '2021-04-30',  # EOL
                '4.0': '2022-04-30',  # EOL
                '4.2': '2023-04-30',  # EOL
            }
        }

    def _load_default_credentials(self) -> Dict[str, List[Dict[str, str]]]:
        """Load common default credentials for each database type"""
        return {
            'mysql': [
                {'username': 'root', 'password': ''},
                {'username': 'root', 'password': 'root'},
                {'username': 'admin', 'password': 'admin'},
                {'username': 'mysql', 'password': 'mysql'},
                {'username': 'test', 'password': 'test'},
            ],
            'postgresql': [
                {'username': 'postgres', 'password': ''},
                {'username': 'postgres', 'password': 'postgres'},
                {'username': 'admin', 'password': 'admin'},
                {'username': 'user', 'password': 'password'},
            ],
            'redis': [
                {'password': ''},
                {'password': 'redis'},
                {'password': 'password'},
            ],
            'mongodb': [
                {'username': 'admin', 'password': ''},
                {'username': 'admin', 'password': 'admin'},
                {'username': 'root', 'password': 'root'},
                {'username': 'mongo', 'password': 'mongo'},
            ]
        }

    def _load_weak_ciphers(self) -> List[str]:
        """Load list of weak/deprecated ciphers"""
        return [
            'RC4',
            'DES',
            'MD5',
            'SHA1',
            'NULL',
            'aNULL',
            'eNULL',
            'EXPORT',
            'DES+MD5',
            'RC4+MD5',
            'RC4+SHA',
            'SEED',
            'IDEA',
            'CAMELLIA'
        ]

    async def check_all_databases(self, servers: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Check health of all configured database servers"""
        results = {
            'scan_timestamp': datetime.utcnow().isoformat(),
            'total_servers': len(servers),
            'servers_checked': 0,
            'total_issues': 0,
            'critical_issues': 0,
            'high_issues': 0,
            'medium_issues': 0,
            'low_issues': 0,
            'server_results': [],
            'summary': {
                'tls_issues': 0,
                'version_issues': 0,
                'credential_issues': 0,
                'configuration_issues': 0,
                'security_issues': 0
            }
        }

        tasks = []
        for server in servers:
            task = self.check_database_server(server)
            tasks.append(task)

        server_results = await asyncio.gather(*tasks, return_exceptions=True)

        for i, result in enumerate(server_results):
            if isinstance(result, Exception):
                logger.error(f"Error checking server {servers[i]['name']}: {result}")
                result = {
                    'server_name': servers[i]['name'],
                    'server_type': servers[i]['type'],
                    'status': 'error',
                    'error': str(result),
                    'issues': []
                }

            results['server_results'].append(result)
            results['servers_checked'] += 1

            # Aggregate statistics
            if 'issues' in result:
                results['total_issues'] += len(result['issues'])
                for issue in result['issues']:
                    severity = issue.get('severity', 'medium').lower()
                    if severity == 'critical':
                        results['critical_issues'] += 1
                    elif severity == 'high':
                        results['high_issues'] += 1
                    elif severity == 'medium':
                        results['medium_issues'] += 1
                    else:
                        results['low_issues'] += 1

                    # Categorize issues
                    category = issue.get('category', 'configuration')
                    if category in results['summary']:
                        results['summary'][category] += 1

        return results

    async def check_database_server(self, server: Dict[str, Any]) -> Dict[str, Any]:
        """Check a single database server for health issues"""
        server_type = server.get('type', '').lower()
        checker = self.checkers.get(server_type)

        if not checker:
            return {
                'server_name': server.get('name', 'unknown'),
                'server_type': server_type,
                'status': 'unsupported',
                'error': f'Unsupported database type: {server_type}',
                'issues': []
            }

        try:
            return await checker.check_server(server, self)
        except Exception as e:
            logger.exception(f"Error checking server {server.get('name')}")
            return {
                'server_name': server.get('name', 'unknown'),
                'server_type': server_type,
                'status': 'error',
                'error': str(e),
                'issues': []
            }

class BaseHealthChecker:
    """Base class for database-specific health checkers"""

    def create_issue(self, issue_type: str, severity: str, title: str,
                    description: str, recommendation: str,
                    category: str = 'configuration', evidence: Dict = None) -> Dict[str, Any]:
        """Create a standardized issue report"""
        return {
            'issue_id': f"{issue_type}_{datetime.utcnow().timestamp()}",
            'type': issue_type,
            'severity': severity,
            'category': category,
            'title': title,
            'description': description,
            'recommendation': recommendation,
            'evidence': evidence or {},
            'detected_at': datetime.utcnow().isoformat()
        }

    async def check_tls_connection(self, host: str, port: int) -> Dict[str, Any]:
        """Check TLS configuration for a database connection"""
        tls_info = {
            'tls_enabled': False,
            'tls_version': None,
            'cipher_suite': None,
            'certificate_valid': False,
            'certificate_expires': None,
            'weak_ciphers': []
        }

        try:
            # Create SSL context
            context = ssl.create_default_context()
            context.check_hostname = False
            context.verify_mode = ssl.CERT_NONE

            # Connect with TLS
            with socket.create_connection((host, port), timeout=10) as sock:
                with context.wrap_socket(sock, server_hostname=host) as ssock:
                    tls_info['tls_enabled'] = True
                    tls_info['tls_version'] = ssock.version()
                    tls_info['cipher_suite'] = ssock.cipher()[0] if ssock.cipher() else None

                    # Get certificate info
                    cert = ssock.getpeercert()
                    if cert:
                        tls_info['certificate_valid'] = True
                        not_after = datetime.strptime(cert['notAfter'], '%b %d %H:%M:%S %Y %Z')
                        tls_info['certificate_expires'] = not_after.isoformat()

                        # Check if certificate expires soon
                        if not_after < datetime.utcnow() + timedelta(days=30):
                            tls_info['certificate_expiring_soon'] = True

        except ssl.SSLError as e:
            logger.debug(f"SSL error connecting to {host}:{port}: {e}")
        except Exception as e:
            logger.debug(f"Connection error to {host}:{port}: {e}")

        return tls_info

    async def check_version_eol(self, db_type: str, version: str, eol_versions: Dict) -> Optional[Dict[str, Any]]:
        """Check if database version is End-of-Life"""
        eol_data = eol_versions.get(db_type, {})

        for eol_version, eol_date in eol_data.items():
            if version.startswith(eol_version):
                if eol_date == 'deprecated':
                    return {
                        'is_eol': True,
                        'status': 'deprecated',
                        'message': f'Version {version} is deprecated due to known vulnerabilities'
                    }
                else:
                    try:
                        eol_datetime = datetime.fromisoformat(eol_date)
                        if eol_datetime < datetime.utcnow():
                            return {
                                'is_eol': True,
                                'status': 'end_of_life',
                                'eol_date': eol_date,
                                'message': f'Version {version} reached End-of-Life on {eol_date}'
                            }
                    except ValueError:
                        continue

        return None

class MySQLHealthChecker(BaseHealthChecker):
    """Health checker for MySQL databases"""

    async def check_server(self, server: Dict[str, Any], parent_checker) -> Dict[str, Any]:
        host = server.get('host', 'localhost')
        port = server.get('port', 3306)
        issues = []

        result = {
            'server_name': server.get('name'),
            'server_type': 'mysql',
            'host': host,
            'port': port,
            'status': 'unknown',
            'version': None,
            'issues': issues,
            'tls_info': {},
            'scan_details': {}
        }

        try:
            # Check TLS configuration
            tls_info = await self.check_tls_connection(host, port)
            result['tls_info'] = tls_info

            # Connect to MySQL
            connection = await aiomysql.connect(
                host=host,
                port=port,
                user=server.get('username', 'root'),
                password=server.get('password', ''),
                connect_timeout=10
            )

            try:
                async with connection.cursor() as cursor:
                    # Get MySQL version
                    await cursor.execute("SELECT VERSION()")
                    version_result = await cursor.fetchone()
                    version = version_result[0] if version_result else 'unknown'
                    result['version'] = version
                    result['status'] = 'connected'

                    # Check for version issues
                    version_check = await self.check_version_eol('mysql', version, parent_checker.eol_versions)
                    if version_check:
                        issues.append(self.create_issue(
                            'version_eol',
                            'high' if version_check['status'] == 'end_of_life' else 'critical',
                            'End-of-Life Database Version',
                            version_check['message'],
                            'Upgrade to a supported MySQL version immediately',
                            'version_issues',
                            {'version': version, 'eol_info': version_check}
                        ))

                    # Check TLS issues
                    if not tls_info['tls_enabled']:
                        issues.append(self.create_issue(
                            'tls_disabled',
                            'high',
                            'TLS Not Enabled',
                            'MySQL connection does not use TLS encryption',
                            'Enable TLS/SSL for MySQL connections',
                            'tls_issues',
                            {'host': host, 'port': port}
                        ))
                    elif tls_info['tls_version'] and tls_info['tls_version'] < 'TLSv1.2':
                        issues.append(self.create_issue(
                            'weak_tls_version',
                            'high',
                            'Weak TLS Version',
                            f'MySQL is using TLS version {tls_info["tls_version"]} which is below recommended TLS 1.2',
                            'Configure MySQL to use TLS 1.2 or higher',
                            'tls_issues',
                            tls_info
                        ))

                    # Check for weak ciphers
                    if tls_info.get('cipher_suite'):
                        for weak_cipher in parent_checker.weak_ciphers:
                            if weak_cipher in tls_info['cipher_suite'].upper():
                                issues.append(self.create_issue(
                                    'weak_cipher',
                                    'medium',
                                    'Weak Cipher Suite',
                                    f'MySQL is using weak cipher: {tls_info["cipher_suite"]}',
                                    'Configure MySQL to use strong cipher suites only',
                                    'tls_issues',
                                    tls_info
                                ))
                                break

                    # Check for default credentials
                    await self.check_default_credentials(cursor, issues)

                    # Check for default databases
                    await self.check_default_databases(cursor, issues)

                    # Check MySQL-specific security settings
                    await self.check_mysql_security_settings(cursor, issues)

            finally:
                connection.close()

        except Exception as e:
            result['status'] = 'connection_failed'
            result['error'] = str(e)

            # Try to check for default credentials if regular connection failed
            if 'Access denied' not in str(e):
                await self.check_default_credential_variants(host, port, issues, parent_checker)

        return result

    async def check_default_credentials(self, cursor, issues: List[Dict[str, Any]]):
        """Check for default MySQL credentials"""
        try:
            await cursor.execute("""
                SELECT User, Host, authentication_string, plugin
                FROM mysql.user
                WHERE (User = 'root' AND authentication_string = '')
                   OR (User IN ('admin', 'test', 'mysql') AND authentication_string = '')
            """)

            weak_users = await cursor.fetchall()
            if weak_users:
                issues.append(self.create_issue(
                    'default_credentials',
                    'critical',
                    'Default/Empty Passwords Detected',
                    f'Found {len(weak_users)} users with empty or default passwords',
                    'Set strong passwords for all database users and remove unnecessary accounts',
                    'credential_issues',
                    {'weak_users': [{'user': u[0], 'host': u[1]} for u in weak_users]}
                ))
        except Exception as e:
            logger.debug(f"Could not check default credentials: {e}")

    async def check_default_databases(self, cursor, issues: List[Dict[str, Any]]):
        """Check for default MySQL databases that should be removed"""
        try:
            await cursor.execute("SHOW DATABASES")
            databases = [row[0] for row in await cursor.fetchall()]

            default_dbs = ['test']
            found_defaults = [db for db in databases if db in default_dbs]

            if found_defaults:
                issues.append(self.create_issue(
                    'default_databases',
                    'medium',
                    'Default Test Databases Present',
                    f'Default databases found: {", ".join(found_defaults)}',
                    'Remove default test databases to reduce attack surface',
                    'configuration_issues',
                    {'default_databases': found_defaults}
                ))
        except Exception as e:
            logger.debug(f"Could not check default databases: {e}")

    async def check_mysql_security_settings(self, cursor, issues: List[Dict[str, Any]]):
        """Check MySQL-specific security settings"""
        try:
            # Check for file privileges
            await cursor.execute("""
                SELECT User, Host FROM mysql.user
                WHERE File_priv = 'Y' AND User != 'root'
            """)
            file_priv_users = await cursor.fetchall()

            if file_priv_users:
                issues.append(self.create_issue(
                    'excessive_file_privileges',
                    'high',
                    'Excessive File Privileges',
                    f'{len(file_priv_users)} non-root users have FILE privileges',
                    'Revoke FILE privileges from non-administrative users',
                    'security_issues',
                    {'users_with_file_priv': [{'user': u[0], 'host': u[1]} for u in file_priv_users]}
                ))

            # Check for remote root access
            await cursor.execute("""
                SELECT Host FROM mysql.user
                WHERE User = 'root' AND Host NOT IN ('localhost', '127.0.0.1', '::1')
            """)
            remote_root = await cursor.fetchall()

            if remote_root:
                issues.append(self.create_issue(
                    'remote_root_access',
                    'critical',
                    'Remote Root Access Enabled',
                    'Root user can connect from remote hosts',
                    'Restrict root user to localhost connections only',
                    'security_issues',
                    {'remote_hosts': [row[0] for row in remote_root]}
                ))

            # Check for wildcard hosts
            await cursor.execute("""
                SELECT User, Host FROM mysql.user
                WHERE Host LIKE '%\\%' OR Host LIKE '%\\_%' OR Host = '%'
            """)
            wildcard_hosts = await cursor.fetchall()

            if wildcard_hosts:
                issues.append(self.create_issue(
                    'wildcard_host_access',
                    'high',
                    'Wildcard Host Access',
                    f'{len(wildcard_hosts)} users allow connections from wildcard hosts',
                    'Use specific IP addresses or hostnames instead of wildcards',
                    'security_issues',
                    {'wildcard_users': [{'user': u[0], 'host': u[1]} for u in wildcard_hosts]}
                ))

            # Check for users with ALL PRIVILEGES
            await cursor.execute("""
                SELECT User, Host FROM mysql.user
                WHERE Super_priv = 'Y' OR
                      (Select_priv = 'Y' AND Insert_priv = 'Y' AND Update_priv = 'Y'
                       AND Delete_priv = 'Y' AND Create_priv = 'Y' AND Drop_priv = 'Y')
            """)
            admin_users = await cursor.fetchall()

            admin_count = len([u for u in admin_users if u[0] != 'root'])
            if admin_count > 2:
                issues.append(self.create_issue(
                    'excessive_admin_users',
                    'medium',
                    'Too Many Administrative Users',
                    f'{admin_count} non-root users have administrative privileges',
                    'Review and reduce administrative privileges based on principle of least privilege',
                    'security_issues',
                    {'admin_users': [{'user': u[0], 'host': u[1]} for u in admin_users if u[0] != 'root']}
                ))

            # Check for password validation plugin
            await cursor.execute("SHOW PLUGINS")
            plugins = await cursor.fetchall()
            has_validate_password = any('validate_password' in str(plugin) for plugin in plugins)

            if not has_validate_password:
                issues.append(self.create_issue(
                    'no_password_validation',
                    'medium',
                    'Password Validation Plugin Not Enabled',
                    'MySQL password validation plugin is not installed or enabled',
                    'Install and configure the validate_password plugin for stronger passwords',
                    'security_issues',
                    {}
                ))

            # Check for SSL configuration
            await cursor.execute("SHOW VARIABLES LIKE 'have_ssl'")
            ssl_result = await cursor.fetchone()
            if not ssl_result or ssl_result[1] != 'YES':
                issues.append(self.create_issue(
                    'ssl_not_configured',
                    'high',
                    'SSL/TLS Not Properly Configured',
                    'MySQL SSL/TLS is not properly configured',
                    'Configure SSL certificates and enable require_ssl for users',
                    'tls_issues',
                    {'ssl_status': ssl_result[1] if ssl_result else 'UNKNOWN'}
                ))

            # Check for log_bin (binary logging)
            await cursor.execute("SHOW VARIABLES LIKE 'log_bin'")
            log_bin_result = await cursor.fetchone()
            if not log_bin_result or log_bin_result[1] != 'ON':
                issues.append(self.create_issue(
                    'binary_logging_disabled',
                    'medium',
                    'Binary Logging Disabled',
                    'MySQL binary logging is disabled, affecting backup and replication security',
                    'Enable binary logging for better recovery and audit capabilities',
                    'configuration_issues',
                    {'log_bin_status': log_bin_result[1] if log_bin_result else 'UNKNOWN'}
                ))

            # Check for general log
            await cursor.execute("SHOW VARIABLES LIKE 'general_log'")
            general_log_result = await cursor.fetchone()
            if general_log_result and general_log_result[1] == 'ON':
                issues.append(self.create_issue(
                    'general_logging_enabled',
                    'low',
                    'General Logging Enabled',
                    'General query logging is enabled, which may impact performance and log sensitive data',
                    'Disable general logging in production or ensure logs are properly secured',
                    'configuration_issues',
                    {}
                ))

            # Check for skip-grant-tables
            await cursor.execute("SHOW VARIABLES LIKE 'skip_grant_tables'")
            skip_grants_result = await cursor.fetchone()
            if skip_grants_result and skip_grants_result[1] == 'ON':
                issues.append(self.create_issue(
                    'skip_grant_tables',
                    'critical',
                    'Grant Tables Bypassed',
                    'MySQL is running with --skip-grant-tables, bypassing all authentication',
                    'Restart MySQL without --skip-grant-tables immediately',
                    'security_issues',
                    {}
                ))

            # Check for old_passwords
            await cursor.execute("SHOW VARIABLES LIKE 'old_passwords'")
            old_passwords_result = await cursor.fetchone()
            if old_passwords_result and old_passwords_result[1] != '0':
                issues.append(self.create_issue(
                    'old_password_hashing',
                    'high',
                    'Old Password Hashing Enabled',
                    'MySQL is using old password hashing algorithm which is less secure',
                    'Set old_passwords=0 and update all user passwords',
                    'security_issues',
                    {'old_passwords_value': old_passwords_result[1]}
                ))

            # Check for anonymous users
            await cursor.execute("""
                SELECT Host FROM mysql.user WHERE User = ''
            """)
            anonymous_users = await cursor.fetchall()

            if anonymous_users:
                issues.append(self.create_issue(
                    'anonymous_users',
                    'high',
                    'Anonymous Users Present',
                    f'Found {len(anonymous_users)} anonymous user accounts',
                    'Remove all anonymous user accounts',
                    'security_issues',
                    {'anonymous_hosts': [row[0] for row in anonymous_users]}
                ))

            # Check for users with weak passwords (if password history is available)
            await cursor.execute("""
                SELECT User, Host FROM mysql.user
                WHERE authentication_string = PASSWORD('') OR
                      authentication_string = PASSWORD('password') OR
                      authentication_string = PASSWORD('123456') OR
                      authentication_string = PASSWORD('admin') OR
                      LENGTH(authentication_string) < 20
            """)
            weak_password_users = await cursor.fetchall()

            if weak_password_users:
                issues.append(self.create_issue(
                    'weak_passwords_detected',
                    'critical',
                    'Weak Passwords Detected',
                    f'Found {len(weak_password_users)} users with weak or common passwords',
                    'Enforce strong password policy and update weak passwords',
                    'credential_issues',
                    {'users': [{'user': u[0], 'host': u[1]} for u in weak_password_users]}
                ))

        except Exception as e:
            logger.debug(f"Could not check MySQL security settings: {e}")

    async def check_default_credential_variants(self, host: str, port: int,
                                              issues: List[Dict[str, Any]],
                                              parent_checker):
        """Try common default credentials to detect weak authentication"""
        default_creds = parent_checker.default_credentials.get('mysql', [])

        for cred in default_creds:
            try:
                connection = await aiomysql.connect(
                    host=host,
                    port=port,
                    user=cred['username'],
                    password=cred['password'],
                    connect_timeout=5
                )
                connection.close()

                issues.append(self.create_issue(
                    'default_credentials_accessible',
                    'critical',
                    'Default Credentials Work',
                    f'Can connect using default credentials: {cred["username"]}/{cred["password"] or "(empty)"}',
                    'Change default passwords immediately and disable unused accounts',
                    'credential_issues',
                    {'username': cred['username'], 'password_empty': not bool(cred['password'])}
                ))
                break

            except Exception:
                continue

class PostgreSQLHealthChecker(BaseHealthChecker):
    """Health checker for PostgreSQL databases"""

    async def check_server(self, server: Dict[str, Any], parent_checker) -> Dict[str, Any]:
        host = server.get('host', 'localhost')
        port = server.get('port', 5432)
        issues = []

        result = {
            'server_name': server.get('name'),
            'server_type': 'postgresql',
            'host': host,
            'port': port,
            'status': 'unknown',
            'version': None,
            'issues': issues,
            'tls_info': {},
            'scan_details': {}
        }

        try:
            # Check TLS configuration
            tls_info = await self.check_tls_connection(host, port)
            result['tls_info'] = tls_info

            # Connect to PostgreSQL
            connection = await asyncpg.connect(
                host=host,
                port=port,
                user=server.get('username', 'postgres'),
                password=server.get('password', ''),
                database=server.get('database', 'postgres'),
                command_timeout=10
            )

            try:
                # Get PostgreSQL version
                version = await connection.fetchval('SELECT version()')
                result['version'] = version
                result['status'] = 'connected'

                # Extract version number
                version_match = re.search(r'PostgreSQL (\d+\.?\d*)', version)
                version_number = version_match.group(1) if version_match else 'unknown'

                # Check for version issues
                version_check = await self.check_version_eol('postgresql', version_number, parent_checker.eol_versions)
                if version_check:
                    issues.append(self.create_issue(
                        'version_eol',
                        'high' if version_check['status'] == 'end_of_life' else 'critical',
                        'End-of-Life Database Version',
                        version_check['message'],
                        'Upgrade to a supported PostgreSQL version immediately',
                        'version_issues',
                        {'version': version_number, 'eol_info': version_check}
                    ))

                # Check TLS issues
                if not tls_info['tls_enabled']:
                    issues.append(self.create_issue(
                        'tls_disabled',
                        'high',
                        'TLS Not Enabled',
                        'PostgreSQL connection does not use TLS encryption',
                        'Enable SSL/TLS for PostgreSQL connections',
                        'tls_issues',
                        {'host': host, 'port': port}
                    ))

                # Check for default credentials and configurations
                await self.check_postgresql_security(connection, issues)

            finally:
                await connection.close()

        except Exception as e:
            result['status'] = 'connection_failed'
            result['error'] = str(e)

        return result

    async def check_postgresql_security(self, connection, issues: List[Dict[str, Any]]):
        """Check PostgreSQL-specific security settings"""
        try:
            # Check for users with empty passwords
            weak_users = await connection.fetch("""
                SELECT rolname FROM pg_roles
                WHERE rolcanlogin = true AND rolpassword IS NULL
            """)

            if weak_users:
                issues.append(self.create_issue(
                    'users_no_password',
                    'critical',
                    'Users Without Passwords',
                    f'Found {len(weak_users)} users without passwords',
                    'Set passwords for all user accounts',
                    'credential_issues',
                    {'users': [row['rolname'] for row in weak_users]}
                ))

            # Check for superusers
            superusers = await connection.fetch("""
                SELECT rolname FROM pg_roles
                WHERE rolsuper = true AND rolname != 'postgres'
            """)

            if superusers:
                issues.append(self.create_issue(
                    'excessive_superuser_accounts',
                    'high',
                    'Excessive Superuser Accounts',
                    f'Found {len(superusers)} non-postgres superuser accounts',
                    'Review and minimize superuser privileges',
                    'security_issues',
                    {'superusers': [row['rolname'] for row in superusers]}
                ))

            # Check for users with CREATEROLE privilege
            create_role_users = await connection.fetch("""
                SELECT rolname FROM pg_roles
                WHERE rolcreaterole = true AND rolname NOT IN ('postgres')
            """)

            if create_role_users:
                issues.append(self.create_issue(
                    'excessive_createrole_privilege',
                    'medium',
                    'Excessive CREATEROLE Privileges',
                    f'Found {len(create_role_users)} users with CREATEROLE privilege',
                    'Review and limit CREATEROLE privileges to necessary users only',
                    'security_issues',
                    {'users': [row['rolname'] for row in create_role_users]}
                ))

            # Check for users with CREATEDB privilege
            createdb_users = await connection.fetch("""
                SELECT rolname FROM pg_roles
                WHERE rolcreatedb = true AND rolname NOT IN ('postgres')
            """)

            if len(createdb_users) > 2:
                issues.append(self.create_issue(
                    'excessive_createdb_privilege',
                    'medium',
                    'Excessive CREATEDB Privileges',
                    f'Found {len(createdb_users)} users with CREATEDB privilege',
                    'Review and limit CREATEDB privileges based on business needs',
                    'security_issues',
                    {'users': [row['rolname'] for row in createdb_users]}
                ))

            # Check for default databases
            databases = await connection.fetch("""
                SELECT datname FROM pg_database
                WHERE datistemplate = false
            """)

            default_dbs = ['template0', 'template1', 'postgres']
            user_dbs = [db['datname'] for db in databases if db['datname'] not in default_dbs]

            if not user_dbs:
                issues.append(self.create_issue(
                    'only_default_databases',
                    'low',
                    'Only Default Databases Present',
                    'Only default PostgreSQL databases are present',
                    'Consider whether default databases should be removed if not needed',
                    'configuration_issues',
                    {}
                ))

            # Check for pg_hba.conf trust authentication
            try:
                hba_entries = await connection.fetch("""
                    SELECT type, database, user_name, address, method
                    FROM pg_hba_file_rules
                    WHERE method = 'trust'
                """)

                if hba_entries:
                    issues.append(self.create_issue(
                        'trust_authentication',
                        'critical',
                        'Trust Authentication Enabled',
                        f'Found {len(hba_entries)} pg_hba.conf entries using trust authentication',
                        'Replace trust authentication with password-based or certificate-based authentication',
                        'security_issues',
                        {'trust_entries': len(hba_entries)}
                    ))
            except:
                # pg_hba_file_rules view might not be available
                pass

            # Check for weak password encryption
            try:
                password_encryption = await connection.fetchval("""
                    SELECT setting FROM pg_settings WHERE name = 'password_encryption'
                """)

                if password_encryption != 'scram-sha-256':
                    issues.append(self.create_issue(
                        'weak_password_encryption',
                        'medium',
                        'Weak Password Encryption',
                        f'PostgreSQL is using {password_encryption} instead of scram-sha-256',
                        'Set password_encryption = scram-sha-256 for stronger password security',
                        'security_issues',
                        {'current_method': password_encryption}
                    ))
            except:
                pass

            # Check for log_connections
            try:
                log_connections = await connection.fetchval("""
                    SELECT setting FROM pg_settings WHERE name = 'log_connections'
                """)

                if log_connections != 'on':
                    issues.append(self.create_issue(
                        'connection_logging_disabled',
                        'medium',
                        'Connection Logging Disabled',
                        'PostgreSQL connection logging is disabled',
                        'Enable log_connections for better audit trails',
                        'configuration_issues',
                        {}
                    ))
            except:
                pass

            # Check for log_disconnections
            try:
                log_disconnections = await connection.fetchval("""
                    SELECT setting FROM pg_settings WHERE name = 'log_disconnections'
                """)

                if log_disconnections != 'on':
                    issues.append(self.create_issue(
                        'disconnection_logging_disabled',
                        'low',
                        'Disconnection Logging Disabled',
                        'PostgreSQL disconnection logging is disabled',
                        'Enable log_disconnections for complete session tracking',
                        'configuration_issues',
                        {}
                    ))
            except:
                pass

            # Check for statement logging
            try:
                log_statement = await connection.fetchval("""
                    SELECT setting FROM pg_settings WHERE name = 'log_statement'
                """)

                if log_statement == 'all':
                    issues.append(self.create_issue(
                        'excessive_statement_logging',
                        'medium',
                        'Excessive Statement Logging',
                        'All SQL statements are being logged, which may impact performance and log sensitive data',
                        'Configure log_statement to ddl or mod for production environments',
                        'configuration_issues',
                        {'current_setting': log_statement}
                    ))
            except:
                pass

            # Check for shared_preload_libraries security
            try:
                shared_preload_libraries = await connection.fetchval("""
                    SELECT setting FROM pg_settings WHERE name = 'shared_preload_libraries'
                """)

                dangerous_libraries = ['auto_explain', 'pg_stat_statements']
                loaded_dangerous = [lib for lib in dangerous_libraries
                                  if lib in shared_preload_libraries.lower()]

                if loaded_dangerous:
                    issues.append(self.create_issue(
                        'potentially_dangerous_extensions',
                        'low',
                        'Performance Monitoring Extensions Loaded',
                        f'Extensions that may expose sensitive information: {", ".join(loaded_dangerous)}',
                        'Review loaded extensions and their security implications',
                        'configuration_issues',
                        {'extensions': loaded_dangerous}
                    ))
            except:
                pass

            # Check for SSL configuration
            try:
                ssl_setting = await connection.fetchval("""
                    SELECT setting FROM pg_settings WHERE name = 'ssl'
                """)

                if ssl_setting != 'on':
                    issues.append(self.create_issue(
                        'ssl_disabled',
                        'high',
                        'SSL Not Enabled',
                        'PostgreSQL SSL is not enabled',
                        'Enable SSL and configure certificates for encrypted connections',
                        'tls_issues',
                        {}
                    ))
            except:
                pass

            # Check for row-level security usage
            try:
                rls_enabled_tables = await connection.fetch("""
                    SELECT schemaname, tablename FROM pg_tables
                    WHERE rowsecurity = true
                """)

                total_tables = await connection.fetchval("""
                    SELECT count(*) FROM pg_tables
                    WHERE schemaname NOT IN ('information_schema', 'pg_catalog')
                """)

                if total_tables > 0 and len(rls_enabled_tables) == 0:
                    issues.append(self.create_issue(
                        'no_row_level_security',
                        'low',
                        'Row-Level Security Not Used',
                        'No tables are using row-level security features',
                        'Consider implementing row-level security for sensitive tables',
                        'security_issues',
                        {'total_tables': total_tables}
                    ))
            except:
                pass

            # Check for public schema permissions
            try:
                public_permissions = await connection.fetch("""
                    SELECT grantee, privilege_type FROM information_schema.schema_privileges
                    WHERE schema_name = 'public' AND grantee = 'PUBLIC'
                """)

                if public_permissions:
                    issues.append(self.create_issue(
                        'public_schema_permissions',
                        'medium',
                        'Public Schema Has Public Permissions',
                        'The public schema allows access to all users',
                        'Revoke public permissions from public schema and grant specific access',
                        'security_issues',
                        {'permissions': [p['privilege_type'] for p in public_permissions]}
                    ))
            except:
                pass

        except Exception as e:
            logger.debug(f"Could not check PostgreSQL security settings: {e}")

class RedisHealthChecker(BaseHealthChecker):
    """Health checker for Redis databases"""

    async def check_server(self, server: Dict[str, Any], parent_checker) -> Dict[str, Any]:
        host = server.get('host', 'localhost')
        port = server.get('port', 6379)
        issues = []

        result = {
            'server_name': server.get('name'),
            'server_type': 'redis',
            'host': host,
            'port': port,
            'status': 'unknown',
            'version': None,
            'issues': issues,
            'scan_details': {}
        }

        try:
            # Connect to Redis
            redis_client = aioredis.from_url(
                f"redis://{host}:{port}",
                password=server.get('password'),
                socket_connect_timeout=10
            )

            # Get Redis info
            info = await redis_client.info()
            result['version'] = info.get('redis_version', 'unknown')
            result['status'] = 'connected'

            # Check for version issues
            version_check = await self.check_version_eol('redis', result['version'], parent_checker.eol_versions)
            if version_check:
                issues.append(self.create_issue(
                    'version_eol',
                    'high' if version_check['status'] == 'end_of_life' else 'critical',
                    'End-of-Life Database Version',
                    version_check['message'],
                    'Upgrade to a supported Redis version immediately',
                    'version_issues',
                    {'version': result['version'], 'eol_info': version_check}
                ))

            # Check Redis security settings
            await self.check_redis_security(redis_client, info, issues)

            await redis_client.close()

        except Exception as e:
            result['status'] = 'connection_failed'
            result['error'] = str(e)

            # Try to connect without password (open Redis)
            if 'authentication required' not in str(e).lower():
                await self.check_open_redis(host, port, issues)

        return result

    async def check_redis_security(self, redis_client, info: Dict, issues: List[Dict[str, Any]]):
        """Check Redis security configuration"""
        try:
            # Check if authentication is disabled
            requirepass = await redis_client.config_get('requirepass')
            if not requirepass.get('requirepass') or requirepass.get('requirepass') == '':
                issues.append(self.create_issue(
                    'no_authentication',
                    'critical',
                    'No Authentication Required',
                    'Redis server does not require authentication',
                    'Enable authentication with AUTH command and requirepass',
                    'credential_issues',
                    {}
                ))

            # Check for weak passwords
            if requirepass.get('requirepass'):
                password = requirepass.get('requirepass')
                if len(password) < 12 or password in ['password', 'redis', '123456', 'admin']:
                    issues.append(self.create_issue(
                        'weak_redis_password',
                        'high',
                        'Weak Redis Password',
                        'Redis password is too short or commonly used',
                        'Use a strong, unique password for Redis authentication',
                        'credential_issues',
                        {'password_length': len(password)}
                    ))

            # Check for dangerous commands
            dangerous_commands = [
                'FLUSHALL', 'FLUSHDB', 'CONFIG', 'DEBUG', 'EVAL', 'SHUTDOWN',
                'SCRIPT', 'KEYS', 'MONITOR', 'CLIENT', 'SLOWLOG', 'LASTSAVE',
                'SAVE', 'BGSAVE', 'BGREWRITEAOF'
            ]

            for cmd in dangerous_commands:
                try:
                    # Try to get renamed command
                    rename_config = await redis_client.config_get(f'rename-command {cmd}')
                    if not rename_config or rename_config.get(f'rename-command {cmd}') == cmd:
                        severity = 'critical' if cmd in ['FLUSHALL', 'FLUSHDB', 'SHUTDOWN'] else 'high'
                        issues.append(self.create_issue(
                            'dangerous_commands_enabled',
                            severity,
                            f'Dangerous Command {cmd} Available',
                            f'Dangerous command {cmd} is available and not renamed',
                            f'Disable or rename dangerous Redis command {cmd}',
                            'security_issues',
                            {'command': cmd}
                        ))
                except:
                    # Command might not be renameable, consider it dangerous
                    pass

            # Check binding configuration
            bind_config = await redis_client.config_get('bind')
            bind_value = bind_config.get('bind', '')

            if '0.0.0.0' in bind_value or bind_value == '*':
                issues.append(self.create_issue(
                    'open_bind_address',
                    'high',
                    'Redis Bound to All Interfaces',
                    'Redis is configured to accept connections from any IP address',
                    'Bind Redis to specific interfaces (127.0.0.1) or use firewall rules',
                    'configuration_issues',
                    {'bind_config': bind_value}
                ))
            elif not bind_value:
                issues.append(self.create_issue(
                    'no_bind_restriction',
                    'medium',
                    'No Bind Restriction Configured',
                    'Redis bind configuration is not explicitly set',
                    'Configure bind directive to restrict network interfaces',
                    'configuration_issues',
                    {}
                ))

            # Check protected mode
            protected_mode = await redis_client.config_get('protected-mode')
            if protected_mode.get('protected-mode') == 'no':
                issues.append(self.create_issue(
                    'protected_mode_disabled',
                    'high',
                    'Protected Mode Disabled',
                    'Redis protected mode is disabled',
                    'Enable protected mode to prevent unauthorized access',
                    'security_issues',
                    {}
                ))

            # Check for SSL/TLS configuration
            tls_port = await redis_client.config_get('tls-port')
            if not tls_port.get('tls-port') or tls_port.get('tls-port') == '0':
                issues.append(self.create_issue(
                    'tls_not_configured',
                    'medium',
                    'TLS Not Configured',
                    'Redis TLS is not configured for encrypted connections',
                    'Configure TLS for secure client connections',
                    'tls_issues',
                    {}
                ))

            # Check for ACL configuration (Redis 6+)
            try:
                users = await redis_client.execute_command('ACL', 'LIST')
                if len(users) <= 1:  # Only default user
                    issues.append(self.create_issue(
                        'no_acl_users',
                        'medium',
                        'No ACL Users Configured',
                        'Redis is not using ACL for user management (only default user)',
                        'Configure ACL users with specific permissions instead of using default user',
                        'security_issues',
                        {}
                    ))
                else:
                    # Check for default user with no password
                    for user in users:
                        if 'user default' in str(user) and 'nopass' in str(user):
                            issues.append(self.create_issue(
                                'default_user_no_password',
                                'critical',
                                'Default User Has No Password',
                                'Default Redis user does not require authentication',
                                'Set password for default user or disable it',
                                'credential_issues',
                                {}
                            ))
            except:
                # ACL commands might not be available (Redis < 6)
                pass

            # Check for keyspace notifications
            notify_keyspace_events = await redis_client.config_get('notify-keyspace-events')
            if notify_keyspace_events.get('notify-keyspace-events'):
                issues.append(self.create_issue(
                    'keyspace_notifications_enabled',
                    'low',
                    'Keyspace Notifications Enabled',
                    'Redis keyspace notifications are enabled, which may impact performance',
                    'Disable keyspace notifications if not needed for your application',
                    'configuration_issues',
                    {'events': notify_keyspace_events.get('notify-keyspace-events')}
                ))

            # Check for persistence configuration
            save_config = await redis_client.config_get('save')
            if not save_config.get('save'):
                issues.append(self.create_issue(
                    'no_persistence_configured',
                    'medium',
                    'No Persistence Configured',
                    'Redis has no persistence configured (no snapshots or AOF)',
                    'Configure RDB snapshots or AOF persistence for data durability',
                    'configuration_issues',
                    {}
                ))

            # Check AOF configuration
            appendonly = await redis_client.config_get('appendonly')
            if appendonly.get('appendonly') == 'yes':
                # Check AOF sync policy
                appendfsync = await redis_client.config_get('appendfsync')
                if appendfsync.get('appendfsync') == 'no':
                    issues.append(self.create_issue(
                        'aof_no_sync',
                        'medium',
                        'AOF No Sync Policy',
                        'AOF is enabled but sync is disabled, risking data loss',
                        'Configure appendfsync to everysec or always',
                        'configuration_issues',
                        {}
                    ))

            # Check max memory configuration
            maxmemory = await redis_client.config_get('maxmemory')
            if not maxmemory.get('maxmemory') or maxmemory.get('maxmemory') == '0':
                issues.append(self.create_issue(
                    'no_memory_limit',
                    'low',
                    'No Memory Limit Configured',
                    'Redis has no memory limit configured, which could lead to system issues',
                    'Set maxmemory directive to limit Redis memory usage',
                    'configuration_issues',
                    {}
                ))

            # Check for potentially exposed information
            client_list = await redis_client.execute_command('CLIENT', 'LIST')
            external_clients = 0
            for client in client_list.decode().split('\n'):
                if client.strip():
                    if '127.0.0.1' not in client and 'localhost' not in client:
                        external_clients += 1

            if external_clients > 0:
                issues.append(self.create_issue(
                    'external_client_connections',
                    'low',
                    'External Client Connections Detected',
                    f'Found {external_clients} connections from external IP addresses',
                    'Review external connections and ensure they are authorized',
                    'security_issues',
                    {'external_clients': external_clients}
                ))

            # Check for database selection restrictions
            databases = await redis_client.config_get('databases')
            db_count = int(databases.get('databases', '16'))
            if db_count > 16:
                issues.append(self.create_issue(
                    'excessive_databases',
                    'low',
                    'Too Many Databases Configured',
                    f'Redis is configured with {db_count} databases',
                    'Reduce number of databases to minimum required for better security',
                    'configuration_issues',
                    {'database_count': db_count}
                ))

        except Exception as e:
            logger.debug(f"Could not check Redis security settings: {e}")

    async def check_open_redis(self, host: str, port: int, issues: List[Dict[str, Any]]):
        """Check if Redis is accessible without authentication"""
        try:
            redis_client = aioredis.from_url(
                f"redis://{host}:{port}",
                socket_connect_timeout=5
            )

            await redis_client.ping()

            issues.append(self.create_issue(
                'open_redis',
                'critical',
                'Redis Accessible Without Authentication',
                'Redis server can be accessed without any authentication',
                'Enable authentication immediately and restrict network access',
                'credential_issues',
                {'host': host, 'port': port}
            ))

            await redis_client.close()

        except Exception:
            pass

class MongoDBHealthChecker(BaseHealthChecker):
    """Health checker for MongoDB databases"""

    async def check_server(self, server: Dict[str, Any], parent_checker) -> Dict[str, Any]:
        host = server.get('host', 'localhost')
        port = server.get('port', 27017)
        issues = []

        result = {
            'server_name': server.get('name'),
            'server_type': 'mongodb',
            'host': host,
            'port': port,
            'status': 'unknown',
            'version': None,
            'issues': issues,
            'tls_info': {},
            'scan_details': {}
        }

        try:
            # Connect to MongoDB
            client = AsyncIOMotorClient(f"mongodb://{host}:{port}/", serverSelectionTimeoutMS=10000)

            # Get server info
            server_info = await client.server_info()
            result['version'] = server_info.get('version', 'unknown')
            result['status'] = 'connected'

            # Check for version issues
            version_check = await self.check_version_eol('mongodb', result['version'], parent_checker.eol_versions)
            if version_check:
                issues.append(self.create_issue(
                    'version_eol',
                    'high' if version_check['status'] == 'end_of_life' else 'critical',
                    'End-of-Life Database Version',
                    version_check['message'],
                    'Upgrade to a supported MongoDB version immediately',
                    'version_issues',
                    {'version': result['version'], 'eol_info': version_check}
                ))

            # Check MongoDB security
            await self.check_mongodb_security(client, issues)

            client.close()

        except Exception as e:
            result['status'] = 'connection_failed'
            result['error'] = str(e)

        return result

    async def check_mongodb_security(self, client, issues: List[Dict[str, Any]]):
        """Check MongoDB security configuration"""
        try:
            # Check if authentication is enabled
            admin_db = client.admin

            try:
                # Try to get server status without auth
                await admin_db.command('serverStatus')

                issues.append(self.create_issue(
                    'no_authentication',
                    'critical',
                    'No Authentication Required',
                    'MongoDB server does not require authentication',
                    'Enable authentication and create user accounts',
                    'credential_issues',
                    {}
                ))
            except Exception:
                # Authentication is likely enabled, which is good
                pass

            # Check for default databases
            db_list = await client.list_database_names()
            default_dbs = ['test']
            found_defaults = [db for db in db_list if db in default_dbs]

            if found_defaults:
                issues.append(self.create_issue(
                    'default_databases',
                    'low',
                    'Default Test Databases Present',
                    f'Default databases found: {", ".join(found_defaults)}',
                    'Remove default test databases',
                    'configuration_issues',
                    {'default_databases': found_defaults}
                ))

        except Exception as e:
            logger.debug(f"Could not check MongoDB security settings: {e}")

class MSSQLHealthChecker(BaseHealthChecker):
    """Health checker for Microsoft SQL Server"""

    async def check_server(self, server: Dict[str, Any], parent_checker) -> Dict[str, Any]:
        # Note: This is a simplified implementation
        # Full MSSQL health checking would require pyodbc or similar

        result = {
            'server_name': server.get('name'),
            'server_type': 'mssql',
            'host': server.get('host', 'localhost'),
            'port': server.get('port', 1433),
            'status': 'not_implemented',
            'version': None,
            'issues': [],
            'scan_details': {'note': 'MSSQL health checking requires additional dependencies'}
        }

        return result

# API integration for py4web
def get_database_health_api():
    """Return API functions for database health checking"""

    async def check_database_health():
        """API endpoint to check database health"""
        try:
            # Get database configuration from your existing config
            # This would integrate with your existing server management
            from . import get_database_servers  # Your existing function

            servers = get_database_servers()

            health_checker = DatabaseHealthChecker({})
            results = await health_checker.check_all_databases(servers)

            return {
                'status': 'success',
                'data': results,
                'timestamp': datetime.utcnow().isoformat()
            }

        except Exception as e:
            logger.exception("Database health check failed")
            return {
                'status': 'error',
                'error': str(e),
                'timestamp': datetime.utcnow().isoformat()
            }

    async def get_health_summary():
        """API endpoint to get health check summary"""
        try:
            # This could cache recent results or provide summary stats
            return {
                'status': 'success',
                'data': {
                    'last_scan': 'Not implemented yet',
                    'total_issues': 0,
                    'critical_issues': 0,
                    'recommendations': []
                }
            }

        except Exception as e:
            logger.exception("Health summary retrieval failed")
            return {
                'status': 'error',
                'error': str(e)
            }

    return {
        'check_database_health': check_database_health,
        'get_health_summary': get_health_summary
    }

# Example usage for testing
if __name__ == "__main__":
    async def test_health_checker():
        # Test configuration
        test_servers = [
            {
                'name': 'test-mysql',
                'type': 'mysql',
                'host': 'localhost',
                'port': 3306,
                'username': 'root',
                'password': ''
            },
            {
                'name': 'test-redis',
                'type': 'redis',
                'host': 'localhost',
                'port': 6379
            }
        ]

        health_checker = DatabaseHealthChecker({})
        results = await health_checker.check_all_databases(test_servers)

        print("Health Check Results:")
        print(f"Total Issues: {results['total_issues']}")
        print(f"Critical Issues: {results['critical_issues']}")

        for server_result in results['server_results']:
            print(f"\nServer: {server_result['server_name']}")
            print(f"Status: {server_result['status']}")
            if server_result['issues']:
                for issue in server_result['issues']:
                    print(f"  - {issue['severity'].upper()}: {issue['title']}")

    # Run test
    asyncio.run(test_health_checker())