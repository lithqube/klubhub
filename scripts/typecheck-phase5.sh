#!/usr/bin/env bash
# scripts/typecheck-phase5.sh
# Phase 5 typecheck: scan the Phase 5 files for fresh errors. Pre-existing
# TypeScript errors outside the Phase 5 scope are out of band for this
# wave; they are tracked in .hermes/plans/phase-5-progress.md. A new file
# or a touched line that introduces a Phase 5 error fails the build.
#
# Phase 5 surface (every file in this list must typecheck cleanly):
#   - apps/dj/app/types/finance.ts
#   - apps/dj/app/types/agreement.ts
#   - apps/dj/app/stores/finance.ts
#   - apps/dj/app/stores/agreement.ts
#   - apps/dj/app/stores/documentEmail.ts
#   - apps/dj/app/components/finance/**
#   - apps/dj/app/components/gig/GigAgreementPanel.vue
#   - apps/dj/app/components/documents/**
#   - api/internal/finance/**
#   - api/internal/gig/agreement_*.go
#   - api/internal/document/**
#   - api/internal/mailer/**
#
# Exit 0 means no new Phase 5 errors.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PHASE5_PATTERNS=(
  'apps/dj/app/types/finance.ts'
  'apps/dj/app/types/agreement.ts'
  'apps/dj/app/stores/finance.ts'
  'apps/dj/app/stores/agreement.ts'
  'apps/dj/app/stores/documentEmail.ts'
  'apps/dj/app/components/finance/'
  'apps/dj/app/components/gig/GigAgreementPanel.vue'
  'apps/dj/app/components/documents/'
  'api/internal/finance/'
  'api/internal/gig/agreement_'
  'api/internal/document/'
  'api/internal/mailer/'
)

# pnpm's pre-run dependency check tries to reinstall node_modules and
# aborts without a TTY (ERR_PNPM_ABORTED_REMOVE_MODULES_DIR_NO_TTY); that
# is an install concern, not a typecheck one.
pnpm() { command pnpm --config.verify-deps-before-run=false "$@"; }

# 1) Run the real Nuxt vue-tsc so we get a complete error list.
if ! command -v pnpm >/dev/null 2>&1; then
  echo "pnpm not found in PATH" >&2
  exit 2
fi

# Generate Nuxt types once. NUXT_PUBLIC_API_BASE is required by the proxy
# guard in apps/dj/nuxt.config.ts; the envFile Nx option supplies it.
NUXT_PUBLIC_API_BASE="${NUXT_PUBLIC_API_BASE:-http://api:8080}" \
  pnpm exec --workspace apps/dj nuxt prepare >/dev/null 2>&1 || true

OUT_FILE="$(mktemp -t phase5-tsc.XXXXXX)"
cd "$ROOT/apps/dj"
TSC_STATUS=0
NUXT_PUBLIC_API_BASE="${NUXT_PUBLIC_API_BASE:-http://api:8080}" \
  pnpm exec vue-tsc --noEmit -p .nuxt/tsconfig.json >"$OUT_FILE" 2>&1 || TSC_STATUS=$?
cd "$ROOT"

# vue-tsc exits 0 with no output when clean, and non-zero with "error TS"
# lines when it found errors. A non-zero exit WITHOUT any "error TS" line
# means vue-tsc never ran (pnpm aborted, binary missing, bad tsconfig) —
# fail loudly instead of reporting a green gate over an empty check.
if [ "$TSC_STATUS" -ne 0 ] && ! grep -q 'error TS' "$OUT_FILE"; then
  echo "Phase 5 typecheck: vue-tsc did not run (exit $TSC_STATUS):" >&2
  cat "$OUT_FILE" >&2
  rm -f "$OUT_FILE"
  exit 2
fi

# 2) Filter the error list to only files that match the Phase 5 surface.
PHASE5_ERRORS="$(mktemp -t phase5-errs.XXXXXX)"
awk -v patterns="${PHASE5_PATTERNS[*]}" '
  /^.*error TS/ {
    matched = 0
    for (i = 1; i <= NF; i++) {
      line = $i
      if (line ~ /^\// || line ~ /^apps\// || line ~ /^api\//) {
        file = line
        break
      }
    }
    for (p in patsplit_arr) {}
    n = split(patterns, patsplit_arr, " ")
    for (i = 1; i <= n; i++) {
      p = patsplit_arr[i]
      if (index(file, p) > 0) {
        matched = 1
        break
      }
    }
    if (matched) print
  }
' "$OUT_FILE" >"$PHASE5_ERRORS"

if [ -s "$PHASE5_ERRORS" ]; then
  echo "Phase 5 typecheck — errors in new surface:"
  cat "$PHASE5_ERRORS"
  rm -f "$OUT_FILE" "$PHASE5_ERRORS"
  exit 1
fi

# 3) Go: every Phase 5 .go file must compile. The api:test target
# already enforces -race -count=1 -p 1 across the whole Go module,
# including these new packages, so we delegate to that for the Go half.
if ! pnpm exec nx run api:test >/dev/null 2>&1; then
  echo "Phase 5 typecheck — api:test failed"
  pnpm exec nx run api:test
  exit 1
fi

rm -f "$OUT_FILE" "$PHASE5_ERRORS"
echo "Phase 5 typecheck OK (zero errors in Phase 5 surface; Go race suite green)."
