# syntax=docker/dockerfile:1
FROM golang:1.25-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o gateway ./services/gateway/cmd/main.go

FROM debian:bookworm-slim
RUN groupadd -r nest && useradd -r -g nest nest
COPY --from=builder /app/gateway /usr/local/bin/gateway
USER nest
EXPOSE 50052 8082
ENTRYPOINT ["/usr/local/bin/gateway"]
