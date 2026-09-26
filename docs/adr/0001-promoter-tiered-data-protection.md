# 0001 — Promoter: tiered data protection ("zero knowledge")

Status: Accepted (2026-09-25, plan decision D5)

## Context
Promoters store third parties' data: guest lists, audiences, artist fees,
bills. The product goal is that the platform, especially a SaaS operator,
knows as little as possible. Full end-to-end encryption would break
server-side work the product needs: sending campaign emails and reminders,
generating PDFs, DATEV exports and scheduled jobs.

## Decision
Every column belongs to a declared class:
- `public` and `internal`: plaintext, isolated per tenant by RLS (ADR 0006).
- `personal` and `financial`: field-level envelope encryption. A deployment KEK
  (never in the database) wraps one data key per tenant and key version;
  AES-256-GCM with AAD = tenant | table | column | row | version; HMAC blind
  indexes for exact lookups. Deleting a tenant's keys crypto-shreds its data,
  including in older backups.
- `sealed`: end-to-end encrypted in the browser with org keys wrapped to
  members' X25519 keys; the server stores ciphertext only. Used for ban
  lists, private notes, the document vault and fee negotiations.

Guards in CI: every table declares its class; personal and financial tables
may only contain sealed columns, blind indexes, keys and timestamps, or
reviewed exceptions; plaintext columns that look like personal data fail.

## Consequences
- The operator honestly sees ciphertext at rest, plaintext only in memory while
  processing, and never `sealed` data (documented in SECURITY.md).
- Losing the KEK loses personal and financial data by design; custody is
  documented and the secrets script never overwrites it.
- Sealed data cannot be searched or processed server-side.

## Alternatives
- Full E2EE for all personal data: rejected (breaks emails, PDFs and jobs).
- Encryption at rest only: rejected (not "zero knowledge" in any tier).
