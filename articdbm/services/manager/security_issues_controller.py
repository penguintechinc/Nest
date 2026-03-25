"""
Security Issues Controller for ArticDBM Manager

Provides admin-only access to database security and configuration issues.
"""

import asyncio
import json
import os
from datetime import datetime, timedelta
from typing import Dict, List, Any, Optional
from py4web import action, request, response, redirect, URL, Field
from py4web.utils.auth import Auth
from py4web.utils.form import Form, FormStyleBulma
from pydal import DAL, Field as DALField

from .database_health_checker import DatabaseHealthChecker
from .health_check_scheduler import HealthCheckScheduler

# Initialize global scheduler instance (would be done in main app)
health_scheduler = None

@action('security_issues')
@action.uses('security_issues.html', auth.user)
def security_issues():
    """Main security issues page - admin only"""

    # Check admin permissions
    if not auth.user or not is_admin(auth.user):
        redirect(URL('auth/login'))

    # Get recent health check results
    recent_results = get_recent_health_results()

    # Get summary statistics
    summary_stats = get_security_summary_stats(recent_results)

    # Get server-specific issues
    server_issues = get_server_security_issues(recent_results)

    return {
        'summary_stats': summary_stats,
        'server_issues': server_issues,
        'recent_results': recent_results,
        'last_scan': recent_results['scan_timestamp'] if recent_results else None,
        'can_run_scan': True
    }

@action('security_issues/run_scan', method='POST')
@action.uses(auth.user)
def run_manual_security_scan():
    """Run manual security scan - admin only"""

    # Check admin permissions
    if not auth.user or not is_admin(auth.user):
        return {'error': 'Access denied', 'status': 'error'}

    try:
        # Run manual health check
        if health_scheduler:
            results = asyncio.run(health_scheduler.run_manual_check())
            return {
                'status': 'success',
                'message': 'Security scan completed successfully',
                'results': {
                    'total_issues': results['total_issues'],
                    'critical_issues': results['critical_issues'],
                    'high_issues': results['high_issues'],
                    'servers_checked': results['servers_checked']
                }
            }
        else:
            return {'error': 'Health checker not initialized', 'status': 'error'}

    except Exception as e:
        return {'error': f'Scan failed: {str(e)}', 'status': 'error'}

@action('security_issues/api/summary')
@action.uses(auth.user)
def security_summary_api():
    """API endpoint for security summary - admin only"""

    # Check admin permissions
    if not auth.user or not is_admin(auth.user):
        response.status = 403
        return {'error': 'Access denied'}

    try:
        recent_results = get_recent_health_results()
        summary = get_security_summary_stats(recent_results)

        return {
            'status': 'success',
            'data': summary,
            'timestamp': datetime.utcnow().isoformat()
        }

    except Exception as e:
        response.status = 500
        return {'error': str(e), 'status': 'error'}

@action('security_issues/api/details/<server_name>')
@action.uses(auth.user)
def security_details_api(server_name):
    """API endpoint for server-specific security details - admin only"""

    # Check admin permissions
    if not auth.user or not is_admin(auth.user):
        response.status = 403
        return {'error': 'Access denied'}

    try:
        recent_results = get_recent_health_results()

        if not recent_results:
            return {'status': 'success', 'data': {'issues': []}}

        # Find server results
        server_data = None
        for server in recent_results.get('server_results', []):
            if server['server_name'] == server_name:
                server_data = server
                break

        if not server_data:
            response.status = 404
            return {'error': 'Server not found'}

        return {
            'status': 'success',
            'data': server_data,
            'timestamp': datetime.utcnow().isoformat()
        }

    except Exception as e:
        response.status = 500
        return {'error': str(e), 'status': 'error'}

@action('security_issues/api/fix_suggestions/<issue_type>')
@action.uses(auth.user)
def fix_suggestions_api(issue_type):
    """API endpoint for detailed fix suggestions - admin only"""

    # Check admin permissions
    if not auth.user or not is_admin(auth.user):
        response.status = 403
        return {'error': 'Access denied'}

    try:
        suggestions = get_detailed_fix_suggestions(issue_type)

        return {
            'status': 'success',
            'data': suggestions,
            'timestamp': datetime.utcnow().isoformat()
        }

    except Exception as e:
        response.status = 500
        return {'error': str(e), 'status': 'error'}

@action('security_issues/scheduler_status')
@action.uses(auth.user)
def scheduler_status():
    """Get scheduler status - admin only"""

    # Check admin permissions
    if not auth.user or not is_admin(auth.user):
        response.status = 403
        return {'error': 'Access denied'}

    try:
        if health_scheduler:
            status = health_scheduler.get_scheduler_status()
            return {'status': 'success', 'data': status}
        else:
            return {'status': 'error', 'error': 'Scheduler not initialized'}

    except Exception as e:
        response.status = 500
        return {'error': str(e), 'status': 'error'}

def is_admin(user) -> bool:
    """Check if user has admin privileges"""
    # This would integrate with your existing auth system
    # For now, simple check for admin role
    if hasattr(user, 'get') and user.get('role') == 'admin':
        return True
    if hasattr(user, 'role') and user.role == 'admin':
        return True
    # Check if user is in admin group
    if hasattr(user, 'groups') and 'admin' in user.groups:
        return True
    return False

def get_recent_health_results() -> Dict[str, Any]:
    """Get the most recent health check results"""
    try:
        if health_scheduler:
            # Get from scheduler's last results
            return health_scheduler.last_scan_results
        else:
            # Try to load from filesystem
            results_dir = '/var/log/articdbm/health_checks'
            if not os.path.exists(results_dir):
                return {}

            # Find most recent result file
            result_files = [f for f in os.listdir(results_dir) if f.startswith('health_check_') and f.endswith('.json')]
            if not result_files:
                return {}

            # Sort by filename (which includes timestamp)
            result_files.sort(reverse=True)
            latest_file = os.path.join(results_dir, result_files[0])

            with open(latest_file, 'r') as f:
                return json.load(f)

    except Exception as e:
        print(f"Error loading health results: {e}")
        return {}

def get_security_summary_stats(results: Dict[str, Any]) -> Dict[str, Any]:
    """Generate security summary statistics"""
    if not results:
        return {
            'total_issues': 0,
            'critical_issues': 0,
            'high_issues': 0,
            'medium_issues': 0,
            'low_issues': 0,
            'servers_scanned': 0,
            'scan_age': 'Never',
            'top_issue_categories': [],
            'security_score': 0
        }

    # Calculate top issue categories
    category_counts = {}
    for server in results.get('server_results', []):
        for issue in server.get('issues', []):
            category = issue.get('category', 'other')
            category_counts[category] = category_counts.get(category, 0) + 1

    top_categories = sorted(category_counts.items(), key=lambda x: x[1], reverse=True)[:5]

    # Calculate security score (0-100, higher is better)
    total_servers = results.get('servers_checked', 1)
    total_possible_issues = total_servers * 20  # Assume max 20 issues per server
    actual_issues = results.get('total_issues', 0)
    security_score = max(0, 100 - (actual_issues / max(total_possible_issues, 1)) * 100)

    # Calculate scan age
    scan_time = results.get('scan_timestamp')
    scan_age = 'Unknown'
    if scan_time:
        try:
            scan_dt = datetime.fromisoformat(scan_time.replace('Z', '+00:00'))
            age_delta = datetime.utcnow() - scan_dt.replace(tzinfo=None)

            if age_delta.days > 0:
                scan_age = f"{age_delta.days} days ago"
            elif age_delta.seconds > 3600:
                scan_age = f"{age_delta.seconds // 3600} hours ago"
            else:
                scan_age = f"{age_delta.seconds // 60} minutes ago"
        except:
            scan_age = 'Unknown'

    return {
        'total_issues': results.get('total_issues', 0),
        'critical_issues': results.get('critical_issues', 0),
        'high_issues': results.get('high_issues', 0),
        'medium_issues': results.get('medium_issues', 0),
        'low_issues': results.get('low_issues', 0),
        'servers_scanned': results.get('servers_checked', 0),
        'scan_age': scan_age,
        'top_issue_categories': [{'category': cat.replace('_', ' ').title(), 'count': count} for cat, count in top_categories],
        'security_score': round(security_score, 1)
    }

def get_server_security_issues(results: Dict[str, Any]) -> List[Dict[str, Any]]:
    """Get security issues organized by server"""
    if not results:
        return []

    servers = []
    for server_result in results.get('server_results', []):
        # Organize issues by severity
        issues_by_severity = {
            'critical': [],
            'high': [],
            'medium': [],
            'low': []
        }

        for issue in server_result.get('issues', []):
            severity = issue.get('severity', 'medium').lower()
            if severity in issues_by_severity:
                issues_by_severity[severity].append({
                    'id': issue.get('issue_id', ''),
                    'type': issue.get('type', ''),
                    'title': issue.get('title', ''),
                    'description': issue.get('description', ''),
                    'recommendation': issue.get('recommendation', ''),
                    'category': issue.get('category', 'configuration'),
                    'evidence': issue.get('evidence', {}),
                    'detected_at': issue.get('detected_at', '')
                })

        # Calculate server security score
        total_issues = len(server_result.get('issues', []))
        critical_weight = len(issues_by_severity['critical']) * 4
        high_weight = len(issues_by_severity['high']) * 3
        medium_weight = len(issues_by_severity['medium']) * 2
        low_weight = len(issues_by_severity['low']) * 1

        weighted_score = critical_weight + high_weight + medium_weight + low_weight
        server_score = max(0, 100 - (weighted_score * 2))  # Scale to 0-100

        server_data = {
            'name': server_result.get('server_name', 'Unknown'),
            'type': server_result.get('server_type', 'unknown'),
            'host': server_result.get('host', ''),
            'port': server_result.get('port', ''),
            'status': server_result.get('status', 'unknown'),
            'version': server_result.get('version', 'unknown'),
            'total_issues': total_issues,
            'security_score': round(server_score, 1),
            'issues_by_severity': issues_by_severity,
            'tls_info': server_result.get('tls_info', {}),
            'scan_details': server_result.get('scan_details', {})
        }

        servers.append(server_data)

    # Sort by number of critical issues, then by total issues
    servers.sort(key=lambda x: (len(x['issues_by_severity']['critical']), x['total_issues']), reverse=True)

    return servers

def get_detailed_fix_suggestions(issue_type: str) -> Dict[str, Any]:
    """Get detailed fix suggestions for a specific issue type"""

    # Comprehensive fix suggestions database
    fix_suggestions = {
        'default_credentials': {
            'title': 'Default or Weak Credentials',
            'description': 'Default passwords and weak authentication pose critical security risks',
            'immediate_actions': [
                'Change all default passwords immediately',
                'Implement strong password policy (minimum 12 characters, mixed case, numbers, special chars)',
                'Remove or disable unused default accounts',
                'Enable account lockout after failed attempts'
            ],
            'long_term_actions': [
                'Implement multi-factor authentication where possible',
                'Use certificate-based authentication for service accounts',
                'Regular password rotation policy',
                'Monitor authentication logs for suspicious activity'
            ],
            'tools': [
                'Password managers (1Password, Bitwarden)',
                'Password strength testing tools',
                'Authentication monitoring systems'
            ],
            'compliance': {
                'SOC2': 'CC6.1 - Logical and physical access controls',
                'NIST': 'IA-5 - Authenticator Management',
                'PCI-DSS': '8.2.3 - Strong authentication parameters'
            }
        },
        'tls_disabled': {
            'title': 'TLS/SSL Not Enabled',
            'description': 'Unencrypted database connections expose data in transit',
            'immediate_actions': [
                'Enable TLS/SSL on database server',
                'Configure valid certificates',
                'Force TLS for all client connections',
                'Test connectivity after enabling TLS'
            ],
            'long_term_actions': [
                'Implement certificate rotation automation',
                'Monitor certificate expiration',
                'Use strong cipher suites only',
                'Regular security assessments'
            ],
            'configuration_examples': {
                'MySQL': 'SET GLOBAL require_ssl=ON; ALTER USER \'user\'@\'host\' REQUIRE SSL;',
                'PostgreSQL': 'ssl = on in postgresql.conf, hostssl entries in pg_hba.conf',
                'Redis': 'Enable TLS port and configure certificates'
            },
            'compliance': {
                'SOC2': 'CC6.7 - System uses encryption to protect data',
                'HIPAA': '164.312(e) - Transmission security',
                'PCI-DSS': '4.1 - Strong cryptography for card data transmission'
            }
        },
        'weak_tls_version': {
            'title': 'Weak TLS Version',
            'description': 'Older TLS versions have known vulnerabilities',
            'immediate_actions': [
                'Configure minimum TLS version to 1.2 or higher',
                'Disable TLS 1.0 and 1.1',
                'Update cipher suite configuration',
                'Test client compatibility'
            ],
            'long_term_actions': [
                'Plan migration to TLS 1.3 where supported',
                'Regular cipher suite reviews',
                'Monitor for new TLS vulnerabilities',
                'Client compatibility testing'
            ],
            'cipher_recommendations': [
                'ECDHE-ECDSA-AES256-GCM-SHA384',
                'ECDHE-RSA-AES256-GCM-SHA384',
                'ECDHE-ECDSA-CHACHA20-POLY1305',
                'ECDHE-RSA-CHACHA20-POLY1305'
            ]
        },
        'version_eol': {
            'title': 'End-of-Life Database Version',
            'description': 'Unsupported database versions lack security updates',
            'immediate_actions': [
                'Plan upgrade to supported version immediately',
                'Implement additional network controls as temporary measure',
                'Review security patches available for current version',
                'Document upgrade timeline and requirements'
            ],
            'long_term_actions': [
                'Establish version lifecycle management process',
                'Regular version upgrade planning',
                'Test environment for upgrade validation',
                'Automated security update monitoring'
            ],
            'upgrade_considerations': [
                'Application compatibility testing',
                'Data migration planning',
                'Downtime scheduling',
                'Rollback procedures',
                'Performance testing'
            ]
        },
        'excessive_privileges': {
            'title': 'Excessive User Privileges',
            'description': 'Users have more privileges than necessary (principle of least privilege)',
            'immediate_actions': [
                'Audit all user permissions',
                'Remove unnecessary privileges',
                'Create role-based access control',
                'Document required permissions per user'
            ],
            'long_term_actions': [
                'Regular access reviews',
                'Automated privilege monitoring',
                'Just-in-time access where possible',
                'Segregation of duties implementation'
            ],
            'best_practices': [
                'Create specific roles for different job functions',
                'Use database-specific permission controls',
                'Implement approval workflows for privilege changes',
                'Monitor privilege usage and violations'
            ]
        },
        'no_authentication': {
            'title': 'Authentication Disabled',
            'description': 'Database allows connections without authentication',
            'immediate_actions': [
                'Enable authentication immediately',
                'Create initial admin user with strong password',
                'Remove guest/anonymous access',
                'Configure network access restrictions'
            ],
            'long_term_actions': [
                'Implement centralized authentication (LDAP/AD)',
                'Multi-factor authentication setup',
                'Regular authentication audits',
                'Session management controls'
            ],
            'emergency_steps': [
                '1. Stop database service',
                '2. Enable authentication in configuration',
                '3. Create admin user',
                '4. Test authentication',
                '5. Restart with authentication enabled',
                '6. Update client configurations'
            ]
        },
        'dangerous_commands_enabled': {
            'title': 'Dangerous Commands Enabled',
            'description': 'Dangerous administrative commands are available to users',
            'immediate_actions': [
                'Disable or rename dangerous commands',
                'Audit who has access to administrative functions',
                'Implement command logging',
                'Review recent command usage'
            ],
            'long_term_actions': [
                'Create administrative command policies',
                'Implement approval workflows for dangerous operations',
                'Regular review of command access',
                'Automated monitoring for dangerous command usage'
            ],
            'commands_to_review': {
                'Redis': ['FLUSHALL', 'FLUSHDB', 'CONFIG', 'DEBUG', 'EVAL', 'SHUTDOWN'],
                'MySQL': ['SHUTDOWN', 'DROP DATABASE', 'TRUNCATE', 'DELETE without WHERE'],
                'PostgreSQL': ['DROP DATABASE', 'TRUNCATE', 'pg_terminate_backend']
            }
        },
        'open_bind_address': {
            'title': 'Database Bound to All Interfaces',
            'description': 'Database accepts connections from any network interface',
            'immediate_actions': [
                'Configure bind address to specific interfaces only',
                'Use 127.0.0.1 for local-only access',
                'Implement firewall rules',
                'Review network architecture'
            ],
            'long_term_actions': [
                'Network segmentation implementation',
                'VPN access for remote connections',
                'Regular network security assessments',
                'Intrusion detection systems'
            ],
            'network_security': [
                'Use private IP ranges only',
                'Implement database firewalls',
                'Network access control lists',
                'Monitor network connections'
            ]
        }
    }

    return fix_suggestions.get(issue_type, {
        'title': 'Unknown Issue Type',
        'description': 'No specific guidance available for this issue type',
        'immediate_actions': ['Review security documentation', 'Consult with security team'],
        'long_term_actions': ['Implement general security best practices']
    })

# Additional utility functions for the security page
def get_severity_color(severity: str) -> str:
    """Get color class for severity levels"""
    colors = {
        'critical': 'is-danger',
        'high': 'is-warning',
        'medium': 'is-info',
        'low': 'is-success'
    }
    return colors.get(severity.lower(), 'is-light')

def get_severity_icon(severity: str) -> str:
    """Get icon for severity levels"""
    icons = {
        'critical': 'fas fa-exclamation-triangle',
        'high': 'fas fa-exclamation-circle',
        'medium': 'fas fa-info-circle',
        'low': 'fas fa-check-circle'
    }
    return icons.get(severity.lower(), 'fas fa-question-circle')

def format_recommendation(recommendation: str, max_length: int = 100) -> str:
    """Format recommendation text for display"""
    if len(recommendation) <= max_length:
        return recommendation
    return recommendation[:max_length] + '...'

# Export functions for use in templates
__all__ = [
    'security_issues',
    'run_manual_security_scan',
    'security_summary_api',
    'security_details_api',
    'fix_suggestions_api',
    'scheduler_status',
    'get_severity_color',
    'get_severity_icon',
    'format_recommendation'
]