# Architecture Decision Records

Decisions for the KlubHub product family. Format: context, decision,
consequences, alternatives. Status is one of Proposed, Accepted, Superseded.

| ADR | Title | Status |
|---|---|---|
| [0001](0001-promoter-tiered-data-protection.md) | Promoter: tiered data protection ("zero knowledge") | Accepted |
| [0002](0002-promoter-identity-providers.md) | Promoter: local identity for self-host, Zitadel for SaaS | Accepted |
| [0003](0003-secrets-infisical-optional.md) | Secrets: Infisical optional, env/file fallback | Accepted |
| [0004](0004-promoter-nats-outbox.md) | Promoter: transactional outbox to NATS JetStream | Accepted |
| [0005](0005-promoter-embedded-opa.md) | Promoter: OPA embedded in the Go binary | Accepted |
| [0006](0006-promoter-rls-tenant-isolation.md) | Promoter: fail-closed Postgres RLS tenant isolation | Accepted |

Some patterns are adapted from other projects by the same author (OpenSchild,
Mettal, nuxt-nats). Patterns learned from client work are reimplemented from
first principles; no client code is copied.
