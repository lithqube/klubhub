# Invoicing: data model, API contract and extension points

Status: Phase 5.1 + earnings (Phase 5 second wave). This document is the
contract between the Go API (`api/internal/finance`) and the Nuxt app
(`apps/dj/app/stores/{invoice,earnings}.ts`,
`apps/dj/server/api/v1/finance/**` mocks). Change both sides together.

Scope today: EU and US invoicing for a single DJ (one billing profile).
It is a product specification, not tax advice; the legal notes printed on
invoices must be reviewed by an accountant for each launch market.

## 1. Concepts

| Concept | Rule |
|---|---|
| **Kind** | `invoice` or `credit_note`. A credit note reverses an issued invoice in full and references it (`credits_invoice_id`). Amounts are stored positive; the kind carries the sign. |
| **Numbering** | Drafts have **no number** (`invoice_number: null`). The number is allocated at **issue** from the series `(number_prefix, currency)`, gap-free for issued documents. Credit notes use the prefix `CN`. Format `PREFIX-0001-EUR`. |
| **Status** | `draft → issued → paid`. `draft → cancelled` (no number consumed). `issued|paid → credited` (via credit note) or `→ corrected` (credit note + new draft). Issued documents are never edited or deleted. |
| **Customer** | Every invoice carries a `customer` party snapshot. Pre-filled from the gig's promoter contact on draft creation, editable while draft, frozen at issue. |
| **Supplier** | The billing profile, snapshotted into `billing_profile` at issue (existing behaviour). |
| **VAT treatment** | One per invoice (see §3). Drives the tax rate, the printed legal note and issue validation. |
| **Tax** | Stored per line (`invoice_lines.tax_bps`) so per-line rates can come later; today every line gets the invoice `tax_rate_bps`. `tax_breakdown` groups by rate. |
| **Withholding** | Optional `withholding_rate_bps` applied to the **subtotal** (pre-VAT fee). `net_payable = total − withholding`. Balances, "paid" and gig payment status use **net payable**. |
| **Money** | Integer minor units everywhere (`*_minor`), including summaries. |
| **Invoice vs payment vs ledger entry** | An invoice is a *billable artefact* (kind, number, snapshot, totals, lines). A payment records money moving against an invoice (deposit / payment / refund). A ledger entry is a *financial record* in the income/expense log. They are independent: invoices do not write ledger entries; payments do not write ledger entries either. The single event that creates a revenue ledger entry is a gig's `payment_status` transitioning to `paid` (see §7). |
| **Gig payment status** | Updated only by invoice payments reaching `received ≥ net_payable`. Completing an invoice's payments does **not** create a ledger entry on its own.

## 2. JSON shapes

```ts
type VatTreatment =
  | 'domestic'        // supplier charges its own country's VAT
  | 'reverse_charge'  // EU B2B cross-border; customer accounts for VAT
  | 'exempt'          // supplier is VAT-exempt (e.g. small-business scheme)
  | 'outside_scope'   // customer outside the EU (EU supplier)
  | 'us_sales_tax'    // US supplier charging state/local tax
  | 'none'            // no tax applies (e.g. US services, non-EU supplier)

interface Party {
  contact_id: string | null   // source contact, if any
  legal_name: string          // person or registered company name
  company: string             // trading / company name ('' if none)
  email: string
  address_line1: string
  address_line2: string
  city: string
  region: string              // state / province
  postal_code: string
  country: string             // ISO 3166-1 alpha-2, uppercase ('' = unknown)
  vat_id: string              // EU VAT ID incl. country prefix, e.g. DE123456789
  tax_id: string              // other tax id (EIN, national number); never an SSN
  is_business: boolean
}

interface TaxBreakdownRow { rate_bps: number; taxable_minor: number; tax_minor: number }

interface Invoice {
  id: string
  kind: 'invoice' | 'credit_note'
  gig_id: string
  credits_invoice_id: string | null     // credit notes: the invoice reversed
  replaced_by_invoice_id: string | null // corrected invoices: the new draft
  invoice_number: string | null         // null while draft
  number_prefix: string
  number_seq: number | null
  currency: string
  status: 'draft' | 'issued' | 'paid' | 'cancelled' | 'credited' | 'corrected'
  supply_date: string | null            // YYYY-MM-DD, defaults to the gig date
  issued_at: string | null
  due_at: string | null
  paid_at: string | null
  payment_ref: string
  internal_notes: string
  customer: Party
  billing_profile: object               // supplier snapshot (set at issue)
  vat_treatment: VatTreatment
  tax_rate_bps: number
  tax_note: string                      // legal wording printed on the invoice
  subtotal_minor: number
  tax_minor: number
  total_minor: number
  withholding_rate_bps: number
  withholding_minor: number
  net_payable_minor: number
  tax_breakdown: TaxBreakdownRow[]
  // computed on read from payments (0 for drafts and credit notes)
  received_minor: number   // completed deposits+payments − completed refunds
  pending_minor: number    // pending deposits+payments
  outstanding_minor: number // net_payable − received − pending, floored at 0
  updated_at: string       // optimistic-concurrency token
  created_at: string
}

interface IssueProblem { field: string; message: string } // field e.g. "customer.vat_id"

// ── Earnings / ledger (Phase 5 second wave) ─────────────────────────────

type EntryKind = 'income' | 'expense'
type EntryStatus = 'active' | 'voided'
type EntrySource = 'manual' | 'gig_payment' | 'invoice_payment'

interface Entry {
  id: string
  kind: EntryKind
  amount_minor: number
  currency: string                // ISO 4217; FIN-09 — no cross-currency aggregation
  category: string
  entry_date: string              // YYYY-MM-DD
  description: string
  notes: string
  gig_id: string | null           // optional link back to a gig
  status: EntryStatus             // 'voided' is a soft reversal; original row kept
  auto_generated: boolean         // true if created by a gig payment_status transition
  source_kind: EntrySource
  source_id: string | null        // gig or invoice that created it
  source_amount_minor: number | null  // snapshot at creation time
  source_currency: string | null
  source_description: string
  created_at: string
  updated_at: string              // optimistic-concurrency token
  deleted_at: string | null
}

interface EntryTotals {           // per currency — FIN-06 / FIN-09
  currency: string
  income_minor: number
  expense_minor: number
}

interface ProfitLossTotals {      // per currency — FIN-07
  currency: string
  income_minor: number
  expense_minor: number
  profit_loss_minor: number
}

type ReconciliationReason = 'fee_changed' | 'currency_changed' | 'payment_reversed'
type ReconciliationAction = 'update' | 'delete' | 'void' | 'keep'
type ReconciliationStatus = 'pending' | 'resolved'

interface EntryReconciliation {   // persistent FIN-04 / FIN-05 decision
  id: string
  entry_id: string
  gig_id: string | null
  reason: ReconciliationReason
  status: ReconciliationStatus
  detected_at: string             // when the source snapshot changed
  resolved_at: string | null
  resolved_action: ReconciliationAction | null
  context: Record<string, unknown>  // fee before/after, currency, etc.
  created_at: string
  updated_at: string
}
```

Contacts gain the address/tax fields of `Party` (`address_line1`, `address_line2`,
`city`, `region`, `postal_code`, `country`, `vat_id`, `tax_id`, `is_business`)
so the customer can be pre-filled. Billing profiles gain
`vat_exempt_small_business: boolean` and `default_vat_rate_bps: number`.

## 3. VAT treatment rules (`internal/finance/tax`)

`Suggest(supplier, customer)` returns `{vat_treatment, tax_rate_bps, tax_note, reason}`:

| Supplier | Customer | Suggestion |
|---|---|---|
| EU, `vat_exempt_small_business` | any | `exempt`, rate 0 |
| EU | same country | `domestic`, rate = `default_vat_rate_bps` |
| EU | other EU country, business with VAT ID | `reverse_charge`, rate 0 |
| EU | other EU country, no VAT ID | `domestic` + reason "no customer VAT ID — confirm" |
| EU | other EU country, VAT ID but `is_business` false | `domestic` + reason "customer not marked as a business — confirm" |
| EU | customer country unknown (`''`) | `domestic` + reason "customer country unknown — confirm" |
| EU | outside EU | `outside_scope`, rate 0 |
| US | any | `none`, rate 0 (user may switch to `us_sales_tax`) |
| other / unknown country | any | `none`, rate 0 |

Default notes (`tax_note`, editable while draft) come from the `tax.Notes`
registry, keyed by the supplier's country (the billing profile's
`address_country`) and the treatment. Domestic VAT, US sales tax and `none`
print no note.

| Treatment | Germany (`DE`): German / English in one string | Every other country |
|---|---|---|
| `exempt` | "Gemäß § 19 UStG wird keine Umsatzsteuer berechnet. / VAT is not charged under § 19 UStG (German small-business scheme)." | "VAT exempt: small business scheme." |
| `reverse_charge` | "Steuerschuldnerschaft des Leistungsempfängers (Reverse Charge): Die Umsatzsteuer ist vom Leistungsempfänger zu entrichten (Art. 196 MwStSystRL). / Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC)." | "Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC)." |
| `outside_scope` | "Nicht steuerbare sonstige Leistung, Leistungsort außerhalb Deutschlands (§ 3a Abs. 2 UStG). / Not subject to German VAT: the place of supply is outside Germany (§ 3a(2) UStG)." | "Outside the scope of EU VAT: place of supply outside the EU." |

The German wording is bilingual because the customer's language is unknown and
the German phrase is the one a German bookkeeper or the Finanzamt looks for
(§ 14a Abs. 5 UStG requires "Steuerschuldnerschaft des Leistungsempfängers" on
a reverse-charge invoice). It is the common practice for these cases, **not
tax advice**: have a Steuerberater confirm it. Other countries get their own
entries when their country module is written (`tax/notes_de.go` is the model);
nothing invents legal text for a country that has none.

`GET /invoices/tax-notes` returns `{data: {country, notes: {treatment: text}}}`
for the billing profile's country. The draft editor loads it once and uses it
when the user switches treatment, so wording that was a default (the country's
or the generic one) is replaced and never left under the wrong treatment, while
a note the user wrote themselves is kept. Notes already stored on existing
drafts and issued invoices are never rewritten.

Issue validation (`GET /invoices/{id}/issue-check`, enforced by `POST …/issue`):
- supplier: `legal_name`, `address_line1`, `city`, `postal_code`, `country`
- customer: `legal_name` or `company`; `address_line1`, `city`, `country`
- `supply_date` set; `total_minor > 0`; a gig currency match (existing rule)
- `domestic`: `tax_rate_bps > 0`, supplier `vat_id` (unless not EU)
- `reverse_charge`: supplier and customer `vat_id`, both EU, different countries, rate 0
- `exempt` / `outside_scope` / `none`: rate 0, `tax_note` non-empty for `exempt`/`outside_scope`
- `us_sales_tax`: any rate (state/local rules vary)
- `withholding_rate_bps` between 0 and 5000

The supplier's `vat_id` is the billing profile `tax_id` when `tax_id_kind` is
`vat` (otherwise the supplier has no VAT ID). A non-draft invoice's
issue-check returns `ready: false` with one problem `{field: "status"}`.

Totals (`tax.ComputeTotals`): integer minor units; lines grouped by rate,
tax computed once per rate group on the group's taxable sum and rounded
half-up (half away from zero); withholding = round-half-up(subtotal ×
rate / 10000); `net_payable = total − withholding`.

## 4. Endpoints (`/api/v1/finance`)

| Method & path | Body | Result |
|---|---|---|
| `GET /invoices?status=&gig_id=&kind=` | | `{data: Invoice[]}` newest first, max 100 |
| `POST /invoices` | `{gig_id, customer?, vat_treatment?, tax_rate_bps?, withholding_rate_bps?, supply_date?, due_at?, number_prefix?}` | `201 {data: Invoice}` draft; omitted fields are pre-filled (customer from gig contact, supply date from gig, treatment from `Suggest`) |
| `GET /invoices/tax-suggestion?customer_country=&customer_vat_id=&customer_is_business=` | | `{data: {vat_treatment, tax_rate_bps, tax_note, reason}}` |
| `GET /invoices/tax-notes` | | `{data: {country, notes: {reverse_charge?, exempt?, outside_scope?}}}`: the legal wording for the billing profile's country (generic English when it has no entry) |
| `GET /invoices/summaries` | | `{data: {[currency]: {currency, draft_count, issued_count, paid_count, outstanding_minor, paid_minor}}}` |
| `GET /invoices/{id}` | | `{data: Invoice, lines: InvoiceLine[]}` |
| `PUT /invoices/{id}` | `{customer, vat_treatment, tax_rate_bps, tax_note, withholding_rate_bps, supply_date, due_at, number_prefix, internal_notes, updated_at}` | draft only; totals recomputed |
| `GET /invoices/{id}/issue-check` | | `{data: {ready: boolean, problems: IssueProblem[]}}` |
| `GET /invoices/{id}/pdf` | | `application/pdf` attachment (`invoice-<number>.pdf`, `credit-note-<number>.pdf`, or `invoice-draft-<id8>.pdf`); rendered on demand, `Cache-Control: private, no-store`. Not persisted yet |
| `POST /invoices/{id}/issue` | `{updated_at}` | allocates number; `422 {error:"not_issuable", problems}` |
| `POST /invoices/{id}/pay` | `{paid_at, payment_ref, updated_at}` | issued → paid |
| `POST /invoices/{id}/cancel` | `{updated_at}` | **draft only** → cancelled |
| `POST /invoices/{id}/credit-note` | `{reason, updated_at}` | `201 {data: {credit_note, original}}` |
| `POST /invoices/{id}/correct` | `{reason, updated_at}` | `201 {data: {credit_note, original, replacement}}` (replacement is a draft copy) |
| `GET/POST /invoices/{id}/payments`, `GET/PUT /payments/{id}` | unchanged | balances use `net_payable_minor`; not allowed on credit notes; on `credited`/`corrected` originals only `kind: refund` is accepted (bounded by completed receipts) — new `deposit`/`payment` kinds are rejected as `payment_kind_not_allowed` |
| `GET /entries?kind=&status=&currency=&category=&gig_id=&from=&to=&receipt=` | | `{data: Entry[]}` newest first; each entry carries `attachment_count`; `receipt=missing\|present`: `missing` = **active expenses** with no files (income such as gig payments, and voided entries, are never "missing"); `present` = any entry with at least one file |
| `POST /entries/{id}/attachments` | multipart, one `file` part | `201 {data: EntryAttachment}`. JPEG, PNG, WebP or PDF only (detected from the bytes, not the name), 15 MB each, 10 per entry. `415 unsupported_media_type`, `413 too_large`, `409 limit_reached`, `409 inactive` (voided entry) |
| `GET /entries/{id}/attachments` | | `{data: EntryAttachment[]}` oldest first |
| `GET /entries/{id}/attachments/{aid}` | | the file; `?inline=1` shows images inline (thumbnails), a PDF is always a download. `private, no-store`, `nosniff`, sandboxed CSP |
| `DELETE /entries/{id}/attachments/{aid}` | | `204`; refused (`409 inactive`) once the entry is voided |
| `POST /entries` | `{kind, amount_minor, currency, category, entry_date, description?, notes?, gig_id?}` | `201 {data: Entry}` |
| `GET /entries/{id}` | | `{data: Entry}` |
| `PUT /entries/{id}` | same as create + `updated_at` | `200 {data: Entry}` |
| `DELETE /entries/{id}` | `{updated_at}` | soft delete; preserves history |
| `POST /entries/{id}/void` | `{updated_at}` | sets `status: 'voided'`; keeps the row for audit |
| `GET /summary?scope=month\|year&from=&to=` | | `{data: EntryTotals[]}` grouped by currency (FIN-06 / FIN-09) |
| `GET /profit-loss?scope=gig\|month\|year&from=&to=&gig_id?` | | `{data: ProfitLossTotals[]}` grouped by currency (FIN-07) |
| `GET /reconciliations?status=&reason=&gig_id=` | | `{data: EntryReconciliation[]}` (FIN-04 / FIN-05 queue) |
| `POST /reconciliations/{id}/resolve` | `{action: 'update'\|'delete'\|'void'\|'keep'}` | `200 {data: EntryReconciliation}`; applies the chosen action to the linked entry |

**Receipts on entries.** A receipt, supplier bill or scan is stored as-is in
object storage (never re-encoded; the sha256 of the original is recorded) and
listed under its entry in `finance_entry_attachments` (migration 030). It is a
separate table from `documents` on purpose: `documents` keeps one current
version per owner, while an expense can carry several files. Files are never
removed with an entry: voiding or soft-deleting an entry keeps them, and a
voided entry's files can no longer be added or removed. Uploads go through the
API (not presigned S3 links), so a phone only needs to reach one address; the
routes answer `503` when object storage is not configured. How long to keep
receipts is a tax-law question (Germany: generally 8 years for Buchungsbelege):
confirm with your Steuerberater. A phone photo needs no HTTPS (the browser hands
off to the camera app), but the phone must be able to reach the server: see
[Phone access](./SELF-HOSTING.md#phone-access).

Errors: `400 validation_failed`, `404 not_found`, `409 conflict` (stale
`updated_at`), `409 bad_state` (wrong status for the action), `422
not_issuable | invoice_not_payable | payment_kind_not_allowed | exceeds_balance`, `503` finance not
configured. Error bodies: `{error, message, problems?}`.

Ledger money uses positive integer minor units. Create/update/generated-entry
validation (including repository/helper paths) rejects values above
`9007199254740991` (`Number.MAX_SAFE_INTEGER`), including optional generated
source amounts. This is the exact JSON-number representation limit, not a
business or currency-specific cap. Summary and profit/loss compute exact
PostgreSQL numeric sums and reject the entire report with `400
validation_failed` if any per-currency income/expense total exceeds that
limit, including legacy data whose sum exceeds int64. No totals are capped,
truncated, or returned partially.

Refunds on credited/corrected originals reference the original invoice and
its receipt history. Pending **plus** completed refunds cannot exceed
completed receipts. Pending refunds reserve refundable cash but do not free
incoming-payment capacity; only completed refunds reduce net collection.
Manual `/pay` alone is not a receipt record and creates no refund capacity.

Generated-source lookup returns only the active, nondeleted entry with a
deterministic ordering; voided/deleted history remains accessible by ID.
Generic generated-entry retries against inactive history are rejected rather
than resurrecting revenue. Explicit gig re-payment may create a replacement.
Concurrent gig-income inserts use `ON CONFLICT DO NOTHING`, then reuse only
an exact matching payload or reject it as `generated_entry_conflict`, without
aborting the caller transaction.

Gig payment status sync: creating a **pending** payment never touches
`gigs.payment_status` (pending money is not cash authority). Completing a
payment, or any other status transition, re-derives the status inside the
same transaction: the invoice row is locked first, then the gig row
(`FOR NO KEY UPDATE`), and the status is aggregated across all of the
gig's `kind=invoice` documents in status `issued`, `paid`, `credited` or
`corrected`. Per invoice, received = completed payments/deposits minus
completed refunds. The gig is `paid` when any invoice has received ≥ net
payable, else `deposit_paid` when any has received > 0, else `unpaid`.
Credit notes, drafts and cancelled invoices never contribute; credited and
corrected originals keep counting their settled receipts until the cash is
refunded, so a later write on a sibling invoice cannot flip a settled gig.
The gig's `updated_at` only moves when the status actually changes, and
the transition hooks (income generation, reconciliation) see the gig fee
re-read under the lock, so a concurrent fee edit is never applied from a
stale snapshot.

Completion follows the same invoice-state rules as creation: a pending
deposit/payment cannot be completed once its invoice is credited,
corrected (`payment_kind_not_allowed`) or cancelled
(`invoice_not_payable`); only refunds may complete on credited/corrected
originals. Failing a pending payment remains allowed.

Amounts are currency-aware minor units: the gig fee (`DECIMAL`) is converted
using the currency's exponent (JPY 0, EUR/USD 2, BHD 3); a fee with more
precision than the currency allows is rejected rather than rounded.
Payment, entry and source amounts are bounded by `MAX_SAFE_INTEGER` minor
units.

### 4.1 Go API details (as implemented)

- **Validation (400 `validation_failed`)**: `vat_treatment` must be one of
  the six values (required on `PUT`); `tax_rate_bps` 0–10000;
  `withholding_rate_bps` 0–10000 on drafts (issue requires ≤ 5000);
  `supply_date` `YYYY-MM-DD`; party `country` two-letter or `''`;
  `number_prefix` is upper-cased, 1–20 letters/digits, default `INV`, and
  `CN` is reserved for credit notes. `due_at` accepts RFC 3339 or
  `YYYY-MM-DD`. `reason` ≤ 1000 chars.
- **Normalisation**: party `country` upper-cased; `vat_id` upper-cased
  with spaces and dots removed (same on contacts).
- **`POST /invoices`**: `currency` is optional and defaults to the gig fee
  currency; if sent it must match. If `vat_treatment` is given and differs
  from the suggestion, the rate defaults to the profile's
  `default_vat_rate_bps` for `domestic` (else 0) and the note to that
  treatment's default note. Lines: one line for the gig fee.
- **`PUT /invoices/{id}`** is a full replacement of the listed fields.
- **`GET /invoices`** also accepts `currency=`; an unknown `status`/`kind`
  or a malformed `gig_id` is a 400.
- **Balances**: `outstanding_minor` is non-zero only for `issued`
  invoices; `paid`, `credited`, `corrected` and `cancelled` owe nothing.
  `/pay` is a manual status change; recording payments never changes the
  invoice status by itself.
- **Summaries** (`kind = invoice` only, credit notes excluded):
  `outstanding_minor` = Σ `outstanding_minor` of issued invoices;
  `paid_minor` = Σ `net_payable_minor` of paid invoices + Σ `received_minor`
  (clamped to 0…net payable) of issued invoices.
- **Credit notes** are issued immediately with the next `CN-NNNN-CUR`
  number of the invoice currency. They copy customer, supply date, VAT
  treatment/rate/note, amounts, withholding, breakdown and lines from the
  original. They snapshot the *current* billing profile (the original's
  snapshot if there is none), have no `due_at`, and keep `reason` in
  `internal_notes`. `correct` also creates an unnumbered draft copy (same
  prefix, `internal_notes: "Replaces <number>"`, supplier snapshot taken
  when it is issued).
- **Migration of existing data** (020–022): drafts, and cancelled rows
  never issued, lose their provisional number (`invoice_number`/
  `number_seq` → null); issued rows keep theirs. `net_payable_minor` =
  `total_minor`, a one-row `tax_breakdown` is derived from the stored
  totals, and drafts get `supply_date` = gig date. Existing drafts keep an
  empty `customer` (issue-check asks for it) and `vat_treatment: none`.

### 4.2 Request boundary: Host allowlist and Origin check

The API has no authentication; it is a network-private, single-owner service.
Unsafe requests (`POST`/`PUT`/`PATCH`/`DELETE` under `/api/v1/`) pass a boundary
(`internal/platform/http/origin.go`):

1. **Host allowlist (always enforced, Origin present or not).** The `Host`
   header must be `localhost`, `127.0.0.1`, `[::1]` or the host of
   `CORS_ORIGIN` (any port). This closes DNS rebinding, where an attacker
   domain is re-pointed at a private address and the browser sends
   `Origin == Host == attacker`. Otherwise `403 csrf` with a message telling
   the operator to set `CORS_ORIGIN`.
2. **Origin check.** If `Origin` is present it must be exactly one value equal
   to scheme+`Host`, or to `CORS_ORIGIN`. Without `Origin`, browser requests
   marked cross-site/same-site via `Sec-Fetch-Site` are rejected; curl/CLI
   without `Origin` are allowed (subject to rule 1).
3. Content-Type must be `application/json` (or multipart on upload routes).

`GET`/`HEAD`/`OPTIONS` and non-`/api/v1/` paths are not gated.

**Deployment tradeoff:** reaching the API by LAN IP (`http://192.168.1.10:8080`)
or any hostname other than the above now returns 403 for writes. Set
`CORS_ORIGIN` to the exact origin users type, bare, e.g.
`CORS_ORIGIN=https://dj.example` (no trailing slash or path; the API logs a
startup warning and ignores malformed values). Only one extra host is
supported. Behind a TLS-terminating proxy, `CORS_ORIGIN` must be the public
`https://` origin. `X-Forwarded-*` headers are never trusted. The default
`http://127.0.0.1:3000` works for local use.

## 5. Frontend structure

- `app/types/finance.ts` — the shapes above.
- `app/utils/money.ts` — parse/format minor units (no float math).
- `app/utils/vatTreatment.ts` — labels and descriptions per treatment.
- `app/stores/invoice.ts` — all `$fetch` calls for invoices, payments and summaries; keeps `updated_at` per invoice.
- `app/stores/earnings.ts` — all `$fetch` calls for the ledger: entries, summary, profit-loss and reconciliations.
- `app/components/finance/` — `InvoiceList`, `InvoiceCreateDialog`,
  `InvoiceDetailSheet`, `InvoicePartyFields` (reusable for supplier later),
  `InvoiceTaxFields`, `InvoiceIssueChecklist`, `PaymentLedger`, `PaymentForm`,
  `InvoiceConfirmDialog`, plus the earnings components
  `EarningsWorkspace`, `EntryFormDialog`, `EntryList`, `EntryProfitLoss`,
  `EntryReconciliationDialog`, `EntryReconciliationPanel`, `EntrySummary`.
- `server/api/v1/finance/**` — in-memory mocks implementing this contract
  (invoices, payments, billing-profile, agreements, emails, entries,
  summary, profit-loss, reconciliations).
- `shared/finance-mock/` — pure-code implementation of the invoicing and
  earnings rules, shared between the Nitro dev mocks and the browser-only
  demo (`apps/dj/app/demo`).

## 6. Extension points (planned, not built)

| Planned feature | Where it plugs in |
|---|---|
| Second supplier tax number, company registration, legal footer (review item 5) | `billing_profiles` columns → `Party`-like supplier snapshot → `InvoicePartyFields` reused for the billing profile form; printed by the PDF renderer footer. |
| More country-specific legal notes (FR late-payment wording, other countries) | **Germany is done** (`tax/notes_de.go`, §3). Add another country the same way: register `(country, treatment)` entries in an `init()`, mirror them in `shared/finance-mock/rules.ts`, and the UI picks them up through `GET /invoices/tax-notes`. |
| Local-currency VAT (Art. 230) | `invoices.fx_rate` + `tax_minor_local`; computed in `tax.ComputeTotals`. |
| Per-line tax / editable lines | `invoice_lines.tax_bps` already per line; `tax.ComputeTotals` already groups by rate. |
| E-invoicing (EN 16931 UBL / Factur-X / Peppol) | new `Exporter` next to `PDFRenderer`, fed the same issued invoice + snapshots. |
| Archive the issued PDF | `GET /invoices/{id}/pdf` is built and renders on demand. Still planned: store the rendered document once at issue via `DocumentService` (immutable original, sha256) and serve that copy; needs a documents HTTP handler (`api/cmd/api/main.go` still wires `nil`). |
| Retention | issued invoices are immutable and never deleted; enforce in any future delete API. |

## 7. Shared rule: gig payment_status → ledger

The gig's `payment_status` is the single source of truth for "this gig was
paid". To avoid duplicate revenue, the following invariant holds across the
whole finance module:

- A gig's `payment_status` transition (`unpaid` → `deposit_paid` → `paid`,
  and the reverse) is consumed by **exactly one** income creator. Today
  that creator is the gig-payment transition itself (`payment_status → paid`
  produces one `Entry { kind: 'income', source_kind: 'gig_payment',
  auto_generated: true }`, FIN-03).
- Completing an invoice's payments updates `gigs.payment_status` but does
  **not** write a ledger entry on its own — the gig transition has already
  done that. `source_kind: 'invoice_payment'` is reserved for a future
  flow that needs to attribute a payment independently of a gig.
- Editing or voiding an invoice, or recording a refund, never creates or
  mutates a ledger entry directly. The user resolves the resulting
  `EntryReconciliation` (FIN-04 / FIN-05) explicitly via the reconciliation
  dialog.
- `Entry` totals, summaries and P&L are computed from ledger rows only.
  Invoice balances and payment records are never summed into the ledger.
