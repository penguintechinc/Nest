# syntax=docker/dockerfile:1
ARG VERSION=dev

FROM golang:1.25-bookworm@sha256:298734aec230b5f3e8cee450ce6d7eccc39f1797ba548ee90d57e9803030c6c3 AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /nest-node-agent \
    ./services/node-agent/

FROM debian:bookworm-slim@sha256:f9c6a2fd2ddbc23e336b6257a5245e31f996953ef06cd13a59fa0a1df2d5c252
RUN apt-get update && apt-get install -y --no-install-recommends \
    util-linux \
    smartmontools \
    && rm -rf /var/lib/apt/lists/*
RUN groupadd -r appuser && useradd -r -g appuser -u 1000 appuser
# K8s DaemonSet manifest handles privilege escalation for /dev and /sys access
# The container process itself runs as UID 1000 (appuser) for security
COPY --from=builder /nest-node-agent /nest-node-agent
USER appuser
EXPOSE 9091
CMD ["/nest-node-agent"]
