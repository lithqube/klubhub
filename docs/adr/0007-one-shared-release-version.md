# 0007 — One shared release version for both products

Status: Accepted (2026-11-24)

## Context
One repo-wide `v*` git tag triggers both `containers-ci.yml` (KlubHub DJ) and
`promoter-ci.yml` (KlubHub Promoter), so a tag publishes both images.

## Decision
- KlubHub DJ and KlubHub Promoter share one version line. Tag `v1.3.0` publishes
  `klubhub-dj-api:v1.3.0` and `klubhub-promoter-api:v1.3.0`.
- The Promoter image published with a release is a preview until the Promoter
  MVP ships. Its version number says nothing about Promoter maturity.
- No workflow change.

## Consequences
Maintainers cut one tag per release and self-hosters pin one version.
A release can carry Promoter changes nobody asked for, and a DJ-only fix still
publishes a Promoter image.

## Alternatives
- Per-product tags (`dj-v1.3.0`, `promoter-v0.1.0`): honest versions, but two
  release flows and two update paths. Revisit when the Promoter MVP ships.
