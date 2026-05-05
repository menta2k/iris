# syntax=docker/dockerfile:1.7
#
# Build context: REPO ROOT (not backend/). This image bundles the Vue 3
# admin SPA into the Go binary via //go:embed, so we need both subtrees:
#
#   stage 1  ── pnpm build of frontend/apps/admin → dist/
#   stage 2  ── go build of backend/, with stage 1's dist/ copied into
#               backend/pkg/spa/dist/ before compilation
#   stage 3  ── distroless runtime; just the binary + configs + sql
#
# Single output image runs the admin API on :8000 (HTTP, also serves the
# embedded SPA) and :9000 (gRPC). No separate frontend container needed
# in production.

# ── stage 1: frontend ─────────────────────────────────────────────────
FROM node:20-alpine AS frontend
RUN corepack enable
WORKDIR /src/frontend

# Manifests first → maximise the dependency-cache layer.
COPY frontend/package.json frontend/pnpm-workspace.yaml frontend/pnpm-lock.yaml frontend/turbo.json frontend/.npmrc ./
COPY frontend/internal/ ./internal/
COPY frontend/packages/ ./packages/
COPY frontend/apps/ ./apps/
COPY frontend/scripts/ ./scripts/

ENV HUSKY=0
ENV NODE_OPTIONS="--max-old-space-size=8192"
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

# pnpm build:antd → turbo build --filter=@vben/web-antd : builds every
# upstream workspace package first (otherwise jiti's runtime ESM stub
# leaks into the bundle and rollup chokes on `createRequire`).
RUN pnpm build:antd

# Output sanity: fail loudly here rather than ship a half-empty embed.
RUN test -f /src/frontend/apps/admin/dist/index.html || \
    (echo "FATAL: frontend build produced no index.html"; ls -la /src/frontend/apps/admin/dist || true; exit 1)

# ── stage 2: backend ──────────────────────────────────────────────────
FROM golang:1.25 AS backend
WORKDIR /src/backend

# IRIS_VERSION is set by CI from the git tag (e.g. v1.2.3). Local
# builds without --build-arg fall back to the build timestamp so
# `iris_build_info{version=...}` always carries a value.
ARG IRIS_VERSION=

# Module deps before source (caches well).
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY backend/ ./

# Drop the placeholder + copy real frontend dist into the embed path.
# rm -rf removes both PLACEHOLDER and any prior stale build; the COPY
# repopulates the directory fresh from stage 1.
RUN rm -rf ./pkg/spa/dist && mkdir -p ./pkg/spa/dist
COPY --from=frontend /src/frontend/apps/admin/dist/ ./pkg/spa/dist/

# Static binary; -trimpath for reproducibility, -s -w to strip symbols.
# version: prefer the build-arg (semver from a git tag); fall back to
# the build timestamp so untagged dev images still get a sensible
# value in the iris_build_info metric.
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    sh -c 'VERSION="${IRIS_VERSION:-$(date +%Y%m%d-%H%M%S)}" && \
           CGO_ENABLED=0 GOOS=linux \
             go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
             -o /out/admin-service ./app/admin/service/cmd/server && \
           echo "${VERSION}" > /out/version.txt'

# ── stage 3: runtime ──────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

ARG IRIS_VERSION=
# Plumb the version into runtime ENV so providers.NewMetrics() picks
# it up and stamps iris_build_info{version=...}. Two surfaces — the
# linker -X main.version (compile-time) and this env var (runtime
# read in metrics provider) — kept in sync from the same build-arg.
ENV IRIS_BUILD_VERSION=${IRIS_VERSION}

WORKDIR /app

COPY --from=backend /out/admin-service /app/admin-service
COPY --from=backend /out/version.txt /app/version.txt
COPY backend/app/admin/service/configs /app/configs
COPY backend/sql /app/sql

# Distroless `nonroot` is UID 65532; the binary only listens on
# unprivileged ports (8000 HTTP + UI, 9000 gRPC).
USER nonroot:nonroot

EXPOSE 8000 9000

ENTRYPOINT ["/app/admin-service", "--conf", "/app/configs"]
