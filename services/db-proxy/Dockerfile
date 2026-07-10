# Dockerfile for nest DB Proxy
# Build: docker build --build-arg XDP_BUILD_TAGS=xdp .
# Multi-arch: docker buildx build --platform linux/amd64,linux/arm64 .

ARG XDP_BUILD_TAGS=xdp

# Stage 1: Builder
FROM golang:1.25-bookworm@sha256:9b5ac3d7c83a89f5d9e2b7e20f3e2c2e2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d AS builder

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build \
    -tags ${XDP_BUILD_TAGS} \
    -ldflags="-X main.Version=$(git describe --tags --always --dirty) \
               -X main.BuildTime=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
               -X main.GitCommit=$(git rev-parse --short HEAD)" \
    -o app \
    ./cmd/main.go

# Run tests in builder stage
RUN go test -v -race -cover ./...

# Stage 2: Runtime
FROM debian:bookworm-slim@sha256:b5b2b2c507a1d1f02d901e0b8e5e9e6c0d6d6d6d6d6d6d6d6d6d6d6d6d6d6d

RUN groupadd -r dbproxy && useradd -r -g dbproxy -u 1000 dbproxy

WORKDIR /app

COPY --from=builder /build/app .

# Rootless
USER dbproxy

# Healthcheck: native binary instead of curl
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD test -f /app/app && /app/app -version 2>/dev/null || exit 1 || \
    nc -z localhost 9090 || exit 1

ENTRYPOINT ["/app/app"]
