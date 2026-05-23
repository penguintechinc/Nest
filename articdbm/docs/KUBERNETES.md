# ☸️ Kubernetes Deployment Guide

This guide covers deploying ArticDBM to Kubernetes clusters including EKS, GKE, and AKS.

## 🏗️ Architecture Overview

ArticDBM on Kubernetes consists of:

- **ArticDBM Proxy**: High-performance Go proxy (multiple replicas)
- **ArticDBM Manager**: Python management interface (single replica)
- **Redis**: Configuration cache and session storage
- **PostgreSQL**: Manager configuration database

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "ArticDBM Namespace"
            LB[LoadBalancer Service]
            P1[Proxy Pod 1]
            P2[Proxy Pod 2]
            P3[Proxy Pod N]
            MGR[Manager Pod]
            REDIS[Redis Pod]
            PG[PostgreSQL Pod]
        end
        
        subgraph "External"
            CLIENTS[Database Clients]
            BACKENDS[Database Backends]
        end
    end
    
    CLIENTS --> LB
    LB --> P1 & P2 & P3
    P1 & P2 & P3 --> MGR
    P1 & P2 & P3 --> REDIS
    P1 & P2 & P3 --> BACKENDS
    MGR --> PG
    MGR --> REDIS
```

## 📦 Prerequisites

- Kubernetes cluster (1.20+)
- kubectl configured and connected
- Persistent storage support (for PostgreSQL/Redis)
- LoadBalancer or Ingress controller
- Container registry access (optional for custom images)

## 🚀 Quick Deployment

### Namespace Creation

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: articdbm
  labels:
    app.kubernetes.io/name: articdbm
    app.kubernetes.io/version: "1.0.0"
```

### ConfigMap for Shared Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: articdbm-config
  namespace: articdbm
data:
  # Redis Configuration
  REDIS_HOST: "redis-service"
  REDIS_PORT: "6379"
  REDIS_DB: "0"
  
  # Database Configuration
  DATABASE_URL: "postgresql://articdbm:password@postgres-service/articdbm"
  
  # Proxy Configuration
  MYSQL_ENABLED: "true"
  MYSQL_PORT: "3306"
  POSTGRESQL_ENABLED: "true"
  POSTGRESQL_PORT: "5432"
  MSSQL_ENABLED: "true"
  MSSQL_PORT: "1433"
  MONGODB_ENABLED: "true"
  MONGODB_PORT: "27017"
  REDIS_PROXY_ENABLED: "true"
  REDIS_PROXY_PORT: "6380"
  SQL_INJECTION_DETECTION: "true"
  MAX_CONNECTIONS: "1000"
  METRICS_PORT: "9090"
```

## 🐳 ArticDBM Proxy Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: articdbm-proxy
  namespace: articdbm
  labels:
    app: articdbm-proxy
    component: proxy
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app: articdbm-proxy
  template:
    metadata:
      labels:
        app: articdbm-proxy
        component: proxy
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      containers:
      - name: proxy
        image: articdbm/proxy:1.0.0
        imagePullPolicy: IfNotPresent
        ports:
        - name: mysql
          containerPort: 3306
          protocol: TCP
        - name: postgresql
          containerPort: 5432
          protocol: TCP
        - name: mssql
          containerPort: 1433
          protocol: TCP
        - name: mongodb
          containerPort: 27017
          protocol: TCP
        - name: redis-proxy
          containerPort: 6380
          protocol: TCP
        - name: metrics
          containerPort: 9090
          protocol: TCP
        env:
        - name: REDIS_ADDR
          value: "redis-service:6379"
        envFrom:
        - configMapRef:
            name: articdbm-config
        - secretRef:
            name: articdbm-secrets
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 9090
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health
            port: 9090
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        securityContext:
          runAsNonRoot: true
          runAsUser: 10001
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
      securityContext:
        fsGroup: 10001
        seccompProfile:
          type: RuntimeDefault
      terminationGracePeriodSeconds: 30
---
apiVersion: v1
kind: Service
metadata:
  name: articdbm-proxy-service
  namespace: articdbm
  labels:
    app: articdbm-proxy
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled: "true"
spec:
  type: LoadBalancer
  sessionAffinity: None
  selector:
    app: articdbm-proxy
  ports:
  - name: mysql
    port: 3306
    targetPort: 3306
    protocol: TCP
  - name: postgresql
    port: 5432
    targetPort: 5432
    protocol: TCP
  - name: mssql
    port: 1433
    targetPort: 1433
    protocol: TCP
  - name: mongodb
    port: 27017
    targetPort: 27017
    protocol: TCP
  - name: redis-proxy
    port: 6380
    targetPort: 6380
    protocol: TCP
---
apiVersion: v1
kind: Service
metadata:
  name: articdbm-proxy-metrics
  namespace: articdbm
  labels:
    app: articdbm-proxy
spec:
  type: ClusterIP
  selector:
    app: articdbm-proxy
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
```

## 🐍 ArticDBM Manager Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: articdbm-manager
  namespace: articdbm
  labels:
    app: articdbm-manager
    component: manager
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: articdbm-manager
  template:
    metadata:
      labels:
        app: articdbm-manager
        component: manager
    spec:
      containers:
      - name: manager
        image: articdbm/manager:1.0.0
        imagePullPolicy: IfNotPresent
        ports:
        - name: http
          containerPort: 8000
          protocol: TCP
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: articdbm-secrets
              key: database-url
        - name: SESSION_SECRET
          valueFrom:
            secretKeyRef:
              name: articdbm-secrets
              key: session-secret
        envFrom:
        - configMapRef:
            name: articdbm-config
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "1Gi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /api/health
            port: 8000
          initialDelaySeconds: 60
          periodSeconds: 30
          timeoutSeconds: 10
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /api/health
            port: 8000
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        securityContext:
          runAsNonRoot: true
          runAsUser: 10001
          readOnlyRootFilesystem: false
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
        volumeMounts:
        - name: tmp-volume
          mountPath: /tmp
      volumes:
      - name: tmp-volume
        emptyDir: {}
      securityContext:
        fsGroup: 10001
        seccompProfile:
          type: RuntimeDefault
      terminationGracePeriodSeconds: 30
---
apiVersion: v1
kind: Service
metadata:
  name: articdbm-manager-service
  namespace: articdbm
  labels:
    app: articdbm-manager
spec:
  type: ClusterIP
  selector:
    app: articdbm-manager
  ports:
  - name: http
    port: 8000
    targetPort: 8000
    protocol: TCP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: articdbm-manager-ingress
  namespace: articdbm
  annotations:
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/force-ssl-redirect: "true"
spec:
  tls:
  - hosts:
    - manager.articdbm.example.com
    secretName: articdbm-manager-tls
  rules:
  - host: manager.articdbm.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: articdbm-manager-service
            port:
              number: 8000
```

## 🔐 Secrets Configuration

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: articdbm-secrets
  namespace: articdbm
type: Opaque
data:
  # Base64 encoded values
  database-url: cG9zdGdyZXNxbDovL2FydGljZGJtOnBhc3N3b3JkQHBvc3RncmVzLXNlcnZpY2UvYXJ0aWNkYm0=
  session-secret: c3VwZXJzZWNyZXRrZXkxMjM0NTY3ODkw
  redis-password: ""
```

## 🔧 Supporting Services

### Redis Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: articdbm
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        args: ["redis-server", "--appendonly", "yes"]
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "256Mi"
            cpu: "200m"
        volumeMounts:
        - name: redis-data
          mountPath: /data
      volumes:
      - name: redis-data
        persistentVolumeClaim:
          claimName: redis-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: redis-service
  namespace: articdbm
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: redis-pvc
  namespace: articdbm
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

### PostgreSQL Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: articdbm
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:15-alpine
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_DB
          value: articdbm
        - name: POSTGRES_USER
          value: articdbm
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: password
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "1Gi"
            cpu: "500m"
        volumeMounts:
        - name: postgres-data
          mountPath: /var/lib/postgresql/data
          subPath: data
      volumes:
      - name: postgres-data
        persistentVolumeClaim:
          claimName: postgres-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: postgres-service
  namespace: articdbm
spec:
  selector:
    app: postgres
  ports:
  - port: 5432
    targetPort: 5432
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
  namespace: articdbm
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
---
apiVersion: v1
kind: Secret
metadata:
  name: postgres-secret
  namespace: articdbm
type: Opaque
data:
  password: cGFzc3dvcmQ=  # base64 encoded "password"
```

## 📊 Monitoring Setup

### ServiceMonitor for Prometheus

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: articdbm-proxy
  namespace: articdbm
  labels:
    app: articdbm-proxy
spec:
  selector:
    matchLabels:
      app: articdbm-proxy
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

### PodDisruptionBudget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: articdbm-proxy-pdb
  namespace: articdbm
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: articdbm-proxy
```

## 🔄 Horizontal Pod Autoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: articdbm-proxy-hpa
  namespace: articdbm
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: articdbm-proxy
  minReplicas: 3
  maxReplicas: 10
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
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 100
        periodSeconds: 30
```

## ☁️ Cloud Provider Specific Configurations

### AWS EKS

```yaml
# Additional annotations for AWS Load Balancer Controller
metadata:
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: "instance"
    service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled: "true"
    service.beta.kubernetes.io/aws-load-balancer-backend-protocol: "tcp"

# StorageClass for EBS
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: articdbm-storage
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  fsType: ext4
allowVolumeExpansion: true
```

### Google GKE

```yaml
# Additional annotations for GCP Load Balancer
metadata:
  annotations:
    cloud.google.com/load-balancer-type: "External"
    networking.gke.io/load-balancer-type: "External"

# StorageClass for GCE Persistent Disk
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: articdbm-storage
provisioner: pd.csi.storage.gke.io
parameters:
  type: pd-ssd
  replication-type: regional-pd
allowVolumeExpansion: true
```

### Azure AKS

```yaml
# Additional annotations for Azure Load Balancer
metadata:
  annotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: "false"
    service.beta.kubernetes.io/azure-pip-name: "articdbm-pip"

# StorageClass for Azure Disk
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: articdbm-storage
provisioner: disk.csi.azure.com
parameters:
  skuName: Premium_LRS
  cachingmode: ReadOnly
allowVolumeExpansion: true
```

## 🚀 Deployment Commands

### Apply All Resources

```bash
# Create namespace
kubectl apply -f namespace.yaml

# Apply configurations
kubectl apply -f configmap.yaml
kubectl apply -f secrets.yaml

# Deploy supporting services
kubectl apply -f postgres.yaml
kubectl apply -f redis.yaml

# Deploy ArticDBM components
kubectl apply -f articdbm-proxy.yaml
kubectl apply -f articdbm-manager.yaml

# Apply monitoring and autoscaling
kubectl apply -f monitoring.yaml
kubectl apply -f hpa.yaml
```

### Verify Deployment

```bash
# Check all pods
kubectl get pods -n articdbm

# Check services
kubectl get services -n articdbm

# Check ingress
kubectl get ingress -n articdbm

# View logs
kubectl logs -f deployment/articdbm-proxy -n articdbm
kubectl logs -f deployment/articdbm-manager -n articdbm

# Port forward for testing
kubectl port-forward service/articdbm-manager-service 8000:8000 -n articdbm
```

## 🔒 Security Best Practices

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: articdbm-network-policy
  namespace: articdbm
spec:
  podSelector:
    matchLabels:
      app: articdbm-proxy
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 3306
    - protocol: TCP
      port: 5432
  egress:
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
  - to: []
    ports:
    - protocol: TCP
      port: 53
    - protocol: UDP
      port: 53
```

### Pod Security Standards

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: articdbm
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

## 📈 Scaling Considerations

### Resource Planning

| Component | CPU Request | Memory Request | CPU Limit | Memory Limit |
|-----------|-------------|----------------|-----------|--------------|
| Proxy     | 100m        | 128Mi          | 500m      | 512Mi        |
| Manager   | 100m        | 256Mi          | 500m      | 1Gi          |
| Redis     | 50m         | 64Mi           | 200m      | 256Mi        |
| PostgreSQL| 100m        | 256Mi          | 500m      | 1Gi          |

### Capacity Planning

```yaml
# For 10,000 concurrent connections:
Proxy Replicas: 5-10
CPU per replica: 500m-1000m
Memory per replica: 512Mi-1Gi

# Storage requirements:
PostgreSQL PVC: 50-100Gi (configuration data)
Redis PVC: 5-10Gi (cache data)
```

## 🔧 Troubleshooting

### Common Issues

1. **Proxy pods not starting**
   ```bash
   kubectl describe pod -l app=articdbm-proxy -n articdbm
   kubectl logs deployment/articdbm-proxy -n articdbm
   ```

2. **Database connection failures**
   ```bash
   # Test connectivity
   kubectl exec -it deployment/articdbm-proxy -n articdbm -- nc -zv postgres-service 5432
   kubectl exec -it deployment/articdbm-proxy -n articdbm -- nc -zv redis-service 6379
   ```

3. **LoadBalancer not provisioning**
   ```bash
   # Check service events
   kubectl describe service articdbm-proxy-service -n articdbm
   
   # Check cloud provider specific annotations
   kubectl get service articdbm-proxy-service -n articdbm -o yaml
   ```

### Debug Commands

```bash
# Get cluster information
kubectl cluster-info

# Check node resources
kubectl top nodes

# Check pod resources
kubectl top pods -n articdbm

# Get events
kubectl get events -n articdbm --sort-by='.lastTimestamp'

# Debug networking
kubectl exec -it deployment/articdbm-proxy -n articdbm -- /bin/sh
```

## 🎯 Production Checklist

- [ ] Resource requests and limits configured
- [ ] Persistent storage provisioned
- [ ] Secrets properly configured
- [ ] Network policies applied
- [ ] Monitoring and alerting setup
- [ ] Horizontal Pod Autoscaler configured
- [ ] Pod Disruption Budgets defined
- [ ] Ingress/LoadBalancer configured
- [ ] SSL certificates provisioned
- [ ] Backup strategy implemented
- [ ] Disaster recovery plan documented

---

*For additional Kubernetes best practices, see the [official Kubernetes documentation](https://kubernetes.io/docs/concepts/).*