# KlubHub DJ — Operations

Day-to-day operations for a self-hosted KlubHub DJ deployment.

## Health

`GET /api/v1/health` returns:

```json
{
  "status": "healthy",
  "integrations": {
    "database":     { "status": "ok" },
    "storage":      { "status": "ok" },
    "spotify":      { "status": "unconfigured" },
    "discogs":      { "status": "ok" },
    "musicbrainz":  { "status": "ok" },
    "instagram":    { "status": "ok" }
  },
  "migrations_ok": true
}
```

Subsystems reporting `error` cause the HTTP status to drop from `200`
to `503` so Docker Compose health-checks can detect them. Use
`docker ps` to see the per-service state and `docker logs` for
individual container output.

## Log shipping

The api binary emits JSON-structured logs via `rs/zerolog`. Pipe
stdout into any aggregator (Loki, journald, Vector, Fluent Bit).

```bash
docker compose -f docker-compose.prod.yml logs api | tee /var/log/klubhub-api.log
```

`rs/zerolog` honors `LOG_LEVEL` (default `info`). Levels: `debug`,
`info`, `warn`, `error`.

## Log redaction (sanitization)

The api redacts credential values (`client_secret`, `access_token`,
etc.) at the integration boundary so a transport error or worker
failure cannot leak them via logs or the persisted `scheduled_posts.last_error`.
See `internal/social/sanitize.go` for the implementation; the regex
patterns are regression-tested in `internal/social/sanitize_test.go`.

## Routine maintenance

### Rotate `TOKEN_ENCRYPTION_KEY`

```bash
NEW_KEY=$(openssl rand -hex 32)
echo "$NEW_KEY" > secrets/TOKEN_ENCRYPTION_KEY.new
docker compose -f docker-compose.prod.yml exec -T api sh -c 'mv /run/secrets/TOKEN_ENCRYPTION_KEY /run/secrets/TOKEN_ENCRYPTION_KEY.bak; ln -sf TOKEN_ENCRYPTION_KEY.new /run/secrets/TOKEN_ENCRYPTION_KEY'
docker compose -f docker-compose.prod.yml restart api
# Then re-authorize Instagram (or any other stored credentials) from
# the UI to encrypt them with the new key. The old key can be deleted
# once no OAuth tokens reference it.
```

### Rotate `ICAL_SECRET`

```bash
NEW=$(openssl rand -hex 32)
echo "$NEW" > secrets/ICAL_SECRET
docker compose -f docker-compose.prod.yml restart api
# Existing calendar URLs (calendar subscribers) MUST be updated to
# use the new secret. The API will return 401 on the old secret.
```

### Verify Garage storage is mounted

```bash
docker compose -f docker-compose.prod.yml exec -T storage /garage status
```

### Run an ad-hoc restore drill

```bash
bash scripts/backup.sh -f docker-compose.prod.yml -p klubhub /tmp/drills
bash scripts/restore.sh -f docker-compose.prod.yml -p klubhub /tmp/drills/klubhub-backup-*.tar.gz
```

The drill exercises the same code path as a real disaster recovery.
Document the result in your change log.

## Disk monitoring

Two volumes persist: `db_data` and `storage_data`. Add a disk-space
alert at 80% full on the host that backs them.

```bash
df -h /var/lib/docker/volumes/klubhub-dj_db_data/_data \
      /var/lib/docker/volumes/klubhub-dj_storage_data/_data
```

## Container resource limits

The prod compose sets conservative per-container mem/cpu. Monitor with
`docker stats`. Tune via:

```yaml
services:
  api:
    mem_limit: 768m
    cpus: '1.0'
```

## Stop / start

```bash
docker compose -f docker-compose.prod.yml stop    # graceful: API runs Shutdown
docker compose -f docker-compose.prod.yml start   # quick resume
docker compose -f docker-compose.prod.yml down -v # DESTRUCTIVE — drops volumes
```

> `down -v` deletes the Postgres and Garage data volumes. Use only
> after a backup is in hand and verified.
