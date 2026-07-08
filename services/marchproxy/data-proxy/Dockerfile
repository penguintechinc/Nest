FROM golang:1.25-bookworm@sha256:a9c020ee3d1508c7be5435c262434e3d3fc1d0e76a11afeb9ddae7d60bc86aa4 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o data-proxy ./cmd

FROM debian:bookworm-slim@sha256:60eac759739651111db372c07be67863818726f754804b8707c90979bda511df
RUN groupadd -r appuser && useradd -r -g appuser appuser
COPY --from=builder /app/data-proxy /usr/local/bin/data-proxy
USER appuser
EXPOSE 50053 8083
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD ["/usr/local/bin/data-proxy", "-health"]
ENTRYPOINT ["/usr/local/bin/data-proxy"]
