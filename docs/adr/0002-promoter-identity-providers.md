# 0002 — Promoter: local identity for self-host, Zitadel for SaaS

Status: Accepted (2026-09-25, plan decision D6)

## Context
The user's platform stack uses Zitadel. Zitadel (with Login V2 and its own
database) is heavy for community self-hosters, who run one organisation.

## Decision
One `auth.Authenticator` interface, two providers, one `authz.Principal`:
- `local` (self-host): single organisation, CLI bootstrap with a one-time
  link (no first-visitor-becomes-owner race), argon2id, TOTP required by
  policy for sensitive roles, server-side sessions, lockout, invites, door
  device + PIN sessions.
- `zitadel` (SaaS): the Nuxt server is the OIDC client (code + PKCE, tokens
  in a sealed cookie); the Go API verifies JWT access tokens via JWKS with a
  required audience. One Zitadel organisation = one tenant (UUIDv5 of the
  org id). The adapter is open source but only wired by SaaS config.

## Consequences
- Authorisation (ADR 0005) and tenancy (ADR 0006) are identical in both.
- Two login paths to maintain; both are covered by tests.

## Alternatives
Zitadel everywhere (too heavy for self-hosters); any generic OIDC provider
(more configuration surface, weaker tenancy mapping).
