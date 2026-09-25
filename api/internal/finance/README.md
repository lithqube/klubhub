# `internal/finance` — Finance Tracker module

Phase 5 ships the entire Finance Tracker module behind this package.
This file is the orientation doc for engineers adding to or extending
it. The release note ([`docs/release-notes/v1.1.0-phase5.md`](../../docs/release-notes/v1.1.0-phase5.md))
is the user-facing reference.

## Surfaces

| Sub-resource | Models | Repository | Service | Handler |
|---|---|---|---|---|
| Billing profile | `model.go` | `repository.go` | `service.go` | `handler.go` |
| Invoices | `invoice_model.go` | `invoice_repository.go` | `invoice_service.go` | `invoice_handler.go` |
| Payments | `payment_model.go` | `payment_repository.go` | (in `payment_model.go`) | `payment_handler.go` |
| Documents | `document_model.go` | `document_repository.go` | (in `document_model.go`) | (planned) |
| Agreements (templates + instances) | `agreement_model.go` | (in `agreement_model.go`) | (in `agreement_model.go`) | `agreement_handler.go` |
| Email outbox | `email_model.go` | (in `email_model.go`) | (in `email_model.go`) | `email_handler.go` |

Shared / cross-cutting files in this package:

- `mux.go` — `Mux` composes all sub-resource handlers under `/api/v1/finance/*` and rewrites paths so each child handler sees its own sub-prefix.
- `mux_test.go` — 9 dispatch tests.
- `pdf_renderer.go` — pure-Go PDF invoice renderer ([`gofpdf`](https://github.com/jung-kurt/gofpdf) v1.16.2).
- `plunk_sender.go` — [`Plunk`](https://github.com/useplunk/plunk) transactional-email sender; the prod wiring.

## Layer contract

Every sub-resource follows the same convention used elsewhere in the
project:

```
handler    ─→  service    ─→  repository    ─→  Postgres
   │            │                 │
   │            │                 └── concrete struct (pgx) — production
   │            └─── implements contract — test fakes
   └─── http.Handler — mounted by finance.Mux
```

The service depends on interfaces, never on concrete repositories:

```go
type RepositoryIface interface { ... }       // billing
type InvoiceRepositoryIface interface { ... }
type BillingServiceIface interface { ... }  // subset of billing.Service the invoice needs
type GigFeeProvider interface { GetGigFeeInfo(ctx, gigID) (*GigFeeInfo, error) }
type EmailSender interface { Send(msg *EmailMessage) error }
type ObjectStore interface {                 // Garage S3 or in-memory fake
    PutObject(ctx, bucket, key, reader, size, contentType) error
    GetObject(ctx, bucket, key) (io.ReadCloser, error)
    DeleteObject(ctx, bucket, key) error
}
```

This is what lets `*_contract_test.go` files use lightweight fakes
instead of a real Postgres container. The integration tests
(`*_integration_test.go`) hit real Postgres via testcontainers.

## HTTP routing

The runtime mux dispatches by the first non-`/api/v1/finance`
segment:

```
/api/v1/finance/billing-profile          → BillingHandler
/api/v1/finance/invoices                 → InvoiceHandler
/api/v1/finance/invoices/{id}            → InvoiceHandler (detail)
/api/v1/finance/invoices/summaries       → InvoiceHandler (summaries)
/api/v1/finance/invoices/{id}/payments   → PaymentHandler (sub-path rewriting)
/api/v1/finance/payments                 → PaymentHandler
/api/v1/finance/documents                → DocumentHandler
/api/v1/finance/agreements/templates/... → AgreementTemplateHandler
/api/v1/finance/agreements/instances/... → AgreementInstanceHandler
/api/v1/finance/emails                   → EmailHandler
```

Path rewriting is necessary because each child handler uses its own
prefix internally (`/api/v1/invoices/...`) without the `finance`
segment. The mux strips the `finance` segment and forwards the rest.

## Email sender swappability

`EmailSender` has two implementations:

- `PlunkSender` (default for prod / dev): `POST {BaseURL}/api/v1/{ProjectID}/emails` with `Authorization: Bearer ***`, `Idempotency-Key: <msg.ID>`, JSON body matching Plunk's `from/to/cc/bcc/subject/body/bodyHtml` schema.
- `DefaultSMTPSender` (legacy / local dev without Plunk): a thin wrapper around `net/smtp.SendMail`.

The contract test `email_contract_test.go` only requires `EmailSender`,
so swapping is a one-line change in `cmd/api/main.go`:

```go
var sender finance.EmailSender
if cfg.Plunk.BaseURL != "" {
    sender = finance.NewPlunkSender(finance.PlunkConfig{...})
} else {
    sender = finance.NewDefaultSMTPSender(finance.SMTPConfig{...})
}
```

If neither is configured, mount the routes but bind a noop sender so
the outbox still records messages for later replay.

## Storage swappability

`ObjectStore` is satisfied by:

- **Garage S3** in production (the existing `storage` Garage container).
- An in-memory `fakeObjectStore` in `document_contract_test.go`.

Tests verify the contract; production wiring lives in
`cmd/api/main.go`.

## Adding a new sub-resource

1. Write the four files (`{resource}_model.go`, `{resource}_repository.go`,
   `{resource}_service.go`, `{resource}_handler.go`).
2. Define the repository interface in `_model.go` so the service can be
   faked.
3. Add `MockXxx` / `fakeXxx` implementations in the contract test.
4. Add the handler to `NewMux(...)` and a sub-case in `ServeHTTP`.
5. Add a contract test that exercises the route via the handler, and
   an integration test that hits real Postgres.

## Mock data policy

Mocks for the finance routes live only in **dev** and **staging**
builds. The prod build pipeline asserts no `mock:`-tagged fixtures
reach `apps/dj/.output/`. The Finance module itself ships **no
mocks**; the handlers are wired against real Postgres and Garage,
gated only by the absence of credentials (in which case email delivery
falls back to a noop sender so the outbox queues without dropping).

## Open production gates

These are tracked under the release note's "Open production gates"
section. The items most relevant to this package:

- Wire `finance.NewDocumentService` to `*storage.Client` (the
  adapter skeleton exists in `garage_object_store.go`; needs the
  final `*minio.Client` signature bridge).
- Document Bucket default fallback (currently must be set explicitly
  via `DOCUMENT_STORE_BUCKET`).

## Testing

```bash
# All finance-package tests at -race:
cd api && go test -race -count=1 ./internal/finance/...

# Just the contract suite (no testcontainers):
cd api && go test -count=1 ./internal/finance/... -short

# Stack setup contract (incl. email overlay):
python3 -m unittest discover -s scripts -p 'test_stack_setup.py'
```

The finance-package contract tests cover:

- Billing profile CRUD (invalid IDs, mismatched currency, missing required fields)
- Invoice drafts / issuance / numbering / cancellation / correction
- Payment create / update / listing / sum-by-invoice (deposits, partials)
- Document versioning (S3-backed via `fakeObjectStore`)
- Agreement template CRUD + instance sign / decline / expire
- Email queue / send / fail / retry lifecycle per kind
- Plunk sender payload shape, auth header, idempotency
- Mux dispatch and path rewriting

## Migration order

`internal/platform/migrations/011_…017_*.sql` are idempotent and run
in numerical order at API boot via `goose`. Each migration is
self-contained; rollback isn't supported (Phase 5 adds money and
contracts — irreversibility is a feature, not an oversight).
