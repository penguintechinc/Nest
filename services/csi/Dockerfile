# syntax=docker/dockerfile:1
ARG VERSION=dev

FROM golang:1.24-bookworm@sha256:1a6d4452c65dea36aac2e2d606b01b4a029ec90cc1ae53890540ce6173ea77ac AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /nest-csi \
    ./services/csi/

FROM debian:bookworm-slim@sha256:f9c6a2fd2ddbc23e336b6257a5245e31f996953ef06cd13a59fa0a1df2d5c252
RUN groupadd -r appuser && useradd -r -g appuser -u 1000 appuser
# CSI plugin communicates via Unix socket (mounted by K8s)
COPY --from=builder /nest-csi /nest-csi
USER appuser
CMD ["/nest-csi"]
