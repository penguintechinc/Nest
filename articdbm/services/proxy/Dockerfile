FROM golang:1.24-bookworm@sha256:1a6d4452c65dea36aac2e2d606b01b4a029ec90cc1ae53890540ce6173ea77ac AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    git ca-certificates build-essential && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -a -o articdbm-proxy .

FROM debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /root/

COPY --from=builder /app/articdbm-proxy .

ENV PROXY_PORT=8080 \
    METRICS_PORT=9090 \
    REDIS_ADDR=redis:6379 \
    MYSQL_ENABLED=true \
    MYSQL_PORT=3306 \
    POSTGRESQL_ENABLED=true \
    POSTGRESQL_PORT=5432 \
    MSSQL_ENABLED=true \
    MSSQL_PORT=1433 \
    MONGODB_ENABLED=true \
    MONGODB_PORT=27017 \
    REDIS_PROXY_ENABLED=true \
    REDIS_PROXY_PORT=6379 \
    SQL_INJECTION_DETECTION=true \
    MAX_CONNECTIONS=1000 \
    CONNECTION_TIMEOUT=30 \
    QUERY_TIMEOUT=60 \
    TLS_ENABLED=false \
    CLUSTER_MODE=false \
    XDP_ENABLED=true \
    XDP_INTERFACE=eth0 \
    XDP_RATE_LIMIT_PPS=100000000 \
    XDP_BURST_LIMIT=10000 \
    AFXDP_ENABLED=true \
    AFXDP_BATCH_SIZE=64 \
    XDP_CACHE_SIZE=1048576 \
    XDP_CACHE_TTL=300

EXPOSE 3306 5432 1433 27017 6379 8080 9090

CMD ["./articdbm-proxy"]