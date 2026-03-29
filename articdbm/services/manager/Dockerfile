FROM python:3.13-slim-bookworm@sha256:01f42367a0a94ad4bc17111776fd66e3500c1d87c15bbd6055b7371d39c124fb

RUN apt-get update && apt-get install -y \
    gcc \
    postgresql-client \
    libpq-dev \
    libxml2-dev \
    libxslt1-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

ENV DATABASE_URL="postgresql://articdbm:articdbm@postgres/articdbm" \
    REDIS_HOST="redis" \
    REDIS_PORT="6379" \
    REDIS_PASSWORD="" \
    REDIS_DB="0" \
    SESSION_SECRET="" \
    PY4WEB_APPS="/app"

EXPOSE 8000

CMD ["python3", "-m", "py4web", "run", "/app", "--host", "0.0.0.0", "--port", "8000"]