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

## Secrets handling

- Never commit `.env`, `garage.toml`, or any file containing real credentials.
  Both are git-ignored; use `.env.example` and `garage.toml.example` as the
  template.
- Generate secrets with `openssl rand -hex 32` (32 bytes of random hex).
- Rotate `TOKEN_ENCRYPTION_KEY` will invalidate all stored OAuth tokens —
  re-authorize Instagram after rotation.

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
