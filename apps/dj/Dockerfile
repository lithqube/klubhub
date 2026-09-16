# KlubHub v1.0.0 single image — linux/arm64 only.
#
# Stages:
#   1. frontend-builder — installs Node 22 + Playwright Chromium, builds the
#      Nuxt frontend into apps/dj/.output. NODE_ENV=production and
#      NUXT_PUBLIC_API_BASE=http://127.0.0.1:8080 are required by
#      apps/dj/scripts/build-prod.mjs so the in-tree mock handlers are
#      physically excluded from the production bundle.
#   2. api-builder — statically compiles the Go API for linux/arm64.
#   3. runtime — distroless static + nonroot. Carries both /api and the Nuxt
#      .output/. ENTRYPOINT is /api; the API also serves the embedded frontend
#      on NUXT_INTERNAL_URL (loopback by default).
#
# The image is pushed as `ghcr.io/<owner>/<repo>:<tag>` (a single package
# derived from the apps/dj project). It is not multi-arch.
FROM --platform=linux/arm64 node:22-bookworm-slim AS frontend-builder
WORKDIR /repo

# System fonts + Chromium shared library dependencies for Playwright
# (rendering tracklist cover art). The runtime stage copies the rendered
# assets, so these are only needed during the build.
RUN apt-get update && apt-get install -y --no-install-recommends \
    fonts-noto fonts-noto-cjk \
    libnss3 libatk1.0-0 libatk-bridge2.0-0 libcups2 libdrm2 libxkbcommon0 \
    libxcomposite1 libxdamage1 libxrandr2 libgbm1 libasound2 libpangocairo-1.0-0 \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g pnpm@12.4.1

# Workspace install — copy the manifests first so this layer caches.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/dj/package.json apps/dj/
RUN pnpm install --frozen-lockfile

# Playwright Chromium binary is staged during the build so the runtime
# image does not need to download it on first start. The plugin degrades
# gracefully (warns + sets _playwright=null) when no browser is present,
# so a missing browser only disables screenshot routes — it does not
# break boot.
RUN cd apps/dj && npx playwright install chromium --with-deps || \
    echo '[warn] Playwright Chromium download failed; screenshot routes will be disabled.'

COPY . .

# Production build: scripts/build-prod.mjs renames apps/dj/server/api/v1/
# outside server/ before launching `nuxt build`, then restores it. The
# wrapper also refuses to run without NUXT_PUBLIC_API_BASE.
ENV NODE_ENV=production
ENV NUXT_PUBLIC_API_BASE=http://127.0.0.1:8080
RUN node apps/dj/scripts/build-prod.mjs

FROM --platform=linux/arm64 golang:1.26-alpine AS api-builder
WORKDIR /repo/api
RUN apk add --no-cache git ca-certificates
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -trimpath -ldflags='-s -w -X main.version=1.0.0' -o /out/api ./cmd/api

# Runtime. Pinned by digest to the linux/arm64/v8 manifest. Re-resolve via
# `docker buildx imagetools inspect gcr.io/distroless/static-debian12:nonroot`
# if the pin breaks. USER nonroot is implicit via :nonroot.
FROM --platform=linux/arm64 \
    gcr.io/distroless/static-debian12:nonroot@sha256:06c3c14b7fa252d9f4285242d2d665ce3b51f74b02d3c529464f220f2912aecb AS runtime
COPY --from=api-builder /out/api /api
COPY --from=frontend-builder /repo/apps/dj/.output /opt/frontend
ENV NUXT_INTERNAL_URL=http://127.0.0.1:3000
ENV BIND_ADDRESS=0.0.0.0
EXPOSE 8080
ENTRYPOINT ["/api"]
