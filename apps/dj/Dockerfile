# Stage 1: builder — install fonts, Chromium deps, Playwright binary, then build Nuxt
FROM node:22-bookworm-slim AS builder
WORKDIR /app

# System fonts and Chromium shared library dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    fonts-noto \
    fonts-noto-cjk \
    libnss3 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libxkbcommon0 \
    libxcomposite1 \
    libxdamage1 \
    libxrandr2 \
    libgbm1 \
    libasound2 \
    libpangocairo-1.0-0 \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g pnpm

# Install workspace dependencies (pnpm workspace aware)
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/dj/package.json apps/dj/
RUN pnpm install --frozen-lockfile

# Install Playwright Chromium at build time (not at runtime — avoids 60s cold start)
RUN cd apps/dj && npx playwright install chromium

# Copy source and build
COPY . .
RUN pnpm nx build dj

# Stage 2: runtime — minimal image with fonts, Chromium, and built output
FROM node:22-bookworm-slim
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    fonts-noto \
    fonts-noto-cjk \
    libnss3 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libxkbcommon0 \
    libxcomposite1 \
    libxdamage1 \
    libxrandr2 \
    libgbm1 \
    libasound2 \
    libpangocairo-1.0-0 \
    && rm -rf /var/lib/apt/lists/*

# Copy Nuxt build output and Playwright Chromium binary
COPY --from=builder /app/apps/dj/.output ./.output
COPY --from=builder /root/.cache/ms-playwright /root/.cache/ms-playwright

ENV PORT=3000
ENV NODE_ENV=production
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
