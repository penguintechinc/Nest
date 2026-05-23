# Threat Intelligence Integration Guide

ArticDBM includes comprehensive threat intelligence capabilities to protect your databases from known malicious actors and attack patterns. This guide covers how to configure and use threat intelligence features, including integration with STIX/TAXII feeds, OpenIOC, and MISP platforms.

## Overview

The threat intelligence system in ArticDBM allows you to:

- Import threat indicators from multiple formats (STIX, TAXII, OpenIOC, MISP)
- Automatically block queries and connections matching threat indicators
- Configure per-database security policies
- Track and analyze threat matches
- Integrate with existing threat intelligence platforms

## Supported Threat Intelligence Formats

### STIX/TAXII

Structured Threat Information Expression (STIX) and Trusted Automated Exchange of Intelligence Information (TAXII) are industry-standard formats for threat intelligence sharing.

**Supported STIX 2.x Objects:**
- Indicators with patterns (IP addresses, domains, URLs, file hashes)
- Malware signatures
- Threat actors
- Attack patterns
- Kill chain phases

**TAXII Feed Configuration:**
```json
{
  "name": "My TAXII Feed",
  "type": "taxii",
  "url": "https://taxii-server.example.com/api/v21/collections/",
  "api_key": "your-api-key",
  "polling_interval": 3600
}
```

### OpenIOC

Open Indicators of Compromise (OpenIOC) is an XML-based format for describing threat indicators.

**Supported OpenIOC Indicators:**
- Network indicators (IPs, domains, URLs)
- File indicators (hashes, paths)
- SQL patterns
- User agents
- Email addresses

**OpenIOC Import Example:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<ioc xmlns="http://openioc.org/schemas/OpenIOC_1.1">
  <metadata>
    <short_description>Malicious IP addresses</short_description>
  </metadata>
  <definition>
    <Indicator>
      <IndicatorItem>
        <Context search="ipv4">
          <Content>192.168.1.100</Content>
        </Context>
      </IndicatorItem>
    </Indicator>
  </definition>
</ioc>
```

### MISP Integration

The Malware Information Sharing Platform (MISP) is an open-source threat intelligence platform that ArticDBM integrates with seamlessly.

**MISP Configuration:**
```json
{
  "name": "MISP Instance",
  "type": "misp",
  "url": "https://misp.example.com/events/view/123",
  "api_key": "your-misp-api-key",
  "polling_interval": 1800
}
```

**Supported MISP Attributes:**
- IP addresses (ip-src, ip-dst)
- Domains and hostnames
- URLs and URIs
- File hashes (MD5, SHA1, SHA256)
- Email addresses
- User agents

## API Endpoints

### Threat Intelligence Feeds

**Create a Feed:**
```bash
curl -X POST http://localhost:8000/api/threat-intel/feeds \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Threat Feed",
    "type": "stix",
    "url": "https://threat-feed.example.com/stix.json",
    "polling_interval": 3600
  }'
```

**List Feeds:**
```bash
curl http://localhost:8000/api/threat-intel/feeds
```

**Poll a Feed:**
```bash
curl -X POST http://localhost:8000/api/threat-intel/feeds/{feed_id}/poll
```

### Threat Indicators

**List Indicators:**
```bash
curl "http://localhost:8000/api/threat-intel/indicators?type=ip&threat_level=high"
```

**Add Manual Indicator:**
```bash
curl -X POST http://localhost:8000/api/threat-intel/indicators \
  -H "Content-Type: application/json" \
  -d '{
    "indicator_type": "ip",
    "value": "10.0.0.1",
    "threat_level": "critical",
    "confidence": 100,
    "description": "Known C&C server"
  }'
```

**Import Threat Intelligence:**
```bash
curl -X POST http://localhost:8000/api/threat-intel/import \
  -H "Content-Type: application/json" \
  -d '{
    "type": "stix",
    "feed_name": "Manual Import",
    "content": "{\"type\": \"bundle\", \"objects\": [...]}"
  }'
```

### Database Security Configuration

**Configure Per-Database Security:**
```bash
curl -X PUT http://localhost:8000/api/databases/{database_id}/security-config \
  -H "Content-Type: application/json" \
  -d '{
    "security_blocks_enabled": true,
    "threat_intel_blocks_enabled": true,
    "sql_injection_detection": true,
    "threat_intel_action": "block"
  }'
```

## How Threat Intelligence Blocking Works

1. **Indicator Import**: Threat indicators are imported from configured feeds or manually added
2. **Redis Sync**: Indicators are synchronized to Redis for fast lookup by the proxy
3. **Query Analysis**: Each database query is checked against:
   - Source IP indicators
   - SQL pattern indicators
   - Username/email indicators
   - Domain/URL mentions in queries
4. **Blocking Decision**: Based on database configuration:
   - `block`: Query is rejected with an error
   - `alert`: Query proceeds but generates an alert
   - `log`: Query proceeds with logging only
5. **Match Recording**: All matches are recorded for analysis

## Integration with MISP

### Setting Up MISP Integration

1. **Install MISP** (if not already installed):
   ```bash
   # Follow MISP installation guide at https://www.misp-project.org/
   ```

2. **Generate MISP API Key**:
   - Log into MISP web interface
   - Go to Event Actions → Automation
   - Generate new API key

3. **Configure ArticDBM to Poll MISP**:
   ```bash
   curl -X POST http://localhost:8000/api/threat-intel/feeds \
     -H "Content-Type: application/json" \
     -d '{
       "name": "MISP Production",
       "type": "misp",
       "url": "https://misp.example.com/events/index",
       "api_key": "your-misp-api-key",
       "polling_interval": 900
     }'
   ```

### MISP Event Correlation

ArticDBM automatically correlates MISP events with database activity:

- Event tags are preserved as indicator tags
- Threat levels are mapped from MISP threat_level_id
- High-confidence indicators (to_ids=true) are prioritized
- Event metadata is included in match reports

### Example MISP Workflow

1. **Security team adds IoCs to MISP**:
   - Malicious IP: 192.168.100.50
   - SQL injection pattern: "UNION SELECT"
   - Threat level: High

2. **ArticDBM polls MISP** (every 15 minutes):
   - Fetches new/updated events
   - Extracts indicators
   - Updates threat intelligence database

3. **Proxy blocks matching queries**:
   - Connection from 192.168.100.50 → Blocked
   - Query containing "UNION SELECT" → Blocked
   - Match details sent back to manager

4. **Analysis and reporting**:
   - View matches in ArticDBM dashboard
   - Export match data back to MISP
   - Generate threat reports

## Indicator Types and Matching

### IP Address Indicators

Matches against client connection source IP:
```json
{
  "indicator_type": "ip",
  "value": "192.168.1.100",
  "threat_level": "high"
}
```

### SQL Pattern Indicators

Matches against query content:
```json
{
  "indicator_type": "sql_pattern",
  "value": "xp_cmdshell",
  "threat_level": "critical",
  "description": "Command execution attempt"
}
```

### Domain/URL Indicators

Matches when found in query strings:
```json
{
  "indicator_type": "domain",
  "value": "malicious.example.com",
  "threat_level": "medium"
}
```

### Hash Indicators

Useful for tracking known malicious file references:
```json
{
  "indicator_type": "hash",
  "value": "5d41402abc4b2a76b9719d911017c592",
  "threat_level": "high"
}
```

## Best Practices

### Feed Management

1. **Prioritize High-Fidelity Feeds**: Use feeds with low false positive rates
2. **Regular Updates**: Configure appropriate polling intervals (15-60 minutes)
3. **Feed Validation**: Test feeds in alert mode before enabling blocking
4. **Indicator Expiration**: Set expiration times for time-sensitive indicators

### Performance Optimization

1. **Redis Caching**: Indicators are cached for 5 minutes by default
2. **Bulk Imports**: Use batch import for large indicator sets
3. **Indicator Limits**: Consider limiting active indicators to < 100,000
4. **Pattern Complexity**: Avoid overly complex regex patterns

### Security Considerations

1. **Feed Authentication**: Always use API keys or authentication for feeds
2. **TLS/HTTPS**: Ensure feeds are accessed over encrypted connections
3. **Indicator Validation**: Validate indicator format before import
4. **Whitelist Critical Resources**: Maintain whitelist for legitimate resources

## Monitoring and Metrics

### Threat Intelligence Metrics

Monitor threat intelligence effectiveness through:

- **Match Rate**: Number of queries matching indicators
- **False Positive Rate**: Legitimate queries incorrectly blocked
- **Indicator Coverage**: Percentage of known threats covered
- **Feed Freshness**: Time since last successful poll

### Match Analysis

View threat intelligence matches:
```bash
curl http://localhost:8000/api/threat-intel/matches
```

Response includes:
- Indicator type and value
- Threat level
- Database and user information
- Source IP
- Query snippet
- Action taken
- Timestamp

## Troubleshooting

### Common Issues

**Feeds Not Updating:**
- Check feed URL accessibility
- Verify API keys/credentials
- Review proxy logs for connection errors
- Ensure Redis is running and accessible

**High False Positive Rate:**
- Review indicator confidence levels
- Adjust threat level thresholds
- Implement whitelisting for known-good sources
- Consider using alert mode instead of blocking

**Performance Issues:**
- Reduce number of active indicators
- Optimize pattern matching rules
- Increase Redis cache timeout
- Use dedicated Redis instance for threat intel

### Debug Commands

**Check Redis Sync:**
```bash
redis-cli get articdbm:threat_indicators
```

**View Feed Status:**
```bash
curl http://localhost:8000/api/threat-intel/feeds
```

**Test Indicator Matching:**
```bash
# Create test indicator
curl -X POST http://localhost:8000/api/threat-intel/indicators \
  -d '{"indicator_type": "ip", "value": "127.0.0.1", "threat_level": "low"}'

# Test connection from that IP
mysql -h localhost -P 3306 -u testuser
```

## Integration Examples

### Python Script for MISP Integration

```python
import requests
import json

class MISPIntegration:
    def __init__(self, articdbm_url, misp_url, misp_key):
        self.articdbm_url = articdbm_url
        self.misp_url = misp_url
        self.misp_key = misp_key
    
    def sync_misp_event(self, event_id):
        # Fetch event from MISP
        headers = {'Authorization': self.misp_key}
        response = requests.get(
            f"{self.misp_url}/events/view/{event_id}",
            headers=headers
        )
        event_data = response.json()
        
        # Import to ArticDBM
        import_response = requests.post(
            f"{self.articdbm_url}/api/threat-intel/import",
            json={
                'type': 'misp',
                'feed_name': f"MISP Event {event_id}",
                'content': json.dumps(event_data)
            }
        )
        
        return import_response.json()

# Usage
integration = MISPIntegration(
    'http://localhost:8000',
    'https://misp.example.com',
    'your-api-key'
)
integration.sync_misp_event(123)
```

### Automated Feed Polling Script

```bash
#!/bin/bash
# Automated threat feed polling

ARTICDBM_API="http://localhost:8000/api"

# Poll all active feeds
FEEDS=$(curl -s "$ARTICDBM_API/threat-intel/feeds" | jq -r '.feeds[].id')

for FEED_ID in $FEEDS; do
    echo "Polling feed $FEED_ID..."
    curl -X POST "$ARTICDBM_API/threat-intel/feeds/$FEED_ID/poll"
    sleep 5
done

# Check for matches
MATCHES=$(curl -s "$ARTICDBM_API/threat-intel/matches?limit=10")
echo "Recent matches: $MATCHES"
```

## Conclusion

ArticDBM's threat intelligence integration provides comprehensive protection against known threats while maintaining flexibility for different security requirements. By leveraging industry-standard formats and platforms like MISP, organizations can build a robust defense against database attacks while benefiting from community threat intelligence sharing.

For additional support or feature requests, please refer to the ArticDBM documentation or contact support.