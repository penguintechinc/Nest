#!/usr/bin/env bash

set -euo pipefail

# Configuration
RELEASE_NAME="articdbm"
NAMESPACE="articdbm-beta"
KUBE_CONTEXT="dal2-beta"
APP_HOST="articdbm.penguintech.cloud"
IMAGE_REGISTRY="registry-dal2.penguintech.io"
CHART_PATH="./k8s/helm/articdbm"

# Directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Defaults
SERVICE=""
TAG=""
SKIP_BUILD=false
DRY_RUN=false
ROLLBACK=false

# Color helpers
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Prerequisites check
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed"
        return 1
    fi

    if ! command -v helm &> /dev/null; then
        log_error "helm is not installed"
        return 1
    fi

    if ! command -v docker &> /dev/null; then
        log_error "docker is not installed"
        return 1
    fi

    # Verify cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        log_error "Cannot connect to Kubernetes cluster"
        return 1
    fi

    # Verify context
    CURRENT_CONTEXT=$(kubectl config current-context)
    if [[ "$CURRENT_CONTEXT" != "$KUBE_CONTEXT" ]]; then
        log_warn "Current context '$CURRENT_CONTEXT' differs from target '$KUBE_CONTEXT'"
        if [[ "$DRY_RUN" != "true" ]]; then
            kubectl config use-context "$KUBE_CONTEXT"
        fi
    fi

    log_success "All prerequisites met"
}

# Build and push Docker images
build_and_push() {
    if [[ "$SKIP_BUILD" == "true" ]]; then
        log_warn "Skipping build (--skip-build provided)"
        return 0
    fi

    local tag="${TAG:-beta-$(date +%s)}"
    log_info "Building and pushing images with tag: $tag"

    # Manager service
    if [[ -z "$SERVICE" ]] || [[ "$SERVICE" == "manager" ]]; then
        log_info "Building manager service..."
        docker build -t "${IMAGE_REGISTRY}/articdbm-manager:${tag}" \
            -t "${IMAGE_REGISTRY}/articdbm-manager:latest" \
            -f "./services/manager/Dockerfile" \
            "./services/manager"

        if [[ "$DRY_RUN" != "true" ]]; then
            log_info "Pushing manager image..."
            docker push "${IMAGE_REGISTRY}/articdbm-manager:${tag}"
            docker push "${IMAGE_REGISTRY}/articdbm-manager:latest"
        fi
        log_success "Manager build complete"
    fi

    # Proxy service
    if [[ -z "$SERVICE" ]] || [[ "$SERVICE" == "proxy" ]]; then
        log_info "Building proxy service..."
        docker build -t "${IMAGE_REGISTRY}/articdbm-proxy:${tag}" \
            -t "${IMAGE_REGISTRY}/articdbm-proxy:latest" \
            -f "./services/proxy/Dockerfile" \
            "./services/proxy"

        if [[ "$DRY_RUN" != "true" ]]; then
            log_info "Pushing proxy image..."
            docker push "${IMAGE_REGISTRY}/articdbm-proxy:${tag}"
            docker push "${IMAGE_REGISTRY}/articdbm-proxy:latest"
        fi
        log_success "Proxy build complete"
    fi

    log_success "All images built and pushed"
}

# Deploy using Helm
deploy() {
    local values_file="${CHART_PATH}/values-beta.yaml"

    if [[ ! -f "$values_file" ]]; then
        log_error "Values file not found: $values_file"
        return 1
    fi

    log_info "Deploying to namespace: $NAMESPACE"

    # Create namespace if it doesn't exist
    if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
        log_info "Creating namespace: $NAMESPACE"
        if [[ "$DRY_RUN" != "true" ]]; then
            kubectl create namespace "$NAMESPACE"
        fi
    fi

    local helm_cmd=(
        "helm"
        "upgrade"
        "--install"
        "$RELEASE_NAME"
        "$CHART_PATH"
        "--namespace"
        "$NAMESPACE"
        "--values"
        "$values_file"
    )

    if [[ ! -z "$TAG" ]]; then
        helm_cmd+=(
            "--set"
            "image.tag=$TAG"
        )
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        helm_cmd+=("--dry-run" "--debug")
        log_info "DRY RUN: ${helm_cmd[*]}"
    fi

    if [[ "$DRY_RUN" != "true" ]]; then
        "${helm_cmd[@]}"
        log_success "Deployment complete"
    else
        log_info "Dry run completed"
    fi
}

# Verify deployment
verify_deployment() {
    log_info "Verifying deployment..."

    local max_retries=30
    local retry=0

    while [[ $retry -lt $max_retries ]]; do
        local ready_replicas=$(kubectl get deployment \
            -n "$NAMESPACE" \
            -l "app.kubernetes.io/name=$RELEASE_NAME" \
            -o jsonpath='{.items[0].status.readyReplicas}' 2>/dev/null || echo "0")

        local desired_replicas=$(kubectl get deployment \
            -n "$NAMESPACE" \
            -l "app.kubernetes.io/name=$RELEASE_NAME" \
            -o jsonpath='{.items[0].spec.replicas}' 2>/dev/null || echo "0")

        if [[ "$ready_replicas" == "$desired_replicas" ]] && [[ "$desired_replicas" -gt 0 ]]; then
            log_success "All replicas are ready ($ready_replicas/$desired_replicas)"
            return 0
        fi

        log_info "Waiting for replicas to be ready ($ready_replicas/$desired_replicas)..."
        sleep 2
        ((retry++))
    done

    log_error "Deployment verification timed out"
    return 1
}

# Rollback deployment
rollback_deployment() {
    log_warn "Rolling back deployment..."

    if ! helm rollback "$RELEASE_NAME" -n "$NAMESPACE"; then
        log_error "Rollback failed"
        return 1
    fi

    log_success "Rollback complete"
}

# Display help
show_help() {
    cat << EOF
Usage: $(basename "$0") [OPTIONS]

Deploy ArticDBM to beta environment

OPTIONS:
    --tag TAG              Image tag to deploy (default: beta-<timestamp>)
    --service SERVICE      Deploy specific service: manager or proxy (default: all)
    --skip-build          Skip Docker build and push
    --dry-run             Execute as dry-run without actual deployment
    --rollback            Rollback to previous release
    --help                Show this help message

EXAMPLES:
    # Deploy with auto-generated tag
    $(basename "$0")

    # Deploy specific tag
    $(basename "$0") --tag v1.2.3

    # Deploy only manager service
    $(basename "$0") --service manager

    # Dry-run deployment
    $(basename "$0") --dry-run

    # Rollback to previous release
    $(basename "$0") --rollback

ENVIRONMENT:
    RELEASE_NAME:       ${RELEASE_NAME}
    NAMESPACE:          ${NAMESPACE}
    KUBE_CONTEXT:       ${KUBE_CONTEXT}
    APP_HOST:           ${APP_HOST}
    IMAGE_REGISTRY:     ${IMAGE_REGISTRY}

EOF
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --tag)
                TAG="$2"
                shift 2
                ;;
            --service)
                SERVICE="$2"
                shift 2
                ;;
            --skip-build)
                SKIP_BUILD=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --rollback)
                ROLLBACK=true
                shift
                ;;
            --help)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Main function
main() {
    log_info "ArticDBM Beta Deployment Script"
    log_info "==============================="

    parse_args "$@"

    cd "$PROJECT_ROOT"

    check_prerequisites || exit 1

    if [[ "$ROLLBACK" == "true" ]]; then
        rollback_deployment
        exit $?
    fi

    build_and_push || exit 1
    deploy || exit 1
    verify_deployment || exit 1

    log_success "Deployment successful!"
    log_info "Access application at: https://${APP_HOST}"
}

main "$@"
