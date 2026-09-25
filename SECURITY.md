# Security Policy

## Reporting a vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

We take security seriously and want to address issues before they become public.
Use one of these private channels:

1. **GitHub Security Advisories** — open a
   [private security advisory](https://github.com/lithqube/klubhub/security/advisories/new)
   on this repository. This is the preferred channel because it gives you a
   dedicated thread and lets us coordinate a fix and disclosure timeline.
2. **Email** — `security@klubhub.app` (PGP key on request).

Include as much of the following as you can:

- Description of the vulnerability and its impact
- Reproduction steps or a minimal proof-of-concept
- Affected versions / commits
- Environment details (OS, Docker version, deployment mode)
- Whether you want public credit in the advisory

We acknowledge new reports within **3 business days** and aim to ship a fix
or mitigation within **30 days**, depending on severity and complexity.

## Supported versions

Only the latest minor release on `main` receives security patches. Older tags
are not maintained. We follow [semver](https://semver.org/):

| Version   | Supported          |
|-----------|--------------------|
| `main`    | ✅ Active development |
| latest tag | ✅ Bug fixes + security |
| older tags | ❌ End of life         |

Until a `v1.0.0` stable tag is cut, treat `main` as the supported line.

## Threat model (what we defend against)

KlubHub DJ is designed for **self-hosting by a single user or small team**:

- All API endpoints are single-user — there is no multi-tenant authorization
  layer. Anyone with network access to the API is treated as the owner.
- OAuth tokens (Instagram) are encrypted at rest with `TOKEN_ENCRYPTION_KEY`
  (AES-256-GCM).
- Passwords, session cookies, CSRF, and rate limiting are **not** implemented
  because there is no public-internet attack surface by default — services bind
  to `localhost` unless you change `BIND_ADDRESS`.
- PostgreSQL and Garage storage have no built-in auth on the cluster network;
  treat the Docker `internal` network as the trust boundary.

If you expose the stack to the internet, you are responsible for adding a
reverse proxy, TLS, and authentication in front of it.

## KlubHub Promoter: security model

Promoter holds other people's data (guest lists, audiences, artist fees,
bills), so unlike DJ it is multi-user and multi-tenant from the first
release. Decisions are recorded in `docs/adr/0001`–`0006`; the rules below
are enforced by code and by tests, not by convention.

### What protects what

| Layer | Mechanism | Enforced by |
|---|---|---|
| Identity | Self-host: argon2id passwords, TOTP, server-side sessions (`__Host-` HttpOnly Secure SameSite=Lax cookie; only a SHA-256 of the token is stored), 5-attempt lockout. SaaS: Zitadel OIDC; the Go API verifies JWT signature, issuer and a required audience. | `internal/promoter/identity`, `internal/platform/auth` tests |
| CSRF | Origin/Referer allowlist plus a mandatory `X-KlubHub-CSRF` header on every state change. | `platform/auth` tests, end-to-end router test |
| Authorisation | OPA policy compiled into the binary; default deny; every refusal has a reason and lands in the append-only audit log. Owners, admins and finance need a second factor for sensitive actions and a login within 15 minutes for destructive ones. Door staff sessions are bound to one event. | Rego tests, route-coverage test (no route without a policy decision) |
| Tenant isolation | Postgres row level security, `ENABLE` + `FORCE` on every table, fail-closed `current_setting('app.tenant_id')`; the runtime role is not the owner and cannot bypass RLS (the binary refuses to start otherwise). | RLS guard test over every migration, integration tests |
| Data at rest | Personal and financial fields are sealed with a per-tenant key (AES-256-GCM, bound to tenant, table, column and row). The most sensitive items are end-to-end encrypted in the browser (`sealed` tier). | Data-class guard test over every migration |
| Messaging | Transactional outbox; events carry identifiers only; NATS users are least-privilege (the relay publishes, the UI server consumes). | events tests, nsc permissions |
| Supply chain | gitleaks, govulncheck, Trivy, SPDX SBOM, build provenance and cosign keyless signatures on published images. | `.github/workflows/promoter-ci.yml` |

### What the operator can and cannot see

This matters most for the future SaaS, where "operator" means us.

| Data class | Examples | Operator sees |
|---|---|---|
| `public` | Event title, lineup, venue city | Plaintext |
| `internal` | Budgets, timetables, settings, audit entries | Plaintext (row-level isolated between tenants) |
| `personal` / `financial` | Guest and contact names, emails, phones, bills, payouts | Ciphertext at rest. Plaintext only inside a running request or job, in memory. **Root on a running host can read this data while it is processed.** |
| `sealed` | Ban list, private notes, document vault, fee negotiations | Never. Encrypted in the browser with keys the server does not hold. |

Deleting an organisation destroys its data key (crypto-shredding), which
makes its personal and financial data unreadable, including in backups
taken earlier.

### Key custody (self-hosted)

- `scripts/promoter-secrets.sh` creates the KEK (`kek`, `kek_id`) once and
  never overwrites it. **Back both files up offline** before storing data:
  without the KEK, personal and financial data is unrecoverable by design.
- The KEK is never written to the database; each tenant's data key is
  stored only in wrapped (encrypted) form.
- KEK rotation: generate a new KEK with a new id, set it as
  `PROMOTER_KEK`/`PROMOTER_KEK_ID`, move the old one to
  `PROMOTER_PREVIOUS_KEKS=id:base64`, restart, and let new data keys be
  wrapped with the new KEK. Remove the old KEK only after every tenant key
  has been re-wrapped.
- NATS NKey seeds (`*.creds`) and database passwords are Docker secrets. With
  Infisical, render them with the Infisical agent; the app only reads
  environment variables or `*_FILE` paths.

### Known limits (P0)

- A self-hosted instance is meant to stay private (plan decision D2): the
  compose file publishes on `127.0.0.1` only. Public event pages, RSVP and
  submission links are a SaaS surface.
- Door PINs are 6 digits. They only work from a registered device and lock
  out after 5 attempts per event.
- Zitadel access tokens may not carry `amr`/`auth_time`; in that case
  second-factor and recent-login checks deny (fail closed) until verified
  against a real Zitadel deployment.
- The `sealed` (end-to-end) tier's browser key management ships with the
  first sealed feature (the P2 ban list); P0 defines the model only.

## Secrets handling

- Never commit `.env`, `garage.toml`, or any file containing real credentials.
  Both are git-ignored; use `.env.example` and `garage.toml.example` as the
  template.
- Generate secrets with `openssl rand -hex 32` (32 bytes of random hex).
- Rotate `TOKEN_ENCRYPTION_KEY` will invalidate all stored OAuth tokens —
  re-authorize Instagram after rotation.

## Plunk API keys (newsletter signup)

The KlubHub site uses [Plunk](https://useplunk.com) for the newsletter signup
form on `klubhub.io`. Plunk has two keys per project, and they are kept
strictly separate:

| Key | Prefix | Where it lives | Where it is used |
|---|---|---|---|
| Public key | `pk_*` | GitHub Actions environment secrets (`PLUNK_PUBLIC_KEY`, `PLUNK_PUBLIC_KEY_STAGING`, `PLUNK_PUBLIC_KEY_PREVIEW`) | Injected into `<meta name="plunk-public-key">` at build time, read by the browser-side `newsletter.js`. Safe to ship in static HTML. |
| Secret key | `sk_*` | Operator's password manager / vault | Server-side only. Never enters the repo, never enters CI, never enters the static site. Wired in Compose as a Docker secret (`/run/secrets/plunk_secret_key`) for the day a KlubHub DJ feature needs transactional email. |

Three independent Plunk projects keep leaked keys from crossing blast
radii:

- **production** — feeds the real newsletter list at `klubhub.io`.
- **staging** — feeds a Plunk sandbox list used by staging deployments.
- **preview** — feeds a Plunk sandbox list used by PR preview deployments.

Per [Plunk's API-keys guide](https://docs.useplunk.com/guides/api-keys):
"Use separate Plunk projects for staging and production so a leaked staging
key can't touch production data."

### Rotation

Both keys rotate together as a single pair; Plunk provides no grace period.
To rotate:

1. Open **Settings → API Keys** in the Plunk dashboard for the affected
   project, or call `POST /users/@me/projects/:id/regenerate-keys`.
2. The old keys stop working immediately.
3. Update the GitHub Actions environment secret (or operator vault) with
   the new value.
4. Re-run the affected deployment(s).

If a `pk_*` is compromised, audit the **Activity** tab in Plunk for
unexpected subscriptions and check **Billing → Consumption** for usage
spikes that suggest abuse before rotating.

## Out of scope

- Vulnerabilities in dependencies that have no exploitable path in this
  codebase (please still report and we will track upstream)
- Denial-of-service attacks against an exposed deployment that lacks a
  reverse proxy / rate limiter

## Disclosure policy

We follow a **coordinated disclosure** model:

1. Reporter notifies us privately.
2. We confirm, develop a fix, and prepare a release.
3. We release the fix and publish a security advisory simultaneously.
4. We credit the reporter in the advisory (unless they prefer to remain
   anonymous).

Thank you for helping keep KlubHub DJ users safe.
