FROM node:22-alpine AS builder
WORKDIR /app
RUN npm install -g pnpm
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/dj/package.json apps/dj/
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm nx build dj

FROM node:22-alpine
WORKDIR /app
COPY --from=builder /app/apps/dj/.output ./.output
ENV PORT=3000
EXPOSE 3000
CMD ["node", ".output/server/index.mjs"]
