#!/bin/bash
# Ceph Cluster Validation and Health Check Script
# Validates Ceph cluster health and configuration
#
# Version: 1.0.0
# Maintained by: Penguin Tech Inc
# License: Limited AGPL3
#
# Usage:
#   ./validate-ceph-cluster.sh [OPTIONS]
#
# Options:
#   -c, --container NAME      LXD container name (default: ceph-node-01)
#   -d, --detailed            Show detailed information
#   -j, --json                Output in JSON format
#   -h, --help                Show this help message

set -euo pipefail

# Default configuration
CONTAINER_NAME="ceph-node-01"
DETAILED=false
JSON_OUTPUT=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    if [ "$JSON_OUTPUT" = false ]; then
        echo -e "${BLUE}[INFO]${NC} $*"
    fi
}

log_success() {
    if [ "$JSON_OUTPUT" = false ]; then
        echo -e "${GREEN}[✓]${NC} $*"
    fi
}

log_warning() {
    if [ "$JSON_OUTPUT" = false ]; then
        echo -e "${YELLOW}[⚠]${NC} $*"
    fi
}

log_error() {
    if [ "$JSON_OUTPUT" = false ]; then
        echo -e "${RED}[✗]${NC} $*"
    fi
}

log_section() {
    if [ "$JSON_OUTPUT" = false ]; then
        echo ""
        echo -e "${CYAN}========================================${NC}"
        echo -e "${CYAN}$*${NC}"
        echo -e "${CYAN}========================================${NC}"
    fi
}

# Help message
show_help() {
    cat << EOF
Ceph Cluster Validation Script

Usage: $0 [OPTIONS]

Options:
  -c, --container NAME      LXD container name (default: ceph-node-01)
  -d, --detailed            Show detailed information
  -j, --json                Output in JSON format
  -h, --help                Show this help message

Examples:
  # Basic validation
  $0

  # Detailed validation
  $0 -d

  # Validate specific container
  $0 -c ceph-node-02

  # JSON output for automation
  $0 -j

EOF
    exit 0
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -c|--container)
                CONTAINER_NAME="$2"
                shift 2
                ;;
            -d|--detailed)
                DETAILED=true
                shift
                ;;
            -j|--json)
                JSON_OUTPUT=true
                shift
                ;;
            -h|--help)
                show_help
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                ;;
        esac
    done
}

# Execute command in container
exec_in_container() {
    local cmd="$*"
    lxc exec "$CONTAINER_NAME" -- bash -c "$cmd"
}

# Check if container exists and is running
check_container() {
    if ! lxc list | grep -q "$CONTAINER_NAME"; then
        log_error "Container '$CONTAINER_NAME' not found"
        exit 1
    fi

    if ! lxc list "$CONTAINER_NAME" -c s -f csv | grep -q "RUNNING"; then
        log_error "Container '$CONTAINER_NAME' is not running"
        exit 1
    fi

    log_success "Container '$CONTAINER_NAME' is running"
}

# Check Ceph installation
check_ceph_installation() {
    log_section "Checking Ceph Installation"

    if exec_in_container "command -v ceph &>/dev/null"; then
        local version
        version=$(exec_in_container "ceph --version")
        log_success "Ceph is installed: $version"
    else
        log_error "Ceph is not installed"
        return 1
    fi
}

# Check cluster health
check_cluster_health() {
    log_section "Checking Cluster Health"

    local health_status
    health_status=$(exec_in_container "ceph health" 2>/dev/null || echo "UNKNOWN")

    case "$health_status" in
        HEALTH_OK)
            log_success "Cluster health: $health_status"
            ;;
        HEALTH_WARN*)
            log_warning "Cluster health: $health_status"
            if [ "$DETAILED" = true ]; then
                exec_in_container "ceph health detail"
            fi
            ;;
        *)
            log_error "Cluster health: $health_status"
            if [ "$DETAILED" = true ]; then
                exec_in_container "ceph health detail" || true
            fi
            return 1
            ;;
    esac
}

# Check cluster status
check_cluster_status() {
    log_section "Cluster Status"

    if [ "$DETAILED" = true ]; then
        exec_in_container "ceph -s"
    else
        exec_in_container "ceph -s" | head -20
    fi
}

# Check MON status
check_mon_status() {
    log_section "Monitor (MON) Status"

    local mon_count
    mon_count=$(exec_in_container "ceph mon stat" | grep -oP '\d+(?= mons)' || echo "0")

    if [ "$mon_count" -gt 0 ]; then
        log_success "Monitors running: $mon_count"
        if [ "$DETAILED" = true ]; then
            exec_in_container "ceph mon dump"
        fi
    else
        log_error "No monitors found"
        return 1
    fi
}

# Check MGR status
check_mgr_status() {
    log_section "Manager (MGR) Status"

    if exec_in_container "ceph mgr stat" &>/dev/null; then
        log_success "Manager is active"
        if [ "$DETAILED" = true ]; then
            exec_in_container "ceph mgr dump"
        fi
    else
        log_error "Manager is not active"
        return 1
    fi
}

# Check OSD status
check_osd_status() {
    log_section "OSD Status"

    local osd_stat
    osd_stat=$(exec_in_container "ceph osd stat" 2>/dev/null || echo "")

    if [ -n "$osd_stat" ]; then
        log_success "OSD Status: $osd_stat"
        if [ "$DETAILED" = true ]; then
            exec_in_container "ceph osd tree"
            echo ""
            exec_in_container "ceph osd df"
        fi
    else
        log_error "Failed to get OSD status"
        return 1
    fi
}

# Check pools
check_pools() {
    log_section "Storage Pools"

    local pool_count
    pool_count=$(exec_in_container "ceph osd pool ls" | wc -l)

    if [ "$pool_count" -gt 0 ]; then
        log_success "Pools configured: $pool_count"
        exec_in_container "ceph osd pool ls"

        if [ "$DETAILED" = true ]; then
            echo ""
            exec_in_container "ceph osd pool ls detail"
        fi
    else
        log_warning "No pools configured"
    fi
}

# Check CephFS
check_cephfs() {
    log_section "CephFS Status"

    if exec_in_container "ceph fs ls" &>/dev/null; then
        local fs_count
        fs_count=$(exec_in_container "ceph fs ls" | wc -l)

        if [ "$fs_count" -gt 0 ]; then
            log_success "CephFS configured: $fs_count filesystem(s)"
            exec_in_container "ceph fs ls"

            if [ "$DETAILED" = true ]; then
                echo ""
                exec_in_container "ceph mds stat"
                echo ""
                exec_in_container "ceph fs status"
            fi
        else
            log_warning "No CephFS configured"
        fi
    else
        log_warning "CephFS not available"
    fi
}

# Check RBD
check_rbd() {
    log_section "RBD (Block Storage) Status"

    local rbd_pools
    rbd_pools=$(exec_in_container "ceph osd pool ls" | grep -E "^rbd|^iscsi" || echo "")

    if [ -n "$rbd_pools" ]; then
        log_success "RBD pools found"
        echo "$rbd_pools" | while read -r pool; do
            local image_count
            image_count=$(exec_in_container "rbd ls $pool 2>/dev/null" | wc -l)
            log_info "Pool '$pool': $image_count image(s)"

            if [ "$DETAILED" = true ] && [ "$image_count" -gt 0 ]; then
                exec_in_container "rbd ls -l $pool"
            fi
        done
    else
        log_warning "No RBD pools configured"
    fi
}

# Check iSCSI
check_iscsi() {
    log_section "iSCSI Gateway Status"

    if exec_in_container "systemctl is-active rbd-target-api" &>/dev/null; then
        log_success "iSCSI gateway (rbd-target-api) is running"

        if [ "$DETAILED" = true ]; then
            exec_in_container "systemctl status rbd-target-api" || true
        fi
    else
        log_warning "iSCSI gateway is not running"
    fi

    # Check for iSCSI pool
    if exec_in_container "ceph osd pool ls" | grep -q "iscsi"; then
        log_success "iSCSI pool exists"
    else
        log_warning "iSCSI pool not found"
    fi
}

# Check RGW (S3)
check_rgw() {
    log_section "RGW (S3) Status"

    # Check if RGW service is running
    if exec_in_container "ceph orch ps --daemon_type rgw" &>/dev/null; then
        log_success "RGW service is deployed"

        # Check RGW users
        local user_count
        user_count=$(exec_in_container "radosgw-admin user list 2>/dev/null" | grep -c '"' || echo "0")

        if [ "$user_count" -gt 0 ]; then
            log_success "RGW users configured: $((user_count / 2))"

            if [ "$DETAILED" = true ]; then
                exec_in_container "radosgw-admin user list"
            fi
        else
            log_warning "No RGW users configured"
        fi

        # Test RGW endpoint
        local container_ip
        container_ip=$(lxc list "$CONTAINER_NAME" -c 4 -f csv | awk '{print $1}' | head -n1)
        log_info "RGW endpoint: http://$container_ip:8080"
    else
        log_warning "RGW service is not deployed"
    fi
}

# Check dashboard
check_dashboard() {
    log_section "Ceph Dashboard"

    if exec_in_container "ceph mgr module ls" | grep -q '"dashboard"'; then
        log_success "Dashboard module is enabled"

        local container_ip
        container_ip=$(lxc list "$CONTAINER_NAME" -c 4 -f csv | awk '{print $1}' | head -n1)
        log_info "Dashboard URL: https://$container_ip:8443"
        log_info "Default credentials: admin / admin"

        if [ "$DETAILED" = true ]; then
            exec_in_container "ceph mgr services" || true
        fi
    else
        log_warning "Dashboard module is not enabled"
    fi
}

# Check system resources
check_resources() {
    log_section "System Resources"

    # CPU
    local cpu_count
    cpu_count=$(exec_in_container "nproc")
    log_info "CPU cores: $cpu_count"

    # Memory
    local mem_total
    mem_total=$(exec_in_container "free -h | grep Mem | awk '{print \$2}'")
    log_info "Total memory: $mem_total"

    # Disk
    if [ "$DETAILED" = true ]; then
        echo ""
        exec_in_container "df -h | grep -E '^/dev|Filesystem'"
    fi
}

# Generate JSON output
generate_json_output() {
    local health_status
    health_status=$(exec_in_container "ceph health" 2>/dev/null || echo "UNKNOWN")

    local osd_count
    osd_count=$(exec_in_container "ceph osd stat" 2>/dev/null | grep -oP '\d+(?= osds)' || echo "0")

    local mon_count
    mon_count=$(exec_in_container "ceph mon stat" 2>/dev/null | grep -oP '\d+(?= mons)' || echo "0")

    local pool_count
    pool_count=$(exec_in_container "ceph osd pool ls" 2>/dev/null | wc -l)

    local container_ip
    container_ip=$(lxc list "$CONTAINER_NAME" -c 4 -f csv | awk '{print $1}' | head -n1)

    cat << EOF
{
  "container": "$CONTAINER_NAME",
  "ip_address": "$container_ip",
  "health_status": "$health_status",
  "monitors": $mon_count,
  "osds": $osd_count,
  "pools": $pool_count,
  "endpoints": {
    "dashboard": "https://$container_ip:8443",
    "s3": "http://$container_ip:8080"
  },
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
}

# Main validation function
main() {
    # Parse arguments
    parse_args "$@"

    if [ "$JSON_OUTPUT" = false ]; then
        echo ""
        echo "=========================================="
        echo "Ceph Cluster Validation"
        echo "Container: $CONTAINER_NAME"
        echo "=========================================="
        echo ""
    fi

    # Run checks
    check_container
    check_ceph_installation
    check_cluster_health
    check_cluster_status
    check_mon_status
    check_mgr_status
    check_osd_status
    check_pools
    check_cephfs
    check_rbd
    check_iscsi
    check_rgw
    check_dashboard
    check_resources

    # Generate output
    if [ "$JSON_OUTPUT" = true ]; then
        generate_json_output
    else
        log_section "Validation Complete"
        log_success "Cluster validation finished"
        echo ""
        log_info "For detailed information, run: $0 -d"
        log_info "For JSON output, run: $0 -j"
        echo ""
    fi
}

# Run main function
main "$@"
