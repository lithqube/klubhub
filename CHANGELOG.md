# Changelog

All notable changes to KlubHub DJ are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Phase 0 — Infrastructure** — Four-service Docker Compose stack (Postgres 16,
  Garage S3, Go API, Nuxt frontend); health endpoint; goose migrations; backup /
  restore scripts.
- **Phase 1 — Tracklist Image Generator** — Upload, parse, cover art lookup
  (Spotify / Discogs), PNG/JPEG export.
- **Phase 1.5 — Design System Foundation** — Tailwind v4, shadcn-vue, Pinia,
  responsive shell, 3-way theme switcher.
- **Phase 1.5.5 — Kinetic HUD UI Migration** — Cyberpunk HUD design system
  (`.hud-card`, `.bracket-box`, `.pulse-dot`, glass panels).
- **Phase 2 — Social Media Scheduler** — Instagram OAuth, timezone-aware
  scheduling, retry with backoff, calendar view.
- **Phase 3 — EPK / Press Kit Builder** — Bio, photos, tech rider, PDF export
  (go-pdf/fpdf).
- **Phase 4 — Gig Tracker** (in progress) — CRUD, status workflow, venue and
  contact database, iCal feed, booking confirmation PDF.

### Changed

- Object storage migrated from MinIO to **Garage v2** (S3-compatible,
  self-hosted).

### Notes

- The repository is pre-`v1.0.0` and the public API surface (HTTP routes, JSON
  shapes) may change without a breaking-version bump. See
  [`docs/v1-release-plan.md`](./docs/v1-release-plan.md) for the path to v1.

[Unreleased]: https://github.com/lithqube/klubhub/compare/main...HEAD
