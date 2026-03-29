# Multi-stage build for NEST Manager service
# Stage 1: Builder - Install dependencies
FROM python:3.13-slim-bookworm@sha256:01f42367a0a94ad4bc17111776fd66e3500c1d87c15bbd6055b7371d39c124fb AS builder

# Set working directory
WORKDIR /app

# Install build dependencies required for psycopg2 and cryptography
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libpq-dev \
    libssl-dev \
    libffi-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python dependencies
COPY requirements.txt .
RUN pip install --user --no-cache-dir --use-deprecated=legacy-resolver -r requirements.txt

# Stage 2: Runtime - Minimal production image
FROM python:3.13-slim-bookworm@sha256:01f42367a0a94ad4bc17111776fd66e3500c1d87c15bbd6055b7371d39c124fb

# Set metadata labels
LABEL maintainer="Penguin Tech Inc"
LABEL description="NEST Manager Service - Infrastructure orchestration and provisioning"

# Set working directory
WORKDIR /app

# Install runtime dependencies only (minimal footprint)
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libpq5 \
    libssl3 \
    libffi8 \
    curl \
    tini \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN useradd -m -u 1000 -s /bin/bash manager

# Development stage with hot reload
FROM python:3.13-slim-bookworm@sha256:01f42367a0a94ad4bc17111776fd66e3500c1d87c15bbd6055b7371d39c124fb AS development

# Set metadata labels
LABEL maintainer="Penguin Tech Inc"
LABEL description="NEST Manager Service - Development with hot reload"

# Set working directory
WORKDIR /app

# Install development dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    libpq-dev \
    libssl-dev \
    libffi-dev \
    curl \
    tini \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Create non-root user for security
RUN useradd -m -u 1000 -s /bin/bash manager

# Copy application code
COPY --chown=manager:manager . .

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONPATH=/app

# Switch to non-root user
USER manager

# Expose API port
EXPOSE 8000

# Health check configuration
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/healthz || exit 1

# Use tini as init process for proper signal handling
ENTRYPOINT ["/usr/bin/tini", "--"]

# Start application with hot reload
CMD ["python3", "app.py"]

# Production stage - Minimal production image
FROM python:3.13-slim-bookworm@sha256:01f42367a0a94ad4bc17111776fd66e3500c1d87c15bbd6055b7371d39c124fb AS production

# Set metadata labels
LABEL maintainer="Penguin Tech Inc"
LABEL description="NEST Manager Service - Infrastructure orchestration and provisioning"

# Set working directory
WORKDIR /app

# Install runtime dependencies only (minimal footprint)
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libpq5 \
    libssl3 \
    libffi8 \
    curl \
    tini \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN useradd -m -u 1000 -s /bin/bash manager

# Copy Python runtime from builder stage
COPY --from=builder /root/.local /home/manager/.local
ENV PATH="/home/manager/.local/bin:$PATH" \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

# Copy application code
COPY --chown=manager:manager app.py .
COPY --chown=manager:manager db_init.py .
COPY --chown=manager:manager models/ ./models/
COPY --chown=manager:manager db_models/ ./db_models/
COPY --chown=manager:manager controllers/ ./controllers/
COPY --chown=manager:manager workers/ ./workers/
COPY --chown=manager:manager routes/ ./routes/
COPY --chown=manager:manager utils/ ./utils/
COPY --chown=manager:manager clients/ ./clients/
COPY --chown=manager:manager templates/ ./templates/

# Switch to non-root user
USER manager

# Expose API port
EXPOSE 5000

# Health check configuration
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:5000/healthz || exit 1

# Use tini as init process for proper signal handling
ENTRYPOINT ["/usr/bin/tini", "--"]

# Start application
CMD ["python3", "app.py"]
