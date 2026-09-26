# syntax=docker/dockerfile:1.7
# KlubHub Promoter application image: ghcr.io/lithqube/klubhub-promoter-api
# (linux/amd64 + linux/arm64). One container runs the Go API (public :8080)
# and the Nuxt server (127.0.0.1:3000) under tini; Go proxies non-API paths.
FROM node:22-bookworm-slim AS frontend-builder
WORKDIR /repo
RUN npm install -g pnpm@12.4.1
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/promoter/package.json apps/promoter/
COPY libs/ui/package.json libs/ui/
RUN pnpm install --frozen-lockfile
COPY . .
# Nitro's production API proxy is baked into the bundle and always targets
# the in-container Go API, never a public address.
ENV NODE_ENV=production \
    NUXT_PUBLIC_API_BASE=http://127.0.0.1:8080
RUN node apps/promoter/scripts/build-prod.mjs

FROM golang:1.26-alpine AS api-builder
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=0.1.0
WORKDIR /repo/api
RUN apk add --no-cache git ca-certificates
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/promoter ./cmd/promoter

FROM node:22-bookworm-slim AS runtime
# PROMOTER_PORT is also read by `promoter healthcheck` (HEALTHCHECK runs
# outside the supervised process tree).
ENV NODE_ENV=production \
    PROMOTER_PORT=8080
RUN apt-get update && apt-get install -y --no-install-recommends tini ca-certificates \
    && rm -rf /var/lib/apt/lists/*
# The runtime only execs node and /promoter. Drop the base image's bundled
# npm and corepack: unused here, and their dependencies carry fixable CVEs
# that fail the Trivy gate.
RUN rm -rf /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/corepack \
    /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/corepack
COPY --from=api-builder /out/promoter /promoter
COPY --from=frontend-builder /repo/apps/promoter/.output /opt/frontend
COPY apps/promoter/scripts/runtime.mjs /opt/runtime.mjs
WORKDIR /opt/frontend
USER node
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=6s --start-period=30s --retries=3 \
    CMD ["/promoter", "healthcheck"]
ENTRYPOINT ["/usr/bin/tini", "--", "node", "/opt/runtime.mjs"]
