#!/bin/bash

# ArticDBM Production Deployment Script with XDP Acceleration
# Automated deployment for production environments

set -euo pipefail

# Deployment configuration
DEPLOYMENT_ENV="${DEPLOYMENT_ENV:-production}"
DEPLOYMENT_REGION="${DEPLOYMENT_REGION:-us-east-1}"
DEPLOYMENT_CLUSTER="${DEPLOYMENT_CLUSTER:-articdbm-cluster}"
DEPLOYMENT_NAMESPACE="${DEPLOYMENT_NAMESPACE:-articdbm}"
DEPLOYMENT_VERSION="${DEPLOYMENT_VERSION:-latest}"

# XDP configuration
XDP_INTERFACE="${XDP_INTERFACE:-eth0}"
XDP_ENABLED="${XDP_ENABLED:-true}"
XDP_RATE_LIMIT="${XDP_RATE_LIMIT:-100000000}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $*"
}

success() {
    echo -e "${GREEN}✓${NC} $*"
}

warning() {
    echo -e "${YELLOW}⚠${NC} $*"
}

error() {
    echo -e "${RED}✗${NC} $*"
    exit 1
}

# Pre-deployment checks
pre_deployment_checks() {
    log "Running pre-deployment checks..."

    # Check required tools
    local tools=("docker" "kubectl" "helm" "terraform" "ansible")
    for tool in "${tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            error "Required tool not found: $tool"
        fi
    done

    # Check cloud credentials
    if [[ "$DEPLOYMENT_REGION" == *"us-"* ]] || [[ "$DEPLOYMENT_REGION" == *"eu-"* ]]; then
        if ! aws sts get-caller-identity &>/dev/null; then
            error "AWS credentials not configured"
        fi
    elif [[ "$DEPLOYMENT_REGION" == *"asia-"* ]]; then
        if ! gcloud auth list --filter=status:ACTIVE --format="value(account)" &>/dev/null; then
            error "GCP credentials not configured"
        fi
    fi

    # Check Kubernetes connectivity
    if ! kubectl cluster-info &>/dev/null; then
        error "Cannot connect to Kubernetes cluster"
    fi

    success "Pre-deployment checks passed"
}

# Infrastructure provisioning with Terraform
provision_infrastructure() {
    log "Provisioning infrastructure with Terraform..."

    cat > terraform/main.tf << 'EOF'
terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.23"
    }
  }
}

provider "aws" {
  region = var.region
}

provider "kubernetes" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
  }
}

# EKS Cluster for ArticDBM
module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 19.0"

  cluster_name    = var.cluster_name
  cluster_version = "1.28"

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  enable_irsa = true

  eks_managed_node_group_defaults = {
    ami_type       = "AL2_x86_64"
    instance_types = ["m5.2xlarge", "m5.4xlarge"]

    attach_cluster_primary_security_group = true
  }

  eks_managed_node_groups = {
    xdp_optimized = {
      name           = "xdp-optimized-nodes"
      min_size       = 3
      max_size       = 10
      desired_size   = 5

      instance_types = ["m5n.4xlarge"]  # Network optimized
      capacity_type  = "ON_DEMAND"

      taints = [{
        key    = "workload"
        value  = "xdp"
        effect = "NO_SCHEDULE"
      }]

      labels = {
        Environment = var.environment
        Workload    = "xdp-proxy"
      }

      user_data = base64encode(<<-EOT
        #!/bin/bash
        # Enable huge pages for AF_XDP
        echo 2048 > /proc/sys/vm/nr_hugepages

        # Mount BPF filesystem
        mount -t bpf bpf /sys/fs/bpf

        # Configure network optimization
        echo 'net.core.netdev_max_backlog = 5000' >> /etc/sysctl.conf
        echo 'net.ipv4.tcp_congestion_control = bbr' >> /etc/sysctl.conf
        sysctl -p
      EOT
      )
    }

    general = {
      name           = "general-nodes"
      min_size       = 2
      max_size       = 5
      desired_size   = 3

      instance_types = ["m5.xlarge"]
      capacity_type  = "SPOT"

      labels = {
        Environment = var.environment
        Workload    = "general"
      }
    }
  }
}

# VPC for ArticDBM
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"

  name = "${var.cluster_name}-vpc"
  cidr = "10.0.0.0/16"

  azs             = data.aws_availability_zones.available.names
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]

  enable_nat_gateway = true
  enable_vpn_gateway = true
  enable_dns_hostnames = true

  tags = {
    "kubernetes.io/cluster/${var.cluster_name}" = "shared"
  }

  public_subnet_tags = {
    "kubernetes.io/cluster/${var.cluster_name}" = "shared"
    "kubernetes.io/role/elb"                    = "1"
  }

  private_subnet_tags = {
    "kubernetes.io/cluster/${var.cluster_name}" = "shared"
    "kubernetes.io/role/internal-elb"           = "1"
  }
}

# RDS for ArticDBM Manager
resource "aws_db_instance" "articdbm_manager" {
  identifier     = "${var.cluster_name}-manager-db"
  engine         = "postgres"
  engine_version = "15.4"
  instance_class = "db.r6g.xlarge"

  allocated_storage     = 100
  storage_encrypted     = true
  storage_type          = "gp3"
  iops                  = 3000

  db_name  = "articdbm"
  username = "articdbm"
  password = random_password.db_password.result

  vpc_security_group_ids = [aws_security_group.rds.id]
  db_subnet_group_name   = aws_db_subnet_group.articdbm.name

  backup_retention_period = 30
  backup_window          = "03:00-04:00"
  maintenance_window     = "sun:04:00-sun:05:00"

  enabled_cloudwatch_logs_exports = ["postgresql"]

  tags = {
    Name        = "${var.cluster_name}-manager-db"
    Environment = var.environment
  }
}

# ElastiCache Redis Cluster
resource "aws_elasticache_replication_group" "articdbm" {
  replication_group_id       = "${var.cluster_name}-redis"
  replication_group_description = "Redis cluster for ArticDBM"

  engine               = "redis"
  engine_version       = "7.0"
  node_type            = "cache.r6g.xlarge"
  number_cache_clusters = 3

  automatic_failover_enabled = true
  multi_az_enabled          = true

  subnet_group_name = aws_elasticache_subnet_group.articdbm.name
  security_group_ids = [aws_security_group.redis.id]

  at_rest_encryption_enabled = true
  transit_encryption_enabled = true

  snapshot_retention_limit = 5
  snapshot_window         = "03:00-05:00"

  tags = {
    Name        = "${var.cluster_name}-redis"
    Environment = var.environment
  }
}

# S3 Bucket for backups
resource "aws_s3_bucket" "backups" {
  bucket = "${var.cluster_name}-backups-${random_id.bucket_suffix.hex}"

  tags = {
    Name        = "${var.cluster_name}-backups"
    Environment = var.environment
  }
}

resource "aws_s3_bucket_versioning" "backups" {
  bucket = aws_s3_bucket.backups.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_encryption" "backups" {
  bucket = aws_s3_bucket.backups.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# Outputs
output "cluster_endpoint" {
  value = module.eks.cluster_endpoint
}

output "rds_endpoint" {
  value = aws_db_instance.articdbm_manager.endpoint
}

output "redis_endpoint" {
  value = aws_elasticache_replication_group.articdbm.primary_endpoint_address
}
EOF

    cd terraform/
    terraform init
    terraform plan -out=tfplan
    terraform apply tfplan

    success "Infrastructure provisioned successfully"
}

# Deploy ArticDBM with Helm
deploy_articdbm() {
    log "Deploying ArticDBM with Helm..."

    # Create namespace
    kubectl create namespace "$DEPLOYMENT_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

    # Create Helm chart
    cat > helm/articdbm/Chart.yaml << EOF
apiVersion: v2
name: articdbm
description: ArticDBM Database Proxy with XDP Acceleration
type: application
version: 1.0.0
appVersion: "$DEPLOYMENT_VERSION"
EOF

    cat > helm/articdbm/values.yaml << EOF
replicaCount: 5

image:
  repository: articdbm/proxy
  tag: "$DEPLOYMENT_VERSION"
  pullPolicy: IfNotPresent

xdp:
  enabled: true
  interface: "$XDP_INTERFACE"
  rateLimitPPS: $XDP_RATE_LIMIT
  burstLimit: 10000
  cacheSize: 1048576
  cacheTTL: 300

proxy:
  mysql:
    enabled: true
    port: 3306
  postgresql:
    enabled: true
    port: 5432
  mssql:
    enabled: true
    port: 1433
  mongodb:
    enabled: true
    port: 27017
  redis:
    enabled: true
    port: 6379

resources:
  limits:
    cpu: 4
    memory: 8Gi
    hugepages-2Mi: 2Gi
  requests:
    cpu: 2
    memory: 4Gi
    hugepages-2Mi: 2Gi

nodeSelector:
  workload: xdp-proxy

tolerations:
- key: workload
  operator: Equal
  value: xdp
  effect: NoSchedule

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchExpressions:
          - key: app
            operator: In
            values:
            - articdbm-proxy
        topologyKey: kubernetes.io/hostname

securityContext:
  privileged: true
  capabilities:
    add:
    - SYS_ADMIN
    - NET_ADMIN
    - BPF

hostNetwork: true

volumeMounts:
- name: bpf-maps
  mountPath: /sys/fs/bpf
- name: proc
  mountPath: /host/proc
  readOnly: true

volumes:
- name: bpf-maps
  hostPath:
    path: /sys/fs/bpf
    type: DirectoryOrCreate
- name: proc
  hostPath:
    path: /proc
    type: Directory

service:
  type: LoadBalancer
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled: "true"

autoscaling:
  enabled: true
  minReplicas: 5
  maxReplicas: 20
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80

metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    interval: 30s
    scrapeTimeout: 10s

monitoring:
  prometheus:
    enabled: true
  grafana:
    enabled: true
    dashboards:
      enabled: true
  alerts:
    enabled: true

backup:
  enabled: true
  schedule: "0 2 * * *"
  retention: 30
  s3Bucket: "${DEPLOYMENT_CLUSTER}-backups"
EOF

    # Deploy with Helm
    helm upgrade --install articdbm ./helm/articdbm \
        --namespace "$DEPLOYMENT_NAMESPACE" \
        --wait \
        --timeout 10m

    success "ArticDBM deployed successfully"
}

# Configure monitoring and observability
setup_monitoring() {
    log "Setting up monitoring and observability..."

    # Deploy Prometheus Operator
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo update

    helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
        --namespace monitoring \
        --create-namespace \
        --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
        --wait

    # Deploy Grafana dashboards
    kubectl apply -f - << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: articdbm-xdp-dashboard
  namespace: monitoring
data:
  dashboard.json: |
    {
      "dashboard": {
        "title": "ArticDBM XDP Performance",
        "panels": [
          {
            "title": "XDP Packet Processing Rate",
            "targets": [
              {"expr": "rate(articdbm_xdp_packets_processed_total[5m])"}
            ]
          },
          {
            "title": "Cache Hit Ratio",
            "targets": [
              {"expr": "articdbm_xdp_cache_hits_total / (articdbm_xdp_cache_hits_total + articdbm_xdp_cache_misses_total)"}
            ]
          },
          {
            "title": "AF_XDP Zero-Copy Performance",
            "targets": [
              {"expr": "rate(articdbm_afxdp_packets_rx_total[5m])"}
            ]
          },
          {
            "title": "Blue/Green Deployment Status",
            "targets": [
              {"expr": "articdbm_deployment_traffic_percentage"}
            ]
          }
        ]
      }
    }
EOF

    # Deploy Jaeger for distributed tracing
    kubectl create namespace tracing --dry-run=client -o yaml | kubectl apply -f -

    helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
    helm upgrade --install jaeger jaegertracing/jaeger \
        --namespace tracing \
        --set provisionDataStore.cassandra=false \
        --set provisionDataStore.elasticsearch=true \
        --set storage.type=elasticsearch \
        --wait

    success "Monitoring and observability configured"
}

# Configure autoscaling
setup_autoscaling() {
    log "Configuring autoscaling policies..."

    kubectl apply -f - << EOF
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: articdbm-proxy-hpa
  namespace: $DEPLOYMENT_NAMESPACE
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: articdbm-proxy
  minReplicas: 5
  maxReplicas: 50
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  - type: Pods
    pods:
      metric:
        name: articdbm_xdp_packets_processed_rate
      target:
        type: AverageValue
        averageValue: "10000000"
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 100
        periodSeconds: 60
      - type: Pods
        value: 5
        periodSeconds: 60
      selectPolicy: Max
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
      - type: Pods
        value: 2
        periodSeconds: 60
      selectPolicy: Min
EOF

    # Configure Vertical Pod Autoscaler
    kubectl apply -f - << EOF
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: articdbm-proxy-vpa
  namespace: $DEPLOYMENT_NAMESPACE
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: articdbm-proxy
  updatePolicy:
    updateMode: "Auto"
  resourcePolicy:
    containerPolicies:
    - containerName: proxy
      minAllowed:
        cpu: 1
        memory: 2Gi
      maxAllowed:
        cpu: 8
        memory: 16Gi
      controlledResources: ["cpu", "memory"]
EOF

    success "Autoscaling configured"
}

# Setup disaster recovery
setup_disaster_recovery() {
    log "Setting up disaster recovery..."

    # Create backup CronJob
    kubectl apply -f - << EOF
apiVersion: batch/v1
kind: CronJob
metadata:
  name: articdbm-backup
  namespace: $DEPLOYMENT_NAMESPACE
spec:
  schedule: "0 2 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: articdbm/backup:latest
            env:
            - name: S3_BUCKET
              value: "${DEPLOYMENT_CLUSTER}-backups"
            - name: REDIS_ADDR
              valueFrom:
                secretKeyRef:
                  name: articdbm-secrets
                  key: redis-endpoint
            - name: RDS_ENDPOINT
              valueFrom:
                secretKeyRef:
                  name: articdbm-secrets
                  key: rds-endpoint
            command:
            - /bin/bash
            - -c
            - |
              # Backup Redis data
              redis-cli --rdb /tmp/redis-backup.rdb
              aws s3 cp /tmp/redis-backup.rdb s3://\${S3_BUCKET}/redis/\$(date +%Y%m%d)/backup.rdb

              # Backup PostgreSQL
              pg_dump -h \${RDS_ENDPOINT} -U articdbm articdbm | gzip > /tmp/postgres-backup.sql.gz
              aws s3 cp /tmp/postgres-backup.sql.gz s3://\${S3_BUCKET}/postgres/\$(date +%Y%m%d)/backup.sql.gz

              # Backup XDP rules
              redis-cli --raw HGETALL articdbm:xdp:rules > /tmp/xdp-rules.json
              aws s3 cp /tmp/xdp-rules.json s3://\${S3_BUCKET}/xdp/\$(date +%Y%m%d)/rules.json
          restartPolicy: OnFailure
EOF

    # Setup cross-region replication
    aws s3api put-bucket-replication \
        --bucket "${DEPLOYMENT_CLUSTER}-backups" \
        --replication-configuration file://replication.json

    success "Disaster recovery configured"
}

# Performance testing
run_performance_tests() {
    log "Running performance tests..."

    # Deploy load testing pod
    kubectl run load-test --image=articdbm/load-tester:latest \
        --namespace "$DEPLOYMENT_NAMESPACE" \
        --rm -i --tty -- \
        /bin/bash -c "
            # Test XDP packet processing
            hping3 -c 1000000 -d 120 -S -w 64 -p 3306 --flood articdbm-proxy

            # Test cache performance
            redis-benchmark -h articdbm-proxy -p 6379 -t get,set -n 1000000

            # Test SQL query performance
            mysqlslap --host=articdbm-proxy --port=3306 \
                --concurrency=100 --iterations=10 \
                --number-int-cols=5 --number-char-cols=5 \
                --auto-generate-sql --auto-generate-sql-write-number=1000
        "

    success "Performance tests completed"
}

# Health checks
perform_health_checks() {
    log "Performing health checks..."

    # Check proxy health
    local proxy_health=$(kubectl exec -n "$DEPLOYMENT_NAMESPACE" deploy/articdbm-proxy -- curl -s http://localhost:9090/health)

    if [[ "$proxy_health" == "OK" ]]; then
        success "Proxy health check passed"
    else
        error "Proxy health check failed"
    fi

    # Check XDP programs
    kubectl exec -n "$DEPLOYMENT_NAMESPACE" deploy/articdbm-proxy -- bpftool prog show

    # Check cache statistics
    kubectl exec -n "$DEPLOYMENT_NAMESPACE" deploy/articdbm-proxy -- redis-cli HGETALL articdbm:cache:stats

    success "All health checks passed"
}

# Main deployment flow
main() {
    log "Starting ArticDBM production deployment"
    log "Environment: $DEPLOYMENT_ENV"
    log "Region: $DEPLOYMENT_REGION"
    log "Cluster: $DEPLOYMENT_CLUSTER"
    echo

    pre_deployment_checks
    provision_infrastructure
    deploy_articdbm
    setup_monitoring
    setup_autoscaling
    setup_disaster_recovery
    run_performance_tests
    perform_health_checks

    echo
    success "ArticDBM deployment completed successfully!"

    log "Access points:"
    log "  Proxy: $(kubectl get svc -n $DEPLOYMENT_NAMESPACE articdbm-proxy -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')"
    log "  Grafana: $(kubectl get svc -n monitoring prometheus-grafana -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')"
    log "  Jaeger: $(kubectl get svc -n tracing jaeger-query -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')"
}

# Run main function
main "$@"