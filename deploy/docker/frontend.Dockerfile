# Iris frontend image.
#
# Build context is the repository root, e.g.:
#   docker build -f deploy/docker/frontend.Dockerfile -t iris-frontend .
#
# Serves the built SPA on :80 and proxies /v1 and /openapi.yaml to the backend.
# Override the backend upstream at runtime:
#   docker run -e IRIS_BACKEND_UPSTREAM=http://iris-backend:8080 iris-frontend

# ---- builder ----
FROM node:22 AS builder

WORKDIR /app

# Install dependencies from the lockfile first for better caching.
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci

# Build the production bundle into dist/.
COPY frontend/ ./
RUN npm run build

# ---- final ----
FROM nginx:alpine AS final

# Default backend upstream; override with -e IRIS_BACKEND_UPSTREAM=...
ENV IRIS_BACKEND_UPSTREAM=http://iris-backend:8080

# Nginx template; envsubst expands ${IRIS_BACKEND_UPSTREAM} at container start.
COPY deploy/docker/frontend.nginx.conf /etc/nginx/templates/default.conf.template

# Built static assets.
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80

# Default nginx:alpine entrypoint renders /etc/nginx/templates/*.template and
# then runs nginx in the foreground.
