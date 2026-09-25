# 0006 — Promoter: fail-closed Postgres RLS tenant isolation

Status: Accepted (2026-09-25)

## Decision
- Every table: `ENABLE` + `FORCE ROW LEVEL SECURITY` and a `tenant_isolation`
  policy with USING and WITH CHECK on `current_setting('app.tenant_id')::uuid`
  (the strict form: a missing tenant raises instead of returning nothing).
- The app connects as `klubhub_app` (not owner, no BYPASSRLS); the binary
  refuses to start on a superuser or BYPASSRLS role.
- `tenantdb.WithTenant` sets the tenant per transaction with
  `set_config(..., true)`; there is no pool-level default. Domain packages
  cannot import the pool (guard test).
- The only pre-tenant lookup is `promoter_instance_tenant()` (SECURITY
  DEFINER, returns an id only), used by self-hosted login.

## Consequences
A bug that forgets the tenant fails loudly. CI guards check every migration.
