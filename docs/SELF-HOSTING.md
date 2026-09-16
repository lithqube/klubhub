# KlubHub DJ — Self-Hosting

This is the long-form deployment guide for self-hosted operators of
KlubHub DJ v1.0.0. It assumes the published images (arm64):

- `ghcr.io/lithqube/klubhub-dj/api:v1.0.0`
- `ghcr.io/lithqube/klubhub-dj/frontend:v1.0.0`

## System requirements

| Resource | Minimum | Recommended |
|---|---|---|
| Architecture | `linux/arm64` | same |
| CPU | 1 vCPU | 2 vCPU |
| Memory | 2 GB | 4 GB |
| Disk | 10 GB free | 20 GB (for backup retention) |
| OS | Linux with Docker 24+ + Compose v2 |  |

> **Why arm64 only?** v1.0.0 ships arm64 images because that's the
> dominant single-board and ARM-server hardware. Build matrix
> expansion to amd64 is tracked as a point release.

## Quick start

### 1. Pull the production compose file

```bash
curl -fsSL https://raw.githubusercontent.com/lithqube/klubhub-dj/v1.0.0/docker-compose.prod.yml -o docker-compose.prod.yml
curl -fsSL https://raw.githubusercontent.com/lithqube/klubhub-dj/v1.0.0/.env.example -o .env.example
```

### 2. Generate secrets

```bash
mkdir -p secrets
chmod 700 secrets
for key in POSTGRES_PASSWORD GARAGE_RPC_SECRET GARAGE_ADMIN_TOKEN \
           TOKEN_ENCRYPTION_KEY ICAL_SECRET; do
  openssl rand -hex 32 > "secrets/$key"
  chmod 600 "secrets/$key"
done

# Generate the Garage S3 access/secret pair via the storage container.
# Leave empty for now; the bootstrap script fills them in.
: > secrets/S3_ACCESS_KEY
: > secrets/S3_SECRET_KEY
```

### 3. Edit `.env` from `.env.example`

```bash
cp .env.example .env
# Fill in S3_PUBLIC_ENDPOINT, CORS_ORIGIN, BIND_ADDRESS as needed.
# The secrets files under ./secrets/ take precedence; do not duplicate
# them in .env.
```

### 4. Bring the stack up

```bash
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

### 5. Bootstrap Garage (idempotent; one-time)

```bash
bash scripts/garage-bootstrap.sh -f docker-compose.prod.yml -p klubhub
```

The script:
- assigns + applies a Garage layout if one is not already configured,
- creates the `klubhub` bucket if missing,
- creates a key named `klubhub-app-key` if missing,
- grants the key admin on the bucket,
- **writes the printed Key ID + Secret to `secrets/S3_ACCESS_KEY` and `secrets/S3_SECRET_KEY`**.

### 6. Restart the API to pick up the new S3 keys

```bash
docker compose -f docker-compose.prod.yml restart api
```

### 7. Verify health

```bash
curl -fs http://127.0.0.1:8080/api/v1/health
curl -fsI http://127.0.0.1:3000/
```

A healthy response is `{"status":"healthy", ...}` with `database`, `storage`, and one or more integration keys reporting `"ok"`.

## Reverse proxy

If you want to expose this on a network or the Internet, **put a
reverse proxy in front**. The provided compose file binds every
service to `127.0.0.1`; a reverse proxy is the only sane addition.
Caddy and nginx examples are below.

### Caddy

```caddyfile
klubhub.example.com {
  reverse_proxy localhost:3000
  reverse_proxy /api/* localhost:8080
}
```

### nginx

```nginx
server {
  listen 443 ssl http2;
  server_name klubhub.example.com;

  ssl_certificate     /etc/letsencrypt/live/klubhub.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/klubhub.example.com/privkey.pem;

  location /api/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_read_timeout 60s;
  }
  location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
  }
}
```

> **Authentication is not implemented in v1.0.0** by design (single-user
> self-hosted). If you expose the stack to the Internet, add an
> authentication gateway in the reverse proxy. Do not just port-forward
> the API to the Internet.

## Backups

`scripts/backup.sh` dumps Postgres and mirrors the Garage bucket into
a timestamped `.tar.gz`. The default output directory is `./backups/`.

```bash
bash scripts/backup.sh
```

For a custom compose file or project name:

```bash
bash scripts/backup.sh -f docker-compose.prod.yml -p klubhub /var/backups/klubhub
```

Set up a cron job to run this nightly. Suggested cadence (per v1 release plan):

```
30 3 * * *   /opt/klubhub/scripts/backup.sh -f docker-compose.prod.yml -p klubhub /var/backups/klubhub
```

## Restore

```bash
bash scripts/restore.sh -f docker-compose.prod.yml -p klubhub /var/backups/klubhub/klubhub-backup-20260914-120000.tar.gz
```

The script:
1. Halts the api service (its pool holds DB connections).
2. Validates the archive (must contain `db.sql` and `storage/`).
3. Drops + recreates `klubhub` with `ON_ERROR_STOP=1`.
4. Mirrors the captured storage back to Garage.
5. Restarts the api service.

A restore run that exits non-zero leaves the original DB intact
(no destructive step runs until all preconditions pass).

## Upgrades

```bash
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml run --rm api  # (auto-migrates on startup)
```

Roll back:

```bash
docker compose -f docker-compose.prod.yml down   # keeps volumes
# Edit .env to point at the previous tag, then:
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

> Migrations are forward-only within a major release. A full rollback
> from v1.1 to v1.0 may require a database restore from a v1.0 backup.

## Day-to-day operations

See `docs/OPERATIONS.md`.

## Threat model & security reporting

See `SECURITY.md`.
