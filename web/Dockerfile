FROM node:18-bookworm-slim@sha256:f9ab18e354e6855ae56ef2b290dd225c1e51a564f87584b9bd21dd651838830e AS builder
WORKDIR /app
COPY package*.json ./
COPY .npmrc* ./
RUN npm ci
COPY . .
RUN npm run build

# Development stage with Vite dev server
FROM node:18-bookworm-slim@sha256:f9ab18e354e6855ae56ef2b290dd225c1e51a564f87584b9bd21dd651838830e AS development
WORKDIR /app
COPY package*.json ./
COPY .npmrc* ./
RUN npm ci
COPY . .
EXPOSE 3000
CMD ["npm", "run", "dev"]

# Production stage - rootless nginx
FROM nginx:stable-bookworm@sha256:552e7481ca93ffccd046aa658dbbed22caefbc09c66fa7cd247cbb90b8a5c609 AS production

# Create non-root user and fix permissions
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r -g 1000 appuser \
    && useradd -r -u 1000 -g appuser appuser \
    && mkdir -p /var/run/nginx /var/cache/nginx /tmp/nginx \
    && chown -R appuser:appuser /var/run/nginx /var/cache/nginx /tmp/nginx /var/log/nginx /usr/share/nginx/html

# Custom nginx config for rootless operation on port 8080
RUN cat > /etc/nginx/nginx.conf <<'EOF'
user appuser;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /tmp/nginx/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';

    access_log /var/log/nginx/access.log main;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    client_max_body_size 20M;

    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css text/xml text/javascript
               application/json application/javascript application/xml+rss;

    server {
        listen 8080;
        listen [::]:8080;
        server_name _;

        root /usr/share/nginx/html;
        index index.html;

        location / {
            try_files $uri $uri/ /index.html;
        }

        location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
            expires 1y;
            add_header Cache-Control "public, immutable";
        }

        error_page 404 /index.html;
        error_page 500 502 503 504 /index.html;
    }
}
EOF

COPY --from=builder /app/dist /usr/share/nginx/html

# Ensure proper ownership of served files
RUN chown -R appuser:appuser /usr/share/nginx/html

EXPOSE 8080

USER appuser

CMD ["nginx", "-g", "daemon off;"]
