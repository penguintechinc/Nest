#!/bin/bash
# Ceph Storage Pool Configuration Script
# Creates and manages Ceph storage pools for different use cases
#
# Version: 1.0.0
# Maintained by: Penguin Tech Inc
# License: Limited AGPL3
#
# Usage:
#   ./configure-ceph-pools.sh [OPTIONS]
#
# Options:
#   -c, --container NAME      LXD container name (default: ceph-node-01)
#   -t, --type TYPE           Pool type: all, rbd, cephfs, rgw, iscsi (default: all)
#   -s, --size NUM            Replica size (default: 3)
#   -p, --pg-num NUM          Placement groups (default: 128)
#   -r, --remove POOL         Remove a pool
#   -l, --list                List all pools
#   -h, --help                Show this help message

set -euo pipefail

# Default configuration
CONTAINER_NAME="ceph-node-01"
POOL_TYPE="all"
REPLICA_SIZE=3
PG_NUM=128
REMOVE_POOL=""
LIST_POOLS=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_section() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}$*${NC}"
    echo -e "${CYAN}========================================${NC}"
}

# Help message
show_help() {
    cat << EOF
Ceph Storage Pool Configuration Script

Usage: $0 [OPTIONS]

Options:
  -c, --container NAME      LXD container name (default: ceph-node-01)
  -t, --type TYPE           Pool type: all, rbd, cephfs, rgw, iscsi (default: all)
  -s, --size NUM            Replica size (default: 3)
  -p, --pg-num NUM          Placement groups (default: 128)
  -r, --remove POOL         Remove a pool
  -l, --list                List all pools
  -h, --help                Show this help message

Pool Types:
  all                       Create all pool types
  rbd                       RADOS Block Device pools
  cephfs                    CephFS filesystem pools
  rgw                       RADOS Gateway (S3) pools
  iscsi                     iSCSI target pools

Examples:
  # Create all pools with defaults
  $0

  # Create only RBD pools
  $0 -t rbd

  # Create pools with custom settings
  $0 -s 2 -p 64

  # List all pools
  $0 -l

  # Remove a pool
  $0 -r mypool

  # Create pools in specific container
  $0 -c ceph-node-02 -t rgw

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
            -t|--type)
                POOL_TYPE="$2"
                shift 2
                ;;
            -s|--size)
                REPLICA_SIZE="$2"
                shift 2
                ;;
            -p|--pg-num)
                PG_NUM="$2"
                shift 2
                ;;
            -r|--remove)
                REMOVE_POOL="$2"
                shift 2
                ;;
            -l|--list)
                LIST_POOLS=true
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

# Check if pool exists
pool_exists() {
    local pool_name="$1"
    if exec_in_container "ceph osd pool ls" | grep -q "^${pool_name}$"; then
        return 0
    else
        return 1
    fi
}

# Create pool
create_pool() {
    local pool_name="$1"
    local pool_type="${2:-replicated}"
    local application="${3:-}"

    if pool_exists "$pool_name"; then
        log_warning "Pool '$pool_name' already exists"
        return 0
    fi

    log_info "Creating pool: $pool_name (type: $pool_type, size: $REPLICA_SIZE, PG: $PG_NUM)"

    # Create the pool
    exec_in_container "ceph osd pool create $pool_name $PG_NUM $PG_NUM $pool_type"

    # Set replica size
    exec_in_container "ceph osd pool set $pool_name size $REPLICA_SIZE"

    # Set application tag if provided
    if [ -n "$application" ]; then
        exec_in_container "ceph osd pool application enable $pool_name $application" || true
    fi

    log_success "Pool '$pool_name' created successfully"
}

# Create RBD pools
create_rbd_pools() {
    log_section "Creating RBD (Block Storage) Pools"

    # Main RBD pool
    create_pool "rbd" "replicated" "rbd"
    exec_in_container "rbd pool init rbd" || true

    # Create example RBD images
    if ! exec_in_container "rbd ls rbd" | grep -q "vm-disk-01"; then
        log_info "Creating example RBD images..."
        exec_in_container "rbd create rbd/vm-disk-01 --size 20G --image-feature layering"
        exec_in_container "rbd create rbd/vm-disk-02 --size 50G --image-feature layering"
        log_success "Example RBD images created"
    fi

    # Performance pool (SSD-backed if available)
    create_pool "rbd-performance" "replicated" "rbd"
    exec_in_container "rbd pool init rbd-performance" || true

    # Archive pool (HDD-backed for cold storage)
    create_pool "rbd-archive" "replicated" "rbd"
    exec_in_container "rbd pool init rbd-archive" || true

    log_success "RBD pools configured"
}

# Create CephFS pools
create_cephfs_pools() {
    log_section "Creating CephFS Pools"

    # CephFS metadata pool (needs higher IOPS)
    create_pool "cephfs_metadata" "replicated" "cephfs"

    # CephFS data pool
    create_pool "cephfs_data" "replicated" "cephfs"

    # Create CephFS if not exists
    if ! exec_in_container "ceph fs ls" | grep -q "cephfs"; then
        log_info "Creating CephFS filesystem..."
        exec_in_container "ceph fs new cephfs cephfs_metadata cephfs_data"
        log_success "CephFS filesystem created"
    fi

    # Enable snapshots
    exec_in_container "ceph fs set cephfs allow_new_snaps true" || true

    log_success "CephFS pools configured"
}

# Create RGW pools
create_rgw_pools() {
    log_section "Creating RGW (S3/Object Storage) Pools"

    # RGW root pool
    create_pool ".rgw.root" "replicated" "rgw"

    # RGW control pool
    create_pool "default.rgw.control" "replicated" "rgw"

    # RGW metadata pool
    create_pool "default.rgw.meta" "replicated" "rgw"

    # RGW log pool
    create_pool "default.rgw.log" "replicated" "rgw"

    # RGW bucket index pool
    create_pool "default.rgw.buckets.index" "replicated" "rgw"

    # RGW data pool
    create_pool "default.rgw.buckets.data" "replicated" "rgw"

    # Multi-site pools (optional)
    create_pool "default.rgw.buckets.non-ec" "replicated" "rgw"

    log_success "RGW pools configured"
}

# Create iSCSI pools
create_iscsi_pools() {
    log_section "Creating iSCSI Pools"

    # iSCSI main pool
    create_pool "iscsi" "replicated" "rbd"
    exec_in_container "rbd pool init iscsi" || true

    # Create example iSCSI LUNs
    if ! exec_in_container "rbd ls iscsi" | grep -q "lun-01"; then
        log_info "Creating example iSCSI LUNs..."
        exec_in_container "rbd create iscsi/lun-01 --size 100G --image-feature layering"
        exec_in_container "rbd create iscsi/lun-02 --size 200G --image-feature layering"
        log_success "Example iSCSI LUNs created"
    fi

    log_success "iSCSI pools configured"
}

# Create erasure-coded pool (for cold storage)
create_ec_pools() {
    log_section "Creating Erasure-Coded Pools (Optional)"

    # Check if enough OSDs for EC
    local osd_count
    osd_count=$(exec_in_container "ceph osd stat" | grep -oP '\d+(?= osds)' || echo "0")

    if [ "$osd_count" -lt 3 ]; then
        log_warning "Not enough OSDs for erasure coding (need at least 3, have $osd_count)"
        return 0
    fi

    # Create EC profile
    if ! exec_in_container "ceph osd erasure-code-profile ls" | grep -q "ec-profile"; then
        log_info "Creating erasure code profile..."
        exec_in_container "ceph osd erasure-code-profile set ec-profile k=2 m=1 crush-failure-domain=osd"
    fi

    # Create EC pool
    if ! pool_exists "ec-pool"; then
        log_info "Creating erasure-coded pool..."
        exec_in_container "ceph osd pool create ec-pool 64 64 erasure ec-profile"
        exec_in_container "ceph osd pool application enable ec-pool rgw" || true
        log_success "Erasure-coded pool created"
    fi
}

# List all pools
list_pools() {
    log_section "Current Storage Pools"

    echo ""
    exec_in_container "ceph osd pool ls detail"

    echo ""
    log_section "Pool Statistics"
    exec_in_container "ceph df"

    echo ""
    log_section "Pool Usage by Application"
    exec_in_container "ceph osd pool ls detail | grep -E 'pool|application'"
}

# Remove pool
remove_pool() {
    local pool_name="$1"

    if ! pool_exists "$pool_name"; then
        log_error "Pool '$pool_name' does not exist"
        return 1
    fi

    log_warning "Removing pool: $pool_name"
    log_warning "This will DELETE ALL DATA in the pool!"

    read -p "Are you sure? Type 'yes' to confirm: " -r
    if [[ ! $REPLY =~ ^yes$ ]]; then
        log_info "Aborted"
        return 0
    fi

    # Enable pool deletion if not already enabled
    exec_in_container "ceph config set mon mon_allow_pool_delete true"

    # Delete the pool
    exec_in_container "ceph osd pool delete $pool_name $pool_name --yes-i-really-really-mean-it"

    log_success "Pool '$pool_name' removed"
}

# Configure pool quotas
configure_quotas() {
    log_section "Configuring Pool Quotas (Optional)"

    # Example: Set quota on RBD pool
    # Uncomment and adjust as needed
    # exec_in_container "ceph osd pool set-quota rbd max_bytes $((100 * 1024 * 1024 * 1024))"  # 100GB
    # exec_in_container "ceph osd pool set-quota rbd max_objects 10000"

    log_info "Pool quotas can be configured using:"
    log_info "  ceph osd pool set-quota <pool> max_bytes <bytes>"
    log_info "  ceph osd pool set-quota <pool> max_objects <num>"
}

# Optimize pool PG numbers
optimize_pg_numbers() {
    log_section "Optimizing Placement Groups"

    # Enable PG autoscaler
    exec_in_container "ceph config set global osd_pool_default_pg_autoscale_mode on" || true
    exec_in_container "ceph mgr module enable pg_autoscaler" || true

    # Set autoscale mode for all pools
    exec_in_container "ceph osd pool ls" | while read -r pool; do
        exec_in_container "ceph osd pool set $pool pg_autoscale_mode on" || true
    done

    log_success "PG autoscaler enabled for all pools"
}

# Main function
main() {
    # Parse arguments
    parse_args "$@"

    echo ""
    echo "=========================================="
    echo "Ceph Storage Pool Configuration"
    echo "Container: $CONTAINER_NAME"
    echo "=========================================="
    echo ""

    # Check container
    if ! lxc list | grep -q "$CONTAINER_NAME"; then
        log_error "Container '$CONTAINER_NAME' not found"
        exit 1
    fi

    # Handle list operation
    if [ "$LIST_POOLS" = true ]; then
        list_pools
        exit 0
    fi

    # Handle remove operation
    if [ -n "$REMOVE_POOL" ]; then
        remove_pool "$REMOVE_POOL"
        exit 0
    fi

    # Create pools based on type
    case "$POOL_TYPE" in
        all)
            create_rbd_pools
            create_cephfs_pools
            create_rgw_pools
            create_iscsi_pools
            create_ec_pools
            ;;
        rbd)
            create_rbd_pools
            ;;
        cephfs)
            create_cephfs_pools
            ;;
        rgw)
            create_rgw_pools
            ;;
        iscsi)
            create_iscsi_pools
            ;;
        *)
            log_error "Invalid pool type: $POOL_TYPE"
            log_info "Valid types: all, rbd, cephfs, rgw, iscsi"
            exit 1
            ;;
    esac

    # Optimize PG numbers
    optimize_pg_numbers

    # Show final status
    log_section "Configuration Complete"
    list_pools

    echo ""
    log_success "Pool configuration completed successfully!"
    echo ""
    log_info "Next steps:"
    echo "  - List pools: $0 -l"
    echo "  - Create specific pool type: $0 -t <type>"
    echo "  - Configure quotas manually using ceph osd pool set-quota"
    echo ""
}

# Run main function
main "$@"
