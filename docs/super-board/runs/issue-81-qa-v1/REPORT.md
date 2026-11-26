# QA v1 · issue #81 · head 760527d

- `go test -race ./internal/promoter/marketing/` : 3/3 pass (headers, transactional, bad-URL)
- `pnpm nx run api:test --skip-nx-cache` : all packages ok
- `go vet` clean; headers.go gofmt clean
- Diff: 150 added, 0 deleted

Test gaps (not written, Low): the marketing test builds the raw message from the helper output, so it proves the headers survive SMTP, not that a real sender calls the helper (no sender exists yet; P3.2-3/4 will).
