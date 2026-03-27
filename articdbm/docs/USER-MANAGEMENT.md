# Enhanced User Management Guide

ArticDBM provides comprehensive user management capabilities designed for enterprise environments and Managed Service Providers (MSPs). This guide covers the enhanced user management features including API keys, temporary access, rate limiting, and advanced security controls.

## Overview

The enhanced user management system in ArticDBM allows administrators to:

- Create users with sophisticated security profiles
- Generate and manage API keys for programmatic access
- Set up temporary access with automatic expiration
- Implement rate limiting and IP whitelisting
- Track usage and monitor security events
- Manage per-database permissions with time limits

## User Types

### Standard Users
Regular user accounts with username/password authentication and basic database permissions.

### Enhanced Users
Users with advanced security profiles including:
- API key authentication
- TLS requirements
- IP whitelisting
- Rate limiting
- Account expiration
- Usage tracking

### Temporary Users
Short-lived accounts designed for:
- Contractor access
- Auditing purposes
- Emergency access
- Time-limited projects

## API Authentication Methods

### Username/Password Authentication
Traditional authentication using hashed passwords:

```bash
# Connect using username/password
mysql -h articdbm-proxy -u myuser -p mydatabase
```

### API Key Authentication
Secure token-based authentication for applications:

```bash
# Connect using API key as password
mysql -h articdbm-proxy -u myuser -p{API_KEY} mydatabase

# Or set as environment variable
export MYSQL_PWD="Xj3kL9mP4qR8sV2tA7cF5hN8zB4eG6iK1oY3uW9xQ2s"
mysql -h articdbm-proxy -u myuser mydatabase
```

### Temporary Token Access
One-time or limited-use tokens for specific operations:

```bash
# Using temporary token
mysql -h articdbm-proxy -u tmp_token -p{TEMP_TOKEN} mydatabase
```

## Management Portal API

### Create Enhanced User

Create a user with full security profile:

```bash
curl -X POST http://localhost:8000/api/users/enhanced \
  -H "Content-Type: application/json" \
  -d '{
    "username": "enterprise_user",
    "email": "user@company.com",
    "password": "secure_password",
    "first_name": "Enterprise",
    "last_name": "User",
    "require_tls": true,
    "allowed_ips": ["10.0.0.0/8", "203.0.113.100"],
    "rate_limit": 1000,
    "expires_at": "2024-12-31T23:59:59Z",
    "is_temporary": false,
    "permissions": [
      {
        "database_name": "production_db",
        "table_name": "*",
        "actions": ["read", "write"],
        "expires_at": "2024-06-30T23:59:59Z",
        "max_queries": 10000
      }
    ]
  }'
```

### List Enhanced Users

Get all users with their profiles and permissions:

```bash
curl http://localhost:8000/api/users/enhanced
```

Response:
```json
{
  "users": [
    {
      "id": 1,
      "username": "enterprise_user",
      "email": "user@company.com",
      "enabled": true,
      "profile": {
        "api_key": "Xj3kL9mP4qR8sV2tA7cF5hN8zB4eG6iK1oY3uW9xQ2s",
        "require_tls": true,
        "allowed_ips": ["10.0.0.0/8", "203.0.113.100"],
        "rate_limit": 1000,
        "expires_at": "2024-12-31T23:59:59Z",
        "usage_count": 1547
      },
      "permissions": [
        {
          "database_name": "production_db",
          "actions": ["read", "write"],
          "max_queries": 10000,
          "query_count": 8432
        }
      ]
    }
  ]
}
```

### Regenerate API Key

Generate a new API key for a user:

```bash
curl -X POST http://localhost:8000/api/users/1/regenerate-api-key
```

### Update Rate Limits

Modify user rate limits dynamically:

```bash
curl -X PUT http://localhost:8000/api/users/1/rate-limit \
  -H "Content-Type: application/json" \
  -d '{"rate_limit": 2000}'
```

## Temporary Access Management

### Create Temporary Access Token

Generate one-time access for specific database operations:

```bash
curl -X POST http://localhost:8000/api/temporary-access \
  -H "Content-Type: application/json" \
  -d '{
    "database_name": "audit_db",
    "table_name": "logs",
    "actions": ["read"],
    "expires_at": "2024-01-15T18:00:00Z",
    "max_uses": 5,
    "client_ip": "192.168.1.100"
  }'
```

Response:
```json
{
  "token": "tmp_Xj3kL9mP4qR8sV2tA7cF5hN8zB4e",
  "id": 123,
  "expires_at": "2024-01-15T18:00:00Z",
  "message": "Temporary access token created successfully"
}
```

### List Temporary Tokens

View all active temporary access tokens:

```bash
curl http://localhost:8000/api/temporary-access
```

### Revoke Temporary Access

Immediately revoke a temporary token:

```bash
curl -X POST http://localhost:8000/api/temporary-access/123/revoke
```

## Security Features

### IP Whitelisting

Restrict user access to specific IP addresses or CIDR ranges:

```json
{
  "allowed_ips": [
    "192.168.1.100",           // Specific IP
    "10.0.0.0/8",             // Corporate network
    "203.0.113.0/24"          // Office subnet
  ]
}
```

### TLS Enforcement

Force encrypted connections for sensitive users:

```json
{
  "require_tls": true
}
```

When enabled, connections without TLS will be rejected:
```
ERROR 1045 (28000): Access denied - TLS required
```

### Rate Limiting

Control query rates per user:

```json
{
  "rate_limit": 1000  // 1000 requests per second max
}
```

Rate limiting is enforced at the proxy level with Redis-based counters.

### Account Expiration

Set automatic account expiration:

```json
{
  "expires_at": "2024-12-31T23:59:59Z"
}
```

Expired accounts are automatically blocked with audit logging.

### Query Quotas

Limit queries per database per time period:

```json
{
  "max_queries": 10000,  // 10k queries per hour
  "query_count": 8432    // Current usage
}
```

## Permission Management

### Database-Level Permissions

Grant access to specific databases:

```json
{
  "database_name": "production_db",
  "table_name": "*",
  "actions": ["read", "write"]
}
```

### Table-Level Permissions

Restrict access to specific tables:

```json
{
  "database_name": "production_db",  
  "table_name": "sensitive_data",
  "actions": ["read"]
}
```

### Time-Limited Permissions

Set permission expiration dates:

```json
{
  "database_name": "project_db",
  "expires_at": "2024-06-30T23:59:59Z"
}
```

### Action Types

Available permission actions:
- `read`: SELECT, SHOW, DESCRIBE, EXPLAIN
- `write`: INSERT, UPDATE, DELETE
- `admin`: CREATE, DROP, ALTER, GRANT

## MSP Use Cases

### Multi-Tenant Customer Management

MSPs can create isolated environments for each customer:

1. **Customer Onboarding**:
   ```bash
   # Create customer user with dedicated database access
   curl -X POST /api/users/enhanced -d '{
     "username": "customer_acme",
     "rate_limit": 500,
     "allowed_ips": ["203.0.113.0/24"],
     "permissions": [{"database_name": "acme_prod", "actions": ["read", "write"]}]
   }'
   ```

2. **Contractor Access**:
   ```bash
   # Create temporary access for contractor
   curl -X POST /api/temporary-access -d '{
     "database_name": "acme_prod", 
     "actions": ["read"],
     "expires_at": "2024-02-01T00:00:00Z",
     "max_uses": 50
   }'
   ```

3. **Usage Monitoring**:
   ```bash
   # Check customer usage
   curl /api/users/enhanced | jq '.users[] | select(.username=="customer_acme") | .profile.usage_count'
   ```

### Compliance and Security

For regulated industries requiring strict access controls:

- **IP Restrictions**: Limit access to corporate networks
- **TLS Enforcement**: Ensure encrypted connections
- **Audit Trails**: Complete logging of all access attempts
- **Time Limits**: Automatic access expiration
- **Rate Limiting**: Prevent abuse and ensure fair usage

## Monitoring and Alerts

### Usage Tracking

Monitor user activity:
- Connection attempts
- Query counts
- Rate limit violations
- Permission denials
- API key usage

### Security Events

Track security-related events:
- Failed authentication attempts
- IP whitelist violations
- TLS requirement violations
- Expired account access attempts
- Suspicious query patterns

## Best Practices

### User Management

1. **Use API Keys for Applications**: Never embed passwords in application code
2. **Regular Key Rotation**: Regenerate API keys periodically
3. **Principle of Least Privilege**: Grant minimal required permissions
4. **Time-Limited Access**: Set expiration dates for all accounts
5. **IP Whitelisting**: Restrict access to known networks

### Security

1. **Enable TLS**: Force encryption for sensitive data
2. **Monitor Usage**: Regular review of user activity
3. **Rate Limiting**: Prevent abuse and ensure performance
4. **Audit Logging**: Maintain comprehensive access logs
5. **Regular Cleanup**: Remove expired tokens and unused accounts

### MSP Operations

1. **Customer Isolation**: Separate databases and users per customer
2. **Automated Provisioning**: Script user creation and permissions
3. **Usage Reporting**: Track usage for billing purposes
4. **Security Policies**: Standardize security settings across customers
5. **Incident Response**: Prepared procedures for security events

## Troubleshooting

### Common Issues

**User Cannot Connect:**
- Check if account is enabled and not expired
- Verify IP address is in whitelist
- Confirm TLS requirements are met
- Check rate limit status

**API Key Authentication Fails:**
- Verify API key is active and not expired
- Check if user account is enabled
- Confirm database permissions
- Verify rate limit compliance

**Temporary Access Not Working:**
- Check token expiration date
- Verify max uses not exceeded
- Confirm database permissions
- Check if token has been revoked

### Debug Commands

Check user status:
```bash
curl /api/users/enhanced | jq '.users[] | select(.username=="problematic_user")'
```

List active temporary tokens:
```bash
curl /api/temporary-access
```

Check Redis user sync:
```bash
redis-cli get articdbm:manager:users | jq .
```

## Conclusion

ArticDBM's enhanced user management system provides enterprise-grade security and flexibility for both direct database access and MSP service offerings. The combination of API keys, temporary access, rate limiting, and comprehensive security controls makes it ideal for production environments requiring strict access control and compliance.

For additional support or feature requests, please refer to the ArticDBM documentation or contact support.