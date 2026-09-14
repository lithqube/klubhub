---
name: Bug report
about: Report something that isn't working as expected
title: "[bug] "
labels: bug
assignees: ''
---

## Describe the bug

A clear and concise description of what is wrong.

## To reproduce

Steps to reproduce the behavior:

1. `…`
2. `…`
3. `…`
4. See error

## Expected behavior

What you expected to happen instead.

## Actual behavior

What actually happens. Paste error output, stack traces, or screenshots here.

## Environment

- OS (e.g. macOS 14.5, Ubuntu 24.04):
- Docker version (`docker --version`):
- pnpm version (`pnpm --version`):
- Go version (`go version`, only for backend issues):
- Node version (`node --version`):
- KlubHub DJ commit / tag:

## Deployment mode

Which mode are you running? (delete the others)

- [ ] Frontend-only mock (`pnpm nx serve dj`)
- [ ] Full Docker stack (`docker compose up -d`)
- [ ] Hybrid (db + storage in Docker, api + frontend via `pnpm nx serve`)

## Additional context

Anything else relevant — related issues, screenshots, config snippets
(redact any secrets!).
