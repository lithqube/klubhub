---
name: klubhub-dev
description: "KlubHub DJ repo operating procedures for autonomous coding agents. Use when working in the lithqube/klubhub repository on Compose dev/prod stacks, Plunk newsletter integration, GHCR image publishing, Nx task invocation, or any change that touches apps/dj, apps/site, scripts/, docs/, .github/workflows/, or docker-compose*.yml. Encodes the project-specific contracts (per-product image naming, Plunk trigger event contract, dev/prod Compose separation, secret-handling rules) so agents don't re-derive them."
version: 1.0.0
author: El Ferru
license: MIT
platforms: [linux, macos]
metadata:
  hermes:
    tags: [klubhub, lithqube, compose, plunk, ghcr, nx, monorepo, pnpm, golang, nuxt]
    homepage: https://github.com/lithqube/klubhub
    related_skills: [nx-workspace, docker-management, verifying-infra-as-code, github-pr-workflow]
---

# KlubHub DJ repo procedures

KlubHub is a multi-product monorepo of independent music tools (DJ ships first; Promoter and Label follow). This skill encodes the project-specific contracts so agents don't rediscover them by reading the same code paths over again.

If you have not already read `.hermes.md` at the repo root, **read it first** — it points here and tells you which Nx project name to use, which secrets never to write down, and which em-dash rule applies.

## When to load this skill

Load when the user's task involves **any** of:

- Editing `docker-compose*.yml`, `apps/dj/Dockerfile`, `scripts/stack_setup.py`, `scripts/setup.sh`, or `scripts/garage-bootstrap.sh`
- Editing `apps/site/public/newsletter.js`, `apps/site/build.mjs`, `apps/site/emails/*`
- Editing `.github/workflows/*.yml` (packaging, deployment, Plunk key wiring)
- Wiring or debugging Plunk newsletter / workflow / templates
- GHCR image naming, tagging, or publishing
- Touching `docs/PRODUCTION.md`, `docs/SELF-HOSTING.md`, `docs/CONFIGURATION.md`, `docs/ARCHITECTURE.md`, `docs/container-images.md`
- Anything `apps/dj/api/**` (Go) or `apps/dj/app/**` (Nuxt) that needs the runtime contract

Do **not** load for purely cosmetic UI changes to `apps/site/public/styles.css` that don't touch the newsletter pipeline.

## Hard rules (read before editing anything)

1. **Nx project name is `@dev/dj`**, not `dj`. Using the shorter form silently resolves to the wrong project or fails. Use `pnpm nx run @dev/dj:<target>`. Set `NX_DAEMON=false` for this repo.

2. **Plunk newsletter event is `klubhub.subscribed`**, not `contact.subscribed`. Plunk's documented `contact.subscribed` system event does NOT fire on `/v1/track` upserts — confirmed live with `figu@figuds.com` after PR #11 was filed. Use namespaced custom events per Plunk's own waitlist recipe.

3. **GHCR image naming**: `ghcr.io/lithqube/klubhub-<product>-api`. DJ ships as `klubhub-dj-api`. Future products: `klubhub-promoter-api`, `klubhub-label-api`. Do not push to the legacy `ghcr.io/lithqube/klubhub-dj` package (retired).

4. **Compose service name is `app`**, not `api`. The rename is consistent across `docker-compose.yml`, `docker-compose.prod.yml`, `docker-compose.dev.yml`, all docs, and `scripts/test_stack_setup.py`. Old `api:` blocks are stale.

5. **Dev runs API-only**, prod runs full stack. Dev sets `SERVE_FRONTEND=false` (correct behavior — `/` and `/tracklist` returning 404 is the proof that gating works); Nuxt runs on the host at `:4200`. Prod sets `SERVE_FRONTEND=true` so the API container serves the static site at `/`. Do not flip these without changing the corresponding docs section.

6. **Distinct compose project names**: dev uses `klubhub-dj-dev` (default from compose file), prod uses `klubhub-dj-prod`. Distinct volume names. No fixed `container_name` in either. Sharing these causes cross-host contamination.

7. **OAuth `gh` cannot push `.github/workflows/*` files**. Use the SSH remote:
   ```
   git remote set-url origin git@github.com:lithqube/klubhub.git
   git push
   ```
   Restore the OAuth remote after.

8. **`main` is protected by an approval gate; force-push hangs**. Create a `fix/...` branch and PR instead. For env-binding fixes to workflows, use a single-commit single-file PR — the smallest diffs clear the gate fastest.

9. **Plunk SKs (`sk_*`) and other secrets**: never in conversation, PR descriptions, commit messages, logs, or repo files. Use Docker secrets mounted at `/run/secrets/<name>` and the runtime env var `<NAME>_FILE`. See `references/plunk-secrets.md`.

10. **Em dashes (`—`)**: forbidden in `apps/site/public/index.html` and `apps/site/emails/*.html` body copy. Replace with `, `. Permitted in code comments, HTML comments, README.md, docs/*.md.

## Reference index

The detail lives in `references/`:

| File | When to load |
|---|---|
| `references/plunk-secrets.md` | Wiring PK/SK, env injection, Docker secret, three-project split |
| `references/plunk-workflow-contract.md` | Workflow trigger names, custom vs system events, what MCP can and cannot do |
| `references/compose-runtime.md` | dev/prod separation, `app` service, `SERVE_FRONTEND`, port mappings, depends_on ordering |
| `references/ghcr-images.md` | Image naming, per-product packages, legacy retirement |
| `references/nx-tasks.md` | Project names (`@dev/dj`, `dj-api`), targets, daemon disabling |
| `references/github-workflows.md` | OAuth gate, SSH push, env binding for secrets, PR-preview placeholder pattern |
| `references/em-dash-policy.md` | Where em dashes are forbidden, where they're allowed, replacement rules |

## Verification matrix (run before claiming "done")

| Change touches | Verify |
|---|---|
| `docker-compose*.yml` | `python3 -m unittest discover -s scripts -p 'test_stack_setup.py'` |
| `apps/site/public/newsletter.js` / `apps/site/build.mjs` | `node --test apps/site/site.test.mjs` (target: 9/9 pass) |
| `apps/dj/api/**` | `cd api && go test ./...` |
| Any workflow change | `gh workflow run` against the branch; check the run logs for the env-bound secret being masked (e.g. `PLUNK_PUBLIC_KEY: ***`) |
| Any Plunk change | `rg -n "contact\.subscribed" apps/ docs/ scripts/` — should return only the deliberate "Why custom, not system" docs paragraph |

## Common pitfalls (real ones hit this session)

- **`contact.subscribed` not firing on `/v1/track` upserts** — see `references/plunk-workflow-contract.md`.
- **`PLUNK_PUBLIC_KEY: ***` empty in workflow logs** — root cause was missing `environment: github-pages` on the `build` job. PR #9 was the fix.
- **Workflow trigger event rename locked after first execution** — set it correctly the first time.
- **`Go` tests fail because `apps/dj/api` isn't on the test runner's `$GOPATH`** — run from inside `apps/dj/api/` or via `pnpm nx run dj-api:test`.
- **OAuth workflow push blocked** — see Hard Rule #7.
- **`force-with-lease` times out on protected `main`** — see Hard Rule #8.
- **Force-push from `git push` may need `git push --force-with-lease origin <branch>` if you've rebased locally** — the lease check prevents stale-force-push.

## What this skill is NOT

- Not a replacement for reading the actual repo files — it's an index of gotchas, not the source of truth.
- Not a CI workflow contract — workflows are versioned in `.github/workflows/`, not here.
- Not a generic "Compose is hard" guide — it only encodes KlubHub-specific defaults.
