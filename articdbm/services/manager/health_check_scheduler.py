"""
Health Check Scheduler for ArticDBM Manager

This module provides automated scheduling for database health checks,
running comprehensive security and configuration scans on a daily basis.
"""

import asyncio
import logging
from datetime import datetime, timedelta
from typing import Dict, List, Any, Optional
import json
import os
from apscheduler.schedulers.asyncio import AsyncIOScheduler
from apscheduler.triggers.cron import CronTrigger
from apscheduler.executors.asyncio import AsyncIOExecutor
import smtplib
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
import requests

from .database_health_checker import DatabaseHealthChecker

logger = logging.getLogger(__name__)

class HealthCheckScheduler:
    """Manages automated health check scheduling and notifications"""

    def __init__(self, config: Dict[str, Any]):
        self.config = config
        self.scheduler = None
        self.health_checker = None
        self.last_scan_results = {}
        self.notification_config = config.get('notifications', {})

        # Default schedule: daily at 2 AM
        self.scan_schedule = config.get('schedule', {
            'hour': 2,
            'minute': 0,
            'second': 0
        })

        # Results storage
        self.results_directory = config.get('results_directory', '/var/log/articdbm/health_checks')
        self.max_stored_results = config.get('max_stored_results', 30)  # Keep 30 days

        # Create results directory
        os.makedirs(self.results_directory, exist_ok=True)

    async def start(self):
        """Start the health check scheduler"""
        try:
            # Initialize scheduler
            self.scheduler = AsyncIOScheduler(
                executors={'default': AsyncIOExecutor()},
                timezone='UTC'
            )

            # Initialize health checker
            self.health_checker = DatabaseHealthChecker(self.config.get('health_checker', {}))

            # Schedule daily health checks
            self.scheduler.add_job(
                self.run_scheduled_health_check,
                CronTrigger(
                    hour=self.scan_schedule['hour'],
                    minute=self.scan_schedule['minute'],
                    second=self.scan_schedule['second']
                ),
                id='daily_health_check',
                name='Daily Database Health Check',
                replace_existing=True
            )

            # Schedule weekly summary report
            self.scheduler.add_job(
                self.send_weekly_summary,
                CronTrigger(day_of_week='monday', hour=9, minute=0),
                id='weekly_summary',
                name='Weekly Health Summary Report',
                replace_existing=True
            )

            # Schedule cleanup of old results
            self.scheduler.add_job(
                self.cleanup_old_results,
                CronTrigger(hour=3, minute=0),
                id='cleanup_results',
                name='Cleanup Old Health Check Results',
                replace_existing=True
            )

            # Start scheduler
            self.scheduler.start()
            logger.info(f"Health check scheduler started - daily scans at {self.scan_schedule['hour']:02d}:{self.scan_schedule['minute']:02d} UTC")

        except Exception as e:
            logger.error(f"Failed to start health check scheduler: {e}")
            raise

    async def stop(self):
        """Stop the health check scheduler"""
        if self.scheduler:
            self.scheduler.shutdown()
            logger.info("Health check scheduler stopped")

    async def run_scheduled_health_check(self):
        """Run the scheduled daily health check"""
        try:
            logger.info("Starting scheduled database health check")

            # Get list of database servers (this would integrate with your existing config)
            servers = await self.get_database_servers()

            if not servers:
                logger.warning("No database servers configured for health checking")
                return

            # Run health check
            results = await self.health_checker.check_all_databases(servers)

            # Store results
            await self.store_results(results)

            # Update last scan results
            self.last_scan_results = results

            # Send notifications if needed
            await self.process_notifications(results)

            logger.info(f"Scheduled health check completed - found {results['total_issues']} issues across {results['servers_checked']} servers")

        except Exception as e:
            logger.error(f"Scheduled health check failed: {e}")
            await self.send_error_notification(str(e))

    async def get_database_servers(self) -> List[Dict[str, Any]]:
        """Get list of database servers from configuration"""
        # This would integrate with your existing server management
        # For now, return example configuration
        return [
            {
                'name': 'primary-mysql',
                'type': 'mysql',
                'host': os.getenv('MYSQL_HOST', 'localhost'),
                'port': int(os.getenv('MYSQL_PORT', '3306')),
                'username': os.getenv('MYSQL_USER', 'root'),
                'password': os.getenv('MYSQL_PASSWORD', ''),
                'enabled': True
            },
            {
                'name': 'primary-redis',
                'type': 'redis',
                'host': os.getenv('REDIS_HOST', 'localhost'),
                'port': int(os.getenv('REDIS_PORT', '6379')),
                'password': os.getenv('REDIS_PASSWORD', ''),
                'enabled': True
            },
            {
                'name': 'primary-postgres',
                'type': 'postgresql',
                'host': os.getenv('POSTGRES_HOST', 'localhost'),
                'port': int(os.getenv('POSTGRES_PORT', '5432')),
                'username': os.getenv('POSTGRES_USER', 'postgres'),
                'password': os.getenv('POSTGRES_PASSWORD', ''),
                'database': os.getenv('POSTGRES_DB', 'postgres'),
                'enabled': True
            }
        ]

    async def store_results(self, results: Dict[str, Any]):
        """Store health check results to disk"""
        try:
            timestamp = datetime.utcnow()
            filename = f"health_check_{timestamp.strftime('%Y%m%d_%H%M%S')}.json"
            filepath = os.path.join(self.results_directory, filename)

            # Add metadata
            results['stored_at'] = timestamp.isoformat()
            results['scheduler_version'] = '1.2.0'

            # Write to file
            with open(filepath, 'w') as f:
                json.dump(results, f, indent=2, default=str)

            logger.info(f"Health check results stored to {filepath}")

        except Exception as e:
            logger.error(f"Failed to store health check results: {e}")

    async def process_notifications(self, results: Dict[str, Any]):
        """Process and send notifications based on health check results"""
        try:
            # Determine if notifications should be sent
            should_notify = self.should_send_notification(results)

            if not should_notify:
                logger.debug("Health check notification threshold not met")
                return

            # Send notifications based on configuration
            notifications = self.notification_config.get('targets', [])

            for notification in notifications:
                try:
                    await self.send_notification(notification, results)
                except Exception as e:
                    logger.error(f"Failed to send notification to {notification.get('type', 'unknown')}: {e}")

        except Exception as e:
            logger.error(f"Failed to process notifications: {e}")

    def should_send_notification(self, results: Dict[str, Any]) -> bool:
        """Determine if notifications should be sent based on results"""
        threshold = self.notification_config.get('threshold', 'high')

        if threshold == 'critical':
            return results['critical_issues'] > 0
        elif threshold == 'high':
            return results['critical_issues'] > 0 or results['high_issues'] > 0
        elif threshold == 'medium':
            return results['total_issues'] > 0
        elif threshold == 'low':
            return True
        elif threshold == 'none':
            return False

        return results['critical_issues'] > 0

    async def send_notification(self, notification_config: Dict[str, Any], results: Dict[str, Any]):
        """Send a notification using the specified method"""
        notification_type = notification_config.get('type')

        if notification_type == 'email':
            await self.send_email_notification(notification_config, results)
        elif notification_type == 'slack':
            await self.send_slack_notification(notification_config, results)
        elif notification_type == 'webhook':
            await self.send_webhook_notification(notification_config, results)
        elif notification_type == 'teams':
            await self.send_teams_notification(notification_config, results)
        else:
            logger.warning(f"Unknown notification type: {notification_type}")

    async def send_email_notification(self, config: Dict[str, Any], results: Dict[str, Any]):
        """Send email notification"""
        try:
            smtp_server = config.get('smtp_server', 'localhost')
            smtp_port = config.get('smtp_port', 587)
            username = config.get('username')
            password = config.get('password')
            from_email = config.get('from_email', 'articdbm@localhost')
            to_emails = config.get('to_emails', [])

            if not to_emails:
                logger.warning("No email recipients configured")
                return

            # Create message
            msg = MIMEMultipart()
            msg['From'] = from_email
            msg['To'] = ', '.join(to_emails)
            msg['Subject'] = f"ArticDBM Health Check Alert - {results['critical_issues']} Critical Issues"

            # Create email body
            body = self.create_email_body(results)
            msg.attach(MIMEText(body, 'html'))

            # Send email
            with smtplib.SMTP(smtp_server, smtp_port) as server:
                if username and password:
                    server.starttls()
                    server.login(username, password)

                server.send_message(msg)

            logger.info(f"Email notification sent to {len(to_emails)} recipients")

        except Exception as e:
            logger.error(f"Failed to send email notification: {e}")

    async def send_slack_notification(self, config: Dict[str, Any], results: Dict[str, Any]):
        """Send Slack notification"""
        try:
            webhook_url = config.get('webhook_url')
            channel = config.get('channel', '#alerts')

            if not webhook_url:
                logger.warning("No Slack webhook URL configured")
                return

            # Create Slack message
            color = 'danger' if results['critical_issues'] > 0 else 'warning'

            message = {
                "channel": channel,
                "username": "ArticDBM Health Check",
                "icon_emoji": ":warning:",
                "attachments": [
                    {
                        "color": color,
                        "title": "Database Health Check Alert",
                        "fields": [
                            {
                                "title": "Critical Issues",
                                "value": str(results['critical_issues']),
                                "short": True
                            },
                            {
                                "title": "High Issues",
                                "value": str(results['high_issues']),
                                "short": True
                            },
                            {
                                "title": "Total Issues",
                                "value": str(results['total_issues']),
                                "short": True
                            },
                            {
                                "title": "Servers Checked",
                                "value": str(results['servers_checked']),
                                "short": True
                            }
                        ],
                        "footer": "ArticDBM Health Checker",
                        "ts": int(datetime.utcnow().timestamp())
                    }
                ]
            }

            # Send to Slack
            response = requests.post(webhook_url, json=message, timeout=30)
            response.raise_for_status()

            logger.info("Slack notification sent successfully")

        except Exception as e:
            logger.error(f"Failed to send Slack notification: {e}")

    async def send_webhook_notification(self, config: Dict[str, Any], results: Dict[str, Any]):
        """Send webhook notification"""
        try:
            webhook_url = config.get('url')
            headers = config.get('headers', {'Content-Type': 'application/json'})

            if not webhook_url:
                logger.warning("No webhook URL configured")
                return

            # Create webhook payload
            payload = {
                "timestamp": datetime.utcnow().isoformat(),
                "service": "ArticDBM Health Check",
                "alert_type": "health_check_alert",
                "severity": "critical" if results['critical_issues'] > 0 else "warning",
                "summary": {
                    "total_issues": results['total_issues'],
                    "critical_issues": results['critical_issues'],
                    "high_issues": results['high_issues'],
                    "servers_checked": results['servers_checked']
                },
                "details": results
            }

            # Send webhook
            response = requests.post(webhook_url, json=payload, headers=headers, timeout=30)
            response.raise_for_status()

            logger.info("Webhook notification sent successfully")

        except Exception as e:
            logger.error(f"Failed to send webhook notification: {e}")

    async def send_teams_notification(self, config: Dict[str, Any], results: Dict[str, Any]):
        """Send Microsoft Teams notification"""
        try:
            webhook_url = config.get('webhook_url')

            if not webhook_url:
                logger.warning("No Teams webhook URL configured")
                return

            # Create Teams message
            color = "ff0000" if results['critical_issues'] > 0 else "ffaa00"

            message = {
                "@type": "MessageCard",
                "@context": "https://schema.org/extensions",
                "summary": "ArticDBM Health Check Alert",
                "themeColor": color,
                "title": "Database Health Check Alert",
                "sections": [
                    {
                        "facts": [
                            {"name": "Critical Issues", "value": str(results['critical_issues'])},
                            {"name": "High Issues", "value": str(results['high_issues'])},
                            {"name": "Total Issues", "value": str(results['total_issues'])},
                            {"name": "Servers Checked", "value": str(results['servers_checked'])},
                            {"name": "Scan Time", "value": results['scan_timestamp']}
                        ]
                    }
                ]
            }

            # Send to Teams
            response = requests.post(webhook_url, json=message, timeout=30)
            response.raise_for_status()

            logger.info("Teams notification sent successfully")

        except Exception as e:
            logger.error(f"Failed to send Teams notification: {e}")

    def create_email_body(self, results: Dict[str, Any]) -> str:
        """Create HTML email body for health check results"""
        html = f"""
        <!DOCTYPE html>
        <html>
        <head>
            <style>
                body {{ font-family: Arial, sans-serif; margin: 40px; }}
                .header {{ background: #f8f9fa; padding: 20px; border-radius: 5px; margin-bottom: 20px; }}
                .critical {{ color: #dc3545; font-weight: bold; }}
                .high {{ color: #fd7e14; font-weight: bold; }}
                .medium {{ color: #ffc107; }}
                .summary {{ margin: 20px 0; }}
                .server {{ margin: 15px 0; padding: 15px; border: 1px solid #ddd; border-radius: 5px; }}
                .issue {{ margin: 10px 0; padding: 10px; border-left: 3px solid #ccc; }}
                .issue.critical {{ border-left-color: #dc3545; }}
                .issue.high {{ border-left-color: #fd7e14; }}
                .issue.medium {{ border-left-color: #ffc107; }}
            </style>
        </head>
        <body>
            <div class="header">
                <h2>ArticDBM Database Health Check Report</h2>
                <p><strong>Scan Time:</strong> {results['scan_timestamp']}</p>
                <p><strong>Servers Checked:</strong> {results['servers_checked']}</p>
            </div>

            <div class="summary">
                <h3>Summary</h3>
                <p><span class="critical">Critical Issues: {results['critical_issues']}</span></p>
                <p><span class="high">High Issues: {results['high_issues']}</span></p>
                <p>Medium Issues: {results['medium_issues']}</p>
                <p>Low Issues: {results['low_issues']}</p>
                <p><strong>Total Issues: {results['total_issues']}</strong></p>
            </div>

            <div class="servers">
                <h3>Server Details</h3>
        """

        # Add server details
        for server in results.get('server_results', []):
            status_color = 'green' if server['status'] == 'connected' else 'red'
            html += f"""
                <div class="server">
                    <h4 style="color: {status_color};">{server['server_name']} ({server['server_type']})</h4>
                    <p><strong>Status:</strong> {server['status']}</p>
                    <p><strong>Issues Found:</strong> {len(server.get('issues', []))}</p>
            """

            # Add issues
            for issue in server.get('issues', [])[:5]:  # Limit to first 5 issues
                html += f"""
                    <div class="issue {issue['severity']}">
                        <strong>{issue['title']}</strong> ({issue['severity']})<br>
                        {issue['description']}<br>
                        <em>Recommendation: {issue['recommendation']}</em>
                    </div>
                """

            if len(server.get('issues', [])) > 5:
                html += f"<p><em>... and {len(server.get('issues', [])) - 5} more issues</em></p>"

            html += "</div>"

        html += """
            </div>

            <div style="margin-top: 30px; padding: 15px; background: #f8f9fa; border-radius: 5px;">
                <p><strong>Action Required:</strong> Please review and address the identified security and configuration issues.</p>
                <p>For detailed information, access the ArticDBM management portal.</p>
            </div>
        </body>
        </html>
        """

        return html

    async def send_weekly_summary(self):
        """Send weekly summary report"""
        try:
            logger.info("Generating weekly health check summary")

            # Get results from the past week
            week_results = await self.get_weekly_results()

            if not week_results:
                logger.info("No health check results from past week")
                return

            # Create summary
            summary = self.create_weekly_summary(week_results)

            # Send summary notifications
            for notification in self.notification_config.get('weekly_targets', []):
                try:
                    await self.send_weekly_summary_notification(notification, summary)
                except Exception as e:
                    logger.error(f"Failed to send weekly summary to {notification.get('type')}: {e}")

        except Exception as e:
            logger.error(f"Failed to send weekly summary: {e}")

    async def get_weekly_results(self) -> List[Dict[str, Any]]:
        """Get health check results from the past week"""
        try:
            results = []
            week_ago = datetime.utcnow() - timedelta(days=7)

            # Read result files from the past week
            for filename in os.listdir(self.results_directory):
                if not filename.startswith('health_check_') or not filename.endswith('.json'):
                    continue

                filepath = os.path.join(self.results_directory, filename)

                try:
                    # Check file modification time
                    mtime = datetime.utcfromtimestamp(os.path.getmtime(filepath))
                    if mtime < week_ago:
                        continue

                    # Load result
                    with open(filepath, 'r') as f:
                        result = json.load(f)
                        results.append(result)

                except Exception as e:
                    logger.warning(f"Failed to load result file {filename}: {e}")

            # Sort by timestamp
            results.sort(key=lambda x: x.get('scan_timestamp', ''))

            return results

        except Exception as e:
            logger.error(f"Failed to get weekly results: {e}")
            return []

    def create_weekly_summary(self, results: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Create weekly summary from health check results"""
        if not results:
            return {}

        # Calculate trends and statistics
        total_scans = len(results)
        latest_result = results[-1] if results else {}

        # Aggregate issues by severity
        critical_trend = [r['critical_issues'] for r in results]
        high_trend = [r['high_issues'] for r in results]
        total_trend = [r['total_issues'] for r in results]

        summary = {
            'week_period': f"{(datetime.utcnow() - timedelta(days=7)).strftime('%Y-%m-%d')} to {datetime.utcnow().strftime('%Y-%m-%d')}",
            'total_scans': total_scans,
            'latest_scan': latest_result.get('scan_timestamp'),
            'current_status': {
                'critical_issues': latest_result.get('critical_issues', 0),
                'high_issues': latest_result.get('high_issues', 0),
                'total_issues': latest_result.get('total_issues', 0),
                'servers_monitored': latest_result.get('servers_checked', 0)
            },
            'trends': {
                'critical_avg': sum(critical_trend) / len(critical_trend) if critical_trend else 0,
                'critical_max': max(critical_trend) if critical_trend else 0,
                'high_avg': sum(high_trend) / len(high_trend) if high_trend else 0,
                'total_avg': sum(total_trend) / len(total_trend) if total_trend else 0,
                'improving': len(total_trend) > 1 and total_trend[-1] < total_trend[0]
            },
            'most_common_issues': self.get_common_issues(results),
            'recommendations': self.get_weekly_recommendations(results)
        }

        return summary

    def get_common_issues(self, results: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Get most common issues across all scans"""
        issue_counts = {}

        for result in results:
            for server_result in result.get('server_results', []):
                for issue in server_result.get('issues', []):
                    issue_type = issue.get('type', 'unknown')
                    if issue_type not in issue_counts:
                        issue_counts[issue_type] = {
                            'type': issue_type,
                            'title': issue.get('title', ''),
                            'count': 0,
                            'severity': issue.get('severity', 'medium')
                        }
                    issue_counts[issue_type]['count'] += 1

        # Sort by frequency and return top 10
        common_issues = sorted(issue_counts.values(), key=lambda x: x['count'], reverse=True)
        return common_issues[:10]

    def get_weekly_recommendations(self, results: List[Dict[str, Any]]) -> List[str]:
        """Generate weekly recommendations based on trends"""
        recommendations = []

        if not results:
            return recommendations

        latest = results[-1]

        if latest.get('critical_issues', 0) > 0:
            recommendations.append("Address critical security issues immediately")

        if latest.get('total_issues', 0) > 20:
            recommendations.append("Consider implementing automated remediation for common issues")

        # Check for persistent issues
        if len(results) > 3:
            persistent_issues = all(r.get('total_issues', 0) > 5 for r in results[-3:])
            if persistent_issues:
                recommendations.append("Review and prioritize persistent security issues")

        if not recommendations:
            recommendations.append("Continue monitoring for new security issues")

        return recommendations

    async def send_weekly_summary_notification(self, notification_config: Dict[str, Any], summary: Dict[str, Any]):
        """Send weekly summary notification"""
        # Similar to daily notifications but with summary data
        await self.send_notification(notification_config, {'summary': summary, 'type': 'weekly_summary'})

    async def cleanup_old_results(self):
        """Clean up old health check result files"""
        try:
            cutoff_date = datetime.utcnow() - timedelta(days=self.max_stored_results)

            cleaned_count = 0
            for filename in os.listdir(self.results_directory):
                if not filename.startswith('health_check_') or not filename.endswith('.json'):
                    continue

                filepath = os.path.join(self.results_directory, filename)

                try:
                    # Check file age
                    mtime = datetime.utcfromtimestamp(os.path.getmtime(filepath))
                    if mtime < cutoff_date:
                        os.remove(filepath)
                        cleaned_count += 1

                except Exception as e:
                    logger.warning(f"Failed to remove old result file {filename}: {e}")

            if cleaned_count > 0:
                logger.info(f"Cleaned up {cleaned_count} old health check result files")

        except Exception as e:
            logger.error(f"Failed to cleanup old results: {e}")

    async def send_error_notification(self, error_message: str):
        """Send notification about scheduler errors"""
        try:
            error_notification = {
                'timestamp': datetime.utcnow().isoformat(),
                'service': 'ArticDBM Health Check Scheduler',
                'alert_type': 'scheduler_error',
                'severity': 'critical',
                'error': error_message
            }

            # Send to configured error notification targets
            for notification in self.notification_config.get('error_targets', []):
                try:
                    await self.send_notification(notification, error_notification)
                except Exception as e:
                    logger.error(f"Failed to send error notification: {e}")

        except Exception as e:
            logger.error(f"Failed to process error notification: {e}")

    async def run_manual_check(self) -> Dict[str, Any]:
        """Run a manual health check (for API endpoint)"""
        try:
            servers = await self.get_database_servers()
            results = await self.health_checker.check_all_databases(servers)

            # Store results with manual flag
            results['manual_scan'] = True
            await self.store_results(results)

            return results

        except Exception as e:
            logger.error(f"Manual health check failed: {e}")
            raise

    def get_scheduler_status(self) -> Dict[str, Any]:
        """Get scheduler status information"""
        return {
            'running': self.scheduler.running if self.scheduler else False,
            'next_run': self.scheduler.get_jobs()[0].next_run_time.isoformat() if self.scheduler and self.scheduler.get_jobs() else None,
            'last_scan': self.last_scan_results.get('scan_timestamp') if self.last_scan_results else None,
            'last_scan_issues': self.last_scan_results.get('total_issues', 0) if self.last_scan_results else 0,
            'schedule': self.scan_schedule,
            'notification_targets': len(self.notification_config.get('targets', [])),
            'results_stored': len([f for f in os.listdir(self.results_directory) if f.startswith('health_check_')]) if os.path.exists(self.results_directory) else 0
        }

# Integration with py4web app
def init_health_check_scheduler(config: Dict[str, Any]) -> HealthCheckScheduler:
    """Initialize the health check scheduler for py4web app"""
    scheduler = HealthCheckScheduler(config)
    return scheduler

# Example configuration
EXAMPLE_CONFIG = {
    "schedule": {
        "hour": 2,
        "minute": 0,
        "second": 0
    },
    "notifications": {
        "threshold": "high",  # critical, high, medium, low, none
        "targets": [
            {
                "type": "email",
                "smtp_server": "localhost",
                "smtp_port": 587,
                "from_email": "articdbm@example.com",
                "to_emails": ["admin@example.com"],
                "username": None,
                "password": None
            },
            {
                "type": "slack",
                "webhook_url": "https://hooks.slack.com/services/...",
                "channel": "#database-alerts"
            }
        ],
        "weekly_targets": [
            {
                "type": "email",
                "smtp_server": "localhost",
                "smtp_port": 587,
                "from_email": "articdbm@example.com",
                "to_emails": ["admin@example.com"]
            }
        ],
        "error_targets": [
            {
                "type": "email",
                "smtp_server": "localhost",
                "smtp_port": 587,
                "from_email": "articdbm@example.com",
                "to_emails": ["admin@example.com"]
            }
        ]
    },
    "results_directory": "/var/log/articdbm/health_checks",
    "max_stored_results": 30,
    "health_checker": {}
}