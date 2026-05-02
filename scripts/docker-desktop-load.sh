#!/usr/bin/env bash
# Load local Docker images into docker-desktop Kubernetes containerd namespace.
# Docker Desktop uses a separate containerd namespace (k8s.io) from Docker (moby),
# so images built with `docker build` must be explicitly imported.
#
# Usage: ./scripts/docker-desktop-load.sh [image1 image2 ...]
#   No args: loads all standard Nest service images
#   With args: loads specified images (e.g. nest-controller:dev)
#
# Requirements: docker-desktop context must be available and the K8s node
#   container must be named `desktop-control-plane`.

set -euo pipefail

K8S_NODE="desktop-control-plane"

load_image() {
    local image="$1"
    local tag="${image%:*}"
    local name="${tag##*/}"
    echo "→ importing ${image} into k8s.io namespace..."
    docker save "${image}" | docker exec -i "${K8S_NODE}" ctr -n k8s.io images import -
    # Tag as localhost:32000/<name>:<tag> so kustomize image refs resolve
    local ver="${image##*:}"
    docker exec "${K8S_NODE}" ctr -n k8s.io images tag \
        "docker.io/library/${name}:${ver}" \
        "localhost:32000/${name}:${ver}" 2>/dev/null || true
    echo "  ✓ ${image}"
}

DEFAULT_IMAGES=(
    nest-controller:dev
    nest-gateway:dev
    nest-api:dev
    nest-node-agent:dev
    nest-scheduler:dev
    nest-manager:dev
)

IMAGES=("${@:-${DEFAULT_IMAGES[@]}}")

echo "Loading ${#IMAGES[@]} image(s) into docker-desktop Kubernetes..."
for img in "${IMAGES[@]}"; do
    load_image "${img}"
done
echo "Done. Restart affected deployments with:"
echo "  kubectl --context docker-desktop -n nest rollout restart deployment"
