#!/usr/bin/env bash
# Provision the selected Compose storage service; credentials never go to stdout.
# Usage: scripts/garage-bootstrap.sh [dev|prod] [-f docker-compose.prod.yml]
set -euo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/stack_setup.py" --bootstrap-only "$@"
