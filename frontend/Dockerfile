# syntax=docker/dockerfile:1.7

# ── build stage ──────────────────────────────────────────────────────────────
# Pinned to Node 20 to match .node-version (Vben Admin v5 baseline).
FROM node:20-alpine AS build

# pnpm via corepack — version pinned via package.json's packageManager field
# (or use the active pnpm in the image; corepack enables it).
RUN corepack enable

WORKDIR /src

# Copy workspace manifests first to maximise the dependency cache layer.
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml turbo.json ./
COPY .npmrc ./
COPY internal/ ./internal/
COPY packages/ ./packages/
COPY apps/ ./apps/
COPY scripts/ ./scripts/

# HUSKY=0 skips the prepare hook (no git hooks needed in the image).
ENV HUSKY=0
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

# IMPORTANT: must use turbo-orchestrated build (pnpm build:antd) — the
# direct `pnpm --filter @vben/web-antd build` bypasses workspace-dep builds
# and leaves Vben's internal packages with `unbuild --stub` artifacts that
# import `jiti`. jiti's runtime ESM transpiler imports node:module, which
# rollup then tries to bundle into the browser and fails:
#
#   jiti.mjs:1:9 "createRequire" is not exported by "__vite-browser-external"
#
# pnpm build:antd → turbo build --filter=@vben/web-antd : builds every
# upstream workspace package first so dist/index.mjs is real Lua-clean ESM
# rather than a jiti stub.
ENV NODE_OPTIONS="--max-old-space-size=8192"
RUN pnpm build:antd

# ── runtime stage ────────────────────────────────────────────────────────────
FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/admin/dist /usr/share/nginx/html

EXPOSE 80
