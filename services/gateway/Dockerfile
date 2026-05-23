# syntax=docker/dockerfile:1
FROM golang:1.25-bookworm@sha256:298734aec230b5f3e8cee450ce6d7eccc39f1797ba548ee90d57e9803030c6c3 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /gateway ./services/gateway/cmd/main.go

FROM debian:bookworm-slim@sha256:f9c6a2fd2ddbc23e336b6257a5245e31f996953ef06cd13a59fa0a1df2d5c252
RUN groupadd -r nest && useradd -r -g nest -u 1000 nest
COPY --from=builder /gateway /usr/local/bin/gateway
USER nest
EXPOSE 50052 8082
ENTRYPOINT ["/usr/local/bin/gateway"]
