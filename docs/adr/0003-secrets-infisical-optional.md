# 0003 — Secrets: Infisical optional, env/file fallback

Status: Accepted (2026-09-25, plan decision D7)

## Decision
Applications read secrets only from environment variables or `NAME_FILE`
paths. Infisical is operator tooling: `scripts/with-secrets.sh` for
development, the Infisical GitHub action with an OIDC machine identity in CI,
and the Infisical agent rendering an env file in production. Self-hosters can
run with Docker secrets created by `scripts/promoter-secrets.sh`.
Root-of-trust material (KEK backup, NATS operator keys, Zitadel master key)
stays in offline custody, not in Infisical.

## Consequences
No SDK coupling; the same image runs with or without Infisical.
