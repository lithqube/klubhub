# KlubHub DJ — Production Run

> **Audience.** This document is written for an autonomous agent that has been asked to deploy, verify, or upgrade KlubHub DJ in production. The first sections are operational and prescriptive. The final section ("For humans") covers the small set of cases that still need a human in the loop.

The KlubHub DJ production stack is **one published app image** running supervised Go + Nuxt/Node + Playwright, plus PostgreSQL 16 and Garage S3. Three Compose services total. UI and API share port 8080 on the host. Internal Nuxt (port 3000) and PostgreSQL are **not** published.

| Service | Image | Role | Host port |
|---|---|---|---|
| `db` | `postgres:16-alpine` | PostgreSQL 16 | not published |
| `storage` | `dxflrs/garage:v2.2.0` | S3-compatible object store | loopback 39000 (S3), admin never published |
| `app` | `ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG}` | Single Go + Node + Chromium container, supervised | loopback 8080 |

Default state: **everything bound to loopback**. Remote access requires explicit override of `BIND_ADDRESS` and `S3_BIND`, and a host reverse proxy with authentication. TLS and CORS are not authentication.

---

## 1. Pre-flight (agent)

Before any command, confirm:

- [ ] Docker Engine 24+ with Compose v2, daemon running.
- [ ] Architecture is `linux/arm64`. Production images do not claim amd64 support.
- [ ] Host can pull `ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG:?Set IMAGE_TAG to the published release}`.
- [ ] Selected `IMAGE_TAG` actually contains the runtime fixes. The default `v1.0.0` is a placeholder; local changes do not propagate to existing GHCR tags.
- [ ] Working directory is the repo root containing `docker-compose.prod.yml`.
- [ ] The current branch is one you intend to deploy. Do not deploy a dirty tree.
- [ ] No `.env` is being silently imported — every Compose call uses `--env-file /dev/null`.
- [ ] Host ports 8080 and 39000 are free for the loopback publication.

If any item fails, **stop and report**. Do not invent credentials, do not relax loopback binds "to make it work," do not pick a different image.

```bash
docker --version
docker compose version
docker info --format '{{.OSType}}/{{.Architecture}}'
git status --porcelain | wc -l   # must be 0 or a known intentional change
docker compose --env-file /dev/null -f docker-compose.prod.yml config --services
```

The last command must print exactly: `db`, `storage`, `app`. If it prints `api`, you are reading a pre-rename checkout — do not proceed.

---

## 2. Private state (agent)

Production private state lives in `.local/prod/` (gitignored). Do not read or print its contents. Do not create it manually — the setup script owns its lifecycle.

| File | Purpose |
|---|---|
| `postgres_password` | PostgreSQL password |
| `garage_rpc_secret` | Garage node RPC secret |
| `garage_admin_token` | Garage admin token |
| `token_encryption_key` | AES-256-GCM key for OAuth tokens |
| `ical_secret` | Calendar/PDF bearer secret |
| `s3_access_key` | Garage application key id (provisioned by setup) |
| `s3_secret_key` | Garage application key secret (provisioned by setup) |
| `garage.toml` | Garage configuration (copied from `garage.toml.example`) |

Setup creates these files with mode `0600` inside a directory of mode `0700`. Rerunning setup preserves existing files and only fills gaps. Setup is **not** a rotation tool — see "Secret rotation" below before changing any of these.

---

## 3. First-time deployment (agent)

Run from the repo root. Each step has a clear success condition; stop and report on the first failure.

```bash
# 3.1 — Provision private state + Garage config + bootstrap storage + start stack.
IMAGE_TAG="${IMAGE_TAG:?}" bash scripts/setup.sh prod
```

Setup prints `Stack ready.` on success. It does **not** print credentials. If you see anything resembling a key, id, or token in the output, treat it as a leak and rotate.

```bash
# 3.2 — Confirm every service is healthy.
docker compose --env-file /dev/null -f docker-compose.prod.yml ps
# Expect three rows: db, storage, app. All "running" or "healthy".
```

```bash
# 3.3 — Probe API.
curl --fail --silent --show-error http://127.0.0.1:8080/api/v1/health
# Expect HTTP 200 with JSON {"status":"healthy",...}.
# Any non-200 means the API container is not ready; inspect logs before continuing.
```

```bash
# 3.4 — Probe UI through the API reverse proxy.
curl --fail --silent --show-error --output /dev/null http://127.0.0.1:8080/
# Expect HTTP 200. A 404 here means the embedded Nuxt is not running — the app
# container is broken, not "just the API".
```

A passing `/api/v1/health` does **not** prove the UI or Playwright renderer works. To prove end-to-end, exercise real flows:

```bash
# 3.5 — Exercise a real upload + a generated tracklist image.
# (Use the project's Bruno suite against the loopback API; requires secrets
# already wired by setup.)
cd tests/bruno
pnpm dlx @usebruno/cli@4.1.0 run --env local
```

If the Bruno run reports failures, the deployment is not healthy even if Compose says `running`. Fix the failures before declaring done.

---

## 4. Verification matrix (agent)

Run all of these after any change to image tag, environment variables, or infrastructure. **All must pass.**

| Check | Command | Pass criteria |
|---|---|---|
| Image pullable | `docker pull "ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG}"` | Exit 0; no `403` |
| Manifest architecture | `docker buildx imagetools inspect "ghcr.io/lithqube/klubhub-dj-api:${IMAGE_TAG}" --raw` | One non-attestation descriptor with `os=linux, architecture=arm64` |
| Binary version | `docker compose ... exec -T app /api -version` | `KlubHub-DJ <version>` matches `IMAGE_TAG` |
| All services healthy | `docker compose ... ps` | Three rows, all `healthy` |
| API health | `curl --fail http://127.0.0.1:8080/api/v1/health` | HTTP 200 |
| UI served by API | `curl --fail --output /dev/null http://127.0.0.1:8080/` | HTTP 200 |
| Database reachable | `docker compose ... exec -T app /api -healthcheck` | Exit 0 |
| Storage reachable | `docker compose ... exec -T storage /garage status` | Exit 0 |
| S3 from API | `docker compose ... exec -T app /api -healthcheck` (exercises bucket) | Exit 0 |
| Garage admin not published | `ss -ltn '( sport = :3901 or sport = :3902 or sport = :3903 )'` | No LISTEN sockets on those ports |
| Postgres not published | `ss -ltn '( sport = :5432 )'` | No LISTEN socket |
| Browser download | `curl --fail --output /dev/null "$S3_PUBLIC_ENDPOINT/klubhub/<any-existing-key>"` | HTTP 200 with image bytes |

If any row fails, capture the output, capture `docker compose logs --tail 200 <service>`, and stop. Do not paper over failures with restarts.

---

## 5. Upgrade (agent)

Upgrades change `IMAGE_TAG`. They do not change infrastructure. Do not invent a migration unless one is described here or in `docs/SELF-HOSTING.md`.

```bash
# 5.1 — Confirm the new tag is reachable.
docker pull "ghcr.io/lithqube/klubhub-dj-api:${NEW_IMAGE_TAG:?}"

# 5.2 — Inspect manifest.
docker buildx imagetools inspect "ghcr.io/lithqube/klubhub-dj-api:${NEW_IMAGE_TAG}" --raw
# Expect one non-attestation descriptor, linux/arm64.

# 5.3 — Back up before rolling the image tag. Never upgrade without a backup.
bash scripts/backup.sh -f docker-compose.prod.yml /var/backups/klubhub
# Verify the archive exists and contains the expected files.

# 5.4 — Roll the new image.
NEW_IMAGE_TAG="${NEW_IMAGE_TAG}" bash scripts/setup.sh prod
# setup.sh preserves existing credentials and only recreates the app container
# with the new tag. It does not migrate data.

# 5.5 — Re-run the verification matrix from §4.
```

Database migrations run during API startup. An image-only rollback to a previous tag may be unsafe if the new image contains migration changes that have already been applied. **Retain the backup from step 5.3 until you confirm a stable state.**

Old `klubhub-dj` Compose project (pre-rename, pre-supervisor) volumes require explicit reuse or migration. Starting a new project does **not** upgrade the old one. See `docs/SELF-HOSTING.md#upgrades-and-existing-installations` before touching legacy state.

---

## 6. Reverse proxy (agent)

The published host port is `127.0.0.1:8080` by default. Production deployments expose this through a reverse proxy on the host.

```nginx
# /etc/nginx/sites-available/klubhub.conf
server {
    listen 443 ssl;
    server_name dj.example.com;

    ssl_certificate     /etc/letsencrypt/live/dj.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/dj.example.com/privkey.pem;

    # App + UI share this upstream.
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
    }

    # S3 presigned URLs point browsers at a separate, browser-reachable URL.
    # Configure S3_PUBLIC_ENDPOINT to that URL. Do not proxy /s3 here — let
    # the browser go straight to S3 with the presigned signature.
}
```

Rules:

- The app has **no general user authentication**. Authentication lives at the proxy (basic auth, OIDC, Authelia, Cloudflare Access — your choice). Do not remove the proxy auth and rely on TLS or CORS.
- Forward `Host`, `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Proto` so logs and any future rate-limiting work correctly.
- S3 is on a separate host port. Set `S3_PUBLIC_ENDPOINT` to whatever URL the browser should use (`https://s3.example.com` if you proxy it, or the bucket's public DNS if Garage exposes one). Container DNS names (`storage:3900`) and server loopback (`http://127.0.0.1:39000`) do not work for browsers.
- Do **not** forward the internal Nuxt port (3000), Garage RPC (3901), or Garage admin (3903). They are intentionally internal.
- The proxy itself does not need to live in Docker. If it does, configure its upstream to the `app` service on the internal Docker network — not loopback.

---

## 7. Backup and restore (agent)

`scripts/backup.sh` and `scripts/restore.sh` accept `-f` for the Compose file. They require the AWS CLI and S3 credentials supplied through environment variables (not command-line arguments). Read their `--help` before invoking; the contracts evolve.

```bash
bash scripts/backup.sh -f docker-compose.prod.yml /var/backups/klubhub
ls -lh /var/backups/klubhub/
bash scripts/restore.sh --help
```

Rules:

- **Restore is destructive.** It drops and recreates the database. Run restore drills against disposable volumes with non-colliding host ports — never against the production database.
- The archive contains a database dump and stored objects. It is **not** complete without the matching `.local/prod/` private state. Back up both, separately, securely.
- Restore does not guarantee the original database survives; it creates a new database from the archive. Document the actual outcome.
- Schedule backups only after a manual backup and an isolated restore drill succeed.
- Backup credentials are not the same as application S3 credentials. The script reads its own S3 endpoint and credentials from environment variables.

---

## 8. Secret rotation (agent)

Setup is provisioning, **not** rotation. Rotating via setup replaces credentials while leaving a database and Garage volume that still expect the old values. Use the procedures below instead.

| Secret | Procedure |
|---|---|
| `TOKEN_ENCRYPTION_KEY` | Replacing makes stored OAuth tokens unreadable. Reauthorize affected accounts before rotating, or migrate tokens using a script that decrypts with the old key and re-encrypts with the new one. Retain the old key with the backup until verified. |
| `ICAL_SECRET` | Invalidates existing calendar and booking URLs. Update subscribers after rotation. |
| `POSTGRES_PASSWORD` | Update both the secret file and the `ALTER USER` inside the running database. Restart the API so it picks up the new credential. |
| `garage_admin_token` | Rotate through `docker compose exec storage /garage admin token rotate ...`. Update the secret file and the admin CLI invoker. |
| `garage_rpc_secret` | Requires restarting Garage. Schedule a short maintenance window. |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | Create a new Garage key, grant it bucket access, update the secret files, restart the API, verify, then delete the old key. |

Rotation in every case means: back up first, change on the host (do not edit `/run/secrets` inside a running container), recreate the affected container so file mounts and processes pick up the change, verify with `/api -healthcheck` and a real S3 download, then revoke the old value.

---

## 9. Stop, start, destroy (agent)

```bash
# Pause without losing state.
docker compose --env-file /dev/null -f docker-compose.prod.yml stop
docker compose --env-file /dev/null -f docker-compose.prod.yml start

# Tear down. Volumes are PRESERVED by default — data survives.
docker compose --env-file /dev/null -f docker-compose.prod.yml down

# Tear down AND DELETE ALL DATA. Only for explicitly disposable environments.
docker compose --env-file /dev/null -f docker-compose.prod.yml down -v
```

`down -v` deletes volumes. It is **not** a repair command, **not** an upgrade, and **not** a troubleshooting step. If you ran `down -v` against a real production database, the data is gone and you need the backup archive plus a clean restore.

---

## 10. Common failure modes (agent)

| Symptom | Likely cause | First check |
|---|---|---|
| API container exits immediately | Migration failure or missing secret file | `docker compose logs --tail 100 app`; verify `/run/secrets/<name>` inside the container |
| `403` from `docker pull` | GHCR package private, no `read:packages` credential, or wrong image name | Check package visibility in GHCR settings; verify `IMAGE_REPOSITORY` |
| `/api/v1/health` 503 with `frontend: error` | Nuxt did not start inside the container | `docker compose logs app`; runtime supervisor exits if Nitro fails to listen |
| Browser cannot download S3 objects | `S3_PUBLIC_ENDPOINT` is loopback or container DNS | Set `S3_PUBLIC_ENDPOINT` to a browser-reachable URL |
| Container OOM killed | Playwright/Chromium + Node + Go exceeds `mem_limit: 768m` | Increase `mem_limit`, or extract Chromium into its own service (see §11) |
| `docker buildx imagetools inspect` shows `linux/amd64` | The runner is x86; you have the wrong image | Verify the runner was `ubuntu-24.04-arm`; do not deploy |

---

## 11. When to escalate the architecture (agent)

This document assumes the combined-image supervisor design. Split the design only when one of these signals fires **with a measured number**:

| Signal | Threshold to split |
|---|---|
| Memory pressure from Chromium | `app` regularly >85% of `mem_limit: 768m` |
| Cold-start latency | Nitro cold-start >2 s on real requests |
| Independent scaling required | One product needs ≫ the throughput of another |
| CVE blast radius | Node/Chromium CVEs require faster isolation than image rebuilds |
| Multi-tenant or multi-region | Public exposure or >1 region |

If splitting, the natural shape is two images per product (`<product>-api`, `<product>-frontend`), then optionally a third (`<product>-renderer`) for Playwright isolation. **Do not jump straight to Kubernetes.** A docker-compose supervisor scales further than people think for self-hosted, single-tenant workloads. Switch orchestrators when you have ≥3 products running concurrently or need rolling updates / secrets rotation / autoscaling.

---

## For humans

This document is written for agents. If you're a human setting up KlubHub DJ, you almost certainly don't need most of what's above. The short version:

1. Pick the published image tag you want from GHCR.
2. Run `bash scripts/setup.sh prod`. It will ask no questions, generate private credentials under `.local/prod/`, bootstrap Garage, and start the stack.
3. Visit http://127.0.0.1:8080. That's the app.

What you do need a human for:

- **First-time GHCR setup.** The published `ghcr.io/lithqube/klubhub-dj-api` package is private by default after the first workflow run. Open Settings → Packages → `klubhub-dj-api` → Danger Zone → Change visibility → Public, if you want anonymous pulls. Otherwise document a credential helper for operators.
- **Production reverse proxy + auth.** KlubHub DJ has no built-in user authentication. You decide what fronts the app: nginx + basic auth, Caddy + OIDC, Cloudflare Access, Tailscale, a VPN. The reverse proxy chapter above gives a starting nginx config.
- **Backups off-host.** The backup script writes to a local path. Decide where the archives actually live (S3, rsync target, Borg repo, etc.) and secure `.local/prod/` with the same backup policy. The two halves of state — database + objects, and private credentials — must be backed up together or restore is meaningless.
- **Disaster recovery drill.** Run `bash scripts/restore.sh` once against a disposable copy of the volumes. Verify the recovered records, the recovered objects, and a rendered image. Until you've done that, you don't have backups — you have a script.
- **Decisions about image architecture changes.** The combined-image design is intentional. Don't split into separate API/frontend containers, switch to Kubernetes, or rewrite Compose for "cleanliness" until you have a measured reason. See §11.
- **Reading the source code.** The runtime supervisor is in `apps/dj/scripts/runtime.mjs` (~95 lines). The Go-side reverse proxy is in `api/internal/platform/http/frontend.go` (~64 lines). The setup script is `scripts/stack_setup.py` (~180 lines). Each is small enough to read in one sitting. Read them before changing the deployment topology.

If something in this document disagrees with what the deployment actually does, the deployment wins — fix the document, not the production state.
