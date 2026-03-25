# XDP Management API for ArticDBM Manager
import json
import time
from datetime import datetime, timedelta
from py4web import action, request, response, abort, Field
from py4web.utils.cors import CORS
from pydal.validators import *

# XDP Management API Endpoints

@action.uses(cors)
@action('api/xdp/rules', method=['GET', 'POST'])
def xdp_rules():
    """Manage XDP IP blocking rules"""
    if request.method == 'GET':
        # Get current XDP rules
        rules = redis_client.hgetall('articdbm:xdp:rules')
        return {"rules": rules, "count": len(rules)}

    elif request.method == 'POST':
        data = request.json

        # Validate input
        if not data.get('ip_address') or not data.get('reason'):
            return {"error": "ip_address and reason are required"}, 400

        rule = {
            "ip_address": data['ip_address'],
            "reason": data['reason'],
            "blocked_at": datetime.utcnow().isoformat(),
            "blocked_by": data.get('blocked_by', 'system'),
            "expires_at": data.get('expires_at'),
            "rule_type": data.get('rule_type', 'manual')
        }

        # Store in Redis
        rule_key = f"ip:{data['ip_address']}"
        redis_client.hset('articdbm:xdp:rules', rule_key, json.dumps(rule))

        # Notify proxies to update XDP rules
        redis_client.publish('articdbm:xdp:rule_update', json.dumps({
            'action': 'add_rule',
            'rule': rule
        }))

        return {"message": "Rule added successfully", "rule": rule}

@action.uses(cors)
@action('api/xdp/rules/<rule_id>', method=['DELETE'])
def delete_xdp_rule(rule_id):
    """Delete XDP rule"""
    # Remove from Redis
    removed = redis_client.hdel('articdbm:xdp:rules', rule_id)

    if removed:
        # Notify proxies
        redis_client.publish('articdbm:xdp:rule_update', json.dumps({
            'action': 'remove_rule',
            'rule_id': rule_id
        }))
        return {"message": "Rule removed successfully"}
    else:
        return {"error": "Rule not found"}, 404

@action.uses(cors)
@action('api/xdp/stats')
def xdp_stats():
    """Get XDP performance statistics"""
    stats = redis_client.hgetall('articdbm:xdp:stats')

    # Convert to proper format
    formatted_stats = {}
    for key, value in stats.items():
        try:
            formatted_stats[key] = json.loads(value)
        except:
            formatted_stats[key] = value

    return {
        "stats": formatted_stats,
        "timestamp": datetime.utcnow().isoformat()
    }

@action.uses(cors)
@action('api/cache/stats')
def cache_stats():
    """Get multi-tier cache statistics"""
    cache_stats = redis_client.hgetall('articdbm:cache:stats')

    stats = {}
    for key, value in cache_stats.items():
        try:
            stats[key] = json.loads(value)
        except:
            stats[key] = value

    return {
        "cache_stats": stats,
        "timestamp": datetime.utcnow().isoformat()
    }

@action.uses(cors)
@action('api/cache/invalidate', method=['POST'])
def invalidate_cache():
    """Invalidate cache patterns"""
    data = request.json

    if not data.get('pattern'):
        return {"error": "pattern is required"}, 400

    pattern = data['pattern']

    # Publish cache invalidation
    redis_client.publish('articdbm:cache:invalidate', json.dumps({
        'pattern': pattern,
        'timestamp': datetime.utcnow().isoformat(),
        'requested_by': data.get('requested_by', 'system')
    }))

    return {"message": f"Cache invalidation requested for pattern: {pattern}"}

@action.uses(cors)
@action('api/bluegreen/deployments', method=['GET', 'POST'])
def bluegreen_deployments():
    """Manage blue/green deployments"""
    if request.method == 'GET':
        # Get current deployments
        deployments = redis_client.hgetall('articdbm:deployments')

        result = {}
        for key, value in deployments.items():
            try:
                result[key] = json.loads(value)
            except:
                result[key] = value

        return {"deployments": result}

    elif request.method == 'POST':
        data = request.json

        # Validate deployment request
        required_fields = ['deployment_id', 'strategy', 'primary_environment']
        for field in required_fields:
            if not data.get(field):
                return {"error": f"{field} is required"}, 400

        deployment = {
            "deployment_id": data['deployment_id'],
            "strategy": data['strategy'],
            "primary_environment": data['primary_environment'],
            "secondary_environment": data.get('secondary_environment'),
            "traffic_percentage": data.get('traffic_percentage', 100),
            "created_at": datetime.utcnow().isoformat(),
            "created_by": data.get('created_by', 'system'),
            "status": "active"
        }

        # Store deployment
        redis_client.hset('articdbm:deployments',
                         deployment['deployment_id'],
                         json.dumps(deployment))

        # Notify proxies
        redis_client.publish('articdbm:deployment:update', json.dumps({
            'action': 'start_deployment',
            'deployment': deployment
        }))

        return {"message": "Deployment started", "deployment": deployment}

@action.uses(cors)
@action('api/bluegreen/rollback', method=['POST'])
def rollback_deployment():
    """Rollback a deployment"""
    data = request.json

    deployment_id = data.get('deployment_id')
    if not deployment_id:
        return {"error": "deployment_id is required"}, 400

    # Get current deployment
    deployment_data = redis_client.hget('articdbm:deployments', deployment_id)
    if not deployment_data:
        return {"error": "Deployment not found"}, 404

    deployment = json.loads(deployment_data)

    # Perform rollback
    rollback_request = {
        "deployment_id": deployment_id,
        "reason": data.get('reason', 'manual_rollback'),
        "rollback_at": datetime.utcnow().isoformat(),
        "rollback_by": data.get('rollback_by', 'system')
    }

    # Notify proxies
    redis_client.publish('articdbm:deployment:rollback', json.dumps(rollback_request))

    # Update deployment status
    deployment['status'] = 'rolled_back'
    deployment['rollback_info'] = rollback_request
    redis_client.hset('articdbm:deployments', deployment_id, json.dumps(deployment))

    return {"message": "Rollback initiated", "deployment": deployment}

@action.uses(cors)
@action('api/multiwrite/execute', method=['POST'])
def execute_multiwrite():
    """Execute multi-write operation"""
    data = request.json

    # Validate multi-write request
    required_fields = ['query', 'databases']
    for field in required_fields:
        if not data.get(field):
            return {"error": f"{field} is required"}, 400

    write_request = {
        "request_id": f"mw_{int(time.time())}_{hash(data['query']) % 10000}",
        "query": data['query'],
        "databases": data['databases'],
        "strategy": data.get('strategy', 'sync'),
        "timeout": data.get('timeout', 30),
        "created_at": datetime.utcnow().isoformat(),
        "created_by": data.get('created_by', 'system')
    }

    # Store request for tracking
    redis_client.hset('articdbm:multiwrite:requests',
                     write_request['request_id'],
                     json.dumps(write_request))

    # Notify proxies to execute
    redis_client.publish('articdbm:multiwrite:execute', json.dumps(write_request))

    return {
        "message": "Multi-write request submitted",
        "request_id": write_request['request_id'],
        "request": write_request
    }

@action.uses(cors)
@action('api/redis-cluster/stats')
def redis_cluster_stats():
    """Get Redis cluster statistics"""
    cluster_stats = redis_client.hgetall('articdbm:redis:cluster_stats')

    stats = {}
    for key, value in cluster_stats.items():
        try:
            stats[key] = json.loads(value)
        except:
            stats[key] = value

    return {
        "cluster_stats": stats,
        "timestamp": datetime.utcnow().isoformat()
    }

@action.uses(cors)
@action('api/numa/topology')
def numa_topology():
    """Get NUMA topology information"""
    topology_info = redis_client.get('articdbm:numa:topology')

    if topology_info:
        try:
            topology = json.loads(topology_info)
            return {"topology": topology}
        except:
            pass

    return {"topology": None, "message": "NUMA topology information not available"}

@action.uses(cors)
@action('api/performance/metrics')
def performance_metrics():
    """Get comprehensive performance metrics"""

    # Collect metrics from various sources
    metrics = {
        "xdp_stats": {},
        "cache_stats": {},
        "cluster_stats": {},
        "deployment_stats": {},
        "timestamp": datetime.utcnow().isoformat()
    }

    # XDP metrics
    xdp_stats = redis_client.hgetall('articdbm:xdp:stats')
    for key, value in xdp_stats.items():
        try:
            metrics["xdp_stats"][key] = json.loads(value)
        except:
            metrics["xdp_stats"][key] = value

    # Cache metrics
    cache_stats = redis_client.hgetall('articdbm:cache:stats')
    for key, value in cache_stats.items():
        try:
            metrics["cache_stats"][key] = json.loads(value)
        except:
            metrics["cache_stats"][key] = value

    # Cluster metrics
    cluster_stats = redis_client.hgetall('articdbm:redis:cluster_stats')
    for key, value in cluster_stats.items():
        try:
            metrics["cluster_stats"][key] = json.loads(value)
        except:
            metrics["cluster_stats"][key] = value

    return metrics

@action.uses(cors)
@action('api/system/health')
def system_health():
    """Get overall system health status"""

    health = {
        "status": "healthy",
        "components": {},
        "timestamp": datetime.utcnow().isoformat()
    }

    # Check component health
    components = [
        "xdp_controller",
        "cache_manager",
        "redis_cluster",
        "deployment_manager",
        "multiwrite_manager"
    ]

    for component in components:
        health_key = f"articdbm:health:{component}"
        component_health = redis_client.get(health_key)

        if component_health:
            try:
                health["components"][component] = json.loads(component_health)
            except:
                health["components"][component] = {"status": "unknown"}
        else:
            health["components"][component] = {"status": "unknown"}

    # Determine overall status
    unhealthy_count = sum(1 for comp in health["components"].values()
                         if comp.get("status") != "healthy")

    if unhealthy_count == 0:
        health["status"] = "healthy"
    elif unhealthy_count < len(components) / 2:
        health["status"] = "degraded"
    else:
        health["status"] = "unhealthy"

    return health

# Dashboard API endpoints
@action.uses(cors)
@action('api/dashboard/overview')
def dashboard_overview():
    """Get dashboard overview data"""

    overview = {
        "system_status": "healthy",
        "total_requests": 0,
        "cache_hit_ratio": 0.0,
        "active_deployments": 0,
        "blocked_ips": 0,
        "healthy_nodes": 0,
        "total_nodes": 0,
        "timestamp": datetime.utcnow().isoformat()
    }

    # Get XDP stats
    xdp_stats = redis_client.hgetall('articdbm:xdp:stats')
    if 'total_packets' in xdp_stats:
        try:
            stats = json.loads(xdp_stats['total_packets'])
            overview["total_requests"] = stats.get('total_packets', 0)
        except:
            pass

    # Get cache stats
    cache_stats = redis_client.hgetall('articdbm:cache:stats')
    if 'hit_ratio' in cache_stats:
        try:
            overview["cache_hit_ratio"] = float(cache_stats['hit_ratio'])
        except:
            pass

    # Get deployment count
    deployments = redis_client.hgetall('articdbm:deployments')
    active_deployments = 0
    for deployment_data in deployments.values():
        try:
            deployment = json.loads(deployment_data)
            if deployment.get('status') == 'active':
                active_deployments += 1
        except:
            pass
    overview["active_deployments"] = active_deployments

    # Get blocked IPs count
    blocked_rules = redis_client.hgetall('articdbm:xdp:rules')
    overview["blocked_ips"] = len(blocked_rules)

    return overview