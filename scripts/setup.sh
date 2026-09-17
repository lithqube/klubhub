#!/usr/bin/env bash
# Prepare private files, bootstrap Garage, then start dev backends or production.
# Defaults: dev; secrets/dev or secrets/prod (already gitignored).
# Explicit exported variables override Compose defaults; .env is never loaded.
set -euo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/stack_setup.py" "$@"
