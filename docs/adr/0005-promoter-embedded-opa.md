# 0005 — Promoter: OPA embedded in the Go binary

Status: Accepted (2026-09-25)

## Decision
Rego policies (`api/internal/platform/authz/policies`) are embedded and
compiled with the OPA Go SDK at boot. Default deny; unknown actions deny; no
fallback for unlisted routes; a prioritised `deny_reason` on every refusal.
Every route declares its action through a registry, and a test fails if any
mounted route lacks a registration or declares an unknown action. Rego unit
tests run inside `go test`.

## Consequences
No sidecar, so no "policy engine unreachable" failure mode, and policies are
versioned with the code. Signed bundles remain an option later.

## Alternatives
OPA as a REST sidecar (extra container and network failure mode).
