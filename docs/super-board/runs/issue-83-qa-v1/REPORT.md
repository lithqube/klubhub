# QA v1 · issue 83 · head 6f2f2bf
Non-visual (API/DB) card: no screenshots by design.
- Local tests: cd api && go test -race -count=1 -run Webhook ./internal/promoter/marketing/  (6 pass, disposable Postgres 16)
- Full api suite: green on rerun (one earlier run exited non-zero, not reproduced)
- Mutation: dropping `AND status = 'active'` in webhook.go is caught by the new ordering test; reverted.
- Diff vs main: 337 added / 1 removed in 4 files (<= 400).
