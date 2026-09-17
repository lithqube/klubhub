# KlubHub DJ — Operations

Use [Setup and Self-Hosting](./SELF-HOSTING.md) for first startup and upgrades. The examples below target production with `docker-compose.prod.yml` alone. For development, select `docker-compose.yml` instead. Preserve the same project name, private-state paths, image tag, and environment overrides used at setup.

## Health and logs

Check the services and both app endpoints:

```bash
docker compose --env-file /dev/null -f docker-compose.prod.yml ps
curl --fail http://127.0.0.1:8080/api/v1/health
curl --fail --output /dev/null http://127.0.0.1:8080/
docker compose --env-file /dev/null -f docker-compose.prod.yml logs --tail=100 api
```

The production `api` service contains supervised Go and Nuxt/Node processes plus Playwright. UI and API share port 8080; internal Nuxt port 3000 is not published. A passing API probe is not sufficient evidence that the frontend or image renderer works: exercise an upload, a stored-image download from the actual browser, and image generation too.

The Go API emits structured logs via `rs/zerolog`. `LOG_LEVEL` defaults to `info`. Treat logs as sensitive: inspect and redact them before sharing, especially integration errors and URLs carrying access credentials. Use Compose service names rather than hard-coded container names.

Check Garage independently when investigating storage failures:

```bash
docker compose --env-file /dev/null -f docker-compose.prod.yml exec -T storage /garage status
```

An unauthenticated S3 request is not a reliable liveness probe. For failed browser downloads, check `S3_PUBLIC_ENDPOINT` and the independent `S3_BIND` setting. A remote browser cannot use the server's loopback URL.

## Backup and restore

Back up PostgreSQL and Garage together, and securely preserve the matching mode-specific private configuration, token encryption key, and Garage credentials. Record the Compose project, volume names, and image tag. A data archive alone is not a complete recovery plan.

The current backup/restore scripts accept `-f` for the Compose file. Do not assume they accept `-p`; use `COMPOSE_PROJECT_NAME` when an explicit project override is required. Inspect their help for the checkout you deploy:

```bash
bash scripts/backup.sh --help
bash scripts/restore.sh --help
```

The scripts require the AWS CLI and S3 credentials supplied through their documented environment. Setup's private file secrets are not automatically exported into the invoking shell. Arrange secure environment injection without printing credentials or placing them in command-line arguments, and set the S3 endpoint reachable from the backup host. Keep production and development credentials separate.

Once those prerequisites are satisfied, select the production file explicitly:

```bash
bash scripts/backup.sh -f docker-compose.prod.yml /var/backups/klubhub
```

Verify that the archive exists and contains the expected database dump and object files. Schedule backups only after a manual backup and an isolated restore drill succeed. Do not use production as the restore-drill target.

**Restore is destructive:** it drops and recreates the target database and copies stored objects. Stop writers and verify the target project, credentials, volumes, and archive before invoking it. Validation does not make a later failure safe or guarantee that the original database survives. Use an exact archive path, not a wildcard that might select multiple backups.

Follow the script's help for restore syntax, and run it first against isolated disposable volumes with noncolliding host ports. Verify records, object downloads, and rendering afterward. Document the actual result; do not label an unexecuted drill as successful.

## Secret rotation

Setup is idempotent provisioning, not a rotation tool. Deleting `.local/prod` and rerunning setup can replace credentials while leaving a database and Garage volume that still expect the old values.

Rotate on the host through a controlled maintenance procedure, not by editing or replacing `/run/secrets` inside a container. Back up the original secret securely, preserve restrictive file permissions, update the relevant service's credential state, and recreate affected containers so file mounts and processes pick up the change.

- **Token encryption key:** replacing `TOKEN_ENCRYPTION_KEY` makes previously encrypted OAuth tokens unreadable. Plan reauthorization or a token migration before switching; retain the old key with matching backups.
- **Calendar secret:** replacing `ICAL_SECRET` invalidates existing calendar and booking links. Update subscribers after the change.
- **Postgres password:** updating the secret file does not rotate the password inside an existing database. Coordinate the database change and API credentials.
- **Garage credentials:** rotate keys/tokens through Garage's supported administration flow, update their private files and configuration, verify object access, then revoke old credentials.

Do not paste generated secrets into terminals, logs, or support requests. CORS and TLS settings do not substitute for user authentication or network restrictions.

## Persistent storage and resource monitoring

Inspect actual volume metadata and container mounts rather than assuming a host path such as `/var/lib/docker/volumes`: Docker Desktop and OrbStack store volumes inside a VM. Monitor Docker disk usage, host free space, and `docker stats`. Track PostgreSQL and Garage capacity separately and retain room for backup staging.

Project-scoped volumes isolate development and production. Old `klubhub-dj` volumes need [explicit reuse or migration](./SELF-HOSTING.md#upgrades-and-existing-installations); a new empty project is not an upgrade. Production has no host-published database port, so use controlled Compose exec access or a reviewed temporary maintenance arrangement.

## Stop and start

Use stop/start to pause the existing containers without removing data:

```bash
docker compose --env-file /dev/null -f docker-compose.prod.yml stop
docker compose --env-file /dev/null -f docker-compose.prod.yml start
```

Use setup again when you need to reconcile the selected stack, retaining the same configuration and tag. Follow the setup guide for upgrades rather than starting a second API instance to trigger migrations.

`docker compose down` normally keeps named volumes. **`docker compose down -v` deletes persistent data and is only appropriate for explicitly disposable environments.** It is not a repair command. Do not delete old deployment volumes until a migration and recovery path have been verified.
