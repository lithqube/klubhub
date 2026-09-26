# 0004 — Promoter: transactional outbox to NATS JetStream

Status: Accepted (2026-09-25, plan decision D8)

## Decision
- Domain code writes `events.Event` rows to an outbox inside its tenant
  transaction; payloads contain identifiers only.
- A relay (role `klubhub_relay`: BYPASSRLS, but granted only the outbox
  delivery columns) publishes to JetStream with `Nats-Msg-Id` = row id.
- Subjects: `tenant.{tenant}.{aggregate}.{verb}` and `audit.*`; streams
  `PROMOTER_EVENTS` and `PROMOTER_AUDIT`.
- nsc JWT/NKey users per service: `promoter-api` publishes only,
  `promoter-bff` (the Nuxt server via nuxt-nats) consumes only and filters by
  the session's tenant for live updates (SSE).

## Consequences
Events are published if and only if the change commits; retries are
de-duplicated; a leaked stream reveals no personal data.
