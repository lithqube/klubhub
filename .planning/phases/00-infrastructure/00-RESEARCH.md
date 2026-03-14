# Phase 0: Infrastructure - Research

**Researched:** 2026-03-14
**Domain:** Docker Compose orchestration, Go module scaffold, PostgreSQL 16 with embedded migrations, MinIO object storage, Nuxt 4 reverse proxy configuration
**Confidence:** HIGH

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| INFRA-01 | System deploys as four Docker Compose services (frontend, api, db, storage) with a single `docker compose up -d` | Full docker-compose.yml structure documented in ARCHITECTURE.md §9.1; service dependency ordering via `depends_on: condition: service_healthy` |
| INFRA-02 | All services bind to `127.0.0.1` by default; configurable via `BIND_ADDRESS` env var | Port binding pattern `${BIND_ADDRESS:-127.0.0.1}:3000:3000` confirmed in ARCHITECTURE.md §9.1 |
| INFRA-03 | Go API runs database migrations automatically on startup using embedded goose migrations | pressly/goose v3 with `embed.FS` pattern documented; API exits non-zero if migrations fail |
| INFRA-04 | `GET /api/v1/health` returns per-integration status: database, storage, Spotify, Discogs, MusicBrainz, Instagram | Health response shape and `< 500ms` requirement confirmed in ARCHITECTURE.md §6.3 |
| INFRA-05 | API starts and degrades gracefully when optional API keys are missing; only PostgreSQL and MinIO are hard dependencies | Graceful degradation pattern identified; optional integrations report status but do not block startup |
| INFRA-06 | Nuxt 4 frontend proxies all `/api/v1/*` requests to Go backend | Nitro `routeRules` + `devProxy` pattern confirmed for Nuxt 4; `nuxt.config.ts` currently has no proxy — must be added |
| INFRA-07 | `user_settings` singleton table stores DJ name, logo path, default colors, default template, visible fields, social links, bio, contact info | Full schema in ARCHITECTURE.md §5.2 ERD |
| INFRA-08 | `scripts/backup.sh` and `scripts/restore.sh` execute without error and produce/restore a valid data snapshot | pg_dump + mc mirror approach documented in ARCHITECTURE.md §5.6 |
| INFRA-09 | Social media OAuth tokens stored encrypted with AES-256-GCM using `TOKEN_ENCRYPTION_KEY` | stdlib `crypto/aes` + `crypto/cipher` pattern confirmed; key generated via `openssl rand -hex 32` |
| INFRA-10 | All entities use UUID primary keys, `created_at`/`updated_at` timestamps, and soft deletion via `deleted_at` | Confirmed across all entities in ARCHITECTURE.md ERD; pgx v5 uses `github.com/google/uuid` |
| INFRA-11 | All entity update (PUT) endpoints implement optimistic concurrency control: client sends `updated_at`, server returns HTTP 409 on conflict | Pattern confirmed in ARCHITECTURE.md §4.5 as FR-046a requirement |
</phase_requirements>

---

## Summary

Phase 0 scaffolds the complete four-service Docker Compose stack that all subsequent modules build on. The architecture is fully specified in `docs/ARCHITECTURE.md` v1.0 — this phase is primarily implementation of a known, well-documented design, not design work. The Go backend (`api/` directory) does not yet exist and must be created from scratch as a new Go module alongside the existing Nuxt 4 monorepo scaffold at `apps/dj/`.

The critical implementation decisions are already locked: chi v5 for HTTP routing, pgx v5 for PostgreSQL, pressly/goose v3 for embedded migrations, minio-go v7 for object storage, zerolog v1 for structured logging, and envconfig v2 for configuration. The Nuxt 4 frontend requires a proxy configuration addition to `nuxt.config.ts` — the current file has no `routeRules` or `devProxy` for `/api/v1/*`. The MinIO `MINIO_PUBLIC_ENDPOINT` env var must be established from day one to avoid the internal Docker hostname pitfall in presigned URL generation.

The initial database schema (migration `001_initial_schema.up.sql`) must cover only the tables needed in Phase 0: `user_settings` (INFRA-07), the AES-256-GCM encrypted `social_accounts` structure stub (INFRA-09), and the UUID/timestamp/soft-delete baseline pattern (INFRA-10) that all future migration files inherit. Future module tables are added in their respective phases.

**Primary recommendation:** Create `api/` as a standalone Go module (`go.mod` at `api/go.mod`), scaffolded per the `api/` package layout in ARCHITECTURE.md §3.1, with `platform/` sub-packages first, then wire the minimal HTTP server, health endpoint, and migration runner before adding any module-specific code.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| go-chi/chi | v5 | HTTP router and middleware | Specified in ARCHITECTURE.md; idiomatic Go, composable middleware, no framework overhead |
| jackc/pgx | v5 | PostgreSQL driver + connection pool | Native PostgreSQL protocol, prepared statements (SQL injection prevention), COPY support |
| pressly/goose | v3 | Database migration runner | Embedded migrations in binary via `embed.FS`; sequential numbering; up/down support |
| minio/minio-go | v7 | MinIO/S3 object storage client | Official MinIO SDK; S3-compatible for future portability |
| rs/zerolog | v1 | Structured JSON logging | Zero-allocation; fits NFR-600 requirement |
| kelseyhightower/envconfig | v2 | Environment variable configuration | Struct-based mapping with documented defaults; fits NFR-702 |
| go-playground/validator | v10 | Request body validation | Battle-tested struct-tag validation; fits NFR-503 |
| google/uuid | v1 | UUID generation | Standard Go UUID library; used with pgx v5 for UUID columns |
| golang-migrate/crypto (stdlib) | stdlib | AES-256-GCM token encryption | `crypto/aes` + `crypto/cipher` from Go stdlib; no external dep needed |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| testcontainers-go | latest | Real PostgreSQL in tests | All repository-layer tests; no mocking the DB layer (NFR-804) |
| swaggo/swag | v2 | OpenAPI 3.x spec generation | Annotate handlers; commit generated `api/openapi.yaml` |
| bluemonday | v1 | HTML sanitizer | EPK rich text only — not needed in Phase 0, but include as dependency stub |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| chi v5 | gin, echo, gorilla/mux | gorilla/mux is archived; gin/echo add framework overhead not warranted here |
| pgx v5 | database/sql + lib/pq | lib/pq is in maintenance mode; pgx has better PostgreSQL-native support |
| goose v3 | golang-migrate | goose supports embedded `embed.FS` pattern more cleanly for single-binary deployments |
| envconfig v2 | viper, godotenv | envconfig is lighter; .env loading is handled by Docker Compose, not the Go binary |

**Installation:**
```bash
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5
go get github.com/pressly/goose/v3
go get github.com/minio/minio-go/v7
go get github.com/rs/zerolog
go get github.com/kelseyhightower/envconfig
go get github.com/go-playground/validator/v10
go get github.com/google/uuid
```

---

## Architecture Patterns

### Recommended Project Structure

```
api/                              # Go module root (go.mod lives here)
├── cmd/
│   └── api/
│       └── main.go               # Wire platform packages, start server
├── internal/
│   ├── platform/
│   │   ├── config/               # envconfig struct, Load() func
│   │   ├── db/                   # pgx pool, transaction helpers, NewPool()
│   │   ├── storage/              # MinIO client wrapper, EnsureBucket()
│   │   ├── http/                 # chi router factory, middleware, error helpers
│   │   ├── log/                  # zerolog setup, request-scoped logger
│   │   └── migrations/           # embed.FS declaration + RunMigrations()
│   └── settings/                 # user_settings singleton (INFRA-07)
│       ├── handler.go
│       ├── service.go
│       └── repository.go
├── migrations/
│   ├── 001_initial_schema.up.sql
│   └── 001_initial_schema.down.sql
├── Dockerfile
└── go.mod
```

Frontend proxy addition to `apps/dj/nuxt.config.ts`:
```
apps/dj/
├── nuxt.config.ts                # Add routeRules + devProxy for /api/v1/*
└── ...
```

Root layout additions:
```
klubhub-dj/                       # Git root
├── api/                          # New: Go backend
├── scripts/
│   ├── backup.sh                 # New: pg_dump + mc mirror
│   └── restore.sh                # New: restore from backup
├── docker-compose.yml            # New: four-service stack
├── docker-compose.dev.yml        # New: dev overrides (MinIO console port)
└── .env.example                  # New: all env vars documented
```

### Pattern 1: Embedded Goose Migrations

**What:** Migrations compiled into the Go binary via `embed.FS`; runner called at startup before HTTP server begins accepting requests.

**When to use:** Always — the binary carries its own migrations, so `docker compose up` is truly single-command.

**Example:**
```go
// internal/platform/migrations/migrations.go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

// RunMigrations runs all pending goose migrations.
// Returns an error (caller should exit non-zero) if any migration fails.
func RunMigrations(db *sql.DB) error {
    goose.SetBaseFS(FS)
    return goose.Up(db, ".")
}
```

```go
// cmd/api/main.go — startup order
func main() {
    cfg := config.Load()
    log := log.New(cfg.LogLevel)

    // 1. Connect to DB
    pool, err := db.NewPool(ctx, cfg.DatabaseURL)
    must(err, "connect to database")

    // 2. Run migrations — fatal on failure (NFR-703)
    sqlDB, _ := sql.Open("pgx", cfg.DatabaseURL)
    must(migrations.RunMigrations(sqlDB), "run migrations")

    // 3. Connect to MinIO, ensure bucket
    store, err := storage.New(cfg)
    must(err, "connect to MinIO")
    must(store.EnsureBucket(ctx, "klubhub-dj"), "ensure MinIO bucket")

    // 4. Start HTTP server
    r := http.NewRouter(cfg, pool, store, log)
    srv := &http.Server{Addr: cfg.BindAddress + ":8080", Handler: r}
    srv.ListenAndServe()
}
```

### Pattern 2: Health Endpoint with Per-Integration Status

**What:** `GET /api/v1/health` performs a lightweight check per integration and returns aggregated status. Optional integrations (Spotify, Discogs, Instagram) report `unconfigured` when keys are absent — not `unhealthy`.

**When to use:** Docker health check depends on this; must respond in < 500ms.

**Example:**
```go
// Response shape
type HealthResponse struct {
    Status       string                     `json:"status"` // "healthy"|"degraded"|"unhealthy"
    Integrations map[string]IntegrationStatus `json:"integrations"`
    MigrationsOK bool                       `json:"migrations_ok"`
}

type IntegrationStatus struct {
    Status  string `json:"status"` // "ok"|"error"|"unconfigured"
    Message string `json:"message,omitempty"`
}
```

### Pattern 3: Nuxt 4 API Proxy via routeRules

**What:** Nuxt's Nitro server proxies all `/api/v1/**` requests to the Go backend. Browser never communicates directly with the Go port.

**When to use:** All environments — dev uses `devProxy`, production uses `routeRules` with the `NUXT_PUBLIC_API_BASE` env var.

**Example:**
```typescript
// apps/dj/nuxt.config.ts
export default defineNuxtConfig({
  // ... existing config ...
  routeRules: {
    '/api/v1/**': { proxy: process.env.NUXT_PUBLIC_API_BASE + '/api/v1/**' },
  },
  nitro: {
    devProxy: {
      '/api/v1': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
```

### Pattern 4: AES-256-GCM Token Encryption

**What:** Encrypt OAuth tokens before writing to DB; decrypt on read. Key comes from `TOKEN_ENCRYPTION_KEY` env var (32 bytes, hex-encoded).

**When to use:** `social_accounts.access_token` column — set this up in Phase 0 even though social OAuth is Phase 2, because the column definition and migration must exist.

**Example:**
```go
// internal/platform/crypto/aes.go
func Encrypt(key []byte, plaintext string) (string, error) {
    block, err := aes.NewCipher(key)
    gcm, err := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(key []byte, ciphertext string) (string, error) {
    data, _ := base64.StdEncoding.DecodeString(ciphertext)
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    nonceSize := gcm.NonceSize()
    plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
    return string(plaintext), err
}
```

### Pattern 5: Optimistic Concurrency Control (PUT endpoints)

**What:** PUT request body includes `updated_at`; handler queries DB and compares; returns 409 if mismatch.

**When to use:** All PUT/PATCH handlers for entities with `updated_at`. Must be established in Phase 0 as the infrastructure-level convention.

**Example:**
```go
// Generic pattern in any repository update method
func (r *Repository) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) error {
    result, err := r.pool.Exec(ctx,
        `UPDATE user_settings
         SET dj_name = $1, updated_at = NOW()
         WHERE id = $2 AND updated_at = $3`,
        req.DJName, id, req.UpdatedAt,
    )
    if result.RowsAffected() == 0 {
        return ErrConflict // map to HTTP 409
    }
    return err
}
```

### Anti-Patterns to Avoid

- **MinIO presigned URL using internal hostname:** Never generate presigned URLs with `storage:9000` as the host — browsers cannot resolve Docker internal hostnames. Use `MINIO_PUBLIC_ENDPOINT` env var (default `http://127.0.0.1:9000`) as the base for presigned URL generation.
- **Migrations running after HTTP server starts:** Always run goose before `ListenAndServe`. If migrations fail, exit non-zero before accepting any requests (NFR-703).
- **Optional API keys causing startup failure:** Spotify, Discogs, Instagram keys are absent from many installs. Config loading must succeed when these are empty; health endpoint reports `unconfigured`, not `error`.
- **Storing `TOKEN_ENCRYPTION_KEY` in source code or logs:** Load from env only; never log the key value at any log level.
- **Cross-module DB queries:** Even in Phase 0 with only `platform/` and `settings/` packages, establish the pattern: no package other than `settings/repository.go` queries `user_settings` directly.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Migration versioning and state | Custom migration table | pressly/goose v3 | Handles version state, up/down, embedded FS, concurrent safety |
| PostgreSQL connection pooling | Manual `*sql.DB` management | pgx v5 `pgxpool` | Handles min/max connections, health checks, prepared statement caching |
| Env var parsing + defaults | `os.Getenv` + manual coercion | kelseyhightower/envconfig v2 | Struct tags, type coercion, required validation, default values |
| JSON structured logging | `fmt.Println` or `log.Printf` | rs/zerolog | Zero allocation, level filtering, request-scoped child loggers |
| UUID generation | `crypto/rand` + formatting | google/uuid v1 | Standards-compliant UUID v4, pgx v5 native support via `pgtype` |
| HTTP middleware chaining | Manual handler wrapping | chi v5 middleware | Request ID, logging, recovery, CORS all composable via `r.Use()` |
| AES-GCM nonce management | Custom nonce generation | stdlib `crypto/cipher` | Nonce size is cipher-determined; use `gcm.NonceSize()`, prepend to ciphertext |

**Key insight:** Go's stdlib is powerful but low-level for operations like migration management and connection pooling. These libraries are thin wrappers that handle the operational correctness details (connection health monitoring, migration lock acquisition, nonce generation) that are easy to get subtly wrong when hand-rolled.

---

## Common Pitfalls

### Pitfall 1: MinIO Internal Hostname in Presigned URLs
**What goes wrong:** The Go API generates a presigned URL for a file upload or download using `storage:9000` (the Docker internal hostname) as the base. The browser tries to resolve `storage:9000` and gets a DNS error.
**Why it happens:** The minio-go SDK uses the endpoint passed to `minio.New()` as the base for presigned URL generation. Inside Docker, the endpoint is `storage:9000`. The browser is outside Docker.
**How to avoid:** Add `MINIO_PUBLIC_ENDPOINT` env var (default `http://127.0.0.1:9000`). Use the internal endpoint for API-to-MinIO communication. Use the public endpoint only when constructing URLs returned to the browser.
**Warning signs:** Presigned URLs in API responses containing `storage:9000`.

### Pitfall 2: Migrations Running After HTTP Server Starts
**What goes wrong:** Race condition — Docker health check hits `/api/v1/health` before migrations complete; health check passes because the server is responding; downstream services start against a partially-migrated schema.
**Why it happens:** Server `ListenAndServe` called before `goose.Up` completes.
**How to avoid:** Strictly order startup: connect DB → run migrations → connect MinIO → start HTTP server. If `goose.Up` returns an error, `log.Fatal()` immediately.
**Warning signs:** API responding to health checks but returning schema errors on first real request.

### Pitfall 3: Optional API Keys Crashing Startup
**What goes wrong:** envconfig marks `SPOTIFY_CLIENT_ID` as `required:"true"` — API crashes on startup for any user without Spotify credentials.
**Why it happens:** Overly strict config validation.
**How to avoid:** All social and cover art API keys must be optional (no `required:"true"` tag). Health endpoint checks `cfg.SpotifyClientID != ""` and reports `unconfigured` for that integration. Only `DATABASE_URL` and `MINIO_*` vars are required.
**Warning signs:** Startup panics when `SPOTIFY_CLIENT_ID` is empty.

### Pitfall 4: Nuxt devProxy vs Production routeRules Mismatch
**What goes wrong:** `/api/v1/*` works in dev (devProxy configured) but fails in production Docker container because `routeRules` proxy target uses a hardcoded `localhost:8080` that resolves to the frontend container's own loopback, not the `api` service.
**Why it happens:** Dev and prod proxy configurations diverge.
**How to avoid:** Production `routeRules` target must use `process.env.NUXT_PUBLIC_API_BASE` (set to `http://api:8080` in docker-compose.yml). Dev `devProxy` uses `http://localhost:8080`. Both must cover the same path pattern `/api/v1/**`.
**Warning signs:** API calls 404 in Docker but work in dev.

### Pitfall 5: Soft Delete Filter Omitted on First Query
**What goes wrong:** First `SELECT` on `user_settings` or any entity table lacks `WHERE deleted_at IS NULL`. Data appears to return but re-appears after soft-delete.
**Why it happens:** Easy to forget on the first implementation when testing with an empty table.
**How to avoid:** Add `WHERE deleted_at IS NULL` to every list/get query as a convention from day one. Use partial indexes (`CREATE INDEX ON user_settings(deleted_at) WHERE deleted_at IS NULL`) to keep these queries fast.
**Warning signs:** Deleted records reappearing in API responses.

### Pitfall 6: TOKEN_ENCRYPTION_KEY Key Length
**What goes wrong:** `aes.NewCipher` panics if the key is not 16, 24, or 32 bytes. A hex-encoded 32-byte key is 64 characters — the raw key passed to `aes.NewCipher` must be the decoded bytes, not the hex string.
**Why it happens:** `TOKEN_ENCRYPTION_KEY` is stored as a 64-character hex string. Passing the string directly gives 64 bytes, not 32.
**How to avoid:** Decode at startup: `key, err := hex.DecodeString(cfg.TokenEncryptionKey)` — the result is 32 bytes. Validate length on startup and fatal if wrong.
**Warning signs:** `crypto/aes: invalid key size` panic at runtime.

### Pitfall 7: docker-compose `depends_on` Not Waiting for Readiness
**What goes wrong:** `depends_on: db` in the `api` service only waits for the container to start, not for PostgreSQL to be ready to accept connections. Migrations fail with `connection refused`.
**Why it happens:** Default `depends_on` checks container start, not service health.
**How to avoid:** Always use `depends_on: db: condition: service_healthy` and define `healthcheck` on all services. PostgreSQL health check: `pg_isready -U $POSTGRES_USER`. MinIO health check: `curl -f http://localhost:9000/minio/health/live`.
**Warning signs:** API exits immediately on startup with a connection error.

---

## Code Examples

Verified patterns from ARCHITECTURE.md (official project blueprint):

### Docker Compose Service with Health Check Dependency
```yaml
# Source: docs/ARCHITECTURE.md §9.1
api:
  image: klubhub-dj-api:latest
  build: ./api
  environment:
    DATABASE_URL: postgres://klubhub:${POSTGRES_PASSWORD}@db:5432/klubhub
    MINIO_ENDPOINT: storage:9000
    MINIO_ACCESS_KEY: ${MINIO_ROOT_USER}
    MINIO_SECRET_KEY: ${MINIO_ROOT_PASSWORD}
    MINIO_PUBLIC_ENDPOINT: ${MINIO_PUBLIC_ENDPOINT:-http://127.0.0.1:9000}
    TOKEN_ENCRYPTION_KEY: ${TOKEN_ENCRYPTION_KEY}
    BIND_ADDRESS: ${BIND_ADDRESS:-127.0.0.1}
    CORS_ORIGIN: ${CORS_ORIGIN:-http://127.0.0.1:3000}
    LOG_LEVEL: ${LOG_LEVEL:-info}
  ports:
    - "${BIND_ADDRESS:-127.0.0.1}:8080:8080"
  networks: [internal]
  depends_on:
    db:
      condition: service_healthy
    storage:
      condition: service_healthy
  healthcheck:
    test: ["CMD", "wget", "-q", "-O-", "http://localhost:8080/api/v1/health"]
    interval: 30s
    timeout: 5s
    retries: 3
```

### Go API Multi-Stage Dockerfile
```dockerfile
# Source: ARCHITECTURE.md §7.3 — golang:1.23-alpine → scratch, target <100MB
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /api ./cmd/api

FROM scratch
COPY --from=builder /api /api
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/api"]
```

### Initial Schema Migration (001_initial_schema.up.sql)
```sql
-- Source: ARCHITECTURE.md §5.2 ERD + cross-cutting conventions (INFRA-07, INFRA-09, INFRA-10)
-- Enable pgcrypto for UUID generation
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- user_settings singleton (INFRA-07)
CREATE TABLE user_settings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dj_name     TEXT NOT NULL DEFAULT '',
    logo_path   TEXT NOT NULL DEFAULT '',
    default_colors      JSONB NOT NULL DEFAULT '{}',
    default_template    TEXT NOT NULL DEFAULT '',
    visible_fields      JSONB NOT NULL DEFAULT '{}',
    social_links        JSONB NOT NULL DEFAULT '{}',
    bio_short   TEXT NOT NULL DEFAULT '',
    bio_long    TEXT NOT NULL DEFAULT '',
    contact_info        TEXT NOT NULL DEFAULT '',
    invoice_prefix      TEXT NOT NULL DEFAULT 'INV',
    user_id     UUID,   -- v3 guardrail (ARCHITECTURE.md §10)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

-- Partial index for soft-delete pattern
CREATE INDEX ON user_settings(deleted_at) WHERE deleted_at IS NULL;

-- social_accounts stub (INFRA-09) — token column present, encryption enforced at app layer
CREATE TABLE social_accounts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    platform        TEXT NOT NULL CHECK (platform IN ('instagram')),
    account_name    TEXT NOT NULL DEFAULT '',
    access_token    TEXT NOT NULL DEFAULT '',  -- AES-256-GCM encrypted at rest
    token_expiry    TIMESTAMPTZ,
    status          TEXT NOT NULL DEFAULT 'disconnected'
                        CHECK (status IN ('connected', 'disconnected')),
    user_id         UUID,   -- v3 guardrail
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### zerolog Request Middleware
```go
// Source: ARCHITECTURE.md §6.3
func RequestLogger(log zerolog.Logger) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            reqID := uuid.New().String()
            logger := log.With().Str("request_id", reqID).Logger()
            ctx := logger.WithContext(r.Context())
            w.Header().Set("X-Request-ID", reqID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### backup.sh Pattern
```bash
#!/usr/bin/env bash
# Source: ARCHITECTURE.md §5.6
set -euo pipefail
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
mkdir -p backup

# 1. PostgreSQL dump
docker exec klubhub-dj-db pg_dump -U "${POSTGRES_USER:-klubhub}" "${POSTGRES_DB:-klubhub}" \
  | gzip > "backup/db-${TIMESTAMP}.sql.gz"

# 2. MinIO mirror
docker exec klubhub-dj-storage \
  mc mirror /data "backup/storage-${TIMESTAMP}/"

# 3. Archive
tar -czf "klubhub-backup-${TIMESTAMP}.tar.gz" backup/
echo "Backup complete: klubhub-backup-${TIMESTAMP}.tar.gz"
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `gorilla/mux` router | `go-chi/chi` v5 | gorilla/mux archived 2022 | gorilla/mux must not be used; chi is the drop-in idiomatic replacement |
| `lib/pq` PostgreSQL driver | `pgx/v5` | lib/pq maintenance-only since 2021 | pgx is the recommended driver for new Go+PostgreSQL projects |
| `jung-kurt/gofpdf` PDF library | `go-pdf/fpdf` v2.7+ | jung-kurt/gofpdf archived 2021 | Only relevant in Phase 3+ but must not be confused during dependency setup |
| golang-migrate | goose v3 | Preference shift for embed.FS pattern | goose v3's `SetBaseFS(embed.FS)` is cleaner for single-binary deployment |
| `docker-compose` (v1 CLI) | `docker compose` (v2 plugin) | Docker Desktop 3.3+, Engine 20.10+ | Scripts must use `docker compose` (space), not `docker-compose` (hyphen) |

**Deprecated/outdated:**
- `gorilla/mux`: Archived. Do not use in go.mod.
- `lib/pq`: Maintenance-only. Use `pgx/v5` for all new code.
- `jung-kurt/gofpdf`: Archived. Use `go-pdf/fpdf` v2.7+ (relevant to Phase 3+).
- `docker-compose` hyphenated CLI: v1 is EOL. All scripts and docs must use `docker compose` (Compose v2).

---

## Open Questions

1. **Nuxt 4 vs Nuxt 3 in docker-compose frontend service**
   - What we know: ARCHITECTURE.md §1 and §7.2 say "Nuxt 3"; `package.json` shows `"nuxt": "^4.0.0"`. The STACK.md codebase analysis confirms Nuxt 4.0.0 is actually installed.
   - What's unclear: The ARCHITECTURE.md was written when Nuxt 4 was not yet released. The `routeRules` proxy API is the same in Nuxt 3 and 4; `devProxy` via `nitro.devProxy` is also consistent. No functional difference for Phase 0.
   - Recommendation: Treat the installed version (Nuxt 4) as authoritative. The `node:22-alpine` base image and build command `pnpm nx build dj` are correct regardless of minor version.

2. **MINIO_PUBLIC_ENDPOINT not listed in ARCHITECTURE.md env table**
   - What we know: ARCHITECTURE.md §9.1 shows `MINIO_ENDPOINT: storage:9000` for the Go API. The pitfall of internal hostnames in presigned URLs is documented in the project research (SUMMARY.md).
   - What's unclear: `MINIO_PUBLIC_ENDPOINT` is not explicitly listed in the env var reference table (§9.2) but is clearly needed.
   - Recommendation: Add `MINIO_PUBLIC_ENDPOINT` to `.env.example` and docker-compose.yml with default `http://127.0.0.1:9000`. This is Phase 0 scope.

3. **MinIO health check — `curl` availability in `minio/minio:latest` image**
   - What we know: ARCHITECTURE.md §9.1 uses `curl -f http://localhost:9000/minio/health/live` as the MinIO health check. Recent MinIO images ship with `mc` but may not include `curl`.
   - What's unclear: Whether `curl` is in the current `minio/minio:latest` image.
   - Recommendation: Use `mc ready local` as the health check alternative if `curl` is unavailable. Or use the `minio/minio` image's built-in `/minio/health/live` endpoint via `wget` (more likely available). Validate during implementation.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` package + testcontainers-go (backend); Vitest 4.0.8 (frontend) |
| Config file | No config file — standard `go test ./...` for backend; `apps/dj/vitest.config.ts` for frontend |
| Quick run command | `go test ./internal/platform/... -count=1` (backend platform packages only) |
| Full suite command | `go test ./... -count=1` (all Go tests) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| INFRA-01 | `docker compose up -d` starts all four services | smoke | `docker compose up -d && docker compose ps` — all services `running (healthy)` | Wave 0 |
| INFRA-02 | Services bind to 127.0.0.1 by default | smoke | `curl -f http://127.0.0.1:3000` and `curl -f http://127.0.0.1:8080/api/v1/health` succeed; verify no `0.0.0.0` bindings | Wave 0 |
| INFRA-03 | Migrations run automatically at startup | integration | `go test ./internal/platform/migrations/... -run TestMigrationsRun` with testcontainers-go | Wave 0 |
| INFRA-04 | `GET /api/v1/health` returns correct per-integration JSON | unit + integration | `go test ./internal/platform/http/... -run TestHealthHandler` | Wave 0 |
| INFRA-05 | API starts without optional keys | integration | `go test ./... -run TestStartupWithoutOptionalKeys` — start with only `DATABASE_URL` + `MINIO_*` | Wave 0 |
| INFRA-06 | Nuxt proxies `/api/v1/*` to Go backend | smoke | `curl -f http://127.0.0.1:3000/api/v1/health` returns health JSON | Wave 0 |
| INFRA-07 | `user_settings` table exists with correct columns | integration | `go test ./internal/settings/... -run TestUserSettingsSchema` | Wave 0 |
| INFRA-08 | `scripts/backup.sh` and `scripts/restore.sh` execute without error | smoke | `bash scripts/backup.sh && bash scripts/restore.sh` — assert no non-zero exit | Wave 0 |
| INFRA-09 | AES-256-GCM encrypt/decrypt round-trip | unit | `go test ./internal/platform/crypto/... -run TestAESRoundTrip` | Wave 0 |
| INFRA-10 | UUID PKs, timestamps, soft-delete pattern on all entities | integration | `go test ./internal/settings/... -run TestSoftDelete` | Wave 0 |
| INFRA-11 | PUT returns 409 on `updated_at` conflict | unit | `go test ./internal/settings/... -run TestOptimisticConcurrency` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/platform/... -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green + `docker compose up -d` smoke test before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `api/internal/platform/migrations/migrations_test.go` — covers INFRA-03 (testcontainers-go real PostgreSQL)
- [ ] `api/internal/platform/http/health_test.go` — covers INFRA-04, INFRA-05
- [ ] `api/internal/platform/crypto/aes_test.go` — covers INFRA-09
- [ ] `api/internal/settings/repository_test.go` — covers INFRA-07, INFRA-10, INFRA-11
- [ ] `api/go.mod` — module does not exist yet; all backend tests require this first

---

## Sources

### Primary (HIGH confidence)
- `docs/ARCHITECTURE.md` v1.0 (2026-03-11) — authoritative project architecture blueprint; primary source for all stack decisions, package layout, docker-compose structure, env vars, health endpoint shape, migration pattern, backup scripts
- `.planning/REQUIREMENTS.md` (2026-03-14) — INFRA-01 through INFRA-11 requirement text
- `apps/dj/package.json` + `apps/dj/nuxt.config.ts` — confirmed Nuxt 4.0.0, current nuxt.config.ts has no proxy configuration (must be added)
- `.planning/research/SUMMARY.md` (2026-03-14) — confirmed stack choices, library version warnings, MinIO presigned URL pitfall

### Secondary (MEDIUM confidence)
- pressly/goose v3 `embed.FS` documentation — training-data knowledge of the `SetBaseFS` API; verified against the known pattern for embedded migrations in single-binary Go applications
- minio-go v7 presigned URL `MINIO_PUBLIC_ENDPOINT` pattern — training-data knowledge; well-established MinIO self-hosting pattern for URL generation

### Tertiary (LOW confidence)
- MinIO `minio/minio:latest` Docker image `curl` availability — training-data only; verify by pulling the image during implementation

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all library choices sourced directly from ARCHITECTURE.md v1.0
- Architecture: HIGH — package layout, docker-compose structure, startup ordering, env vars all directly from ARCHITECTURE.md §3.1, §8, §9
- Pitfalls: HIGH for MinIO hostname and migration ordering (well-documented in project research); MEDIUM for MinIO curl availability (unverified)

**Research date:** 2026-03-14
**Valid until:** 2026-06-14 (stable tech; 90-day window is conservative)
