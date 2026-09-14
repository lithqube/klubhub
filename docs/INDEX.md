# Documentation index

KlubHub DJ is a self-hosted career-management platform for independent DJs.
The docs in this folder are organised by audience. Start with the doc that
matches your role.

## Visitors — "what is this?"

| Doc | Why read it |
|-----|-------------|
| [`../README.md`](../README.md) | Feature overview, quick-start, environment variables, project structure |
| [`v1-release-plan.md`](./v1-release-plan.md) | The roadmap to v1.0.0 — what modules ship, what is gated behind the future cloud product |
| [`v2-saas-migration-plan.md`](./v2-saas-migration-plan.md) | Future SaaS evolution (KlubHub Cloud) and how it relates to the open-source build |

## Contributors — "how do I build / extend this?"

| Doc | Why read it |
|-----|-------------|
| [`../CONTRIBUTING.md`](../CONTRIBUTING.md) | Dev setup, code conventions, PR workflow, how to add a new module |
| | [`../AGENTS.md`](../AGENTS.md) | Nx-specific guidance (always run tasks through `pnpm nx …`) |
| | [`../CLAUDE.md`](../CLAUDE.md) | Conventions specifically for Claude / AI coding assistants |
| [`ARCHITECTURE.md`](./ARCHITECTURE.md) | System architecture: services, module boundaries, data flow, deployment topology |
| [`../scripts/backup.sh`](../scripts/backup.sh) / [`../scripts/restore.sh`](../scripts/restore.sh) | Database + storage backup / restore (Postgres + Garage S3) |

## Maintainers / spec authors

| Doc | Why read it |
|-----|-------------|
| [`KlubHub_DJ_BRD_v2.0_Platform.md`](./KlubHub_DJ_BRD_v2.0_Platform.md) | Business Requirements Document — product vision, scope, personas |
| [`KlubHub_DJ_BRD_v2.0_Addendum.md`](./KlubHub_DJ_BRD_v2.0_Addendum.md) | Addendum to the BRD (clarifications, scope refinements) |
| [`KlubHub_DJ_NFR_v1.0.md`](./KlubHub_DJ_NFR_v1.0.md) | Non-Functional Requirements — performance, security, reliability targets |

## Screenshots

UI screenshots live in [`screenshots/`](./screenshots/). See
[`screenshots/README.md`](./screenshots/README.md) for the naming convention
when adding new images.
