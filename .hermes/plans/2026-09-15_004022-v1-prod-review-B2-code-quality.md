---
gsd_state_version: 1.0
review_for: B.2
plan_file: 2026-09-15_004022-v1-prod-release.md
reviewer: code-quality reviewer (delegated)
reviewed_at: 2026-09-15
status: APPROVED-WITH-MINOR-NOTES
---

# B.2 Code-Quality Review

## Scope

Independent review of the four B.2 code surfaces (`api/Dockerfile`,
`api/cmd/api/main.go`, `api/internal/platform/http/health.go`,
`api/internal/platform/storage/storage.go`, `docker-compose.yml`) and the
new test (`api/cmd/api/main_test.go`) plus the extended
`api/internal/platform/http/health_test.go`. Focus axes per task brief:
**style, determinism, test design, secret safety**.

The spec-compliance reviewer already approved all eight spec requirements
(APPROVED-WITH-NOTES, file
`.hermes/plans/2026-09-15_004022-v1-prod-review-B2.md`). This review
covers a different surface and is independent.

## Verification performed

```bash
cd api
go build ./...                              # exit 0
go vet ./...                                # exit 0
go test -race -count=1 ./cmd/api/...        # PASS (5 tests, ~1.5s)
go test -race -count=1 ./internal/platform/http/...   # PASS (8 tests, ~1.3s)
go test -race -count=1 ./...                # PASS except pre-existing config baseline failure
gofmt -l internal/platform/http/health.go internal/platform/http/health_test.go  # both files flagged (pre-existing on base)
```

Targeted micro-tests (added in tmp, run, then removed) confirmed:

- `flag.Parse(["-healthcheck"])` correctly sets bool=true under the
  JSON-array ENTRYPOINT shape.
- `errors.Is(err, flag.ErrHelp)` correctly distinguishes help from
  parse errors.
- `fs.SetOutput(io.Discard)` discards ALL output including `-help`
  text (see Style finding 1 below).

## Findings

### Style

**S1 (minor): `-help` text is silently dropped.** `main.go:43` sets
`fs.SetOutput(io.Discard)` before `fs.Parse`. The rationale comment
("do not pollute stdout when used in containers") is correct for the
healthcheck path, but `io.Discard` also swallows `-help` output and
flag-parse error messages. The user running `docker run --rm
ghcr.io/.../api -help` gets exit 0 and **no output** — a poor UX
outcome for a CLI escape hatch that the plan explicitly calls out as
"non-fatal success".

Suggested fix (low priority, non-blocking):

```go
fs := flag.NewFlagSet("api", flag.ContinueOnError)
fs.SetOutput(os.Stderr)  // route help + errors to stderr, not stdout
healthcheck := fs.Bool("healthcheck", false, "...")
```

stderr is already used for `run()`'s fatal errors, so the routing is
consistent. The healthcheck path doesn't print anything to stdout
or stderr on success — only the `fmt.Fprintf(os.Stderr, ...)` calls in
`runHealthcheck()` failure paths, which is correct.

**S2 (nit): `var _ = errors.New` sentinel in `main_test.go:188`.** The
`errors` package is imported but only referenced by a sentinel variable
with a comment that says "remove if/when an error path test is added
that uses errors." Either remove the import + sentinel, or add the
actual error-path test. Current state is dead-weight import that
confuses readers.

**S3 (nit): field-tag alignment in `health.go:23-27`.** The struct
tags are aligned but the field widths are not consistent — gofmt wants
extra column padding. This is a **pre-existing** formatting drift on
the base commit (verified by `git stash` + gofmt); not introduced by
B.2.

**S4 (nit): import ordering in `health_test.go:13-14`.** gofmt wants
`config` before `apphttp` (alphabetical). Also pre-existing.

### Determinism

**D1 (pass with caveat): `runHealthcheck()` reads from the same env
that drives `run()`.** This is correct — the binary uses one source of
truth for `BIND_ADDRESS`/`PORT`, so the healthcheck probe points at the
right address in container or local dev. **Caveat:** if
`BIND_ADDRESS=127.0.0.1` and the API is bound only to a Unix socket or
to a different interface, the probe will fail. The plan does not address
multi-interface binding; current single-address assumption is fine for
v1.0.0.

**D2 (pass): no `time.Now()` / no random in either new test or
production code path.** All tests use `httptest.NewServer` and
`httptest.NewRecorder` which are deterministic.

**D3 (pass): test fixtures are literal `localhost:9000` /
`postgres://localhost/test` / `"test"` credentials.** No timing
dependencies, no flaky assertions. The 500 ms response budget in
`TestHealthHandler_RespondsUnder500ms` is loose enough (500 ms vs.
~0 ms in practice) to be stable on a slow CI runner.

### Test design

**T1 (pass): the suite mirrors the spec one-to-one.** For every code
branch in `runHealthcheck()` (config load failure, unreachable
server, 503 response, 200 response) there is exactly one test. The
flag-parsing table-driven test (`TestHealthcheckFlag_Parsing`) covers
all four arg shapes the plan calls out.

**T2 (pass): tests do not share global state.** Each test calls
`minimalRequiredEnv(t)` which snapshots previous values and restores
them in a `defer cleanup()`. The pattern is correctly applied to
`TestRunHealthcheck_ConfigLoadFailure_Returns1` as a separate
save/restore because it `Unsetenv`s the required keys instead of
overwriting them.

**T3 (pass): no `os.Exit` calls inside the test path.** The split
into `main()` → `run()` / `runHealthcheck()` is the right shape for
testability. The `runHealthcheck` returns an int instead of calling
`os.Exit`, so tests can assert on the code directly.

**T4 (suggestion): no concurrency test for `-healthcheck` against a
real `srv.Shutdown` mid-flight.** A test that starts the real API
server (not a fake), fires a `-healthcheck` while shutting it down
would catch a class of timing bugs. Out of scope for B.2 (the plan
doesn't ask for it) but worth noting for follow-up if the parent
wants belt-and-suspenders coverage for B.3 graceful shutdown.

**T5 (suggestion): no negative test for malformed URLs.**
`net/http.NewRequest` with `cfg.BindAddress=""` or `cfg.Port="not-a-port"`
would build `http://:not-a-port/api/v1/health`. Current code falls
through to `client.Do` which errors with a parse-failure → exit 1. The
code is correct but untested. Low value to add — the stdlib handles
this defensively.

### Secret safety

**SS1 (pass): no real credentials in the new code.** Searched all
touched files for `AKIA|ghp_|glpat-|sk-|xox[bpsr]-` and similar patterns.
Only matches are:

- `main_test.go:23-37` — env-var name lookups and `"test"` placeholders.
- `health_test.go:48` — `DiscogsAPIKey: ***` placeholder (pre-existing,
  git blame `3f274c20` from 2026-03-14). This is a placeholder, not
  a real key.

**SS2 (pass): `Dockerfile` does not bake any secret.** No `ARG`, no
`ENV` carrying keys, no `COPY` of `.env`. The `.dockerignore`
(`api/.dockerignore`, untracked but part of this PR) explicitly
excludes `.env`, `garage.toml`, `backups/`, `.git/`, `.claude/`,
`.opencode/`, plus the entire monorepo tree above `api/`. With
`.dockerignore` in place, `COPY . .` in the builder stage no longer
leaks secrets into the build context.

**SS3 (pass): `runHealthcheck()` does not log secrets.** Only env
*names* (not values) appear in error strings, and those are config
keys, not credentials.

**SS4 (observation): plan §C.3 says production compose will pass
`*_FILE` env vars "that the API already supports".** The
spec-compliance reviewer already flagged that `api/internal/platform/config/config.go`
does **not** yet read `*_FILE` env (envconfig.Process reads only direct
env). This is a release blocker for C.3 / G.1, **not** a regression
introduced by B.2. Out of scope for this review.

## Risk assessment for B.2 specifically

- **Style:** the only meaningful finding (S1) is a UX nit, not a
  correctness bug. `docker run --rm <image> -help` returns 0 but
  prints nothing — acceptable in a container, awkward in dev.
- **Determinism:** clean. No time/random sources in tests or
  production paths.
- **Test design:** comprehensive and well-isolated. Tests do not
  leak state between cases.
- **Secret safety:** clean. No real credentials introduced or
  exposed. `.dockerignore` correctly scopes the build context.

## Verdict

**APPROVED-WITH-MINOR-NOTES.**

All four spec requirements and their acceptance criteria are met (per
the spec-compliance reviewer's pass). This code-quality review adds:

- One **minor UX finding** (S1): `-help` output is silently discarded
  due to `fs.SetOutput(io.Discard)`. Recommend routing to `os.Stderr`.
- One **nit** (S2): dead `errors.New` sentinel in `main_test.go:188`.
  Recommend removing the import and the sentinel, or adding the
  promised error-path test.
- Two **pre-existing formatting drifts** (S3, S4) that gofmt wants
  re-aligned but are not regressions of this PR.

None of these are release blockers for B.2. The code is correct, the
tests are thorough, determinism is solid, and no secrets are exposed.

Recommended actions for the parent agent, in priority order:

1. (Optional) Patch S1: change `fs.SetOutput(io.Discard)` to
   `fs.SetOutput(os.Stderr)` so `-help` is actually visible.
2. (Optional) Patch S2: remove the dead `errors` import + sentinel.
3. (Optional, can be batched) Run `gofmt -w api/internal/platform/http/health.go
   api/internal/platform/http/health_test.go` to clean up S3/S4.
   Note: this also touches pre-existing formatting drift not
   introduced by B.2.

The PR can land as-is; the suggested fixes are polish, not blockers.
