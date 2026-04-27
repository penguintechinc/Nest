# TODO: replace with SHA256 digest: docker inspect python:3.13-slim-bookworm | grep -i digest
FROM python:3.13-slim-bookworm AS builder
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

FROM python:3.13-slim-bookworm
RUN groupadd -r appuser && useradd -r -g appuser appuser
WORKDIR /app
COPY --from=builder /usr/local/lib/python3.13/site-packages /usr/local/lib/python3.13/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin
COPY . .
USER appuser
EXPOSE 8080 50051
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD python3 -c "import socket; socket.create_connection(('localhost', 8080)).close()" || exit 1
ENTRYPOINT ["python3", "main.py"]
