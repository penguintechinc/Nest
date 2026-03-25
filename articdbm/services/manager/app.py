import os
import json
import asyncio
import secrets
import hashlib
import re
import uuid
import tempfile
import shutil
import xml.etree.ElementTree as ET
import yaml
import random
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Any
from pathlib import Path
from io import StringIO
import threading
from concurrent.futures import ThreadPoolExecutor, ProcessPoolExecutor

from py4web import action, request, response, abort, Session, Cache, DAL, Field
from py4web.utils.cors import CORS
from py4web.utils.auth import Auth
from pydal.validators import *

import redis.asyncio as aioredis
import redis
from pydantic import BaseModel, Field as PydanticField
import requests

from kubernetes import client, config
from kubernetes.client.rest import ApiException
import boto3
from botocore.exceptions import ClientError, NoCredentialsError
from google.cloud import sql_v1
from google.oauth2 import service_account
import openai
import anthropic

db = DAL(
    os.getenv("DATABASE_URL", "postgresql://articdbm:articdbm@postgres/articdbm"),
    pool_size=20,
    migrate=True,
    fake_migrate_all=False
)

session = Session(secret=os.getenv("SESSION_SECRET", secrets.token_urlsafe(32)))
cache = Cache(size=1000)
auth = Auth(session, db, registration_requires_confirmation=False)
cors = CORS(origin="*", headers="*", methods="*")

redis_client = redis.Redis(
    host=os.getenv("REDIS_HOST", "redis"),
    port=int(os.getenv("REDIS_PORT", 6379)),
    password=os.getenv("REDIS_PASSWORD", ""),
    db=int(os.getenv("REDIS_DB", 0)),
    decode_responses=True
)

aio_redis = None

# Thread pools for performance optimization
thread_pool = ThreadPoolExecutor(max_workers=10, thread_name_prefix='articdbm-worker')
cpu_pool = ProcessPoolExecutor(max_workers=4)

# Cache for expensive operations
operation_cache = {}
cache_lock = threading.Lock()

async def init_aio_redis():
    global aio_redis
    aio_redis = await aioredis.create_redis_pool(
        f'redis://{os.getenv("REDIS_HOST", "redis")}:{os.getenv("REDIS_PORT", 6379)}',
        password=os.getenv("REDIS_PASSWORD", None),
        db=int(os.getenv("REDIS_DB", 0))
    )

def get_cached_result(key: str) -> Optional[Any]:
    """Get cached result for expensive operations"""
    with cache_lock:
        if key in operation_cache:
            result, timestamp = operation_cache[key]
            if datetime.utcnow() - timestamp < timedelta(minutes=5):
                return result
            else:
                del operation_cache[key]
    return None

def set_cached_result(key: str, result: Any) -> None:
    """Cache result for expensive operations"""
    with cache_lock:
        operation_cache[key] = (result, datetime.utcnow())
        
        # Clean old entries if cache gets too large
        if len(operation_cache) > 100:
            oldest_keys = sorted(operation_cache.keys(), 
                               key=lambda k: operation_cache[k][1])[:20]
            for old_key in oldest_keys:
                del operation_cache[old_key]

async def run_in_thread(func, *args, **kwargs):
    """Run CPU-intensive function in thread pool"""
    loop = asyncio.get_event_loop()
    return await loop.run_in_executor(thread_pool, func, *args, **kwargs)

async def run_in_process(func, *args, **kwargs):
    """Run CPU-intensive function in process pool"""
    loop = asyncio.get_event_loop()
    return await loop.run_in_executor(cpu_pool, func, *args, **kwargs)

def batch_database_operations(operations: List[callable], batch_size: int = 50):
    """Execute database operations in batches for better performance"""
    results = []
    for i in range(0, len(operations), batch_size):
        batch = operations[i:i + batch_size]
        batch_results = []
        
        # Start a transaction for the batch
        db._adapter.execute('BEGIN')
        try:
            for op in batch:
                batch_results.append(op())
            db._adapter.execute('COMMIT')
            results.extend(batch_results)
        except Exception as e:
            db._adapter.execute('ROLLBACK')
            raise e
    
    return results

async def parallel_api_calls(api_calls: List[tuple]) -> List[Any]:
    """Execute multiple API calls in parallel"""
    tasks = []
    for func, args, kwargs in api_calls:
        if asyncio.iscoroutinefunction(func):
            tasks.append(func(*args, **kwargs))
        else:
            tasks.append(run_in_thread(func, *args, **kwargs))
    
    return await asyncio.gather(*tasks, return_exceptions=True)

db.define_table(
    'database_server',
    Field('name', 'string', required=True, unique=True),
    Field('type', 'string', requires=IS_IN_SET(['mysql', 'postgresql', 'mssql', 'mongodb', 'redis'])),
    Field('host', 'string', required=True),
    Field('port', 'integer', required=True),
    Field('username', 'string'),
    Field('password', 'string'),
    Field('database', 'string'),
    Field('role', 'string', requires=IS_IN_SET(['read', 'write', 'both']), default='both'),
    Field('weight', 'integer', default=1),
    Field('tls_enabled', 'boolean', default=False),
    Field('active', 'boolean', default=True),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('updated_at', 'datetime', update=datetime.utcnow)
)

db.define_table(
    'user_permission',
    Field('user_id', 'reference auth_user', required=True),
    Field('database_name', 'string', required=True),
    Field('table_name', 'string', default='*'),
    Field('actions', 'list:string', default=['read']),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('updated_at', 'datetime', update=datetime.utcnow),
    Field('expires_at', 'datetime'),  # Time-limited permissions
    Field('max_queries', 'integer', default=0),  # 0 = unlimited
    Field('query_count', 'integer', default=0)   # Track usage
)

# Enhanced user profiles with security settings
db.define_table(
    'user_profile',
    Field('user_id', 'reference auth_user', unique=True, required=True),
    Field('api_key', 'string', unique=True),
    Field('require_tls', 'boolean', default=False),
    Field('allowed_ips', 'list:string'),  # IP whitelist
    Field('rate_limit', 'integer', default=0),  # requests per second, 0 = no limit
    Field('expires_at', 'datetime'),  # Account expiration
    Field('is_temporary', 'boolean', default=False),
    Field('created_by', 'reference auth_user'),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('last_used', 'datetime'),
    Field('usage_count', 'integer', default=0)
)

# One-time access tokens for temporary database access
db.define_table(
    'temporary_access',
    Field('token', 'string', unique=True, required=True),
    Field('database_name', 'string', required=True),
    Field('table_name', 'string', default='*'),
    Field('actions', 'list:string', required=True),  # ['read'] or ['write'] or both
    Field('created_by', 'reference auth_user', required=True),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('expires_at', 'datetime', required=True),
    Field('max_uses', 'integer', default=1),  # How many times it can be used
    Field('use_count', 'integer', default=0),
    Field('last_used', 'datetime'),
    Field('client_ip', 'string'),  # Restrict to specific IP
    Field('is_active', 'boolean', default=True)
)

db.define_table(
    'audit_log',
    Field('user_id', 'reference auth_user'),
    Field('action', 'string'),
    Field('database_name', 'string'),
    Field('table_name', 'string'),
    Field('query', 'text'),
    Field('result', 'string'),
    Field('ip_address', 'string'),
    Field('timestamp', 'datetime', default=datetime.utcnow)
)

db.define_table(
    'security_rule',
    Field('name', 'string', required=True, unique=True),
    Field('pattern', 'text', required=True),
    Field('action', 'string', requires=IS_IN_SET(['block', 'alert', 'log'])),
    Field('severity', 'string', requires=IS_IN_SET(['low', 'medium', 'high', 'critical'])),
    Field('active', 'boolean', default=True),
    Field('created_at', 'datetime', default=datetime.utcnow)
)

db.define_table(
    'managed_database',
    Field('name', 'string', required=True, unique=True),
    Field('server_id', 'reference database_server', required=True),
    Field('database_name', 'string', required=True),
    Field('description', 'text'),
    Field('schema_version', 'string'),
    Field('auto_backup', 'boolean', default=False),
    Field('backup_schedule', 'string'),
    Field('active', 'boolean', default=True),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('updated_at', 'datetime', update=datetime.utcnow)
)

db.define_table(
    'sql_file',
    Field('name', 'string', required=True),
    Field('database_id', 'reference managed_database', required=True),
    Field('file_type', 'string', requires=IS_IN_SET(['init', 'backup', 'migration', 'patch'])),
    Field('file_path', 'string', required=True),
    Field('file_size', 'integer'),
    Field('checksum', 'string'),
    Field('syntax_validated', 'boolean', default=False),
    Field('security_validated', 'boolean', default=False),
    Field('validation_errors', 'text'),
    Field('executed', 'boolean', default=False),
    Field('executed_at', 'datetime'),
    Field('executed_by', 'reference auth_user'),
    Field('created_at', 'datetime', default=datetime.utcnow)
)

db.define_table(
    'blocked_database',
    Field('name', 'string', required=True),
    Field('type', 'string', requires=IS_IN_SET(['database', 'username', 'table'])),
    Field('pattern', 'string', required=True),
    Field('reason', 'text'),
    Field('active', 'boolean', default=True),
    Field('created_at', 'datetime', default=datetime.utcnow)
)

db.define_table(
    'database_schema',
    Field('database_id', 'reference managed_database', required=True),
    Field('table_name', 'string', required=True),
    Field('column_name', 'string', required=True),
    Field('data_type', 'string', required=True),
    Field('is_nullable', 'boolean', default=True),
    Field('default_value', 'string'),
    Field('is_primary_key', 'boolean', default=False),
    Field('is_foreign_key', 'boolean', default=False),
    Field('foreign_table', 'string'),
    Field('foreign_column', 'string'),
    Field('created_at', 'datetime', default=datetime.utcnow)
)

db.define_table(
    'threat_intel_feed',
    Field('name', 'string', required=True, unique=True),
    Field('type', 'string', requires=IS_IN_SET(['stix', 'taxii', 'openioc', 'misp', 'custom'])),
    Field('url', 'string'),
    Field('api_key', 'string'),
    Field('username', 'string'),
    Field('password', 'string'),
    Field('polling_interval', 'integer', default=3600),
    Field('last_polled', 'datetime'),
    Field('active', 'boolean', default=True),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('updated_at', 'datetime', update=datetime.utcnow)
)

db.define_table(
    'threat_intel_indicator',
    Field('feed_id', 'reference threat_intel_feed'),
    Field('indicator_type', 'string', requires=IS_IN_SET(['ip', 'domain', 'url', 'hash', 'email', 'pattern', 'sql_pattern', 'user_agent'])),
    Field('value', 'string', required=True),
    Field('threat_level', 'string', requires=IS_IN_SET(['info', 'low', 'medium', 'high', 'critical'])),
    Field('confidence', 'integer', default=50),
    Field('description', 'text'),
    Field('tags', 'list:string'),
    Field('first_seen', 'datetime', default=datetime.utcnow),
    Field('last_seen', 'datetime', default=datetime.utcnow),
    Field('expires', 'datetime'),
    Field('active', 'boolean', default=True),
    Field('matched_count', 'integer', default=0),
    Field('created_at', 'datetime', default=datetime.utcnow)
)

db.define_table(
    'database_security_config',
    Field('database_id', 'reference managed_database', required=True, unique=True),
    Field('security_blocks_enabled', 'boolean', default=True),
    Field('threat_intel_blocks_enabled', 'boolean', default=True),
    Field('sql_injection_detection', 'boolean', default=True),
    Field('audit_logging', 'boolean', default=True),
    Field('block_default_resources', 'boolean', default=True),
    Field('threat_intel_action', 'string', requires=IS_IN_SET(['block', 'alert', 'log']), default='block'),
    Field('custom_rules', 'text'),
    Field('whitelist_patterns', 'text'),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('updated_at', 'datetime', update=datetime.utcnow)
)

db.define_table(
    'threat_intel_match',
    Field('indicator_id', 'reference threat_intel_indicator', required=True),
    Field('database_name', 'string'),
    Field('user_id', 'reference auth_user'),
    Field('source_ip', 'string'),
    Field('query', 'text'),
    Field('action_taken', 'string', requires=IS_IN_SET(['blocked', 'alerted', 'logged'])),
    Field('match_details', 'text'),
    Field('timestamp', 'datetime', default=datetime.utcnow)
)

db.define_table(
    'license_info',
    Field('license_key', 'string', unique=True),
    Field('tier', 'string', requires=IS_IN_SET(['community', 'enterprise']), default='community'),
    Field('features', 'json'),
    Field('database_count', 'integer', default=0),
    Field('is_active', 'boolean', default=False),
    Field('last_validated', 'datetime'),
    Field('next_validation', 'datetime'),
    Field('validation_failures', 'integer', default=0),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('updated_at', 'datetime', update=datetime.utcnow)
)

db.define_table(
    'cloud_provider',
    Field('name', 'string', required=True),
    Field('provider_type', 'string', requires=IS_IN_SET(['kubernetes', 'aws', 'gcp']), required=True),
    Field('configuration', 'json', required=True),
    Field('credentials_path', 'string'),
    Field('is_active', 'boolean', default=True),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('updated_at', 'datetime', update=datetime.utcnow),
    Field('last_tested', 'datetime'),
    Field('test_status', 'string', requires=IS_IN_SET(['success', 'failed', 'pending']), default='pending')
)

db.define_table(
    'cloud_database_instance',
    Field('name', 'string', required=True),
    Field('provider_id', 'reference cloud_provider', required=True),
    Field('instance_type', 'string', requires=IS_IN_SET(['mysql', 'postgresql', 'mssql', 'mongodb', 'redis']), required=True),
    Field('instance_class', 'string'),
    Field('storage_size', 'integer'),
    Field('engine_version', 'string'),
    Field('multi_az', 'boolean', default=False),
    Field('backup_retention', 'integer', default=7),
    Field('monitoring_enabled', 'boolean', default=True),
    Field('auto_scaling_enabled', 'boolean', default=False),
    Field('auto_scaling_config', 'json'),
    Field('cloud_instance_id', 'string'),
    Field('endpoint', 'string'),
    Field('port', 'integer'),
    Field('status', 'string', requires=IS_IN_SET(['creating', 'available', 'modifying', 'deleting', 'failed']), default='creating'),
    Field('server_id', 'reference database_server'),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('updated_at', 'datetime', update=datetime.utcnow),
    Field('metrics_data', 'json'),
    Field('last_scaled', 'datetime')
)

db.define_table(
    'scaling_policy',
    Field('cloud_instance_id', 'reference cloud_database_instance', required=True),
    Field('metric_type', 'string', requires=IS_IN_SET(['cpu', 'memory', 'connections', 'iops']), required=True),
    Field('scale_up_threshold', 'double', required=True),
    Field('scale_down_threshold', 'double', required=True),
    Field('scale_up_adjustment', 'integer', default=1),
    Field('scale_down_adjustment', 'integer', default=-1),
    Field('cooldown_period', 'integer', default=300),
    Field('ai_enabled', 'boolean', default=False),
    Field('ai_model', 'string', requires=IS_IN_SET(['openai', 'anthropic', 'ollama']), default='openai'),
    Field('is_active', 'boolean', default=True),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('updated_at', 'datetime', update=datetime.utcnow)
)

db.define_table(
    'scaling_event',
    Field('cloud_instance_id', 'reference cloud_database_instance', required=True),
    Field('trigger_type', 'string', requires=IS_IN_SET(['threshold', 'ai', 'manual']), required=True),
    Field('action', 'string', requires=IS_IN_SET(['scale_up', 'scale_down']), required=True),
    Field('old_instance_class', 'string'),
    Field('new_instance_class', 'string'),
    Field('trigger_metric', 'string'),
    Field('trigger_value', 'double'),
    Field('ai_confidence', 'double'),
    Field('ai_reasoning', 'text'),
    Field('status', 'string', requires=IS_IN_SET(['pending', 'in_progress', 'completed', 'failed']), default='pending'),
    Field('error_message', 'text'),
    Field('created_at', 'datetime', default=datetime.utcnow),
    Field('completed_at', 'datetime')
)

class DatabaseServerModel(BaseModel):
    name: str
    type: str
    host: str
    port: int
    username: Optional[str] = None
    password: Optional[str] = None
    database: Optional[str] = None
    role: str = 'both'
    weight: int = 1
    tls_enabled: bool = False
    active: bool = True

class PermissionModel(BaseModel):
    user_id: int
    database_name: str
    table_name: str = '*'
    actions: List[str] = ['read']

class SecurityRuleModel(BaseModel):
    name: str
    pattern: str
    action: str
    severity: str
    active: bool = True

class ManagedDatabaseModel(BaseModel):
    name: str
    server_id: int
    database_name: str
    description: Optional[str] = None
    schema_version: Optional[str] = None
    auto_backup: bool = False
    backup_schedule: Optional[str] = None
    active: bool = True

class SQLFileModel(BaseModel):
    name: str
    database_id: int
    file_type: str
    file_content: str

class BlockedDatabaseModel(BaseModel):
    name: str
    type: str
    pattern: str
    reason: Optional[str] = None
    active: bool = True

class ThreatIntelFeedModel(BaseModel):
    name: str
    type: str
    url: Optional[str] = None
    api_key: Optional[str] = None
    username: Optional[str] = None
    password: Optional[str] = None
    polling_interval: int = 3600
    active: bool = True

class ThreatIntelIndicatorModel(BaseModel):
    feed_id: Optional[int] = None
    indicator_type: str
    value: str
    threat_level: str = 'medium'
    confidence: int = 50
    description: Optional[str] = None
    tags: List[str] = []
    expires: Optional[datetime] = None
    active: bool = True

class DatabaseSecurityConfigModel(BaseModel):
    database_id: int
    security_blocks_enabled: bool = True
    threat_intel_blocks_enabled: bool = True
    sql_injection_detection: bool = True
    audit_logging: bool = True
    block_default_resources: bool = True
    threat_intel_action: str = 'block'
    custom_rules: Optional[str] = None
    whitelist_patterns: Optional[str] = None

class CloudProviderModel(BaseModel):
    name: str
    provider_type: str
    configuration: Dict[str, Any]
    credentials_path: Optional[str] = None
    is_active: bool = True

class CloudDatabaseInstanceModel(BaseModel):
    name: str
    provider_id: int
    instance_type: str
    instance_class: Optional[str] = None
    storage_size: Optional[int] = None
    engine_version: Optional[str] = None
    multi_az: bool = False
    backup_retention: int = 7
    monitoring_enabled: bool = True
    auto_scaling_enabled: bool = False
    auto_scaling_config: Optional[Dict[str, Any]] = None

class ScalingPolicyModel(BaseModel):
    cloud_instance_id: int
    metric_type: str
    scale_up_threshold: float
    scale_down_threshold: float
    scale_up_adjustment: int = 1
    scale_down_adjustment: int = -1
    cooldown_period: int = 300
    ai_enabled: bool = False
    ai_model: str = 'openai'
    is_active: bool = True

def parse_stix_indicators(stix_content: str) -> List[Dict[str, Any]]:
    """Parse STIX 2.x JSON format for threat indicators"""
    indicators = []
    try:
        stix_data = json.loads(stix_content)
        
        for obj in stix_data.get('objects', []):
            if obj.get('type') == 'indicator':
                pattern = obj.get('pattern', '')
                
                indicator = {
                    'indicator_type': 'pattern',
                    'value': pattern,
                    'description': obj.get('description', ''),
                    'threat_level': 'medium',
                    'confidence': 75,
                    'tags': obj.get('labels', []),
                    'first_seen': obj.get('created'),
                    'expires': obj.get('valid_until')
                }
                
                if '[ipv4-addr:value =' in pattern:
                    match = re.search(r"\[ipv4-addr:value = '([^']+)'\]", pattern)
                    if match:
                        indicator['indicator_type'] = 'ip'
                        indicator['value'] = match.group(1)
                elif '[domain-name:value =' in pattern:
                    match = re.search(r"\[domain-name:value = '([^']+)'\]", pattern)
                    if match:
                        indicator['indicator_type'] = 'domain'
                        indicator['value'] = match.group(1)
                elif '[url:value =' in pattern:
                    match = re.search(r"\[url:value = '([^']+)'\]", pattern)
                    if match:
                        indicator['indicator_type'] = 'url'
                        indicator['value'] = match.group(1)
                elif '[file:hashes.' in pattern:
                    match = re.search(r"\[file:hashes\.[\w]+\s*=\s*'([^']+)'\]", pattern)
                    if match:
                        indicator['indicator_type'] = 'hash'
                        indicator['value'] = match.group(1)
                
                if 'kill_chain_phases' in obj:
                    phases = [phase.get('phase_name', '') for phase in obj['kill_chain_phases']]
                    indicator['tags'].extend(phases)
                
                severity_map = {
                    'low': 'low',
                    'medium': 'medium', 
                    'high': 'high',
                    'critical': 'critical'
                }
                for label in obj.get('labels', []):
                    if label.lower() in severity_map:
                        indicator['threat_level'] = severity_map[label.lower()]
                
                indicators.append(indicator)
                
            elif obj.get('type') == 'malware':
                indicator = {
                    'indicator_type': 'pattern',
                    'value': obj.get('name', 'unknown'),
                    'description': f"Malware: {obj.get('description', '')}",
                    'threat_level': 'high',
                    'confidence': 80,
                    'tags': obj.get('labels', []) + ['malware'],
                    'first_seen': obj.get('created')
                }
                indicators.append(indicator)
                
            elif obj.get('type') == 'threat-actor':
                indicator = {
                    'indicator_type': 'pattern',
                    'value': obj.get('name', 'unknown'),
                    'description': f"Threat Actor: {obj.get('description', '')}",
                    'threat_level': 'high',
                    'confidence': 70,
                    'tags': obj.get('labels', []) + ['threat-actor'],
                    'first_seen': obj.get('created')
                }
                indicators.append(indicator)
                
    except json.JSONDecodeError as e:
        print(f"Error parsing STIX JSON: {e}")
    except Exception as e:
        print(f"Error processing STIX data: {e}")
    
    return indicators

def parse_taxii_feed(taxii_url: str, username: str = None, password: str = None, api_key: str = None) -> List[Dict[str, Any]]:
    """Fetch and parse TAXII 2.x feed"""
    indicators = []
    try:
        headers = {
            'Accept': 'application/taxii+json;version=2.1',
            'Content-Type': 'application/taxii+json;version=2.1'
        }
        
        if api_key:
            headers['Authorization'] = f'Bearer {api_key}'
        
        auth = None
        if username and password:
            auth = (username, password)
        
        response = requests.get(taxii_url, headers=headers, auth=auth, timeout=30)
        response.raise_for_status()
        
        taxii_data = response.json()
        
        if 'objects' in taxii_data:
            stix_content = json.dumps(taxii_data)
            indicators = parse_stix_indicators(stix_content)
        else:
            collections = taxii_data.get('collections', [])
            for collection in collections:
                collection_url = f"{taxii_url}/collections/{collection['id']}/objects"
                coll_response = requests.get(collection_url, headers=headers, auth=auth, timeout=30)
                if coll_response.status_code == 200:
                    coll_data = coll_response.json()
                    stix_content = json.dumps(coll_data)
                    indicators.extend(parse_stix_indicators(stix_content))
                    
    except requests.RequestException as e:
        print(f"Error fetching TAXII feed: {e}")
    except Exception as e:
        print(f"Error processing TAXII data: {e}")
    
    return indicators

def parse_openioc_indicators(openioc_content: str) -> List[Dict[str, Any]]:
    """Parse OpenIOC 1.1 XML format for threat indicators"""
    indicators = []
    try:
        root = ET.fromstring(openioc_content)
        
        ns = {'ioc': 'http://openioc.org/schemas/OpenIOC_1.1'}
        
        ioc_id = root.get('id', 'unknown')
        description = root.findtext('.//ioc:short_description', '', ns) or \
                     root.findtext('.//ioc:description', '', ns)
        
        for indicator_item in root.findall('.//ioc:IndicatorItem', ns):
            context = indicator_item.find('.//ioc:Context', ns)
            content = indicator_item.find('.//ioc:Content', ns)
            
            if context is not None and content is not None:
                context_type = context.get('search', '').lower()
                value = content.text
                
                if not value:
                    continue
                
                indicator_type = 'pattern'
                
                if 'ipv4' in context_type or 'remoteip' in context_type:
                    indicator_type = 'ip'
                elif 'hostname' in context_type or 'dns' in context_type:
                    indicator_type = 'domain'
                elif 'url' in context_type or 'uri' in context_type:
                    indicator_type = 'url'
                elif 'md5' in context_type or 'sha' in context_type or 'hash' in context_type:
                    indicator_type = 'hash'
                elif 'email' in context_type:
                    indicator_type = 'email'
                elif 'useragent' in context_type:
                    indicator_type = 'user_agent'
                elif 'sql' in context_type or 'query' in context_type:
                    indicator_type = 'sql_pattern'
                
                indicator = {
                    'indicator_type': indicator_type,
                    'value': value,
                    'description': f"{description} (IOC: {ioc_id})",
                    'threat_level': 'medium',
                    'confidence': 60,
                    'tags': ['openioc'],
                    'first_seen': datetime.utcnow()
                }
                
                indicators.append(indicator)
        
        for link in root.findall('.//ioc:link', ns):
            rel = link.get('rel', '')
            if rel == 'category':
                category = link.text
                for indicator in indicators:
                    indicator['tags'].append(category)
                    
    except ET.ParseError as e:
        print(f"Error parsing OpenIOC XML: {e}")
    except Exception as e:
        print(f"Error processing OpenIOC data: {e}")
    
    return indicators

def parse_misp_event(misp_content: str) -> List[Dict[str, Any]]:
    """Parse MISP event JSON format for threat indicators"""
    indicators = []
    try:
        misp_data = json.loads(misp_content)
        
        event = misp_data.get('Event', misp_data)
        
        threat_level_map = {
            '1': 'critical',
            '2': 'high',
            '3': 'medium',
            '4': 'low'
        }
        
        event_threat_level = threat_level_map.get(str(event.get('threat_level_id', '3')), 'medium')
        event_tags = [tag.get('name', '') for tag in event.get('Tag', [])]
        
        for attribute in event.get('Attribute', []):
            attr_type = attribute.get('type', '').lower()
            value = attribute.get('value', '')
            
            if not value:
                continue
            
            indicator_type = 'pattern'
            
            if attr_type in ['ip-src', 'ip-dst', 'ip']:
                indicator_type = 'ip'
            elif attr_type in ['domain', 'hostname']:
                indicator_type = 'domain'
            elif attr_type in ['url', 'uri']:
                indicator_type = 'url'
            elif attr_type in ['md5', 'sha1', 'sha256', 'sha512', 'hash']:
                indicator_type = 'hash'
            elif attr_type in ['email', 'email-src', 'email-dst']:
                indicator_type = 'email'
            elif attr_type == 'user-agent':
                indicator_type = 'user_agent'
            
            indicator = {
                'indicator_type': indicator_type,
                'value': value,
                'description': attribute.get('comment', '') or event.get('info', ''),
                'threat_level': event_threat_level,
                'confidence': 100 if attribute.get('to_ids', False) else 50,
                'tags': event_tags + [attr_type, 'misp'],
                'first_seen': attribute.get('timestamp'),
                'expires': None
            }
            
            indicators.append(indicator)
        
        for obj in event.get('Object', []):
            for attribute in obj.get('Attribute', []):
                attr_type = attribute.get('type', '').lower()
                value = attribute.get('value', '')
                
                if not value:
                    continue
                
                indicator_type = 'pattern'
                
                if attr_type in ['ip-src', 'ip-dst', 'ip']:
                    indicator_type = 'ip'
                elif attr_type in ['domain', 'hostname']:
                    indicator_type = 'domain'
                elif attr_type in ['url', 'uri']:
                    indicator_type = 'url'
                elif attr_type in ['md5', 'sha1', 'sha256', 'sha512']:
                    indicator_type = 'hash'
                
                indicator = {
                    'indicator_type': indicator_type,
                    'value': value,
                    'description': f"{obj.get('name', 'Object')}: {attribute.get('comment', '')}",
                    'threat_level': event_threat_level,
                    'confidence': 75,
                    'tags': event_tags + [attr_type, 'misp', obj.get('name', '')],
                    'first_seen': attribute.get('timestamp'),
                    'expires': None
                }
                
                indicators.append(indicator)
                
    except json.JSONDecodeError as e:
        print(f"Error parsing MISP JSON: {e}")
    except Exception as e:
        print(f"Error processing MISP data: {e}")
    
    return indicators

def validate_license_with_server(license_key: str) -> Dict[str, any]:
    """Validate license with license.penguintech.io server"""
    try:
        response = requests.post(
            'https://license.penguintech.io/api/validate',
            json={'license_key': license_key, 'product': 'articdbm'},
            timeout=30
        )
        
        if response.status_code == 200:
            data = response.json()
            return {
                'valid': data.get('valid', False),
                'tier': data.get('tier', 'community'),
                'features': data.get('features', []),
                'expires_at': data.get('expires_at'),
                'error': None
            }
        else:
            return {
                'valid': False,
                'tier': 'community',
                'features': [],
                'expires_at': None,
                'error': f'Server returned {response.status_code}'
            }
    except Exception as e:
        return {
            'valid': False,
            'tier': 'community',
            'features': [],
            'expires_at': None,
            'error': str(e)
        }

def get_current_license_info():
    """Get current license information from database"""
    license_record = db(db.license_info).select().first()
    if not license_record:
        return {
            'tier': 'community',
            'features': ['single_threat_intel_feed'],
            'is_active': False,
            'database_count': 0
        }
    
    return {
        'tier': license_record.tier,
        'features': license_record.features or [],
        'is_active': license_record.is_active,
        'database_count': license_record.database_count,
        'license_key': license_record.license_key if license_record.license_key else None
    }

def update_license_validation():
    """Update license validation status"""
    license_record = db(db.license_info).select().first()
    if not license_record or not license_record.license_key:
        return
    
    validation_result = validate_license_with_server(license_record.license_key)
    
    # Calculate next validation time (random between 45-180 seconds)
    next_validation = datetime.utcnow() + timedelta(seconds=random.randint(45, 180))
    
    if validation_result['valid']:
        license_record.update_record(
            tier=validation_result['tier'],
            features=validation_result['features'],
            is_active=True,
            last_validated=datetime.utcnow(),
            next_validation=next_validation,
            validation_failures=0
        )
    else:
        failures = license_record.validation_failures + 1
        # After 3 failures, disable enterprise features
        is_active = failures < 3
        
        license_record.update_record(
            is_active=is_active,
            last_validated=datetime.utcnow(),
            next_validation=next_validation,
            validation_failures=failures
        )

def is_enterprise_feature_enabled(feature: str) -> bool:
    """Check if an enterprise feature is enabled"""
    license_info = get_current_license_info()
    
    if not license_info['is_active'] or license_info['tier'] != 'enterprise':
        return False
    
    return feature in license_info['features']

def get_threat_intel_limit() -> int:
    """Get the maximum number of threat intel feeds allowed"""
    if is_enterprise_feature_enabled('unlimited_threat_intel_feeds'):
        return -1  # Unlimited
    return 1  # Community limit

def validate_sql_security(content: str) -> Dict[str, any]:
    """Comprehensive SQL security validation"""
    errors = []
    warnings = []
    severity = "low"
    
    # Normalize content for analysis
    normalized = content.lower().strip()
    lines = content.split('\n')
    
    # Dangerous patterns that should be blocked
    dangerous_patterns = {
        r'exec\s*\(': "Potentially dangerous EXEC statement",
        r'xp_cmdshell': "System command execution detected",
        r'sp_oacreate': "OLE automation detected",
        r'openrowset': "External data source access",
        r'opendatasource': "External data source access",
        r'bulk\s+insert': "Bulk insert operation",
        r'into\s+outfile': "File write operation detected",
        r'load_file\s*\(': "File read operation detected",
        r'@@version': "System information disclosure",
        r'information_schema': "Schema introspection detected",
        r'sys\.': "System table access",
        r'master\.': "Master database access",
        r'msdb\.': "MSDB database access",
        r'tempdb\.': "TempDB access",
        r'--\s*[^\r\n]*(\r?\n|$)': "SQL comments detected",
        r'/\*.*?\*/': "Block comments detected",
        r'\bor\s+1\s*=\s*1\b': "Classic SQL injection pattern",
        r'\bunion\s+select\b': "Union-based injection pattern",
        r';\s*(drop|truncate|delete)\s+': "Destructive operations in sequence",
    }
    
    # Shell command patterns
    shell_patterns = {
        r'\bcmd\b': "Command shell reference",
        r'\bpowershell\b': "PowerShell reference",
        r'\bbash\b': "Bash shell reference",
        r'\bsh\b': "Shell reference",
        r'/bin/': "Unix binary path",
        r'system\s*\(': "System call",
        r'shell_exec': "Shell execution",
        r'passthru': "Command execution",
        r'proc_open': "Process execution",
    }
    
    # Check for dangerous SQL patterns
    for pattern, description in dangerous_patterns.items():
        if re.search(pattern, normalized, re.IGNORECASE | re.MULTILINE):
            errors.append(f"{description}: {pattern}")
            severity = "high"
    
    # Check for shell command patterns
    for pattern, description in shell_patterns.items():
        if re.search(pattern, normalized, re.IGNORECASE):
            errors.append(f"Shell command detected - {description}: {pattern}")
            severity = "critical"
    
    # Check for default/test database access patterns
    default_db_patterns = {
        r'\btest\b': "Test database access",
        r'\bsample\b': "Sample database access",  
        r'\bdemo\b': "Demo database access",
        r'\bexample\b': "Example database access",
        r'\bsa\b': "Default 'sa' user detected",
        r'\broot\b': "Root user access",
        r'\badmin\b': "Admin user access",
        r'\bguest\b': "Guest user access",
    }
    
    for pattern, description in default_db_patterns.items():
        if re.search(pattern, normalized, re.IGNORECASE):
            warnings.append(f"Potential default/test resource access - {description}")
            if severity == "low":
                severity = "medium"
    
    # Basic syntax validation
    bracket_count = content.count('(') - content.count(')')
    if bracket_count != 0:
        errors.append("Unmatched parentheses in SQL")
    
    quote_count = content.count("'") % 2
    if quote_count != 0:
        errors.append("Unmatched single quotes in SQL")
    
    # Check for multiple statements (potential SQL injection)
    semicolon_statements = [s.strip() for s in content.split(';') if s.strip()]
    if len(semicolon_statements) > 1:
        statement_types = []
        for stmt in semicolon_statements:
            first_word = stmt.split()[0].upper() if stmt.split() else ""
            statement_types.append(first_word)
        
        if len(set(statement_types)) > 1:
            warnings.append(f"Multiple different statement types detected: {', '.join(set(statement_types))}")
            if severity == "low":
                severity = "medium"
    
    # Check for extremely long lines (potential obfuscation)
    for i, line in enumerate(lines, 1):
        if len(line) > 1000:
            warnings.append(f"Extremely long line detected (line {i}): {len(line)} characters")
    
    # Check for non-ASCII characters (potential encoding attacks)
    try:
        content.encode('ascii')
    except UnicodeEncodeError:
        warnings.append("Non-ASCII characters detected - potential encoding attack")
    
    return {
        'valid': len(errors) == 0,
        'errors': errors,
        'warnings': warnings,
        'severity': severity,
        'patterns_checked': len(dangerous_patterns) + len(shell_patterns) + len(default_db_patterns)
    }

# ==================== USER MANAGEMENT HELPER FUNCTIONS ====================

def generate_api_key():
    """Generate a secure API key"""
    return secrets.token_urlsafe(32)

def generate_temp_token():
    """Generate a temporary access token"""
    return f"tmp_{secrets.token_urlsafe(24)}"

def create_user_with_profile(username, email, password, **profile_options):
    """Create a user with enhanced profile settings"""
    # Create basic user account
    user_id = db.auth_user.insert(
        username=username,
        email=email,
        password=auth.password.encrypt(password),
        first_name=profile_options.get('first_name', ''),
        last_name=profile_options.get('last_name', '')
    )
    
    # Create enhanced user profile
    profile_data = {
        'user_id': user_id,
        'api_key': generate_api_key(),
        'require_tls': profile_options.get('require_tls', False),
        'allowed_ips': profile_options.get('allowed_ips', []),
        'rate_limit': profile_options.get('rate_limit', 0),
        'expires_at': profile_options.get('expires_at'),
        'is_temporary': profile_options.get('is_temporary', False),
        'created_by': profile_options.get('created_by')
    }
    
    db.user_profile.insert(**profile_data)
    db.commit()
    
    return user_id

def sync_users_to_redis():
    """Sync enhanced user data to Redis for proxy consumption"""
    users_data = {}
    permissions_data = {}
    
    # Get all users with profiles
    query = db.auth_user.id > 0
    users = db(query).select(db.auth_user.ALL, db.user_profile.ALL,
                            left=db.user_profile.on(db.auth_user.id == db.user_profile.user_id))
    
    for row in users:
        user = row.auth_user
        profile = row.user_profile
        
        # Build user data for proxy
        user_data = {
            'username': user.username,
            'password_hash': user.password,
            'enabled': user.registration_key is None,  # py4web auth pattern
            'api_key': profile.api_key if profile else None,
            'require_tls': profile.require_tls if profile else False,
            'allowed_ips': profile.allowed_ips if profile else [],
            'rate_limit': profile.rate_limit if profile else 0,
            'expires_at': profile.expires_at.isoformat() if profile and profile.expires_at else None,
            'created_at': user.registration_created_at.isoformat() if user.registration_created_at else None,
            'updated_at': user.registration_updated_at.isoformat() if user.registration_updated_at else None
        }
        users_data[user.username] = user_data
    
    # Get all permissions
    perms = db(db.user_permission).select()
    for perm in perms:
        user = db.auth_user[perm.user_id]
        perm_data = {
            'user_id': user.username,
            'database': perm.database_name,
            'table': perm.table_name,
            'actions': perm.actions,
            'expires_at': perm.expires_at.isoformat() if perm.expires_at else None,
            'max_queries': perm.max_queries
        }
        permissions_data[f"{user.username}:{perm.database_name}"] = perm_data
    
    # Store in Redis
    redis_client.set('articdbm:manager:users', json.dumps(users_data))
    redis_client.set('articdbm:manager:permissions', json.dumps(permissions_data))
    
    return len(users_data), len(permissions_data)

@action('api/health', method=['GET'])
@cors
def health():
    return {'status': 'healthy', 'timestamp': datetime.utcnow().isoformat()}

@action('api/servers', method=['GET'])
@action.uses(auth, cors)
def get_servers():
    servers = db(db.database_server.active == True).select()
    result = []
    for server in servers:
        result.append({
            'id': server.id,
            'name': server.name,
            'type': server.type,
            'host': server.host,
            'port': server.port,
            'role': server.role,
            'weight': server.weight,
            'tls_enabled': server.tls_enabled,
            'active': server.active
        })
    return {'servers': result}

@action('api/servers', method=['POST'])
@action.uses(auth, cors, db)
def create_server():
    try:
        data = request.json
        server = DatabaseServerModel(**data)
        
        server_id = db.database_server.insert(
            name=server.name,
            type=server.type,
            host=server.host,
            port=server.port,
            username=server.username,
            password=server.password,
            database=server.database,
            role=server.role,
            weight=server.weight,
            tls_enabled=server.tls_enabled,
            active=server.active
        )
        
        sync_to_redis()
        
        return {'id': server_id, 'message': 'Server created successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/servers/<server_id:int>', method=['PUT'])
@action.uses(auth, cors, db)
def update_server(server_id):
    try:
        data = request.json
        server = db.database_server[server_id]
        if not server:
            abort(404, 'Server not found')
        
        server.update_record(**data)
        sync_to_redis()
        
        return {'message': 'Server updated successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/servers/<server_id:int>', method=['DELETE'])
@action.uses(auth, cors, db)
def delete_server(server_id):
    try:
        server = db.database_server[server_id]
        if not server:
            abort(404, 'Server not found')
        
        server.update_record(active=False)
        sync_to_redis()
        
        return {'message': 'Server deleted successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/permissions', method=['GET'])
@action.uses(auth, cors)
def get_permissions():
    permissions = db(db.user_permission).select()
    result = []
    for perm in permissions:
        user = db.auth_user[perm.user_id]
        result.append({
            'id': perm.id,
            'user_id': perm.user_id,
            'username': user.email if user else 'Unknown',
            'database_name': perm.database_name,
            'table_name': perm.table_name,
            'actions': perm.actions
        })
    return {'permissions': result}

@action('api/permissions', method=['POST'])
@action.uses(auth, cors, db)
def create_permission():
    try:
        data = request.json
        perm = PermissionModel(**data)
        
        perm_id = db.user_permission.insert(
            user_id=perm.user_id,
            database_name=perm.database_name,
            table_name=perm.table_name,
            actions=perm.actions
        )
        
        sync_to_redis()
        
        return {'id': perm_id, 'message': 'Permission created successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/permissions/<perm_id:int>', method=['PUT'])
@action.uses(auth, cors, db)
def update_permission(perm_id):
    try:
        data = request.json
        perm = db.user_permission[perm_id]
        if not perm:
            abort(404, 'Permission not found')
        
        perm.update_record(**data)
        sync_to_redis()
        
        return {'message': 'Permission updated successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/permissions/<perm_id:int>', method=['DELETE'])
@action.uses(auth, cors, db)
def delete_permission(perm_id):
    try:
        perm = db.user_permission[perm_id]
        if not perm:
            abort(404, 'Permission not found')
        
        db(db.user_permission.id == perm_id).delete()
        sync_to_redis()
        
        return {'message': 'Permission deleted successfully'}
    except Exception as e:
        abort(400, str(e))

# ==================== ENHANCED USER MANAGEMENT ENDPOINTS ====================

@action('api/users/enhanced', method=['GET'])
@action.uses(auth, cors, db)
def get_enhanced_users():
    """Get all users with their enhanced profiles and permissions"""
    try:
        query = db.auth_user.id > 0
        users = db(query).select(db.auth_user.ALL, db.user_profile.ALL,
                                left=db.user_profile.on(db.auth_user.id == db.user_profile.user_id))
        
        result = []
        for row in users:
            user = row.auth_user
            profile = row.user_profile
            
            # Get user permissions
            perms = db(db.user_permission.user_id == user.id).select()
            permissions = []
            for perm in perms:
                permissions.append({
                    'id': perm.id,
                    'database_name': perm.database_name,
                    'table_name': perm.table_name,
                    'actions': perm.actions,
                    'expires_at': perm.expires_at.isoformat() if perm.expires_at else None,
                    'max_queries': perm.max_queries,
                    'query_count': perm.query_count
                })
            
            user_data = {
                'id': user.id,
                'username': user.username,
                'email': user.email,
                'first_name': user.first_name,
                'last_name': user.last_name,
                'enabled': user.registration_key is None,
                'created_at': user.created_at.isoformat() if user.created_at else None,
                'profile': {
                    'api_key': profile.api_key if profile else None,
                    'require_tls': profile.require_tls if profile else False,
                    'allowed_ips': profile.allowed_ips if profile else [],
                    'rate_limit': profile.rate_limit if profile else 0,
                    'expires_at': profile.expires_at.isoformat() if profile and profile.expires_at else None,
                    'is_temporary': profile.is_temporary if profile else False,
                    'last_used': profile.last_used.isoformat() if profile and profile.last_used else None,
                    'usage_count': profile.usage_count if profile else 0
                },
                'permissions': permissions
            }
            result.append(user_data)
        
        return {'users': result, 'count': len(result)}
    except Exception as e:
        abort(500, str(e))

@action('api/users/enhanced', method=['POST'])
@action.uses(auth, cors, db)
def create_enhanced_user():
    """Create a new user with enhanced profile settings"""
    try:
        data = request.json
        
        # Validate required fields
        if not data.get('username') or not data.get('email') or not data.get('password'):
            abort(400, 'Username, email, and password are required')
        
        # Check if user exists
        if db(db.auth_user.username == data['username']).count() > 0:
            abort(400, 'Username already exists')
        
        if db(db.auth_user.email == data['email']).count() > 0:
            abort(400, 'Email already exists')
        
        # Parse expiration dates
        expires_at = None
        if data.get('expires_at'):
            expires_at = datetime.fromisoformat(data['expires_at'].replace('Z', '+00:00'))
        
        # Create user with profile
        user_id = create_user_with_profile(
            username=data['username'],
            email=data['email'],
            password=data['password'],
            first_name=data.get('first_name', ''),
            last_name=data.get('last_name', ''),
            require_tls=data.get('require_tls', False),
            allowed_ips=data.get('allowed_ips', []),
            rate_limit=data.get('rate_limit', 0),
            expires_at=expires_at,
            is_temporary=data.get('is_temporary', False),
            created_by=auth.current_user.id if auth.current_user else None
        )
        
        # Create permissions if provided
        if data.get('permissions'):
            for perm_data in data['permissions']:
                perm_expires_at = None
                if perm_data.get('expires_at'):
                    perm_expires_at = datetime.fromisoformat(perm_data['expires_at'].replace('Z', '+00:00'))
                
                db.user_permission.insert(
                    user_id=user_id,
                    database_name=perm_data['database_name'],
                    table_name=perm_data.get('table_name', '*'),
                    actions=perm_data.get('actions', ['read']),
                    expires_at=perm_expires_at,
                    max_queries=perm_data.get('max_queries', 0)
                )
        
        db.commit()
        sync_users_to_redis()
        
        return {'id': user_id, 'message': 'Enhanced user created successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/users/<user_id:int>/regenerate-api-key', method=['POST'])
@action.uses(auth, cors, db)
def regenerate_api_key(user_id):
    """Regenerate API key for a user"""
    try:
        user = db.auth_user[user_id]
        if not user:
            abort(404, 'User not found')
        
        profile = db(db.user_profile.user_id == user_id).select().first()
        
        new_api_key = generate_api_key()
        
        if profile:
            db(db.user_profile.user_id == user_id).update(api_key=new_api_key)
        else:
            db.user_profile.insert(user_id=user_id, api_key=new_api_key)
        
        db.commit()
        sync_users_to_redis()
        
        return {'api_key': new_api_key, 'message': 'API key regenerated successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/users/<user_id:int>/rate-limit', method=['PUT'])
@action.uses(auth, cors, db)
def update_user_rate_limit(user_id):
    """Update rate limit for a user"""
    try:
        user = db.auth_user[user_id]
        if not user:
            abort(404, 'User not found')
        
        data = request.json
        rate_limit = data.get('rate_limit', 0)
        
        if rate_limit < 0:
            abort(400, 'Rate limit cannot be negative')
        
        profile = db(db.user_profile.user_id == user_id).select().first()
        
        if profile:
            db(db.user_profile.user_id == user_id).update(rate_limit=rate_limit)
        else:
            db.user_profile.insert(user_id=user_id, rate_limit=rate_limit)
        
        db.commit()
        sync_users_to_redis()
        
        return {'rate_limit': rate_limit, 'message': 'Rate limit updated successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/temporary-access', method=['POST'])
@action.uses(auth, cors, db)
def create_temporary_access():
    """Create a one-time access token for temporary database access"""
    try:
        data = request.json
        
        # Validate required fields
        if not data.get('database_name') or not data.get('actions'):
            abort(400, 'Database name and actions are required')
        
        if not data.get('expires_at'):
            abort(400, 'Expiration time is required')
        
        # Parse expiration time
        expires_at = datetime.fromisoformat(data['expires_at'].replace('Z', '+00:00'))
        
        if expires_at <= datetime.utcnow():
            abort(400, 'Expiration time must be in the future')
        
        # Generate token
        token = generate_temp_token()
        
        # Create temporary access
        temp_id = db.temporary_access.insert(
            token=token,
            database_name=data['database_name'],
            table_name=data.get('table_name', '*'),
            actions=data['actions'],
            created_by=auth.current_user.id if auth.current_user else None,
            expires_at=expires_at,
            max_uses=data.get('max_uses', 1),
            client_ip=data.get('client_ip')
        )
        
        db.commit()
        
        return {
            'token': token,
            'id': temp_id,
            'expires_at': expires_at.isoformat(),
            'message': 'Temporary access token created successfully'
        }
    except Exception as e:
        abort(400, str(e))

@action('api/temporary-access', method=['GET'])
@action.uses(auth, cors, db)
def get_temporary_access():
    """Get all temporary access tokens"""
    try:
        tokens = db(db.temporary_access.is_active == True).select()
        result = []
        
        for token in tokens:
            created_by_user = db.auth_user[token.created_by] if token.created_by else None
            
            token_data = {
                'id': token.id,
                'token': token.token,
                'database_name': token.database_name,
                'table_name': token.table_name,
                'actions': token.actions,
                'created_by': created_by_user.username if created_by_user else None,
                'created_at': token.created_at.isoformat(),
                'expires_at': token.expires_at.isoformat(),
                'max_uses': token.max_uses,
                'use_count': token.use_count,
                'last_used': token.last_used.isoformat() if token.last_used else None,
                'client_ip': token.client_ip,
                'is_active': token.is_active,
                'expired': token.expires_at < datetime.utcnow()
            }
            result.append(token_data)
        
        return {'tokens': result, 'count': len(result)}
    except Exception as e:
        abort(500, str(e))

@action('api/temporary-access/<token_id:int>/revoke', method=['POST'])
@action.uses(auth, cors, db)
def revoke_temporary_access(token_id):
    """Revoke a temporary access token"""
    try:
        token = db.temporary_access[token_id]
        if not token:
            abort(404, 'Token not found')
        
        db(db.temporary_access.id == token_id).update(is_active=False)
        db.commit()
        
        return {'message': 'Temporary access token revoked successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/security-rules', method=['GET'])
@action.uses(auth, cors)
def get_security_rules():
    rules = db(db.security_rule.active == True).select()
    result = []
    for rule in rules:
        result.append({
            'id': rule.id,
            'name': rule.name,
            'pattern': rule.pattern,
            'action': rule.action,
            'severity': rule.severity,
            'active': rule.active
        })
    return {'rules': result}

@action('api/security-rules', method=['POST'])
@action.uses(auth, cors, db)
def create_security_rule():
    try:
        data = request.json
        rule = SecurityRuleModel(**data)
        
        rule_id = db.security_rule.insert(
            name=rule.name,
            pattern=rule.pattern,
            action=rule.action,
            severity=rule.severity,
            active=rule.active
        )
        
        sync_to_redis()
        
        return {'id': rule_id, 'message': 'Security rule created successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/audit-log', method=['GET'])
@action.uses(auth, cors)
def get_audit_log():
    limit = request.params.get('limit', 100)
    offset = request.params.get('offset', 0)
    
    logs = db(db.audit_log).select(
        limitby=(int(offset), int(offset) + int(limit)),
        orderby=~db.audit_log.timestamp
    )
    
    result = []
    for log in logs:
        user = db.auth_user[log.user_id] if log.user_id else None
        result.append({
            'id': log.id,
            'username': user.email if user else 'System',
            'action': log.action,
            'database_name': log.database_name,
            'table_name': log.table_name,
            'query': log.query[:100] if log.query else None,
            'result': log.result,
            'ip_address': log.ip_address,
            'timestamp': log.timestamp.isoformat()
        })
    
    return {'logs': result, 'total': db(db.audit_log).count()}

@action('api/stats', method=['GET'])
@action.uses(auth, cors)
def get_stats():
    stats = {
        'total_servers': db(db.database_server.active == True).count(),
        'total_users': db(db.auth_user).count(),
        'total_permissions': db(db.user_permission).count(),
        'total_security_rules': db(db.security_rule.active == True).count(),
        'recent_queries': db(db.audit_log).count(),
        'servers_by_type': {}
    }
    
    for server_type in ['mysql', 'postgresql', 'mssql', 'mongodb', 'redis']:
        count = db((db.database_server.type == server_type) & 
                  (db.database_server.active == True)).count()
        stats['servers_by_type'][server_type] = count
    
    return stats

# Database Management API Endpoints

@action('api/databases', method=['GET'])
@action.uses(auth, cors)
def get_managed_databases():
    databases = db(db.managed_database.active == True).select()
    result = []
    for database in databases:
        server = db.database_server[database.server_id]
        result.append({
            'id': database.id,
            'name': database.name,
            'database_name': database.database_name,
            'server_name': server.name if server else 'Unknown',
            'server_type': server.type if server else 'Unknown',
            'description': database.description,
            'schema_version': database.schema_version,
            'auto_backup': database.auto_backup,
            'backup_schedule': database.backup_schedule,
            'active': database.active,
            'created_at': database.created_at.isoformat()
        })
    return {'databases': result}

@action('api/databases', method=['POST'])
@action.uses(auth, cors, db)
def create_managed_database():
    try:
        data = request.json
        database = ManagedDatabaseModel(**data)
        
        # Verify server exists
        server = db.database_server[database.server_id]
        if not server:
            abort(400, 'Server not found')
        
        database_id = db.managed_database.insert(
            name=database.name,
            server_id=database.server_id,
            database_name=database.database_name,
            description=database.description,
            schema_version=database.schema_version,
            auto_backup=database.auto_backup,
            backup_schedule=database.backup_schedule,
            active=database.active
        )
        
        # Log the action
        db.audit_log.insert(
            user_id=auth.user_id,
            action='create_managed_database',
            database_name=database.name,
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_to_redis()
        
        return {'id': database_id, 'message': 'Managed database created successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/databases/<database_id:int>', method=['PUT'])
@action.uses(auth, cors, db)
def update_managed_database(database_id):
    try:
        data = request.json
        database = db.managed_database[database_id]
        if not database:
            abort(404, 'Database not found')
        
        database.update_record(**data)
        
        # Log the action
        db.audit_log.insert(
            user_id=auth.user_id,
            action='update_managed_database',
            database_name=database.name,
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_to_redis()
        
        return {'message': 'Managed database updated successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/databases/<database_id:int>', method=['DELETE'])
@action.uses(auth, cors, db)
def delete_managed_database(database_id):
    try:
        database = db.managed_database[database_id]
        if not database:
            abort(404, 'Database not found')
        
        database.update_record(active=False)
        
        # Log the action
        db.audit_log.insert(
            user_id=auth.user_id,
            action='delete_managed_database',
            database_name=database.name,
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_to_redis()
        
        return {'message': 'Managed database deleted successfully'}
    except Exception as e:
        abort(400, str(e))

# SQL File Management API Endpoints

@action('api/sql-files', method=['GET'])
@action.uses(auth, cors)
def get_sql_files():
    database_id = request.params.get('database_id')
    query = db.sql_file
    if database_id:
        query = db(db.sql_file.database_id == database_id)
    else:
        query = db(db.sql_file)
    
    files = query.select()
    result = []
    for file in files:
        database = db.managed_database[file.database_id]
        executed_by_user = db.auth_user[file.executed_by] if file.executed_by else None
        result.append({
            'id': file.id,
            'name': file.name,
            'database_name': database.name if database else 'Unknown',
            'file_type': file.file_type,
            'file_size': file.file_size,
            'syntax_validated': file.syntax_validated,
            'security_validated': file.security_validated,
            'validation_errors': file.validation_errors,
            'executed': file.executed,
            'executed_at': file.executed_at.isoformat() if file.executed_at else None,
            'executed_by': executed_by_user.email if executed_by_user else None,
            'created_at': file.created_at.isoformat()
        })
    return {'sql_files': result}

@action('api/sql-files', method=['POST'])
@action.uses(auth, cors, db)
def upload_sql_file():
    try:
        data = request.json
        sql_file = SQLFileModel(**data)
        
        # Verify database exists
        database = db.managed_database[sql_file.database_id]
        if not database:
            abort(400, 'Database not found')
        
        # Validate SQL content
        validation_result = validate_sql_security(sql_file.file_content)
        
        # Create file path and save content
        upload_dir = Path(tempfile.gettempdir()) / "articdbm" / "sql_files"
        upload_dir.mkdir(parents=True, exist_ok=True)
        
        file_uuid = str(uuid.uuid4())
        file_path = upload_dir / f"{file_uuid}.sql"
        
        with open(file_path, 'w', encoding='utf-8') as f:
            f.write(sql_file.file_content)
        
        # Calculate checksum
        checksum = hashlib.sha256(sql_file.file_content.encode('utf-8')).hexdigest()
        
        file_id = db.sql_file.insert(
            name=sql_file.name,
            database_id=sql_file.database_id,
            file_type=sql_file.file_type,
            file_path=str(file_path),
            file_size=len(sql_file.file_content),
            checksum=checksum,
            syntax_validated=validation_result['valid'],
            security_validated=validation_result['severity'] in ['low', 'medium'],
            validation_errors=json.dumps({
                'errors': validation_result['errors'],
                'warnings': validation_result['warnings'],
                'severity': validation_result['severity']
            })
        )
        
        # Log the action
        db.audit_log.insert(
            user_id=auth.user_id,
            action='upload_sql_file',
            database_name=database.name,
            query=f"Uploaded file: {sql_file.name}",
            result=f"Validation: {'passed' if validation_result['valid'] else 'failed'}",
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        return {
            'id': file_id, 
            'message': 'SQL file uploaded successfully',
            'validation': validation_result
        }
    except Exception as e:
        abort(400, str(e))

@action('api/sql-files/<file_id:int>/execute', method=['POST'])
@action.uses(auth, cors, db)
def execute_sql_file(file_id):
    try:
        sql_file = db.sql_file[file_id]
        if not sql_file:
            abort(404, 'SQL file not found')
        
        if sql_file.executed:
            abort(400, 'SQL file has already been executed')
        
        if not sql_file.security_validated:
            abort(400, 'SQL file failed security validation')
        
        database = db.managed_database[sql_file.database_id]
        if not database:
            abort(400, 'Associated database not found')
        
        server = db.database_server[database.server_id]
        if not server:
            abort(400, 'Database server not found')
        
        # Read file content
        with open(sql_file.file_path, 'r', encoding='utf-8') as f:
            content = f.read()
        
        # Mark as executed (this would normally execute against the actual database)
        sql_file.update_record(
            executed=True,
            executed_at=datetime.utcnow(),
            executed_by=auth.user_id
        )
        
        # Log the action
        db.audit_log.insert(
            user_id=auth.user_id,
            action='execute_sql_file',
            database_name=database.name,
            query=f"Executed file: {sql_file.name}",
            result="success",
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        return {'message': f'SQL file {sql_file.name} executed successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/sql-files/<file_id:int>/validate', method=['POST'])
@action.uses(auth, cors, db)
def validate_sql_file(file_id):
    try:
        sql_file = db.sql_file[file_id]
        if not sql_file:
            abort(404, 'SQL file not found')
        
        # Read file content
        with open(sql_file.file_path, 'r', encoding='utf-8') as f:
            content = f.read()
        
        # Re-validate
        validation_result = validate_sql_security(content)
        
        # Update validation status
        sql_file.update_record(
            syntax_validated=validation_result['valid'],
            security_validated=validation_result['severity'] in ['low', 'medium'],
            validation_errors=json.dumps({
                'errors': validation_result['errors'],
                'warnings': validation_result['warnings'],
                'severity': validation_result['severity']
            })
        )
        
        return {
            'message': 'SQL file validated successfully',
            'validation': validation_result
        }
    except Exception as e:
        abort(400, str(e))

# Blocked Database Management API Endpoints

@action('api/blocked-databases', method=['GET'])
@action.uses(auth, cors)
def get_blocked_databases():
    blocked = db(db.blocked_database.active == True).select()
    result = []
    for item in blocked:
        result.append({
            'id': item.id,
            'name': item.name,
            'type': item.type,
            'pattern': item.pattern,
            'reason': item.reason,
            'active': item.active,
            'created_at': item.created_at.isoformat()
        })
    return {'blocked_databases': result}

@action('api/blocked-databases', method=['POST'])
@action.uses(auth, cors, db)
def create_blocked_database():
    try:
        data = request.json
        blocked = BlockedDatabaseModel(**data)
        
        blocked_id = db.blocked_database.insert(
            name=blocked.name,
            type=blocked.type,
            pattern=blocked.pattern,
            reason=blocked.reason,
            active=blocked.active
        )
        
        # Log the action
        db.audit_log.insert(
            user_id=auth.user_id,
            action='create_blocked_database',
            database_name=blocked.name,
            query=f"Blocked {blocked.type}: {blocked.pattern}",
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_to_redis()
        
        return {'id': blocked_id, 'message': 'Blocked database rule created successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/blocked-databases/<blocked_id:int>', method=['DELETE'])
@action.uses(auth, cors, db)
def delete_blocked_database(blocked_id):
    try:
        blocked = db.blocked_database[blocked_id]
        if not blocked:
            abort(404, 'Blocked database rule not found')
        
        blocked.update_record(active=False)
        
        # Log the action
        db.audit_log.insert(
            user_id=auth.user_id,
            action='delete_blocked_database',
            database_name=blocked.name,
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_to_redis()
        
        return {'message': 'Blocked database rule deleted successfully'}
    except Exception as e:
        abort(400, str(e))

# Database Schema Management API Endpoints

@action('api/databases/<database_id:int>/schema', method=['GET'])
@action.uses(auth, cors)
def get_database_schema(database_id):
    database = db.managed_database[database_id]
    if not database:
        abort(404, 'Database not found')
    
    schema_entries = db(db.database_schema.database_id == database_id).select()
    
    # Group by table
    tables = {}
    for entry in schema_entries:
        table_name = entry.table_name
        if table_name not in tables:
            tables[table_name] = {
                'name': table_name,
                'columns': []
            }
        
        tables[table_name]['columns'].append({
            'name': entry.column_name,
            'data_type': entry.data_type,
            'is_nullable': entry.is_nullable,
            'default_value': entry.default_value,
            'is_primary_key': entry.is_primary_key,
            'is_foreign_key': entry.is_foreign_key,
            'foreign_table': entry.foreign_table,
            'foreign_column': entry.foreign_column
        })
    
    return {
        'database_name': database.name,
        'tables': list(tables.values())
    }

@action('api/databases/<database_id:int>/schema', method=['POST'])
@action.uses(auth, cors, db)
def update_database_schema(database_id):
    try:
        database = db.managed_database[database_id]
        if not database:
            abort(404, 'Database not found')
        
        data = request.json
        schema_data = data.get('schema', [])
        
        # Clear existing schema
        db(db.database_schema.database_id == database_id).delete()
        
        # Insert new schema
        for table in schema_data:
            table_name = table['name']
            for column in table.get('columns', []):
                db.database_schema.insert(
                    database_id=database_id,
                    table_name=table_name,
                    column_name=column['name'],
                    data_type=column['data_type'],
                    is_nullable=column.get('is_nullable', True),
                    default_value=column.get('default_value'),
                    is_primary_key=column.get('is_primary_key', False),
                    is_foreign_key=column.get('is_foreign_key', False),
                    foreign_table=column.get('foreign_table'),
                    foreign_column=column.get('foreign_column')
                )
        
        # Update database schema version
        schema_version = f"v{datetime.utcnow().strftime('%Y%m%d_%H%M%S')}"
        database.update_record(schema_version=schema_version)
        
        # Log the action
        db.audit_log.insert(
            user_id=auth.user_id,
            action='update_database_schema',
            database_name=database.name,
            query=f"Updated schema to {schema_version}",
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        return {'message': 'Database schema updated successfully', 'version': schema_version}
    except Exception as e:
        abort(400, str(e))

def sync_to_redis():
    try:
        users = {}
        for user in db(db.auth_user).select():
            users[user.email] = {
                'username': user.email,
                'password_hash': user.password,
                'enabled': True,
                'created_at': user.created_on.isoformat() if hasattr(user, 'created_on') else None,
                'updated_at': user.modified_on.isoformat() if hasattr(user, 'modified_on') else None
            }
        
        permissions = {}
        for perm in db(db.user_permission).select():
            user = db.auth_user[perm.user_id]
            if user:
                permissions[user.email] = {
                    'user_id': user.email,
                    'database': perm.database_name,
                    'table': perm.table_name,
                    'actions': perm.actions
                }
        
        backends = {
            'mysql': [],
            'postgresql': [],
            'mssql': [],
            'mongodb': [],
            'redis': []
        }
        
        for server in db(db.database_server.active == True).select():
            backend_data = {
                'host': server.host,
                'port': server.port,
                'type': server.role,
                'weight': server.weight,
                'tls': server.tls_enabled,
                'user': server.username,
                'password': server.password,
                'database': server.database
            }
            if server.type in backends:
                backends[server.type].append(backend_data)
        
        # Get blocked databases for security rules
        blocked_databases = {}
        for blocked in db(db.blocked_database.active == True).select():
            blocked_databases[blocked.id] = {
                'name': blocked.name,
                'type': blocked.type,
                'pattern': blocked.pattern,
                'reason': blocked.reason,
                'active': blocked.active
            }
        
        # Get managed databases
        managed_databases = {}
        for managed_db in db(db.managed_database.active == True).select():
            server = db.database_server[managed_db.server_id]
            managed_databases[managed_db.id] = {
                'name': managed_db.name,
                'database_name': managed_db.database_name,
                'server_name': server.name if server else None,
                'server_type': server.type if server else None,
                'description': managed_db.description,
                'schema_version': managed_db.schema_version,
                'auto_backup': managed_db.auto_backup,
                'backup_schedule': managed_db.backup_schedule,
                'active': managed_db.active
            }
        
        redis_client.set('articdbm:users', json.dumps(users))
        redis_client.set('articdbm:permissions', json.dumps(permissions))
        redis_client.set('articdbm:backends', json.dumps(backends))
        redis_client.set('articdbm:blocked_databases', json.dumps(blocked_databases))
        redis_client.set('articdbm:managed_databases', json.dumps(managed_databases))
        
        redis_client.set('articdbm:manager:users', json.dumps(users))
        redis_client.set('articdbm:manager:permissions', json.dumps(permissions))
        redis_client.set('articdbm:manager:blocked_databases', json.dumps(blocked_databases))
        
        redis_client.expire('articdbm:users', 300)
        redis_client.expire('articdbm:permissions', 300)
        redis_client.expire('articdbm:backends', 300)
        redis_client.expire('articdbm:blocked_databases', 300)
        redis_client.expire('articdbm:managed_databases', 300)
        
        # Also sync threat intelligence data
        sync_threat_intel_to_redis()
        
        return True
    except Exception as e:
        print(f"Error syncing to Redis: {e}")
        return False

# License Management API Endpoints

@action('api/license', method=['GET'])
@action.uses(auth, cors)
def get_license_info():
    license_info = get_current_license_info()
    database_count = db(db.managed_database.active == True).count()
    threat_feed_count = db(db.threat_intel_feed.active == True).count()
    threat_feed_limit = get_threat_intel_limit()
    
    return {
        'tier': license_info['tier'],
        'is_active': license_info['is_active'],
        'features': license_info['features'],
        'database_count': database_count,
        'threat_feed_count': threat_feed_count,
        'threat_feed_limit': threat_feed_limit,
        'license_key_present': bool(license_info.get('license_key')),
        'pricing': {
            'enterprise_per_database': 5.00,
            'currency': 'USD',
            'billing_period': 'monthly'
        }
    }

@action('api/license', method=['POST'])
@action.uses(auth, cors, db)
def activate_license():
    try:
        data = request.json
        license_key = data.get('license_key', '').strip()
        
        if not license_key:
            abort(400, 'License key is required')
        
        # Validate with license server
        validation_result = validate_license_with_server(license_key)
        
        if not validation_result['valid']:
            abort(400, f"Invalid license: {validation_result.get('error', 'License validation failed')}")
        
        # Check if license is for ArticDBM enterprise
        features = validation_result.get('features', [])
        if 'unlimited_threat_intel_feeds' not in features:
            abort(400, 'This license does not include ArticDBM Enterprise features')
        
        # Update or create license record
        existing_license = db(db.license_info).select().first()
        database_count = db(db.managed_database.active == True).count()
        
        next_validation = datetime.utcnow() + timedelta(seconds=random.randint(45, 180))
        
        if existing_license:
            existing_license.update_record(
                license_key=license_key,
                tier='enterprise',
                features=features,
                database_count=database_count,
                is_active=True,
                last_validated=datetime.utcnow(),
                next_validation=next_validation,
                validation_failures=0
            )
        else:
            db.license_info.insert(
                license_key=license_key,
                tier='enterprise',
                features=features,
                database_count=database_count,
                is_active=True,
                last_validated=datetime.utcnow(),
                next_validation=next_validation,
                validation_failures=0
            )
        
        # Log the activation
        db.audit_log.insert(
            user_id=auth.user_id,
            action='activate_license',
            query=f"Activated enterprise license",
            result='success',
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        return {
            'message': 'Enterprise license activated successfully',
            'tier': 'enterprise',
            'features': features
        }
        
    except Exception as e:
        abort(400, str(e))

@action('api/license', method=['DELETE'])
@action.uses(auth, cors, db)
def deactivate_license():
    try:
        license_record = db(db.license_info).select().first()
        if license_record:
            license_record.update_record(
                license_key=None,
                tier='community',
                features=['single_threat_intel_feed'],
                is_active=False,
                last_validated=None,
                next_validation=None,
                validation_failures=0
            )
        
        # Log the deactivation
        db.audit_log.insert(
            user_id=auth.user_id,
            action='deactivate_license',
            query=f"Deactivated license - reverted to community",
            result='success',
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        return {'message': 'License deactivated - reverted to Community edition'}
        
    except Exception as e:
        abort(400, str(e))

async def periodic_license_validation():
    """Periodic license validation task"""
    while True:
        try:
            license_record = db(db.license_info).select().first()
            if license_record and license_record.license_key and license_record.next_validation:
                if datetime.utcnow() >= license_record.next_validation:
                    update_license_validation()
            
            # Check every 30 seconds
            await asyncio.sleep(30)
        except Exception as e:
            print(f"Error in periodic license validation: {e}")
            await asyncio.sleep(60)

@action('api/sync', method=['POST'])
@action.uses(auth, cors)
def manual_sync():
    if sync_to_redis():
        return {'message': 'Configuration synced to Redis successfully'}
    else:
        abort(500, 'Failed to sync configuration')

async def periodic_sync():
    while True:
        await asyncio.sleep(60)
        sync_to_redis()

def seed_default_blocked_resources():
    """Seed default blocked databases and accounts if they don't exist"""
    try:
        # Check if any blocked resources already exist
        existing_count = db(db.blocked_database).count()
        if existing_count > 0:
            print(f"Blocked resources already exist ({existing_count} entries). Skipping seed.")
            return
        
        # Default blocked databases
        default_databases = [
            # Common test/demo databases
            {"name": "test", "type": "database", "pattern": "^test$", "reason": "Default test database"},
            {"name": "sample", "type": "database", "pattern": "^sample$", "reason": "Default sample database"},
            {"name": "demo", "type": "database", "pattern": "^demo$", "reason": "Default demo database"},
            {"name": "example", "type": "database", "pattern": "^example$", "reason": "Default example database"},
            {"name": "temp", "type": "database", "pattern": "^temp$", "reason": "Temporary database"},
            {"name": "tmp", "type": "database", "pattern": "^tmp$", "reason": "Temporary database"},
            
            # SQL Server system databases
            {"name": "master", "type": "database", "pattern": "^master$", "reason": "SQL Server system database"},
            {"name": "msdb", "type": "database", "pattern": "^msdb$", "reason": "SQL Server system database"},
            {"name": "tempdb", "type": "database", "pattern": "^tempdb$", "reason": "SQL Server system database"},
            {"name": "model", "type": "database", "pattern": "^model$", "reason": "SQL Server system database"},
            
            # MySQL system databases
            {"name": "mysql", "type": "database", "pattern": "^mysql$", "reason": "MySQL system database"},
            {"name": "sys", "type": "database", "pattern": "^sys$", "reason": "MySQL system database"},
            {"name": "information_schema", "type": "database", "pattern": "^information_schema$", "reason": "MySQL information schema"},
            {"name": "performance_schema", "type": "database", "pattern": "^performance_schema$", "reason": "MySQL performance schema"},
            
            # PostgreSQL system databases
            {"name": "postgres", "type": "database", "pattern": "^postgres$", "reason": "PostgreSQL default database"},
            {"name": "template0", "type": "database", "pattern": "^template0$", "reason": "PostgreSQL template database"},
            {"name": "template1", "type": "database", "pattern": "^template1$", "reason": "PostgreSQL template database"},
            
            # MongoDB system databases
            {"name": "admin", "type": "database", "pattern": "^admin$", "reason": "MongoDB admin database"},
            {"name": "local", "type": "database", "pattern": "^local$", "reason": "MongoDB local database"},
            {"name": "config", "type": "database", "pattern": "^config$", "reason": "MongoDB config database"},
        ]
        
        # Default blocked users
        default_users = [
            # Common default admin accounts
            {"name": "sa", "type": "username", "pattern": "^sa$", "reason": "SQL Server default admin account"},
            {"name": "root", "type": "username", "pattern": "^root$", "reason": "Default root account"},
            {"name": "admin", "type": "username", "pattern": "^admin$", "reason": "Default admin account"},
            {"name": "administrator", "type": "username", "pattern": "^administrator$", "reason": "Default administrator account"},
            {"name": "guest", "type": "username", "pattern": "^guest$", "reason": "Default guest account"},
            
            # Test/demo accounts
            {"name": "test", "type": "username", "pattern": "^test$", "reason": "Test user account"},
            {"name": "demo", "type": "username", "pattern": "^demo$", "reason": "Demo user account"},
            {"name": "sample", "type": "username", "pattern": "^sample$", "reason": "Sample user account"},
            {"name": "user", "type": "username", "pattern": "^user$", "reason": "Generic user account"},
            
            # Database-specific default accounts
            {"name": "mysql", "type": "username", "pattern": "^mysql$", "reason": "MySQL service account"},
            {"name": "postgres", "type": "username", "pattern": "^postgres$", "reason": "PostgreSQL default account"},
            {"name": "oracle", "type": "username", "pattern": "^oracle$", "reason": "Oracle default account"},
            {"name": "sqlserver", "type": "username", "pattern": "^sqlserver$", "reason": "SQL Server service account"},
            
            # Empty/blank username
            {"name": "empty", "type": "username", "pattern": "^$", "reason": "Empty/anonymous username"},
            {"name": "anonymous", "type": "username", "pattern": "^anonymous$", "reason": "Anonymous user account"},
        ]
        
        # Default blocked tables
        default_tables = [
            {"name": "user", "type": "table", "pattern": "^user$", "reason": "System user table"},
            {"name": "users", "type": "table", "pattern": "^users$", "reason": "System users table"},
            {"name": "mysql.user", "type": "table", "pattern": "^mysql\\.user$", "reason": "MySQL user table"},
            {"name": "pg_user", "type": "table", "pattern": "^pg_user$", "reason": "PostgreSQL user table"},
            {"name": "sysusers", "type": "table", "pattern": "^sysusers$", "reason": "SQL Server system users"},
            {"name": "sysobjects", "type": "table", "pattern": "^sysobjects$", "reason": "SQL Server system objects"},
        ]
        
        # Insert all default blocked resources
        all_defaults = default_databases + default_users + default_tables
        
        for resource in all_defaults:
            db.blocked_database.insert(
                name=resource["name"],
                type=resource["type"],
                pattern=resource["pattern"],
                reason=resource["reason"],
                active=True
            )
        
        db.commit()
        
        print(f"Successfully seeded {len(all_defaults)} default blocked resources")
        
        # Sync to Redis
        sync_to_redis()
        
        return True
    except Exception as e:
        print(f"Error seeding default blocked resources: {e}")
        return False

# API endpoint to trigger seeding
@action('api/seed-blocked-resources', method=['POST'])
@action.uses(auth, cors, db)
def seed_blocked_resources():
    try:
        if seed_default_blocked_resources():
            return {'message': 'Default blocked resources seeded successfully'}
        else:
            abort(500, 'Failed to seed blocked resources')
    except Exception as e:
        abort(400, str(e))

# API endpoint to get blocking configuration status
@action('api/blocking-config', method=['GET'])
@action.uses(auth, cors)
def get_blocking_config():
    config = {
        'blocking_enabled': True,  # This would normally come from config
        'default_blocking': True,
        'custom_blocking': True,
        'total_blocked_resources': db(db.blocked_database).count(),
        'active_blocked_resources': db(db.blocked_database.active == True).count(),
        'blocked_by_type': {
            'databases': db((db.blocked_database.type == 'database') & (db.blocked_database.active == True)).count(),
            'users': db((db.blocked_database.type == 'username') & (db.blocked_database.active == True)).count(),
            'tables': db((db.blocked_database.type == 'table') & (db.blocked_database.active == True)).count(),
        }
    }
    return config

@action('api/blocking-config', method=['PUT'])
@action.uses(auth, cors, db)
def update_blocking_config():
    try:
        data = request.json
        
        # In a real implementation, this would update configuration settings
        # For now, we'll just log the configuration change
        db.audit_log.insert(
            user_id=auth.user_id,
            action='update_blocking_config',
            query=json.dumps(data),
            result='success',
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        return {'message': 'Blocking configuration updated successfully'}
    except Exception as e:
        abort(400, str(e))

# Threat Intelligence API Endpoints

@action('api/threat-intel/feeds', method=['GET'])
@action.uses(auth, cors)
def get_threat_intel_feeds():
    feeds = db(db.threat_intel_feed.active == True).select()
    result = []
    for feed in feeds:
        indicator_count = db(db.threat_intel_indicator.feed_id == feed.id).count()
        result.append({
            'id': feed.id,
            'name': feed.name,
            'type': feed.type,
            'url': feed.url,
            'polling_interval': feed.polling_interval,
            'last_polled': feed.last_polled.isoformat() if feed.last_polled else None,
            'active': feed.active,
            'indicator_count': indicator_count,
            'created_at': feed.created_at.isoformat()
        })
    return {'feeds': result}

@action('api/threat-intel/feeds', method=['POST'])
@action.uses(auth, cors, db)
def create_threat_intel_feed():
    try:
        data = request.json
        feed = ThreatIntelFeedModel(**data)
        
        # Check license limits
        current_feed_count = db(db.threat_intel_feed.active == True).count()
        threat_intel_limit = get_threat_intel_limit()
        
        if threat_intel_limit != -1 and current_feed_count >= threat_intel_limit:
            license_info = get_current_license_info()
            if license_info['tier'] == 'community':
                abort(403, f'Community edition is limited to {threat_intel_limit} threat intelligence feed. Upgrade to Enterprise for unlimited feeds.')
            else:
                abort(403, f'License limit reached: {threat_intel_limit} feeds maximum.')
        
        feed_id = db.threat_intel_feed.insert(
            name=feed.name,
            type=feed.type,
            url=feed.url,
            api_key=feed.api_key,
            username=feed.username,
            password=feed.password,
            polling_interval=feed.polling_interval,
            active=feed.active
        )
        
        db.audit_log.insert(
            user_id=auth.user_id,
            action='create_threat_intel_feed',
            database_name=feed.name,
            query=f"Created {feed.type} feed: {feed.name}",
            result='success',
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        return {'id': feed_id, 'message': 'Threat intelligence feed created successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/threat-intel/feeds/<feed_id:int>/poll', method=['POST'])
@action.uses(auth, cors, db)
def poll_threat_intel_feed(feed_id):
    try:
        feed = db.threat_intel_feed[feed_id]
        if not feed:
            abort(404, 'Feed not found')
        
        indicators = []
        
        if feed.type == 'taxii':
            indicators = parse_taxii_feed(feed.url, feed.username, feed.password, feed.api_key)
        elif feed.type == 'stix':
            if feed.url:
                response = requests.get(feed.url, timeout=30)
                indicators = parse_stix_indicators(response.text)
        elif feed.type == 'openioc':
            if feed.url:
                response = requests.get(feed.url, timeout=30)
                indicators = parse_openioc_indicators(response.text)
        elif feed.type == 'misp':
            if feed.url:
                headers = {}
                if feed.api_key:
                    headers['Authorization'] = feed.api_key
                response = requests.get(feed.url, headers=headers, timeout=30)
                indicators = parse_misp_event(response.text)
        
        added_count = 0
        updated_count = 0
        
        for indicator in indicators:
            existing = db(
                (db.threat_intel_indicator.value == indicator['value']) &
                (db.threat_intel_indicator.indicator_type == indicator['indicator_type'])
            ).select().first()
            
            if existing:
                existing.update_record(
                    last_seen=datetime.utcnow(),
                    threat_level=indicator.get('threat_level', existing.threat_level),
                    confidence=max(existing.confidence, indicator.get('confidence', 50)),
                    active=True
                )
                updated_count += 1
            else:
                db.threat_intel_indicator.insert(
                    feed_id=feed_id,
                    indicator_type=indicator['indicator_type'],
                    value=indicator['value'],
                    threat_level=indicator.get('threat_level', 'medium'),
                    confidence=indicator.get('confidence', 50),
                    description=indicator.get('description', ''),
                    tags=indicator.get('tags', []),
                    first_seen=indicator.get('first_seen', datetime.utcnow()),
                    last_seen=datetime.utcnow(),
                    expires=indicator.get('expires'),
                    active=True
                )
                added_count += 1
        
        feed.update_record(last_polled=datetime.utcnow())
        
        db.audit_log.insert(
            user_id=auth.user_id,
            action='poll_threat_intel_feed',
            database_name=feed.name,
            query=f"Polled {feed.type} feed",
            result=f"Added {added_count}, Updated {updated_count} indicators",
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_threat_intel_to_redis()
        
        return {
            'message': 'Feed polled successfully',
            'added': added_count,
            'updated': updated_count,
            'total': added_count + updated_count
        }
    except Exception as e:
        abort(400, str(e))

@action('api/threat-intel/indicators', method=['GET'])
@action.uses(auth, cors)
def get_threat_intel_indicators():
    limit = request.params.get('limit', 100)
    offset = request.params.get('offset', 0)
    indicator_type = request.params.get('type')
    threat_level = request.params.get('threat_level')
    
    query = db(db.threat_intel_indicator.active == True)
    
    if indicator_type:
        query = query & (db.threat_intel_indicator.indicator_type == indicator_type)
    if threat_level:
        query = query & (db.threat_intel_indicator.threat_level == threat_level)
    
    indicators = query.select(
        limitby=(int(offset), int(offset) + int(limit)),
        orderby=~db.threat_intel_indicator.last_seen
    )
    
    result = []
    for indicator in indicators:
        feed = db.threat_intel_feed[indicator.feed_id] if indicator.feed_id else None
        result.append({
            'id': indicator.id,
            'feed_name': feed.name if feed else 'Manual',
            'indicator_type': indicator.indicator_type,
            'value': indicator.value,
            'threat_level': indicator.threat_level,
            'confidence': indicator.confidence,
            'description': indicator.description,
            'tags': indicator.tags,
            'matched_count': indicator.matched_count,
            'first_seen': indicator.first_seen.isoformat(),
            'last_seen': indicator.last_seen.isoformat(),
            'expires': indicator.expires.isoformat() if indicator.expires else None,
            'active': indicator.active
        })
    
    total = query.count()
    
    return {'indicators': result, 'total': total}

@action('api/threat-intel/indicators', method=['POST'])
@action.uses(auth, cors, db)
def create_threat_intel_indicator():
    try:
        data = request.json
        indicator = ThreatIntelIndicatorModel(**data)
        
        indicator_id = db.threat_intel_indicator.insert(
            feed_id=indicator.feed_id,
            indicator_type=indicator.indicator_type,
            value=indicator.value,
            threat_level=indicator.threat_level,
            confidence=indicator.confidence,
            description=indicator.description,
            tags=indicator.tags,
            expires=indicator.expires,
            active=indicator.active
        )
        
        db.audit_log.insert(
            user_id=auth.user_id,
            action='create_threat_intel_indicator',
            query=f"Added {indicator.indicator_type}: {indicator.value}",
            result='success',
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_threat_intel_to_redis()
        
        return {'id': indicator_id, 'message': 'Threat indicator created successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/threat-intel/indicators/<indicator_id:int>', method=['DELETE'])
@action.uses(auth, cors, db)
def delete_threat_intel_indicator(indicator_id):
    try:
        indicator = db.threat_intel_indicator[indicator_id]
        if not indicator:
            abort(404, 'Indicator not found')
        
        indicator.update_record(active=False)
        
        db.audit_log.insert(
            user_id=auth.user_id,
            action='delete_threat_intel_indicator',
            query=f"Deleted {indicator.indicator_type}: {indicator.value}",
            result='success',
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_threat_intel_to_redis()
        
        return {'message': 'Threat indicator deleted successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/threat-intel/import', method=['POST'])
@action.uses(auth, cors, db)
def import_threat_intel():
    try:
        data = request.json
        feed_type = data.get('type', 'stix')
        content = data.get('content', '')
        feed_name = data.get('feed_name', f'Manual Import - {datetime.utcnow().isoformat()}')
        
        indicators = []
        
        if feed_type == 'stix':
            indicators = parse_stix_indicators(content)
        elif feed_type == 'openioc':
            indicators = parse_openioc_indicators(content)
        elif feed_type == 'misp':
            indicators = parse_misp_event(content)
        else:
            abort(400, f'Unsupported feed type: {feed_type}')
        
        feed_id = db.threat_intel_feed.insert(
            name=feed_name,
            type=feed_type,
            url=None,
            active=True,
            last_polled=datetime.utcnow()
        )
        
        added_count = 0
        for indicator in indicators:
            db.threat_intel_indicator.insert(
                feed_id=feed_id,
                indicator_type=indicator['indicator_type'],
                value=indicator['value'],
                threat_level=indicator.get('threat_level', 'medium'),
                confidence=indicator.get('confidence', 50),
                description=indicator.get('description', ''),
                tags=indicator.get('tags', []),
                first_seen=indicator.get('first_seen', datetime.utcnow()),
                last_seen=datetime.utcnow(),
                expires=indicator.get('expires'),
                active=True
            )
            added_count += 1
        
        db.audit_log.insert(
            user_id=auth.user_id,
            action='import_threat_intel',
            database_name=feed_name,
            query=f"Imported {feed_type} data",
            result=f"Added {added_count} indicators",
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_threat_intel_to_redis()
        
        return {
            'message': 'Threat intelligence imported successfully',
            'feed_id': feed_id,
            'indicators_added': added_count
        }
    except Exception as e:
        abort(400, str(e))

@action('api/threat-intel/matches', method=['GET'])
@action.uses(auth, cors)
def get_threat_intel_matches():
    limit = request.params.get('limit', 100)
    offset = request.params.get('offset', 0)
    
    matches = db(db.threat_intel_match).select(
        limitby=(int(offset), int(offset) + int(limit)),
        orderby=~db.threat_intel_match.timestamp
    )
    
    result = []
    for match in matches:
        indicator = db.threat_intel_indicator[match.indicator_id]
        user = db.auth_user[match.user_id] if match.user_id else None
        result.append({
            'id': match.id,
            'indicator_type': indicator.indicator_type if indicator else 'unknown',
            'indicator_value': indicator.value if indicator else 'unknown',
            'threat_level': indicator.threat_level if indicator else 'unknown',
            'database_name': match.database_name,
            'username': user.email if user else 'System',
            'source_ip': match.source_ip,
            'query': match.query[:200] if match.query else None,
            'action_taken': match.action_taken,
            'match_details': match.match_details,
            'timestamp': match.timestamp.isoformat()
        })
    
    total = db(db.threat_intel_match).count()
    
    return {'matches': result, 'total': total}

# Database Security Configuration API Endpoints

@action('api/databases/<database_id:int>/security-config', method=['GET'])
@action.uses(auth, cors)
def get_database_security_config(database_id):
    database = db.managed_database[database_id]
    if not database:
        abort(404, 'Database not found')
    
    config = db(db.database_security_config.database_id == database_id).select().first()
    
    if not config:
        config = {
            'security_blocks_enabled': True,
            'threat_intel_blocks_enabled': True,
            'sql_injection_detection': True,
            'audit_logging': True,
            'block_default_resources': True,
            'threat_intel_action': 'block',
            'custom_rules': None,
            'whitelist_patterns': None
        }
    else:
        config = {
            'security_blocks_enabled': config.security_blocks_enabled,
            'threat_intel_blocks_enabled': config.threat_intel_blocks_enabled,
            'sql_injection_detection': config.sql_injection_detection,
            'audit_logging': config.audit_logging,
            'block_default_resources': config.block_default_resources,
            'threat_intel_action': config.threat_intel_action,
            'custom_rules': config.custom_rules,
            'whitelist_patterns': config.whitelist_patterns
        }
    
    return {
        'database_name': database.name,
        'config': config
    }

@action('api/databases/<database_id:int>/security-config', method=['PUT'])
@action.uses(auth, cors, db)
def update_database_security_config(database_id):
    try:
        database = db.managed_database[database_id]
        if not database:
            abort(404, 'Database not found')
        
        data = request.json
        config_data = DatabaseSecurityConfigModel(database_id=database_id, **data)
        
        existing = db(db.database_security_config.database_id == database_id).select().first()
        
        if existing:
            existing.update_record(
                security_blocks_enabled=config_data.security_blocks_enabled,
                threat_intel_blocks_enabled=config_data.threat_intel_blocks_enabled,
                sql_injection_detection=config_data.sql_injection_detection,
                audit_logging=config_data.audit_logging,
                block_default_resources=config_data.block_default_resources,
                threat_intel_action=config_data.threat_intel_action,
                custom_rules=config_data.custom_rules,
                whitelist_patterns=config_data.whitelist_patterns
            )
        else:
            db.database_security_config.insert(
                database_id=database_id,
                security_blocks_enabled=config_data.security_blocks_enabled,
                threat_intel_blocks_enabled=config_data.threat_intel_blocks_enabled,
                sql_injection_detection=config_data.sql_injection_detection,
                audit_logging=config_data.audit_logging,
                block_default_resources=config_data.block_default_resources,
                threat_intel_action=config_data.threat_intel_action,
                custom_rules=config_data.custom_rules,
                whitelist_patterns=config_data.whitelist_patterns
            )
        
        db.audit_log.insert(
            user_id=auth.user_id,
            action='update_database_security_config',
            database_name=database.name,
            query=json.dumps(data),
            result='success',
            ip_address=request.environ.get('REMOTE_ADDR')
        )
        
        sync_to_redis()
        
        return {'message': 'Security configuration updated successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/cloud-providers', method=['GET'])
@action.uses(auth, cors, db)
def get_cloud_providers():
    providers = []
    for provider in db(db.cloud_provider.is_active == True).select():
        providers.append({
            'id': provider.id,
            'name': provider.name,
            'provider_type': provider.provider_type,
            'is_active': provider.is_active,
            'test_status': provider.test_status,
            'created_at': provider.created_at.isoformat() if provider.created_at else None,
            'last_tested': provider.last_tested.isoformat() if provider.last_tested else None
        })
    return {'providers': providers}

@action('api/cloud-providers', method=['POST'])
@action.uses(auth, cors, db)
def create_cloud_provider():
    try:
        data = request.json
        provider_data = CloudProviderModel(**data)
        
        config_json = provider_data.configuration
        credentials_path = None
        
        if provider_data.credentials_path and os.path.exists(provider_data.credentials_path):
            credentials_path = provider_data.credentials_path
        
        provider_id = db.cloud_provider.insert(
            name=provider_data.name,
            provider_type=provider_data.provider_type,
            configuration=config_json,
            credentials_path=credentials_path,
            is_active=provider_data.is_active
        )
        
        asyncio.create_task(test_cloud_provider_async(provider_id))
        
        return {'id': provider_id, 'message': 'Cloud provider created successfully'}
    except Exception as e:
        abort(400, str(e))

@action('api/cloud-providers/<provider_id:int>/test', method=['POST'])
@action.uses(auth, cors, db)
def test_cloud_provider_connection(provider_id):
    provider = db.cloud_provider[provider_id]
    if not provider:
        abort(404, "Provider not found")
    
    try:
        result = test_cloud_provider_sync(provider_id)
        return {'test_result': result}
    except Exception as e:
        return {'test_result': 'failed', 'error': str(e)}

@action('api/cloud-instances', method=['GET'])
@action.uses(auth, cors, db)
def get_cloud_instances():
    instances = []
    for instance in db().select(db.cloud_database_instance.ALL):
        provider = db.cloud_provider[instance.provider_id]
        instances.append({
            'id': instance.id,
            'name': instance.name,
            'provider_name': provider.name if provider else 'Unknown',
            'instance_type': instance.instance_type,
            'instance_class': instance.instance_class,
            'status': instance.status,
            'endpoint': instance.endpoint,
            'port': instance.port,
            'created_at': instance.created_at.isoformat() if instance.created_at else None
        })
    return {'instances': instances}

@action('api/cloud-instances', method=['POST'])
@action.uses(auth, cors, db)
def create_cloud_instance():
    try:
        data = request.json
        instance_data = CloudDatabaseInstanceModel(**data)
        
        provider = db.cloud_provider[instance_data.provider_id]
        if not provider:
            abort(404, "Provider not found")
        
        instance_id = db.cloud_database_instance.insert(
            name=instance_data.name,
            provider_id=instance_data.provider_id,
            instance_type=instance_data.instance_type,
            instance_class=instance_data.instance_class,
            storage_size=instance_data.storage_size,
            engine_version=instance_data.engine_version,
            multi_az=instance_data.multi_az,
            backup_retention=instance_data.backup_retention,
            monitoring_enabled=instance_data.monitoring_enabled,
            auto_scaling_enabled=instance_data.auto_scaling_enabled,
            auto_scaling_config=instance_data.auto_scaling_config
        )
        
        asyncio.create_task(provision_cloud_instance_async(instance_id))
        
        return {'id': instance_id, 'message': 'Cloud instance creation initiated'}
    except Exception as e:
        abort(400, str(e))

@action('api/cloud-instances/<instance_id:int>/scale', method=['POST'])
@action.uses(auth, cors, db)
def scale_cloud_instance(instance_id):
    try:
        data = request.json
        action_type = data.get('action', 'scale_up')
        new_instance_class = data.get('instance_class')
        ai_enabled = data.get('ai_enabled', False)
        
        instance = db.cloud_database_instance[instance_id]
        if not instance:
            abort(404, "Instance not found")
        
        if ai_enabled:
            asyncio.create_task(ai_scale_recommendation_async(instance_id, action_type))
        else:
            asyncio.create_task(manual_scale_instance_async(instance_id, action_type, new_instance_class))
        
        return {'message': f'Scaling {action_type} initiated for instance {instance.name}'}
    except Exception as e:
        abort(400, str(e))

@action('api/scaling-policies', method=['GET'])
@action.uses(auth, cors, db)
def get_scaling_policies():
    policies = []
    for policy in db().select(db.scaling_policy.ALL):
        instance = db.cloud_database_instance[policy.cloud_instance_id]
        policies.append({
            'id': policy.id,
            'instance_name': instance.name if instance else 'Unknown',
            'metric_type': policy.metric_type,
            'scale_up_threshold': policy.scale_up_threshold,
            'scale_down_threshold': policy.scale_down_threshold,
            'ai_enabled': policy.ai_enabled,
            'ai_model': policy.ai_model,
            'is_active': policy.is_active
        })
    return {'policies': policies}

@action('api/scaling-policies', method=['POST'])
@action.uses(auth, cors, db)
def create_scaling_policy():
    try:
        data = request.json
        policy_data = ScalingPolicyModel(**data)
        
        instance = db.cloud_database_instance[policy_data.cloud_instance_id]
        if not instance:
            abort(404, "Instance not found")
        
        policy_id = db.scaling_policy.insert(
            cloud_instance_id=policy_data.cloud_instance_id,
            metric_type=policy_data.metric_type,
            scale_up_threshold=policy_data.scale_up_threshold,
            scale_down_threshold=policy_data.scale_down_threshold,
            scale_up_adjustment=policy_data.scale_up_adjustment,
            scale_down_adjustment=policy_data.scale_down_adjustment,
            cooldown_period=policy_data.cooldown_period,
            ai_enabled=policy_data.ai_enabled,
            ai_model=policy_data.ai_model,
            is_active=policy_data.is_active
        )
        
        return {'id': policy_id, 'message': 'Scaling policy created successfully'}
    except Exception as e:
        abort(400, str(e))

async def test_cloud_provider_async(provider_id):
    """Asynchronously test cloud provider connection"""
    provider = db.cloud_provider[provider_id]
    if not provider:
        return False
    
    try:
        result = await asyncio.get_event_loop().run_in_executor(
            None, test_cloud_provider_sync, provider_id
        )
        
        db.cloud_provider[provider_id] = dict(
            test_status='success' if result else 'failed',
            last_tested=datetime.utcnow()
        )
        db.commit()
        return result
    except Exception as e:
        db.cloud_provider[provider_id] = dict(
            test_status='failed',
            last_tested=datetime.utcnow()
        )
        db.commit()
        return False

def test_cloud_provider_sync(provider_id):
    """Synchronously test cloud provider connection"""
    provider = db.cloud_provider[provider_id]
    if not provider:
        return False
    
    try:
        if provider.provider_type == 'kubernetes':
            return test_kubernetes_connection(provider)
        elif provider.provider_type == 'aws':
            return test_aws_connection(provider)
        elif provider.provider_type == 'gcp':
            return test_gcp_connection(provider)
        return False
    except Exception as e:
        print(f"Error testing cloud provider {provider.name}: {e}")
        return False

def test_kubernetes_connection(provider):
    """Test Kubernetes cluster connectivity"""
    try:
        config_data = provider.configuration
        
        if provider.credentials_path:
            config.load_kube_config(config_file=provider.credentials_path)
        else:
            config.load_incluster_config()
        
        v1 = client.CoreV1Api()
        namespaces = v1.list_namespace(timeout_seconds=10)
        return len(namespaces.items) >= 0
    except Exception as e:
        print(f"Kubernetes connection test failed: {e}")
        return False

def test_aws_connection(provider):
    """Test AWS credentials and connectivity"""
    try:
        config_data = provider.configuration
        
        if provider.credentials_path:
            import json
            with open(provider.credentials_path, 'r') as f:
                creds = json.load(f)
            
            session = boto3.Session(
                aws_access_key_id=creds.get('access_key_id'),
                aws_secret_access_key=creds.get('secret_access_key'),
                region_name=config_data.get('region', 'us-east-1')
            )
        else:
            session = boto3.Session(region_name=config_data.get('region', 'us-east-1'))
        
        rds = session.client('rds')
        rds.describe_db_instances(MaxRecords=1)
        return True
    except Exception as e:
        print(f"AWS connection test failed: {e}")
        return False

def test_gcp_connection(provider):
    """Test GCP service account and connectivity"""
    try:
        config_data = provider.configuration
        
        if provider.credentials_path:
            credentials = service_account.Credentials.from_service_account_file(
                provider.credentials_path
            )
        else:
            credentials = service_account.Credentials.from_service_account_file(
                config_data.get('service_account_path')
            )
        
        client_obj = sql_v1.SqlInstancesServiceClient(credentials=credentials)
        project_id = config_data.get('project_id')
        
        request = sql_v1.SqlInstancesListRequest(project=project_id)
        client_obj.list(request=request)
        return True
    except Exception as e:
        print(f"GCP connection test failed: {e}")
        return False

async def provision_cloud_instance_async(instance_id):
    """Asynchronously provision cloud database instance"""
    instance = db.cloud_database_instance[instance_id]
    provider = db.cloud_provider[instance.provider_id]
    
    if not instance or not provider:
        return False
    
    try:
        if provider.provider_type == 'kubernetes':
            result = await provision_kubernetes_database(instance, provider)
        elif provider.provider_type == 'aws':
            result = await provision_aws_database(instance, provider)
        elif provider.provider_type == 'gcp':
            result = await provision_gcp_database(instance, provider)
        else:
            result = False
        
        if result:
            db.cloud_database_instance[instance_id] = dict(status='available')
        else:
            db.cloud_database_instance[instance_id] = dict(status='failed')
        
        db.commit()
        return result
    except Exception as e:
        print(f"Error provisioning instance: {e}")
        db.cloud_database_instance[instance_id] = dict(status='failed')
        db.commit()
        return False

async def provision_kubernetes_database(instance, provider):
    """Provision database in Kubernetes"""
    try:
        if provider.credentials_path:
            config.load_kube_config(config_file=provider.credentials_path)
        else:
            config.load_incluster_config()
        
        apps_v1 = client.AppsV1Api()
        v1 = client.CoreV1Api()
        
        namespace = provider.configuration.get('namespace', 'default')
        
        deployment_manifest = create_k8s_database_deployment(instance, provider)
        service_manifest = create_k8s_database_service(instance, provider)
        
        deployment = apps_v1.create_namespaced_deployment(
            namespace=namespace,
            body=deployment_manifest
        )
        
        service = v1.create_namespaced_service(
            namespace=namespace,
            body=service_manifest
        )
        
        endpoint = f"{service.metadata.name}.{namespace}.svc.cluster.local"
        port = service.spec.ports[0].port
        
        db.cloud_database_instance[instance.id] = dict(
            cloud_instance_id=deployment.metadata.name,
            endpoint=endpoint,
            port=port
        )
        
        server_id = create_database_server_entry(instance, endpoint, port)
        db.cloud_database_instance[instance.id] = dict(server_id=server_id)
        
        db.commit()
        return True
    except Exception as e:
        print(f"Error provisioning Kubernetes database: {e}")
        return False

async def provision_aws_database(instance, provider):
    """Provision database in AWS RDS"""
    try:
        config_data = provider.configuration
        
        if provider.credentials_path:
            import json
            with open(provider.credentials_path, 'r') as f:
                creds = json.load(f)
            
            session = boto3.Session(
                aws_access_key_id=creds.get('access_key_id'),
                aws_secret_access_key=creds.get('secret_access_key'),
                region_name=config_data.get('region', 'us-east-1')
            )
        else:
            session = boto3.Session(region_name=config_data.get('region', 'us-east-1'))
        
        rds = session.client('rds')
        
        db_params = {
            'DBInstanceIdentifier': f"articdbm-{instance.name}",
            'DBInstanceClass': instance.instance_class or 'db.t3.micro',
            'Engine': map_instance_type_to_aws_engine(instance.instance_type),
            'MasterUsername': 'articdbm',
            'MasterUserPassword': secrets.token_urlsafe(16),
            'AllocatedStorage': instance.storage_size or 20,
            'StorageType': 'gp2',
            'MultiAZ': instance.multi_az,
            'BackupRetentionPeriod': instance.backup_retention,
            'MonitoringInterval': 60 if instance.monitoring_enabled else 0,
            'VpcSecurityGroupIds': config_data.get('security_group_ids', []),
            'DBSubnetGroupName': config_data.get('subnet_group_name')
        }
        
        if instance.engine_version:
            db_params['EngineVersion'] = instance.engine_version
        
        response = rds.create_db_instance(**db_params)
        
        db_instance = response['DBInstance']
        
        db.cloud_database_instance[instance.id] = dict(
            cloud_instance_id=db_instance['DBInstanceIdentifier']
        )
        
        await wait_for_aws_instance_available(rds, db_instance['DBInstanceIdentifier'])
        
        instance_details = rds.describe_db_instances(
            DBInstanceIdentifier=db_instance['DBInstanceIdentifier']
        )['DBInstances'][0]
        
        endpoint = instance_details['Endpoint']['Address']
        port = instance_details['Endpoint']['Port']
        
        db.cloud_database_instance[instance.id] = dict(
            endpoint=endpoint,
            port=port
        )
        
        server_id = create_database_server_entry(instance, endpoint, port)
        db.cloud_database_instance[instance.id] = dict(server_id=server_id)
        
        db.commit()
        return True
    except Exception as e:
        print(f"Error provisioning AWS database: {e}")
        return False

async def provision_gcp_database(instance, provider):
    """Provision database in Google Cloud SQL"""
    try:
        config_data = provider.configuration
        
        if provider.credentials_path:
            credentials = service_account.Credentials.from_service_account_file(
                provider.credentials_path
            )
        else:
            credentials = service_account.Credentials.from_service_account_file(
                config_data.get('service_account_path')
            )
        
        client_obj = sql_v1.SqlInstancesServiceClient(credentials=credentials)
        project_id = config_data.get('project_id')
        
        instance_body = sql_v1.DatabaseInstance()
        instance_body.name = f"articdbm-{instance.name}"
        instance_body.database_version = map_instance_type_to_gcp_version(instance.instance_type, instance.engine_version)
        instance_body.region = config_data.get('region', 'us-central1')
        
        settings = sql_v1.Settings()
        settings.tier = instance.instance_class or 'db-f1-micro'
        settings.disk_size_gb = instance.storage_size or 20
        settings.disk_type = 'PD_SSD'
        
        backup_config = sql_v1.BackupConfiguration()
        backup_config.enabled = True
        backup_config.start_time = "03:00"
        
        settings.backup_configuration = backup_config
        instance_body.settings = settings
        
        request = sql_v1.SqlInstancesInsertRequest(
            project=project_id,
            body=instance_body
        )
        
        operation = client_obj.insert(request=request)
        
        db.cloud_database_instance[instance.id] = dict(
            cloud_instance_id=instance_body.name
        )
        
        await wait_for_gcp_operation(client_obj, project_id, operation.name)
        
        instance_request = sql_v1.SqlInstancesGetRequest(
            project=project_id,
            instance=instance_body.name
        )
        gcp_instance = client_obj.get(request=instance_request)
        
        endpoint = gcp_instance.ip_addresses[0].ip_address
        port = 3306 if instance.instance_type == 'mysql' else 5432
        
        db.cloud_database_instance[instance.id] = dict(
            endpoint=endpoint,
            port=port
        )
        
        server_id = create_database_server_entry(instance, endpoint, port)
        db.cloud_database_instance[instance.id] = dict(server_id=server_id)
        
        db.commit()
        return True
    except Exception as e:
        print(f"Error provisioning GCP database: {e}")
        return False

def create_database_server_entry(instance, endpoint, port):
    """Create corresponding database_server entry for cloud instance"""
    try:
        server_id = db.database_server.insert(
            name=f"cloud-{instance.name}",
            type=instance.instance_type,
            host=endpoint,
            port=port,
            username='articdbm',
            password='managed-by-cloud',
            active=True
        )
        
        sync_to_redis()
        return server_id
    except Exception as e:
        print(f"Error creating database server entry: {e}")
        return None

def create_k8s_database_deployment(instance, provider):
    """Create Kubernetes deployment manifest for database"""
    deployment_name = f"articdbm-{instance.instance_type}-{instance.name}"
    
    containers = []
    if instance.instance_type == 'mysql':
        containers.append({
            'name': 'mysql',
            'image': 'mysql:8.0',
            'env': [
                {'name': 'MYSQL_ROOT_PASSWORD', 'value': 'articdbm123'},
                {'name': 'MYSQL_DATABASE', 'value': 'articdbm'},
                {'name': 'MYSQL_USER', 'value': 'articdbm'},
                {'name': 'MYSQL_PASSWORD', 'value': 'articdbm'}
            ],
            'ports': [{'containerPort': 3306}],
            'resources': {
                'requests': {'memory': '256Mi', 'cpu': '250m'},
                'limits': {'memory': '512Mi', 'cpu': '500m'}
            }
        })
    elif instance.instance_type == 'postgresql':
        containers.append({
            'name': 'postgresql',
            'image': 'postgres:15',
            'env': [
                {'name': 'POSTGRES_DB', 'value': 'articdbm'},
                {'name': 'POSTGRES_USER', 'value': 'articdbm'},
                {'name': 'POSTGRES_PASSWORD', 'value': 'articdbm'}
            ],
            'ports': [{'containerPort': 5432}],
            'resources': {
                'requests': {'memory': '256Mi', 'cpu': '250m'},
                'limits': {'memory': '512Mi', 'cpu': '500m'}
            }
        })
    elif instance.instance_type == 'redis':
        containers.append({
            'name': 'redis',
            'image': 'redis:7',
            'ports': [{'containerPort': 6379}],
            'resources': {
                'requests': {'memory': '128Mi', 'cpu': '100m'},
                'limits': {'memory': '256Mi', 'cpu': '200m'}
            }
        })
    
    return {
        'apiVersion': 'apps/v1',
        'kind': 'Deployment',
        'metadata': {
            'name': deployment_name,
            'labels': {
                'app': deployment_name,
                'managed-by': 'articdbm'
            }
        },
        'spec': {
            'replicas': 1,
            'selector': {
                'matchLabels': {
                    'app': deployment_name
                }
            },
            'template': {
                'metadata': {
                    'labels': {
                        'app': deployment_name
                    }
                },
                'spec': {
                    'containers': containers
                }
            }
        }
    }

def create_k8s_database_service(instance, provider):
    """Create Kubernetes service manifest for database"""
    service_name = f"articdbm-{instance.instance_type}-{instance.name}"
    deployment_name = f"articdbm-{instance.instance_type}-{instance.name}"
    
    port = 3306
    if instance.instance_type == 'postgresql':
        port = 5432
    elif instance.instance_type == 'redis':
        port = 6379
    
    return {
        'apiVersion': 'v1',
        'kind': 'Service',
        'metadata': {
            'name': service_name,
            'labels': {
                'app': deployment_name,
                'managed-by': 'articdbm'
            }
        },
        'spec': {
            'selector': {
                'app': deployment_name
            },
            'ports': [{
                'protocol': 'TCP',
                'port': port,
                'targetPort': port
            }],
            'type': 'ClusterIP'
        }
    }

def map_instance_type_to_aws_engine(instance_type):
    """Map ArticDBM instance type to AWS RDS engine"""
    mapping = {
        'mysql': 'mysql',
        'postgresql': 'postgres',
        'mssql': 'sqlserver-ex',
        'redis': 'redis'
    }
    return mapping.get(instance_type, 'mysql')

def map_instance_type_to_gcp_version(instance_type, engine_version=None):
    """Map ArticDBM instance type to GCP SQL database version"""
    if engine_version:
        return engine_version
    
    mapping = {
        'mysql': 'MYSQL_8_0',
        'postgresql': 'POSTGRES_15',
        'mssql': 'SQLSERVER_2019_STANDARD'
    }
    return mapping.get(instance_type, 'MYSQL_8_0')

async def wait_for_aws_instance_available(rds_client, db_instance_id):
    """Wait for AWS RDS instance to become available"""
    max_attempts = 60
    attempt = 0
    
    while attempt < max_attempts:
        try:
            response = rds_client.describe_db_instances(DBInstanceIdentifier=db_instance_id)
            status = response['DBInstances'][0]['DBInstanceStatus']
            
            if status == 'available':
                return True
            elif status in ['failed', 'stopped']:
                return False
            
            await asyncio.sleep(30)
            attempt += 1
        except Exception as e:
            print(f"Error checking AWS instance status: {e}")
            await asyncio.sleep(30)
            attempt += 1
    
    return False

async def wait_for_gcp_operation(client_obj, project_id, operation_name):
    """Wait for GCP SQL operation to complete"""
    max_attempts = 60
    attempt = 0
    
    while attempt < max_attempts:
        try:
            request = sql_v1.SqlOperationsGetRequest(
                project=project_id,
                operation=operation_name
            )
            operation = client_obj.get_operation(request=request)
            
            if operation.status == sql_v1.Operation.Status.DONE:
                return True
            elif operation.status == sql_v1.Operation.Status.ERROR:
                return False
            
            await asyncio.sleep(30)
            attempt += 1
        except Exception as e:
            print(f"Error checking GCP operation status: {e}")
            await asyncio.sleep(30)
            attempt += 1
    
    return False

async def ai_scale_recommendation_async(instance_id, action_type):
    """Get AI-powered scaling recommendations"""
    try:
        instance = db.cloud_database_instance[instance_id]
        if not instance:
            return False
        
        policies = db(db.scaling_policy.cloud_instance_id == instance_id).select()
        ai_policy = None
        
        for policy in policies:
            if policy.ai_enabled:
                ai_policy = policy
                break
        
        if not ai_policy:
            return False
        
        metrics_data = instance.metrics_data or {}
        
        if ai_policy.ai_model == 'openai':
            recommendation = await get_openai_scaling_recommendation(instance, metrics_data, action_type)
        elif ai_policy.ai_model == 'anthropic':
            recommendation = await get_anthropic_scaling_recommendation(instance, metrics_data, action_type)
        else:
            recommendation = await get_ollama_scaling_recommendation(instance, metrics_data, action_type)
        
        if recommendation:
            event_id = db.scaling_event.insert(
                cloud_instance_id=instance_id,
                trigger_type='ai',
                action=action_type,
                old_instance_class=instance.instance_class,
                new_instance_class=recommendation.get('recommended_class'),
                ai_confidence=recommendation.get('confidence', 0.5),
                ai_reasoning=recommendation.get('reasoning'),
                status='pending'
            )
            
            if recommendation.get('should_scale', False):
                await execute_scaling_action(instance_id, recommendation.get('recommended_class'))
        
        return True
    except Exception as e:
        print(f"Error in AI scaling recommendation: {e}")
        return False

async def get_openai_scaling_recommendation(instance, metrics, action_type):
    """Get scaling recommendation from OpenAI"""
    try:
        client = openai.OpenAI(api_key=os.getenv('OPENAI_API_KEY'))
        
        prompt = f"""
        Analyze the following database instance metrics and recommend scaling action:
        
        Instance: {instance.name}
        Type: {instance.instance_type}
        Current Class: {instance.instance_class}
        Metrics: {json.dumps(metrics, indent=2)}
        Requested Action: {action_type}
        
        Provide a JSON response with:
        - should_scale: boolean
        - recommended_class: string (new instance class)
        - confidence: float (0.0-1.0)
        - reasoning: string (explanation)
        """
        
        response = client.chat.completions.create(
            model="gpt-4",
            messages=[
                {"role": "system", "content": "You are a database performance optimization expert. Analyze metrics and provide scaling recommendations in JSON format."},
                {"role": "user", "content": prompt}
            ],
            temperature=0.3
        )
        
        content = response.choices[0].message.content
        return json.loads(content)
    except Exception as e:
        print(f"Error getting OpenAI recommendation: {e}")
        return None

async def get_anthropic_scaling_recommendation(instance, metrics, action_type):
    """Get scaling recommendation from Anthropic Claude"""
    try:
        client = anthropic.Client(api_key=os.getenv('ANTHROPIC_API_KEY'))
        
        prompt = f"""
        Analyze the following database instance metrics and recommend scaling action:
        
        Instance: {instance.name}
        Type: {instance.instance_type}
        Current Class: {instance.instance_class}
        Metrics: {json.dumps(metrics, indent=2)}
        Requested Action: {action_type}
        
        Provide a JSON response with:
        - should_scale: boolean
        - recommended_class: string (new instance class)
        - confidence: float (0.0-1.0)
        - reasoning: string (explanation)
        """
        
        message = client.messages.create(
            model="claude-3-sonnet-20240229",
            max_tokens=1000,
            messages=[
                {"role": "user", "content": prompt}
            ]
        )
        
        return json.loads(message.content[0].text)
    except Exception as e:
        print(f"Error getting Anthropic recommendation: {e}")
        return None

async def get_ollama_scaling_recommendation(instance, metrics, action_type):
    """Get scaling recommendation from Ollama (local LLM)"""
    try:
        ollama_url = os.getenv('OLLAMA_URL', 'http://localhost:11434')
        
        prompt = f"""
        Analyze database metrics and recommend scaling:
        Instance: {instance.name}, Type: {instance.instance_type}
        Current: {instance.instance_class}, Action: {action_type}
        Metrics: {json.dumps(metrics)}
        
        JSON response: should_scale, recommended_class, confidence, reasoning
        """
        
        response = requests.post(f"{ollama_url}/api/generate", json={
            "model": "llama2:7b",
            "prompt": prompt,
            "stream": False
        })
        
        if response.status_code == 200:
            result = response.json()
            return json.loads(result['response'])
        return None
    except Exception as e:
        print(f"Error getting Ollama recommendation: {e}")
        return None

async def manual_scale_instance_async(instance_id, action_type, new_instance_class):
    """Manual scaling without AI"""
    try:
        instance = db.cloud_database_instance[instance_id]
        if not instance:
            return False
        
        event_id = db.scaling_event.insert(
            cloud_instance_id=instance_id,
            trigger_type='manual',
            action=action_type,
            old_instance_class=instance.instance_class,
            new_instance_class=new_instance_class,
            status='pending'
        )
        
        success = await execute_scaling_action(instance_id, new_instance_class)
        
        db.scaling_event[event_id] = dict(
            status='completed' if success else 'failed',
            completed_at=datetime.utcnow()
        )
        db.commit()
        
        return success
    except Exception as e:
        print(f"Error in manual scaling: {e}")
        return False

async def execute_scaling_action(instance_id, new_instance_class):
    """Execute the actual scaling operation"""
    try:
        instance = db.cloud_database_instance[instance_id]
        provider = db.cloud_provider[instance.provider_id]
        
        if provider.provider_type == 'aws':
            success = await scale_aws_instance(instance, provider, new_instance_class)
        elif provider.provider_type == 'gcp':
            success = await scale_gcp_instance(instance, provider, new_instance_class)
        elif provider.provider_type == 'kubernetes':
            success = await scale_kubernetes_instance(instance, provider, new_instance_class)
        else:
            success = False
        
        if success:
            db.cloud_database_instance[instance_id] = dict(
                instance_class=new_instance_class,
                last_scaled=datetime.utcnow()
            )
            db.commit()
        
        return success
    except Exception as e:
        print(f"Error executing scaling action: {e}")
        return False

async def scale_aws_instance(instance, provider, new_instance_class):
    """Scale AWS RDS instance"""
    try:
        config_data = provider.configuration
        
        if provider.credentials_path:
            import json
            with open(provider.credentials_path, 'r') as f:
                creds = json.load(f)
            
            session = boto3.Session(
                aws_access_key_id=creds.get('access_key_id'),
                aws_secret_access_key=creds.get('secret_access_key'),
                region_name=config_data.get('region', 'us-east-1')
            )
        else:
            session = boto3.Session(region_name=config_data.get('region', 'us-east-1'))
        
        rds = session.client('rds')
        
        rds.modify_db_instance(
            DBInstanceIdentifier=instance.cloud_instance_id,
            DBInstanceClass=new_instance_class,
            ApplyImmediately=True
        )
        
        return True
    except Exception as e:
        print(f"Error scaling AWS instance: {e}")
        return False

async def scale_gcp_instance(instance, provider, new_instance_class):
    """Scale GCP Cloud SQL instance"""
    try:
        config_data = provider.configuration
        
        if provider.credentials_path:
            credentials = service_account.Credentials.from_service_account_file(
                provider.credentials_path
            )
        else:
            credentials = service_account.Credentials.from_service_account_file(
                config_data.get('service_account_path')
            )
        
        client_obj = sql_v1.SqlInstancesServiceClient(credentials=credentials)
        project_id = config_data.get('project_id')
        
        instance_body = sql_v1.DatabaseInstance()
        settings = sql_v1.Settings()
        settings.tier = new_instance_class
        instance_body.settings = settings
        
        request = sql_v1.SqlInstancesPatchRequest(
            project=project_id,
            instance=instance.cloud_instance_id,
            body=instance_body
        )
        
        operation = client_obj.patch(request=request)
        await wait_for_gcp_operation(client_obj, project_id, operation.name)
        
        return True
    except Exception as e:
        print(f"Error scaling GCP instance: {e}")
        return False

async def scale_kubernetes_instance(instance, provider, new_instance_class):
    """Scale Kubernetes database deployment"""
    try:
        if provider.credentials_path:
            config.load_kube_config(config_file=provider.credentials_path)
        else:
            config.load_incluster_config()
        
        apps_v1 = client.AppsV1Api()
        namespace = provider.configuration.get('namespace', 'default')
        deployment_name = instance.cloud_instance_id
        
        deployment = apps_v1.read_namespaced_deployment(
            name=deployment_name,
            namespace=namespace
        )
        
        resource_map = {
            'small': {'memory': '256Mi', 'cpu': '250m'},
            'medium': {'memory': '512Mi', 'cpu': '500m'},
            'large': {'memory': '1Gi', 'cpu': '1000m'},
            'xlarge': {'memory': '2Gi', 'cpu': '2000m'}
        }
        
        resources = resource_map.get(new_instance_class, resource_map['medium'])
        
        deployment.spec.template.spec.containers[0].resources.requests = resources
        deployment.spec.template.spec.containers[0].resources.limits = resources
        
        apps_v1.patch_namespaced_deployment(
            name=deployment_name,
            namespace=namespace,
            body=deployment
        )
        
        return True
    except Exception as e:
        print(f"Error scaling Kubernetes instance: {e}")
        return False

async def periodic_scaling_check():
    """Periodically check scaling policies and trigger scaling if needed"""
    while True:
        try:
            active_policies = db(
                (db.scaling_policy.is_active == True) & 
                (db.cloud_database_instance.id == db.scaling_policy.cloud_instance_id) &
                (db.cloud_database_instance.status == 'available')
            ).select()
            
            for policy in active_policies:
                instance = db.cloud_database_instance[policy.cloud_instance_id]
                
                should_scale = await check_scaling_thresholds(policy, instance)
                if should_scale:
                    if policy.ai_enabled:
                        await ai_scale_recommendation_async(instance.id, should_scale)
                    else:
                        await trigger_threshold_scaling(policy, instance, should_scale)
            
        except Exception as e:
            print(f"Error in periodic scaling check: {e}")
        
        await asyncio.sleep(300)

async def check_scaling_thresholds(policy, instance):
    """Check if instance metrics exceed scaling thresholds"""
    try:
        metrics = await collect_instance_metrics(instance)
        if not metrics:
            return None
        
        metric_value = metrics.get(policy.metric_type, 0)
        
        if metric_value >= policy.scale_up_threshold:
            return 'scale_up'
        elif metric_value <= policy.scale_down_threshold:
            return 'scale_down'
        
        return None
    except Exception as e:
        print(f"Error checking scaling thresholds: {e}")
        return None

async def collect_instance_metrics(instance):
    """Collect metrics for cloud database instance"""
    try:
        provider = db.cloud_provider[instance.provider_id]
        
        if provider.provider_type == 'aws':
            return await collect_aws_metrics(instance, provider)
        elif provider.provider_type == 'gcp':
            return await collect_gcp_metrics(instance, provider)
        elif provider.provider_type == 'kubernetes':
            return await collect_k8s_metrics(instance, provider)
        
        return {}
    except Exception as e:
        print(f"Error collecting instance metrics: {e}")
        return {}

async def collect_aws_metrics(instance, provider):
    """Collect CloudWatch metrics for AWS RDS"""
    try:
        config_data = provider.configuration
        
        if provider.credentials_path:
            import json
            with open(provider.credentials_path, 'r') as f:
                creds = json.load(f)
            
            session = boto3.Session(
                aws_access_key_id=creds.get('access_key_id'),
                aws_secret_access_key=creds.get('secret_access_key'),
                region_name=config_data.get('region', 'us-east-1')
            )
        else:
            session = boto3.Session(region_name=config_data.get('region', 'us-east-1'))
        
        cloudwatch = session.client('cloudwatch')
        
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(minutes=10)
        
        cpu_response = cloudwatch.get_metric_statistics(
            Namespace='AWS/RDS',
            MetricName='CPUUtilization',
            Dimensions=[{
                'Name': 'DBInstanceIdentifier',
                'Value': instance.cloud_instance_id
            }],
            StartTime=start_time,
            EndTime=end_time,
            Period=300,
            Statistics=['Average']
        )
        
        memory_response = cloudwatch.get_metric_statistics(
            Namespace='AWS/RDS',
            MetricName='DatabaseConnections',
            Dimensions=[{
                'Name': 'DBInstanceIdentifier',
                'Value': instance.cloud_instance_id
            }],
            StartTime=start_time,
            EndTime=end_time,
            Period=300,
            Statistics=['Average']
        )
        
        metrics = {}
        
        if cpu_response['Datapoints']:
            cpu_avg = sum(dp['Average'] for dp in cpu_response['Datapoints']) / len(cpu_response['Datapoints'])
            metrics['cpu'] = cpu_avg
        
        if memory_response['Datapoints']:
            conn_avg = sum(dp['Average'] for dp in memory_response['Datapoints']) / len(memory_response['Datapoints'])
            metrics['connections'] = conn_avg
        
        db.cloud_database_instance[instance.id] = dict(metrics_data=metrics)
        db.commit()
        
        return metrics
    except Exception as e:
        print(f"Error collecting AWS metrics: {e}")
        return {}

async def collect_gcp_metrics(instance, provider):
    """Collect GCP monitoring metrics"""
    try:
        from google.cloud import monitoring_v3
        
        config_data = provider.configuration
        
        if provider.credentials_path:
            credentials = service_account.Credentials.from_service_account_file(
                provider.credentials_path
            )
        else:
            credentials = service_account.Credentials.from_service_account_file(
                config_data.get('service_account_path')
            )
        
        client = monitoring_v3.MetricServiceClient(credentials=credentials)
        project_name = f"projects/{config_data.get('project_id')}"
        
        interval = monitoring_v3.TimeInterval({
            "end_time": {"seconds": int(datetime.utcnow().timestamp())},
            "start_time": {"seconds": int((datetime.utcnow() - timedelta(minutes=10)).timestamp())},
        })
        
        cpu_filter = f'resource.type="cloudsql_database" AND resource.labels.database_id="{instance.cloud_instance_id}"'
        
        request = monitoring_v3.ListTimeSeriesRequest({
            "name": project_name,
            "filter": cpu_filter,
            "interval": interval,
            "view": monitoring_v3.ListTimeSeriesRequest.TimeSeriesView.FULL,
        })
        
        results = client.list_time_series(request=request)
        
        metrics = {}
        for result in results:
            if result.metric.type == "cloudsql.googleapis.com/database/cpu/utilization":
                if result.points:
                    cpu_avg = sum(point.value.double_value for point in result.points) / len(result.points)
                    metrics['cpu'] = cpu_avg * 100
        
        db.cloud_database_instance[instance.id] = dict(metrics_data=metrics)
        db.commit()
        
        return metrics
    except Exception as e:
        print(f"Error collecting GCP metrics: {e}")
        return {}

async def collect_k8s_metrics(instance, provider):
    """Collect Kubernetes metrics via metrics server"""
    try:
        if provider.credentials_path:
            config.load_kube_config(config_file=provider.credentials_path)
        else:
            config.load_incluster_config()
        
        v1 = client.CoreV1Api()
        namespace = provider.configuration.get('namespace', 'default')
        
        pods = v1.list_namespaced_pod(
            namespace=namespace,
            label_selector=f"app={instance.cloud_instance_id}"
        )
        
        metrics = {}
        
        if pods.items:
            pod = pods.items[0]
            
            try:
                custom_api = client.CustomObjectsApi()
                pod_metrics = custom_api.get_namespaced_custom_object(
                    group="metrics.k8s.io",
                    version="v1beta1",
                    namespace=namespace,
                    plural="pods",
                    name=pod.metadata.name
                )
                
                if 'containers' in pod_metrics and pod_metrics['containers']:
                    container_metrics = pod_metrics['containers'][0]
                    
                    if 'usage' in container_metrics:
                        cpu_usage = container_metrics['usage'].get('cpu', '0')
                        memory_usage = container_metrics['usage'].get('memory', '0')
                        
                        cpu_millicores = int(cpu_usage.replace('n', '')) / 1000000 if 'n' in cpu_usage else int(cpu_usage.replace('m', ''))
                        memory_bytes = int(memory_usage.replace('Ki', '')) * 1024 if 'Ki' in memory_usage else int(memory_usage)
                        
                        metrics['cpu'] = (cpu_millicores / 1000) * 100
                        metrics['memory'] = (memory_bytes / (1024*1024*1024)) * 100
                
            except Exception as e:
                print(f"Could not get pod metrics: {e}")
        
        db.cloud_database_instance[instance.id] = dict(metrics_data=metrics)
        db.commit()
        
        return metrics
    except Exception as e:
        print(f"Error collecting K8s metrics: {e}")
        return {}

async def trigger_threshold_scaling(policy, instance, action):
    """Trigger scaling based on threshold breach"""
    try:
        current_class = instance.instance_class
        
        if action == 'scale_up':
            new_class = get_next_instance_class(current_class, 'up')
        else:
            new_class = get_next_instance_class(current_class, 'down')
        
        if new_class == current_class:
            return False
        
        event_id = db.scaling_event.insert(
            cloud_instance_id=instance.id,
            trigger_type='threshold',
            action=action,
            old_instance_class=current_class,
            new_instance_class=new_class,
            trigger_metric=policy.metric_type,
            trigger_value=instance.metrics_data.get(policy.metric_type, 0) if instance.metrics_data else 0,
            status='pending'
        )
        
        success = await execute_scaling_action(instance.id, new_class)
        
        db.scaling_event[event_id] = dict(
            status='completed' if success else 'failed',
            completed_at=datetime.utcnow()
        )
        db.commit()
        
        return success
    except Exception as e:
        print(f"Error triggering threshold scaling: {e}")
        return False

def get_next_instance_class(current_class, direction):
    """Get the next instance class for scaling up or down"""
    aws_classes = ['db.t3.micro', 'db.t3.small', 'db.t3.medium', 'db.t3.large', 'db.t3.xlarge', 'db.t3.2xlarge']
    gcp_classes = ['db-f1-micro', 'db-g1-small', 'db-n1-standard-1', 'db-n1-standard-2', 'db-n1-standard-4', 'db-n1-standard-8']
    k8s_classes = ['small', 'medium', 'large', 'xlarge', '2xlarge']
    
    for class_list in [aws_classes, gcp_classes, k8s_classes]:
        if current_class in class_list:
            current_index = class_list.index(current_class)
            
            if direction == 'up' and current_index < len(class_list) - 1:
                return class_list[current_index + 1]
            elif direction == 'down' and current_index > 0:
                return class_list[current_index - 1]
    
    return current_class

def sync_threat_intel_to_redis():
    """Sync threat intelligence indicators to Redis for proxy consumption"""
    try:
        threat_indicators = {}
        
        for indicator in db(db.threat_intel_indicator.active == True).select():
            key = f"{indicator.indicator_type}:{indicator.value}"
            threat_indicators[key] = {
                'id': indicator.id,
                'type': indicator.indicator_type,
                'value': indicator.value,
                'threat_level': indicator.threat_level,
                'confidence': indicator.confidence,
                'description': indicator.description,
                'tags': indicator.tags,
                'action': 'block'
            }
        
        database_configs = {}
        for config in db(db.database_security_config).select():
            database = db.managed_database[config.database_id]
            if database:
                database_configs[database.name] = {
                    'security_blocks_enabled': config.security_blocks_enabled,
                    'threat_intel_blocks_enabled': config.threat_intel_blocks_enabled,
                    'sql_injection_detection': config.sql_injection_detection,
                    'audit_logging': config.audit_logging,
                    'block_default_resources': config.block_default_resources,
                    'threat_intel_action': config.threat_intel_action
                }
        
        redis_client.set('articdbm:threat_indicators', json.dumps(threat_indicators))
        redis_client.set('articdbm:database_security_configs', json.dumps(database_configs))
        
        redis_client.expire('articdbm:threat_indicators', 300)
        redis_client.expire('articdbm:database_security_configs', 300)
        
        return True
    except Exception as e:
        print(f"Error syncing threat intel to Redis: {e}")
        return False

if __name__ == "__main__":
    # Seed default blocked resources on startup if needed
    seed_default_blocked_resources()
    
    asyncio.create_task(init_aio_redis())
    asyncio.create_task(periodic_sync())
    asyncio.create_task(periodic_license_validation())
    asyncio.create_task(periodic_scaling_check())
    
    from py4web import start
    start(host='0.0.0.0', port=8000)