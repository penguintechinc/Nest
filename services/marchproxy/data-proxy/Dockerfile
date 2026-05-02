FROM golang:1.25-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o data-proxy ./cmd

FROM debian:bookworm-slim
RUN groupadd -r appuser && useradd -r -g appuser appuser
COPY --from=builder /app/data-proxy /usr/local/bin/data-proxy
USER appuser
EXPOSE 50053 8083
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD ["/usr/local/bin/data-proxy", "-health"]
ENTRYPOINT ["/usr/local/bin/data-proxy"]
