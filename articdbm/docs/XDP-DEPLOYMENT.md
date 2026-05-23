# XDP-Accelerated ArticDBM Proxy Deployment Guide

This guide covers the deployment and configuration of ArticDBM proxy with XDP (eXpress Data Path) acceleration for maximum performance and security.

## Overview

The XDP-accelerated ArticDBM proxy provides:

- **100M+ packets/second processing** at kernel level
- **Sub-microsecond IP blocking** with XDP programs
- **Zero-copy networking** with AF_XDP sockets
- **Multi-tier caching** (XDP L1, Redis L2, Backend L3)
- **NUMA-aware architecture** for optimal memory locality
- **Blue/green deployments** with XDP traffic splitting
- **Advanced rate limiting** with token bucket algorithm
- **Multi-write synchronization** for cluster scenarios

## System Requirements

### Hardware Requirements

- **CPU:** Modern x86_64 with at least 8 cores
- **Memory:** Minimum 16GB RAM (32GB+ recommended)
- **Network:** 10Gbps+ NICs with XDP support
- **Storage:** NVMe SSD for optimal cache performance

### Software Requirements

- **Kernel:** Linux 5.4+ with eBPF support
- **Distribution:** Ubuntu 20.04+, RHEL 8+, or equivalent
- **Dependencies:**
  - clang 10+
  - llvm-objcopy
  - libbpf-dev
  - linux-headers-generic

### Network Interface Support

Verify XDP support on your network interface:

```bash
# Check XDP driver support
ethtool -i eth0 | grep driver

# Verify XDP capabilities
ip link show eth0 | grep -i xdp

# Test XDP program loading
sudo ip link set dev eth0 xdp obj test.o sec prog
```

## Installation

### 1. Install System Dependencies

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install -y build-essential clang llvm libbpf-dev \
    linux-headers-generic ethtool iproute2 redis-server
```

**RHEL/CentOS:**
```bash
sudo yum groupinstall -y "Development Tools"
sudo yum install -y clang llvm libbpf-devel kernel-headers \
    ethtool iproute redis
```

### 2. Mount BPF Filesystem

```bash
sudo mkdir -p /sys/fs/bpf
sudo mount -t bpf bpf /sys/fs/bpf
echo 'bpf /sys/fs/bpf bpf defaults 0 0' | sudo tee -a /etc/fstab
```

### 3. Build XDP Programs

```bash
cd services/proxy/
make build-xdp
```

### 4. Configure System Limits

```bash
# Increase memory limits for eBPF
echo 'kernel.bpf_stats_enabled = 1' | sudo tee -a /etc/sysctl.conf
echo 'vm.max_map_count = 2147483647' | sudo tee -a /etc/sysctl.conf
sudo sysctl -p

# Configure huge pages for AF_XDP
echo 1024 | sudo tee /proc/sys/vm/nr_hugepages
```

## Configuration

### Environment Variables

```bash
# Core XDP Configuration
export XDP_ENABLED=true
export XDP_INTERFACE=eth0
export XDP_RATE_LIMIT_PPS=100000000
export XDP_BURST_LIMIT=10000

# AF_XDP Socket Configuration
export AFXDP_ENABLED=true
export AFXDP_BATCH_SIZE=64

# Cache Configuration
export XDP_CACHE_SIZE=1048576    # 1MB L1 cache
export XDP_CACHE_TTL=300         # 5 minutes

# NUMA Configuration (auto-detected)
export NUMA_OPTIMIZE=true
```

### XDP Program Configuration

The proxy automatically loads the following XDP programs:

1. **IP Blocklist Filter** (`ip_blocklist.o`)
2. **Rate Limiter** (`rate_limiter.o`)
3. **Query Cache** (`query_cache.o`)
4. **Traffic Splitter** (`traffic_splitter.o`)

### Redis Configuration for XDP

```bash
# XDP rule management
redis-cli CONFIG SET maxmemory 2gb
redis-cli CONFIG SET maxmemory-policy allkeys-lru

# Enable keyspace notifications
redis-cli CONFIG SET notify-keyspace-events KEA
```

## Deployment Patterns

### Single Instance Deployment

```yaml
version: '3.8'
services:
  articdbm-proxy:
    image: articdbm/proxy:xdp-latest
    privileged: true  # Required for XDP
    network_mode: host
    cap_add:
      - SYS_ADMIN
      - NET_ADMIN
    environment:
      - XDP_ENABLED=true
      - XDP_INTERFACE=eth0
      - REDIS_ADDR=localhost:6379
    volumes:
      - /sys/fs/bpf:/sys/fs/bpf:rw
```

### Cluster Deployment with Kubernetes

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: articdbm-xdp-proxy
spec:
  selector:
    matchLabels:
      app: articdbm-xdp-proxy
  template:
    metadata:
      labels:
        app: articdbm-xdp-proxy
    spec:
      hostNetwork: true
      containers:
      - name: proxy
        image: articdbm/proxy:xdp-latest
        securityContext:
          privileged: true
        env:
        - name: XDP_ENABLED
          value: "true"
        - name: XDP_INTERFACE
          value: "eth0"
        - name: XDP_RATE_LIMIT_PPS
          value: "100000000"
        volumeMounts:
        - name: bpf-maps
          mountPath: /sys/fs/bpf
        - name: proc
          mountPath: /host/proc
          readOnly: true
        - name: sys
          mountPath: /host/sys
          readOnly: true
      volumes:
      - name: bpf-maps
        hostPath:
          path: /sys/fs/bpf
      - name: proc
        hostPath:
          path: /proc
      - name: sys
        hostPath:
          path: /sys
```

### Production Load Balancer Setup

```bash
# Configure multiple proxy instances with XDP
for i in {1..4}; do
    docker run -d --name articdbm-xdp-$i \
        --privileged --network host \
        -e XDP_ENABLED=true \
        -e XDP_INTERFACE=eth$((i-1)) \
        -e REDIS_ADDR=redis-cluster.local:6379 \
        -v /sys/fs/bpf:/sys/fs/bpf:rw \
        articdbm/proxy:xdp-latest
done
```

## Performance Tuning

### NUMA Optimization

```bash
# Check NUMA topology
numactl --hardware

# Pin proxy processes to NUMA nodes
numactl --cpunodebind=0 --membind=0 ./articdbm-proxy &
numactl --cpunodebind=1 --membind=1 ./articdbm-proxy &
```

### XDP Queue Configuration

```bash
# Configure RSS queues to match CPU cores
ethtool -L eth0 combined 16

# Set XDP program per queue
for i in {0..15}; do
    sudo tc filter add dev eth0 ingress protocol ip \
        flower skip_sw action bpf obj xdp_program.o sec prog
done
```

### Cache Optimization

```bash
# Optimize cache sizes based on workload
export XDP_CACHE_SIZE=$(($(nproc) * 256 * 1024))  # 256KB per core
export XDP_CACHE_TTL=600  # 10 minutes for high-traffic
```

## Monitoring and Observability

### XDP Program Statistics

```bash
# View XDP program statistics
sudo bpftool prog show
sudo bpftool map dump name xdp_stats_map

# Monitor XDP performance
watch -n1 'sudo bpftool prog show | grep xdp'
```

### Performance Metrics

The proxy exposes XDP metrics via Prometheus:

- `articdbm_xdp_packets_processed_total`
- `articdbm_xdp_packets_dropped_total`
- `articdbm_xdp_cache_hits_total`
- `articdbm_xdp_cache_misses_total`
- `articdbm_afxdp_packets_rx_total`
- `articdbm_afxdp_packets_tx_total`

### Grafana Dashboard

```json
{
  "dashboard": {
    "title": "ArticDBM XDP Performance",
    "panels": [
      {
        "title": "XDP Packet Processing Rate",
        "targets": [
          {
            "expr": "rate(articdbm_xdp_packets_processed_total[5m])"
          }
        ]
      },
      {
        "title": "Cache Hit Ratio",
        "targets": [
          {
            "expr": "articdbm_xdp_cache_hits_total / (articdbm_xdp_cache_hits_total + articdbm_xdp_cache_misses_total)"
          }
        ]
      }
    ]
  }
}
```

## Security Configuration

### IP Blocking Rules

```bash
# Add IP blocking rule
redis-cli HSET articdbm:xdp:rules ip:192.168.1.100 \
    '{"ip_address":"192.168.1.100","reason":"security_threat","blocked_at":"2024-01-01T00:00:00Z"}'

# Add CIDR block
redis-cli HSET articdbm:xdp:rules cidr:10.0.0.0/8 \
    '{"network":"10.0.0.0/8","reason":"internal_block","blocked_at":"2024-01-01T00:00:00Z"}'

# Publish rule update
redis-cli PUBLISH articdbm:xdp:rule_update \
    '{"action":"add_rule","rule":{"ip_address":"192.168.1.100"}}'
```

### Rate Limiting Configuration

```bash
# Configure rate limits per IP
redis-cli HSET articdbm:xdp:rate_limits ip:192.168.1.0/24 \
    '{"rate_pps":1000,"burst_limit":100,"window_seconds":60}'

# Global rate limiting
export XDP_RATE_LIMIT_PPS=50000000  # 50M PPS global limit
export XDP_BURST_LIMIT=10000        # 10K packet burst
```

## Troubleshooting

### Common Issues

**XDP Program Load Failure:**
```bash
# Check kernel version and eBPF support
uname -r
cat /boot/config-$(uname -r) | grep BPF

# Verify interface XDP support
sudo ethtool -k eth0 | grep xdp
```

**Performance Issues:**
```bash
# Check CPU affinity
cat /proc/interrupts | grep eth0

# Monitor XDP statistics
sudo bpftool prog tracelog

# Check NUMA memory allocation
numastat -p $(pgrep articdbm-proxy)
```

**Cache Performance:**
```bash
# Monitor cache statistics
redis-cli HGETALL articdbm:cache:stats

# Check cache memory usage
redis-cli INFO memory
```

### Debug Commands

```bash
# Enable XDP debug logging
export XDP_DEBUG=1

# Trace XDP program execution
sudo bpftool prog tracelog

# Monitor network traffic
sudo tcpdump -i eth0 -c 100

# Check eBPF verifier logs
dmesg | grep bpf
```

## Best Practices

### Production Deployment

1. **Use dedicated network interfaces** for XDP programs
2. **Configure appropriate rate limits** based on expected traffic
3. **Monitor cache hit ratios** and adjust TTL settings
4. **Implement proper backup strategies** for XDP rule configurations
5. **Use NUMA-aware deployment** for multi-socket systems

### Security Hardening

1. **Regularly update IP blocking rules** from threat intelligence feeds
2. **Monitor for unusual traffic patterns** and adjust rate limits
3. **Implement proper access controls** for XDP rule management
4. **Use encrypted connections** for rule synchronization

### Performance Optimization

1. **Tune XDP batch sizes** based on network card capabilities
2. **Configure appropriate cache sizes** for your workload
3. **Use CPU isolation** for XDP processing cores
4. **Optimize memory allocation** with huge pages

## Scaling Guidelines

### Horizontal Scaling

- Deploy multiple proxy instances with XDP on different interfaces
- Use consistent hashing for traffic distribution
- Implement shared rule synchronization via Redis cluster

### Vertical Scaling

- Increase XDP cache sizes proportionally to memory
- Add more processing queues for high-traffic interfaces
- Optimize NUMA node assignments for worker threads

### Traffic Patterns

- **High-frequency, small queries:** Optimize L1 cache hit ratio
- **Large result sets:** Use L2/L3 cache tiers effectively
- **Mixed workloads:** Configure adaptive cache policies

## Support and Maintenance

### Regular Maintenance Tasks

1. **Monitor XDP program performance** and update as needed
2. **Clean up expired cache entries** to maintain performance
3. **Update IP blocking rules** from threat intelligence feeds
4. **Review and optimize** rate limiting policies
5. **Backup XDP rule configurations** regularly

### Upgrade Process

1. **Test new XDP programs** in staging environment
2. **Gradually roll out** to production instances
3. **Monitor performance metrics** during upgrade
4. **Keep rollback procedures** ready

This deployment guide ensures optimal performance and security for your XDP-accelerated ArticDBM proxy deployment.