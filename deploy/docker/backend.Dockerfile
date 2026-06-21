# Iris backend image, with the frontend SPA embedded into the binary.
#
# Build context is the repository root, e.g.:
#   docker build -f deploy/docker/backend.Dockerfile -t iris-backend .
#
# Serves HTTP on :8080 (API + embedded SPA) and gRPC on :9090.

# ---- frontend builder ----
FROM node:22 AS frontend

WORKDIR /web

# Cache npm install on the lockfile.
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

# Then the rest of the frontend source, and build the static SPA.
COPY frontend/ ./
RUN npm run build

# ---- backend builder ----
FROM golang:1.25 AS builder

WORKDIR /src

# Cache module downloads first.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Then the rest of the backend source.
COPY backend/ ./

# Embed the built SPA: the embed_ui build tag compiles internal/webui/dist in.
COPY --from=frontend /web/dist ./internal/webui/dist

# Build a static binary so it runs on distroless/base.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -tags embed_ui -ldflags="-s -w" -o /out/iris ./cmd/iris

# ---- final ----
FROM gcr.io/distroless/base-debian12 AS final

WORKDIR /app

# Application binary and default configuration.
COPY --from=builder /out/iris /app/iris
COPY --from=builder /src/configs /app/configs

EXPOSE 8080 9090

ENTRYPOINT ["/app/iris"]
