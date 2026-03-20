# v2 SaaS Architecture Research

**Researched:** 2026-03-20
**Status:** Research — informs v2 SaaS migration plan
**Source:** Analysis of `docs/ARCHITECTURE.md`, `docs/KlubHub_DJ_NFR_v1.0.md`, `.planning/research/MONETIZATION.md`

---

## 1. Multi-Tenancy: Row-Level Security (RLS)

### Why RLS Over Application-Level Isolation

PostgreSQL Row-Level Security is the recommended approach for multi-tenant isolation in v2:

**Advantages:**
- Tenant isolation enforced at the database level — cannot be bypassed by application bugs
- No code duplication — single set of queries works for all tenants
- Simpler than schema-per-tenant (no DDL management) or database-per-tenant (no connection pool explosion)
- Audit-friendly — policy definitions are inspectable SQL

**Implementation pattern:**

```sql
-- Enable RLS on all tables
ALTER TABLE tracklists ENABLE ROW LEVEL SECURITY;

-- Create tenant isolation policy
CREATE POLICY tenant_isolation ON tracklists
  USING (user_id = current_setting('app.user_id')::uuid);

-- Force RLS even for table owners (important for security)
ALTER TABLE tracklists FORCE ROW LEVEL SECURITY;
```

**Middleware pattern (Go):**

```go
func TenantMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID := r.Context().Value(authUserIDKey).(uuid.UUID)
        // Set PostgreSQL session variable for RLS
        ctx := context.WithValue(r.Context(), tenantKey, userID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// In repository layer, before each query:
func (r *Repo) setTenant(ctx context.Context, tx *sql.Tx) error {
    userID := ctx.Value(tenantKey).(uuid.UUID)
    _, err := tx.ExecContext(ctx, "SET LOCAL app.user_id = $1", userID.String())
    return err
}
```

### v1 → v2 Migration Steps

1. Create `users` table and `subscriptions` table
2. Create a "migration user" for existing v1 data
3. `UPDATE tracklists SET user_id = '{migration-user-uuid}' WHERE user_id IS NULL` (repeat for all tables)
4. `ALTER TABLE tracklists ALTER COLUMN user_id SET NOT NULL` (repeat for all tables)
5. Enable RLS policies on all tables
6. Add composite indexes: `(user_id, created_at)`, `(user_id, deleted_at)` on all tables

**Risk:** Step 4 requires a full table scan; run during maintenance window for large datasets. For SaaS launch with fresh data this is not a concern.

---

## 2. Service Extraction Strategy

### Extraction Order (by SaaS value and coupling)

| Order | Service | Extraction Complexity | SaaS Value |
|-------|---------|----------------------|------------|
| 1 | Auth | New service (not extraction) | Prerequisite |
| 2 | Scheduler | Medium — extract goroutine + job polling | Highest |
| 3 | EPK Hosting | Low — new service for public pages | High |
| 4 | WebSocket | New service (not extraction) | Medium |
| 5 | API Gateway | Infrastructure addition | Prerequisite for scaling |

### Scheduler Extraction Detail

**v1 architecture:**
```
Go API Process
├── HTTP handlers
├── Scheduler goroutine (polls DB every 30s)
│   ├── Find due posts
│   ├── Publish to Instagram
│   └── Update post status
└── Everything shares one process
```

**v2 architecture:**
```
API Service                    Scheduler Service
├── HTTP handlers              ├── Asynq worker (Redis-backed)
├── Enqueue job on             │   ├── ProcessPublishPost
│   POST /api/v1/social/posts  │   ├── ProcessRefreshToken
└── No scheduler logic         │   └── ProcessRetryFailed
                               └── Leader election (only 1 active)

Redis
├── asynq:queue:social (job queue)
├── asynq:queue:tokens (token refresh)
└── asynq:scheduled (delayed jobs)
```

**Why Asynq (Go Redis queue)?**
- Same language as API — shared types, no serialization boundary
- Redis-backed — survives process crashes
- Built-in retry, dead-letter, scheduled jobs
- Leader election support for singleton scheduler
- Prometheus metrics built-in

### EPK Hosting Service

**New service, not extraction.** The v1 EPK module generates PDFs. The v2 EPK hosting service adds:
- Server-rendered HTML pages (Nuxt or Go templates)
- Public URL routing: `klubhub.dj/epk/{username}`
- CDN integration for assets
- Custom domain management (TEAM tier)

This service reads from the existing EPK data in PostgreSQL but serves a separate HTTP interface (public-facing, not behind auth).

---

## 3. Authentication Architecture

### Recommended: Custom JWT + OIDC

**Why custom over Auth0/Clerk/Supabase Auth?**
- Full control over token claims (tenant ID, subscription tier)
- No per-MAU pricing surprise
- JWT format compatible with all Go middleware
- OIDC integration for social login (Google, GitHub)

**Token structure:**

```json
{
  "sub": "user-uuid",
  "email": "dj@example.com",
  "tier": "pro",
  "team_id": "team-uuid-or-null",
  "iat": 1711929600,
  "exp": 1711933200
}
```

**Flow:**
1. User signs up → email verification → account created
2. Login → issue access token (15min) + refresh token (30 days, rotated)
3. All API requests include `Authorization: Bearer <access-token>`
4. Middleware extracts `sub` → sets `app.user_id` for RLS
5. Middleware extracts `tier` → feature gating middleware checks access

### Self-Hosted Compatibility

v1 self-hosted has NO auth. To keep backward compatibility:
- v1 API accepts all requests without auth (as today)
- v2 cloud API requires auth on all routes except `/api/v1/health` and `/api/v1/auth/*`
- Feature flag: `AUTH_ENABLED=true` (cloud) vs `AUTH_ENABLED=false` (self-hosted)

---

## 4. Billing Integration (Stripe)

### Subscription Model

```
Stripe Products:
├── klubhub-pro    ($9/mo or $89/yr)
├── klubhub-team   ($25/mo or $249/yr)
└── klubhub-agency ($99/mo — custom)

Stripe Webhook Events to Handle:
├── customer.subscription.created
├── customer.subscription.updated
├── customer.subscription.deleted
├── invoice.payment_succeeded
├── invoice.payment_failed
└── customer.subscription.trial_will_end
```

### Feature Gating Middleware

```go
func RequireTier(minTier string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims := auth.FromContext(r.Context())
            if !tierAtLeast(claims.Tier, minTier) {
                http.Error(w, "Upgrade required", http.StatusPaymentRequired)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// Route registration:
r.With(RequireTier("pro")).Mount("/api/v1/social", social.NewHandler(deps))
r.With(RequireTier("team")).Mount("/api/v1/tour/collaborate", collab.NewHandler(deps))
```

### Usage Metering

Track per billing period:
- Social posts scheduled (free tier: 4/month cap)
- Storage used in MinIO/S3 (free: 500MB, pro: 10GB, team: 50GB)
- EPK exports (free: 1 total, pro/team: unlimited)
- API calls (for future agency tier rate limiting)

Store in PostgreSQL `usage_meters` table, reset on billing cycle start.

---

## 5. Kubernetes Architecture

### Helm Chart Structure

```
charts/klubhub-dj-cloud/
├── Chart.yaml
├── values.yaml              # Default values
├── values-staging.yaml      # Staging overrides
├── values-production.yaml   # Production overrides
├── templates/
│   ├── api-deployment.yaml
│   ├── api-service.yaml
│   ├── api-hpa.yaml
│   ├── auth-deployment.yaml
│   ├── scheduler-deployment.yaml
│   ├── websocket-deployment.yaml
│   ├── epk-deployment.yaml
│   ├── frontend-deployment.yaml
│   ├── ingress.yaml
│   ├── configmap.yaml
│   ├── secrets.yaml
│   └── jobs/
│       ├── migrate.yaml     # Pre-deploy migration job
│       └── seed.yaml        # Optional seed data
```

### Resource Estimates (Initial SaaS Launch)

| Service | Replicas | CPU Request | Memory Request |
|---------|----------|-------------|----------------|
| API | 2 | 250m | 256Mi |
| Auth | 2 | 100m | 128Mi |
| Scheduler | 1 (leader) | 200m | 256Mi |
| WebSocket | 2 | 100m | 128Mi |
| EPK Hosting | 2 | 100m | 128Mi |
| Frontend | 2 | 100m | 256Mi |
| PostgreSQL | Managed | - | - |
| Redis | Managed | - | - |

**Estimated monthly cloud cost (AWS, us-east-1):** ~$200-400/mo for initial launch. Scales with traffic.

### Database: Managed PostgreSQL

- **AWS:** RDS PostgreSQL 16, db.t4g.medium ($55/mo)
- **GCP:** Cloud SQL, db-custom-2-4096 ($65/mo)
- Automated backups, point-in-time recovery
- Read replica for dashboard/analytics queries
- Connection pooling via PgBouncer sidecar

---

## 6. Frontend Feature Flags

### Runtime Config Approach

```typescript
// nuxt.config.ts
export default defineNuxtConfig({
  runtimeConfig: {
    public: {
      mode: 'self-hosted', // or 'saas'
    }
  }
})
```

### Component-Level Gating

```vue
<template>
  <div>
    <!-- Always visible -->
    <TracklistGenerator />

    <!-- SaaS PRO+ only -->
    <SocialScheduler v-if="$features.socialScheduler" />

    <!-- SaaS TEAM only -->
    <TourCollaboration v-if="$features.collaboration" />

    <!-- Upgrade prompt for gated features -->
    <UpgradePrompt
      v-else
      :feature="'Social Scheduler'"
      :required-tier="'pro'"
    />
  </div>
</template>
```

This approach means ONE Nuxt codebase serves both self-hosted and SaaS. Self-hosted users see all open-source features. SaaS users see premium features based on their tier.

---

## 7. Observability Stack

### Recommended: Open-Source Grafana Stack

| Component | Tool | Purpose |
|-----------|------|---------|
| Metrics | Prometheus | Request latency, error rates, queue depth |
| Logs | Loki | Structured log aggregation |
| Traces | Tempo | Distributed request tracing |
| Visualization | Grafana | Dashboards, alerts |
| Errors | Sentry | Error grouping, stack traces |
| Uptime | Grafana Synthetic Monitoring | External health checks |

**Why Grafana stack over Datadog/New Relic?**
- Open-source — no per-host pricing
- Self-hostable (dogfooding the self-hosted ethos)
- Grafana Cloud free tier covers initial launch
- Migrate to managed Grafana Cloud if scale demands it

### Key SLOs

| SLO | Target | Measurement |
|-----|--------|-------------|
| API availability | 99.9% | Successful responses / total requests |
| API latency (p95) | <500ms | Prometheus histogram |
| Image generation (p99) | <2s | Prometheus histogram |
| Scheduler accuracy | Posts published within 60s of scheduled time | Job completion timestamp vs scheduled time |
| Data durability | 99.99% | No data loss events |

---

*v2 SaaS Architecture Research for: KlubHub DJ*
*Researched: 2026-03-20*
