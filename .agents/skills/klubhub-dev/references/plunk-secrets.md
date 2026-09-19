# Plunk secret handling — KlubHub

KlubHub uses three Plunk projects: `klubhub-prod`, `klubhub-staging`, `klubhub-preview`. Each has its own API keypair. This file documents where each key lives and how it crosses repo boundaries.

## Two-key separation

| Key | Where it's used | Where it lives |
|---|---|---|
| **Public key (`pk_*`)** | Static site browser code (`apps/site/public/newsletter.js`) | Shipped via env injection: GitHub Actions environment secrets in the `github-pages` environment → `build.mjs` reads `PLUNK_PUBLIC_KEY` → fails-closed if missing / placeholder / wrong prefix → injects as `<meta name="plunk-public-key">` |
| **Secret key (`sk_*`)** | API container server-side (sends transactional email) | Mounted as a Docker secret at `/run/secrets/plunk_secret_key`; the runtime reads it via `PLUNK_SECRET_KEY_FILE` env var |

The `sk_*` never enters the repo. The `pk_*` is shipped via env injection only — it does NOT live in any committed file.

## Where the keys are configured

### Public key (per project)

GitHub repository → Settings → Environments → `github-pages` → Secrets:

- `PLUNK_PUBLIC_KEY` — production `pk_*` (used in prod deployments)
- `PLUNK_PUBLIC_KEY_STAGING` — staging `pk_*` (used in PR preview deployments)
- `PLUNK_PUBLIC_KEY_PREVIEW` — preview `pk_*` (used in dev preview deployments)

The `pages.yml` workflow selects which one to use based on the trigger event:

- `push` to `main` (production deploy) → `PLUNK_PUBLIC_KEY`
- `pull_request` (PR preview) → uses the inline placeholder `pk_ci_pr_preview_only_do_not_subscribe` (the PR-build step is gated with `if: github.event_name != 'pull_request'` to avoid double-deploy; the static-site build still runs to validate the bundle)
- `workflow_dispatch` → uses the env-bound `PLUNK_PUBLIC_KEY`

### Secret key (per project)

Stored outside the repo, in the operator's secret manager (1Password / Infisical / HashiCorp Vault / etc.) — never committed. The CI/CD pipeline that deploys to a host reads the SK from the secret manager and passes it to the host via the Docker secret mount.

## How the SK reaches the API container

1. Operator deploys KlubHub DJ to a host. The deployment tool reads the SK from the secret manager.
2. The deployment tool writes the SK to `/etc/klubhub/secrets/plunk_secret_key` on the host (chmod 400, owned by root).
3. The `docker-compose.prod.yml` mounts this file as a Docker secret:

   ```yaml
   secrets:
     plunk_secret_key:
       file: /etc/klubhub/secrets/plunk_secret_key

   services:
     app:
       secrets:
         - plunk_secret_key
       environment:
         PLUNK_SECRET_KEY_FILE: /run/secrets/plunk_secret_key
   ```

4. Inside the container, the Go API reads `os.Getenv("PLUNK_SECRET_KEY_FILE")` and opens that file. The secret is never an env var.

For dev: `scripts/setup.sh dev` (now `scripts/stack_setup.py`) handles secret preparation automatically — it asks the operator to paste the SK and writes it to `.local/dev/secrets/plunk_secret_key` (chmod 400). The compose file mounts the same path.

## Rotation procedure

When rotating an SK (e.g. compromise, periodic rotation):

1. Generate the new SK in Plunk → Project settings → API keys.
2. **Deploy with both old + new SK accepted** — Plunk's API key UI doesn't support overlap, so the path is: rotate → update all environments → redeploy within the SK's still-valid window. Plan a 5-minute window.
3. Update the secret manager entry.
4. Redeploy.
5. Revoke the old SK in Plunk.

When rotating a PK:

1. Generate the new PK in Plunk.
2. Update the GitHub environment secret.
3. Trigger a `pages.yml` run on `main` — the build picks up the new PK and redeploys.
4. Revoke the old PK.

## What NOT to do

- Do not commit an SK to the repo. `.gitignore` blocks `plunk_secret_key` files but the safety net is the operator, not the file.
- Do not put the SK in `apps/site/build.mjs` or any browser-side code.
- Do not log the SK. The API uses structured logging and the SK field is in the redact list (`SK_REDACT_PATTERNS` env var).
- Do not echo the SK in PR descriptions, commit messages, or shell output.

## What to verify after wiring

1. `gh workflow run pages.yml --ref main` — check the run logs that `PLUNK_PUBLIC_KEY: ***` is masked (the `***` proves the secret reached the build; empty means env binding is broken).
2. `docker exec <api-container> cat /run/secrets/plunk_secret_key` — should fail with permission denied (the secret is mounted but the API process opens it via env var, not `cat`). If it succeeds, the file mode is wrong (must be 400 inside the container).
3. Submit the newsletter form on the static site with a real email. Plunk → Activity → one workflow execution should appear.
