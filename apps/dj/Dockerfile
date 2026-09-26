# syntax=docker/dockerfile:1.7
# Single linux/arm64 application image: ghcr.io/lithqube/klubhub-dj-api.
FROM --platform=linux/arm64 node:22-bookworm-slim AS frontend-builder
WORKDIR /repo
RUN npm install -g pnpm@12.4.1
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/dj/package.json apps/dj/
COPY libs/ui/package.json libs/ui/
RUN pnpm install --frozen-lockfile
COPY . .
# The root solution tsconfig references every app, and the build's TS
# transform follows those references into apps whose generated
# .nuxt/tsconfig.json never exists here (only DJ is built). Keep just the
# DJ reference for this build.
RUN node -e "const f='tsconfig.json',t=JSON.parse(require('fs').readFileSync(f)); \
    t.references=t.references.filter(r=>r.path==='./apps/dj'); \
    require('fs').writeFileSync(f,JSON.stringify(t,null,2))"
# Nitro's production API proxy is baked into the bundle. Never point it at
# the public ingress: the Go API owns /api/v1 and proxies everything else.
ENV NODE_ENV=production \
    NUXT_PUBLIC_API_BASE=http://127.0.0.1:8080
RUN node apps/dj/scripts/build-prod.mjs
# Use exactly the browser revision expected by the bundled playwright-core,
# not an independently resolved npx playwright version.
RUN cp -aL apps/dj/node_modules/playwright-core /opt/playwright-core

FROM --platform=linux/arm64 golang:1.26-alpine AS api-builder
WORKDIR /repo/api
RUN apk add --no-cache git ca-certificates
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
    go build -trimpath -ldflags='-s -w -X main.productName=KlubHub-DJ -X main.version=1.1.0' -o /out/api ./cmd/api

# Retain Node, Chromium, its Debian shared libraries and fonts at runtime.
# Browser installation is mandatory: never silently ship disabled rendering.
FROM --platform=linux/arm64 node:22-bookworm-slim AS runtime
ENV NODE_ENV=production \
    PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright \
    SERVE_FRONTEND=true \
    NUXT_INTERNAL_URL=http://127.0.0.1:3000 \
    BIND_ADDRESS=0.0.0.0 \
    PORT=8080
COPY --from=frontend-builder /opt/playwright-core /tmp/playwright-core
RUN apt-get update && apt-get install -y --no-install-recommends tini ca-certificates fonts-noto fonts-noto-cjk \
    && node /tmp/playwright-core/cli.js install --with-deps chromium \
    && chmod -R a+rX /opt/ms-playwright \
    && rm -rf /tmp/playwright-core /var/lib/apt/lists/*
COPY --from=api-builder /out/api /api
COPY --from=frontend-builder /repo/apps/dj/.output /opt/frontend
COPY apps/dj/scripts/runtime.mjs /opt/runtime.mjs
WORKDIR /opt/frontend
USER node
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=6s --start-period=60s --retries=3 CMD ["/api", "-healthcheck"]
ENTRYPOINT ["/usr/bin/tini", "--", "node", "/opt/runtime.mjs"]
