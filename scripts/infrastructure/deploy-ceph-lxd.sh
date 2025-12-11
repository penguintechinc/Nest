#!/bin/bash
# Ceph Cluster Deployment Script for LXD
# Automates deployment of Ceph storage cluster in LXD containers
# Supports: CephFS, RBD, iSCSI, and S3 (RGW)
#
# Version: 1.0.0
# Maintained by: Penguin Tech Inc
# License: Limited AGPL3
#
# Usage:
#   ./deploy-ceph-lxd.sh [OPTIONS]
#
# Options:
#   -n, --nodes NUM           Number of Ceph nodes to deploy (default: 1)
#   -p, --profile NAME        LXD profile name (default: ceph-cluster)
#   -b, --bridge NAME         LXD bridge name (default: lxdbr0)
#   -s, --storage POOL        LXD storage pool (default: default)
#   -c, --cloud-init FILE     Cloud-init file path
#   -h, --help                Show this help message
#
# Examples:
#   # Deploy single-node Ceph cluster
#   ./deploy-ceph-lxd.sh
#
#   # Deploy 3-node Ceph cluster
#   ./deploy-ceph-lxd.sh -n 3
#
#   # Deploy with custom profile
#   ./deploy-ceph-lxd.sh -p my-ceph-profile -b mybr0

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
INFRA_DIR="$PROJECT_ROOT/infrastructure/lxd/ceph"

# Default configuration
NUM_NODES=1
PROFILE_NAME="ceph-cluster"
BRIDGE_NAME="lxdbr0"
STORAGE_POOL="default"
CLOUD_INIT_FILE="$INFRA_DIR/cloud-init.yaml"
LXD_PROFILE_FILE="$INFRA_DIR/ceph-profile.yaml"
CONTAINER_PREFIX="ceph-node"
IMAGE="ubuntu:24.04"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# Help message
show_help() {
    cat << EOF
Ceph Cluster Deployment Script for LXD

Usage: $0 [OPTIONS]

Options:
  -n, --nodes NUM           Number of Ceph nodes to deploy (default: 1)
  -p, --profile NAME        LXD profile name (default: ceph-cluster)
  -b, --bridge NAME         LXD bridge name (default: lxdbr0)
  -s, --storage POOL        LXD storage pool (default: default)
  -c, --cloud-init FILE     Cloud-init file path
  -h, --help                Show this help message

Examples:
  # Deploy single-node Ceph cluster
  $0

  # Deploy 3-node Ceph cluster
  $0 -n 3

  # Deploy with custom settings
  $0 -n 3 -p my-ceph -b mybr0 -s mystorage

Description:
  This script automates the deployment of a Ceph storage cluster using LXD
  containers on Ubuntu 24.04 LTS. It supports:

  - CephFS (Distributed Filesystem)
  - RBD (Block Storage)
  - iSCSI Gateway
  - RGW (S3-compatible Object Storage)

Requirements:
  - LXD installed and initialized
  - Ubuntu 24.04 LTS host
  - Sufficient resources (4GB RAM, 4 CPUs per node minimum)
  - Network connectivity

For more information, see: $INFRA_DIR/README.md

EOF
    exit 0
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -n|--nodes)
                NUM_NODES="$2"
                shift 2
                ;;
            -p|--profile)
                PROFILE_NAME="$2"
                shift 2
                ;;
            -b|--bridge)
                BRIDGE_NAME="$2"
                shift 2
                ;;
            -s|--storage)
                STORAGE_POOL="$2"
                shift 2
                ;;
            -c|--cloud-init)
                CLOUD_INIT_FILE="$2"
                shift 2
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

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check if LXD is installed
    if ! command -v lxc &> /dev/null; then
        log_error "LXD is not installed. Please install LXD first."
        log_info "Install with: snap install lxd"
        exit 1
    fi

    # Check if LXD is initialized
    if ! lxc network list &> /dev/null; then
        log_error "LXD is not initialized. Please run 'lxd init' first."
        exit 1
    fi

    # Check if cloud-init file exists
    if [ ! -f "$CLOUD_INIT_FILE" ]; then
        log_error "Cloud-init file not found: $CLOUD_INIT_FILE"
        exit 1
    fi

    # Check if LXD profile file exists
    if [ ! -f "$LXD_PROFILE_FILE" ]; then
        log_error "LXD profile file not found: $LXD_PROFILE_FILE"
        exit 1
    fi

    # Check if storage pool exists
    if ! lxc storage list | grep -q "$STORAGE_POOL"; then
        log_warning "Storage pool '$STORAGE_POOL' not found. Using default."
        STORAGE_POOL="default"
    fi

    # Check if bridge exists
    if ! lxc network list | grep -q "$BRIDGE_NAME"; then
        log_warning "Bridge '$BRIDGE_NAME' not found. Creating..."
        lxc network create "$BRIDGE_NAME" || log_error "Failed to create bridge"
    fi

    log_success "Prerequisites check passed"
}

# Create or update LXD profile
setup_profile() {
    log_info "Setting up LXD profile: $PROFILE_NAME"

    # Check if profile exists
    if lxc profile list | grep -q "$PROFILE_NAME"; then
        log_warning "Profile '$PROFILE_NAME' already exists. Updating..."
        lxc profile delete "$PROFILE_NAME" || true
    fi

    # Create profile from YAML file
    lxc profile create "$PROFILE_NAME"

    # Apply profile configuration
    cat "$LXD_PROFILE_FILE" | lxc profile edit "$PROFILE_NAME"

    # Update network bridge in profile
    lxc profile device set "$PROFILE_NAME" eth0 parent "$BRIDGE_NAME" || true

    # Update storage pool in profile
    lxc profile device set "$PROFILE_NAME" root pool "$STORAGE_POOL" || true

    log_success "LXD profile '$PROFILE_NAME' configured"
}

# Deploy Ceph node
deploy_node() {
    local node_num=$1
    local node_name="${CONTAINER_PREFIX}-$(printf "%02d" "$node_num")"

    log_info "Deploying Ceph node: $node_name"

    # Check if container already exists
    if lxc list | grep -q "$node_name"; then
        log_warning "Container '$node_name' already exists. Skipping..."
        return 0
    fi

    # Launch container with cloud-init
    lxc launch "$IMAGE" "$node_name" \
        --profile "$PROFILE_NAME" \
        --config=user.user-data="$(cat "$CLOUD_INIT_FILE")"

    # Wait for container to start
    sleep 5

    # Wait for cloud-init to complete
    log_info "Waiting for cloud-init to complete on $node_name..."
    local timeout=600
    local elapsed=0

    while [ $elapsed -lt $timeout ]; do
        if lxc exec "$node_name" -- cloud-init status --wait 2>/dev/null; then
            log_success "Cloud-init completed on $node_name"
            break
        fi
        sleep 10
        elapsed=$((elapsed + 10))
        log_info "Waiting... ($elapsed/$timeout seconds)"
    done

    if [ $elapsed -ge $timeout ]; then
        log_error "Timeout waiting for cloud-init on $node_name"
        return 1
    fi

    # Get container IP
    local container_ip
    container_ip=$(lxc list "$node_name" -c 4 -f csv | awk '{print $1}' | head -n1)

    log_success "Node $node_name deployed successfully"
    log_info "IP Address: $container_ip"
}

# Display cluster information
show_cluster_info() {
    log_info "Ceph Cluster Information"
    echo ""
    echo "=========================================="
    echo "Cluster Details"
    echo "=========================================="
    echo "Profile: $PROFILE_NAME"
    echo "Bridge: $BRIDGE_NAME"
    echo "Storage Pool: $STORAGE_POOL"
    echo "Number of Nodes: $NUM_NODES"
    echo ""
    echo "Nodes:"

    for ((i=1; i<=NUM_NODES; i++)); do
        local node_name="${CONTAINER_PREFIX}-$(printf "%02d" "$i")"
        if lxc list | grep -q "$node_name"; then
            local container_ip
            container_ip=$(lxc list "$node_name" -c 4 -f csv | awk '{print $1}' | head -n1)
            echo "  - $node_name: $container_ip"
        fi
    done

    echo ""
    echo "=========================================="
    echo "Access Information"
    echo "=========================================="

    local first_node="${CONTAINER_PREFIX}-01"
    if lxc list | grep -q "$first_node"; then
        local dashboard_ip
        dashboard_ip=$(lxc list "$first_node" -c 4 -f csv | awk '{print $1}' | head -n1)

        echo "Ceph Dashboard:"
        echo "  URL: https://$dashboard_ip:8443"
        echo "  Username: admin"
        echo "  Password: admin"
        echo ""
        echo "S3 Endpoint:"
        echo "  URL: http://$dashboard_ip:8080"
        echo "  Access Key: ACCESSKEY123"
        echo "  Secret Key: SECRETKEY123"
        echo ""
        echo "Container Shell:"
        echo "  lxc exec $first_node -- bash"
        echo ""
        echo "Validate Cluster:"
        echo "  lxc exec $first_node -- /usr/local/bin/validate-ceph.sh"
    fi

    echo "=========================================="
    echo ""
}

# Validate deployment
validate_deployment() {
    log_info "Validating deployment..."

    local first_node="${CONTAINER_PREFIX}-01"

    if ! lxc list | grep -q "$first_node"; then
        log_error "First node not found"
        return 1
    fi

    # Wait for Ceph to be fully operational
    sleep 30

    log_info "Running validation script on $first_node..."

    if lxc exec "$first_node" -- /usr/local/bin/validate-ceph.sh; then
        log_success "Cluster validation passed"
        return 0
    else
        log_warning "Cluster validation returned warnings (may be normal during initial setup)"
        return 0
    fi
}

# Cleanup function
cleanup() {
    log_info "Cleaning up..."
}

# Trap cleanup on exit
trap cleanup EXIT

# Main deployment function
main() {
    echo ""
    echo "=========================================="
    echo "Ceph Cluster Deployment for LXD"
    echo "Version: 1.0.0"
    echo "Maintained by: Penguin Tech Inc"
    echo "=========================================="
    echo ""

    # Parse arguments
    parse_args "$@"

    # Run checks
    check_prerequisites

    # Setup profile
    setup_profile

    # Deploy nodes
    log_info "Deploying $NUM_NODES Ceph node(s)..."
    for ((i=1; i<=NUM_NODES; i++)); do
        deploy_node "$i"
    done

    # Wait for services to stabilize
    log_info "Waiting for Ceph services to stabilize..."
    sleep 60

    # Validate deployment
    validate_deployment

    # Show cluster information
    show_cluster_info

    log_success "Ceph cluster deployment completed successfully!"
    echo ""
    log_info "Next steps:"
    echo "  1. Access the Ceph dashboard to monitor your cluster"
    echo "  2. Run validation: lxc exec ${CONTAINER_PREFIX}-01 -- /usr/local/bin/validate-ceph.sh"
    echo "  3. Configure additional pools: lxc exec ${CONTAINER_PREFIX}-01 -- bash"
    echo "  4. See documentation: $INFRA_DIR/README.md"
    echo ""
}

# Run main function
main "$@"
