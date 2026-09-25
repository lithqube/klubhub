# Invoicing: data model, API contract and extension points

Status: Phase 5.1. This document is the contract between the Go API
(`api/internal/finance`) and the Nuxt app (`apps/dj/app/stores/invoice.ts`,
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

Default notes (`tax_note`, editable while draft):
- `reverse_charge`: "Reverse charge: VAT to be accounted for by the recipient (Art. 196 Directive 2006/112/EC)."
- `exempt`: "VAT exempt: small business scheme." (country-specific wording is a §6 extension)
- `outside_scope`: "Outside the scope of EU VAT: place of supply outside the EU."

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
| `GET /invoices/summaries` | | `{data: {[currency]: {currency, draft_count, issued_count, paid_count, outstanding_minor, paid_minor}}}` |
| `GET /invoices/{id}` | | `{data: Invoice, lines: InvoiceLine[]}` |
| `PUT /invoices/{id}` | `{customer, vat_treatment, tax_rate_bps, tax_note, withholding_rate_bps, supply_date, due_at, number_prefix, internal_notes, updated_at}` | draft only; totals recomputed |
| `GET /invoices/{id}/issue-check` | | `{data: {ready: boolean, problems: IssueProblem[]}}` |
| `POST /invoices/{id}/issue` | `{updated_at}` | allocates number; `422 {error:"not_issuable", problems}` |
| `POST /invoices/{id}/pay` | `{paid_at, payment_ref, updated_at}` | issued → paid |
| `POST /invoices/{id}/cancel` | `{updated_at}` | **draft only** → cancelled |
| `POST /invoices/{id}/credit-note` | `{reason, updated_at}` | `201 {data: {credit_note, original}}` |
| `POST /invoices/{id}/correct` | `{reason, updated_at}` | `201 {data: {credit_note, original, replacement}}` (replacement is a draft copy) |
| `GET/POST /invoices/{id}/payments`, `GET/PUT /payments/{id}` | unchanged | balances use `net_payable_minor`; not allowed on credit notes |

Errors: `400 validation_failed`, `404 not_found`, `409 conflict` (stale
`updated_at`), `409 bad_state` (wrong status for the action), `422
not_issuable | invoice_not_payable | exceeds_balance`, `503` finance not
configured. Error bodies: `{error, message, problems?}`.

Side effect: every payment write updates `gigs.payment_status` from the
invoice balance (`paid` when received ≥ net payable, `deposit_paid` when
received > 0, else `unpaid`) in the same transaction. The gig's
`updated_at` only moves when the status actually changes.

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

## 5. Frontend structure

- `app/types/finance.ts` — the shapes above.
- `app/utils/money.ts` — parse/format minor units (no float math).
- `app/utils/vatTreatment.ts` — labels and descriptions per treatment.
- `app/stores/invoice.ts` — all `$fetch` calls; keeps `updated_at` per invoice.
- `app/components/finance/` — `InvoiceList`, `InvoiceCreateDialog`,
  `InvoiceDetailSheet`, `InvoicePartyFields` (reusable for supplier later),
  `InvoiceTaxFields`, `InvoiceIssueChecklist`, `PaymentLedger`, `PaymentForm`,
  `InvoiceConfirmDialog`.
- `server/api/v1/finance/**` — in-memory mocks implementing this contract.

## 6. Extension points (planned, not built)

| Planned feature | Where it plugs in |
|---|---|
| Second supplier tax number, company registration, legal footer (review item 5) | `billing_profiles` columns → `Party`-like supplier snapshot → `InvoicePartyFields` reused for the billing profile form; printed by the PDF renderer footer. |
| Country-specific legal notes (e.g. DE §19 UStG, FR late-payment wording) | `tax.Notes` registry keyed by `(country, treatment)`; `Suggest` already returns `tax_note`. |
| Local-currency VAT (Art. 230) | `invoices.fx_rate` + `tax_minor_local`; computed in `tax.ComputeTotals`. |
| Per-line tax / editable lines | `invoice_lines.tax_bps` already per line; `tax.ComputeTotals` already groups by rate. |
| E-invoicing (EN 16931 UBL / Factur-X / Peppol) | new `Exporter` next to `PDFRenderer`, fed the same issued invoice + snapshots. |
| PDF download endpoint | `GET /invoices/{id}/pdf` using `PDFRenderer` + `DocumentService` (store once at issue). |
| Retention | issued invoices are immutable and never deleted; enforce in any future delete API. |
