# E-invoicing for KlubHub DJ + Promoter: research and plan

*Generated 2026-10-02 · Sources: ~45 (repo source at `dc96ee0`, BMF, EU Commission, KoSIT, OpenPeppol, vendor/secondary pages) · Confidence: High for the repo and German law; Medium for other EU countries; Low where marked*

> Have a Steuerberater check the tax-law items before shipping.

## Executive summary

[gflohr/e-invoice-eu](https://github.com/gflohr/e-invoice-eu) is a **stateless e-invoice generator**. It takes a UBL-shaped JSON invoice (or a spreadsheet plus a mapping file) and produces 10 formats: UBL, CII, XRechnung UBL/CII, and five Factur-X/ZUGFeRD profiles as hybrid PDF/A-3. It ships as a CLI, an npm library and a NestJS REST server.

What it does **not** do:
- calculate totals or VAT;
- check EN 16931 business rules;
- read or parse incoming invoices;
- send invoices (no Peppol);
- authenticate callers.

Its PDF/A step is "not battle-tested", by the maintainer's own README. It is WTFPL-licensed, actively released (v3.3.3 on 2026-09-30), and has effectively one maintainer.

That makes it a good fit for the job KlubHub lacks today: turning our issued invoice, whose numbers our Go `finance/tax` package already calculates, into a legally valid structured invoice. It is not a finance platform to adopt wholesale.

**Recommendation:**
1. **Close the EN 16931 data gaps in our own model first.** This is needed whatever library we pick.
2. **Add a format adapter behind an `Exporter` interface in Go.** `docs/INVOICING.md` §6 already plans this.
3. **Start with e-invoice-eu's `slim` image as a stateless, internal-only sidecar.** Add a validator sidecar (KoSIT or Mustang) for CI golden tests and a stored validation report.
4. **Build inbound parsing in Go for Promoter.** German businesses have had to *receive* e-invoices since 2025-01-01, and e-invoice-eu can't parse.
5. **Organise `billing` as one module per country.** Germany and the EU are the first; the US (§7) is the second, where the work is payee tax forms, withholding and 1099/1042-S reporting rather than an e-invoice format.

On the "standalone service for all products" idea: **yes for the stateless part** (one `einvoice` sidecar that both stacks share). **No for a stateful finance microservice for now.** Instead, extract a shared Go `billing` library that both binaries use. A stateful service would have to cross Promoter's security invariants (RLS tenancy, envelope encryption, OPA, IDs-only events) and DJ's single-tenant, no-auth model. That needs its own ADR, ideally alongside the v2 SaaS migration (`docs/v2-saas-migration-plan.md`).

---

## 1. What e-invoice-eu is

| Aspect | Finding |
|---|---|
| Formats | `UBL`, `CII`, `XRECHNUNG-UBL`, `XRECHNUNG-CII`, `Factur-X-{Minimum,BasicWL,Basic,EN16931,Extended,XRechnung}`. `ZUGFeRD-*` names are accepted as aliases. XRechnung is hard-coded to 3.0, and you can't choose the Factur-X version (issue #52). Source: `packages/core/src/format/format.factory.service.ts` |
| Peppol BIS 3.0 | No named format. Choose UBL and set `cbc:CustomizationID` yourself (docs: country-specific extensions). [Untested] |
| Input | JSON mirror of the Peppol UBL `Invoice` tree (`"cbc:ID"`, `"cbc:Amount@currencyID"`). **Every value is a string**, and amounts allow at most 2 decimals. Checked with Ajv against a JSON Schema generated from Peppol BIS 3, including code lists (UNCL5305 tax categories, UNCL1001 type codes, UNCL4461 payment means, VATEX, ISO 4217) |
| Spreadsheet mapping | YAML/JSON mapping of cells (`=Sheet.D3`) with repeating `:InvoiceLine` sections. Sheets are read with SheetJS; the visual PDF is rendered by **LibreOffice headless** (only in the full image) |
| Hybrid PDF | Built with `@cantoo/pdf-lib`. It writes PDF/A-3b XMP and an sRGB OutputIntent and attaches `factur-x.xml` / `xrechnung.xml`. **Fonts are not embedded** (issue #165), so the PDF you supply must already embed them. The README admits the PDF/A step may fail and points to the sibling `pdfa-lab` (planned for late 2026) |
| Attachments | Factur-X: extra embedded files marked `Supplement`. UBL/CII: base64 `AdditionalDocumentReference` (CSV, PDF, PNG, JPEG, XLSX, ODS) |
| Validation | Only the JSON Schema check on input. EN 16931 / CIUS **business rules are explicitly not checked**. The project recommends its sibling [e-invoice-eu-validator](https://github.com/gflohr/e-invoice-eu-validator) (Java/Mustang, `POST /validate`) or KoSIT; wrapper scripts are in `contrib/validators/` |
| REST API | `POST /api/invoice/create/:format`: multipart, where `invoice` must be a **file part** (a text field gets a 400). Optional `pdf`, `lang`, `embedPDF` and repeatable `attachment` parts. Also `POST /api/mapping/transform/:format`, `GET /api/format/list`, `GET /api/schema/{invoice,mapping}`, and Swagger at `GET /api` |
| Security | **No auth, CORS policy or rate limits.** Per-file limit is 10 MB (`MAX_SIZE_MB`); `MAX_ATTACHMENTS` is parsed but never enforced |
| Docker | `gflohr/e-invoice-eu:slim`: about 99 MB, no LibreOffice. `:latest`: about 675 MB, with LibreOffice and a JRE. Both amd64/arm64 and non-root, with nightly Trivy scans. Docker tags skipped 3.1–3.3.0, so **pin by version or digest** |
| Credit notes | UBL credit-note codes other than 384 are rewritten to `<CreditNote>`. The maintainer considers CII to have no credit notes (issue #257). [Check 381-in-CII against a validator before relying on it] |
| Maturity | 344★, 159 issues (20 open), releases every 1–6 weeks, about 43k npm downloads/month (core). **Bus factor of 1**: gflohr has 1,658 commits, the next contributor 11 |
| License | WTFPL v2: permissive, but not OSI-approved, which some legal reviewers dislike |

Agent-directed content: the repo contains `AGENTS.md`/`CLAUDE.md` contributor rules for coding agents. They were benign and were not followed.

## 2. Where KlubHub stands today

### DJ (`api/internal/finance/`, migrations 011–023)

**What we already have, which e-invoice-eu does *not* provide:**
- **VAT logic:** treatment suggestion (domestic, reverse charge, exempt, outside scope); issue checks that block an incomplete invoice; totals grouped by rate (`finance/tax/*`).
- **Invoice lifecycle:** gap-free numbering per prefix and currency; draft → issued → paid / cancelled / credited / corrected; credit notes; payments with deposits and refunds.
- **Documents:** a versioned document store in Garage, recording sha256 and the current version.
- **Email:** an outbox sent through Plunk.

**What's missing or not wired up:**
- `RenderInvoice` (fpdf) **is never called outside tests**. There is no `GET /invoices/{id}/pdf` route, and the Documents handler is wired as `nil` (`api/cmd/api/main.go:448`).
- Every draft has exactly one line, taken from the gig fee, and lines can't be edited (`invoice_model.go:289`).
- `tax/notes.go` contains no `Notes.Register` calls, so there is no §19 UStG wording.
- `billing_profiles` holds one row for the whole deployment with no owner column. It stores a single `tax_id` + `tax_id_kind`, so a seller can't hold both a VAT ID and a Steuernummer. `payment_instructions` is free text.

**EN 16931 gaps** (field IDs are BT-*; BG-* are field groups):

| Missing | Needed for |
|---|---|
| BT-32 seller tax number (Steuernummer), alongside BT-31 VAT ID | Every German invoice without a VAT ID, and §14 UStG |
| BT-10 buyer reference / Leitweg-ID, BT-13 PO, BT-12 contract reference | XRechnung (BT-10 is mandatory there); public-sector clients |
| BT-34 / BT-49 electronic addresses (e.g. scheme `EM` for email) | XRechnung (mandatory) and Peppol |
| BG-16: BT-81 payment means code (58 = SEPA), BT-84 IBAN, BT-86 BIC, BT-83 remittance information | Every format. We only have free text |
| BT-20 payment terms text | Required together with a due date |
| BT-130 unit code (`C62` piece, `HUR` hour, `LS` lump sum) | Every line |
| BT-151 / BT-118 VAT category code (S/AE/E/O/K/Z), BT-121 VATEX reason code | Every line and every VAT breakdown row. We only have `vat_treatment` and a free-text note |
| BG-14 invoice period / BT-72 per line (gig date) | Optional, but cheap to add |
| BG-20/21 allowances and charges | Travel, accommodation, hospitality buy-outs |
| Withholding (`withholding_*`) | **No EN 16931 field.** Keep it on the settlement / PDF, not in the XML totals |

### Promoter (`api/internal/promoter/*`)

There is **no money data yet**:
- No migration has amount, VAT or invoice columns; only `organizations.currency` exists.
- The OPA policy already defines `finance.read/write/approve` and `artist_fee.read`, and `finance.approve` requires the owner role.
- `promoter-app.plan.md` §P5 plans:
  - outgoing invoices reusing the DJ invoice engine (P5.2);
  - bills (P5.3), ticket payouts (P5.5) and a VAT/withholding summary (P5.7);
  - DATEV export with a ZIP bundle and GoBD-style immutability (P5.8);
  - an `AccountingExporter` interface.
- Its stated prerequisite is to **add org/owner scoping to the shared finance tables**.

## 3. Regulatory drivers (why this matters now)

**Germany (BMF letter of 15 Oct 2025, amending the 15 Oct 2024 letter):**
- **Receiving:** every domestic business must be able to receive e-invoices since **2025-01-01**, Kleinunternehmer included. An email inbox is enough.
  - For Promoter this applies today.
- **Issuing:** voluntary in 2025–26. In **2027** it is mandatory unless prior-year turnover was ≤ €800k, and from **2028-01-01** it is mandatory for all domestic B2B.
  - This affects **every DJ above the §19 threshold** (€25k prior year / €100k current year) who invoices German clubs and promoters.
- **Exempt from issuing:** Kleinunternehmer (§34a UStDV), Kleinbetragsrechnungen ≤ €250, and any supply where **either party is not established in Germany**. That last exemption covers foreign artists and gigs abroad.
- **Valid formats:** any EN 16931 format, meaning XRechnung or ZUGFeRD/Factur-X **except MINIMUM and BASIC-WL**. Peppol is not required.
- In a hybrid file, the XML prevails over the PDF. A syntax error means the file is not an e-invoice. The BMF recommends **keeping the validation report**, and the original XML must be archived unaltered for 8 years.
- Self-billing (Gutschrift) is covered in the same way. This matters for promoters who self-bill artists.

**EU (ViDA, Directive 2025/516):** from **2030-07-01**, intra-EU B2B supplies require EN 16931 e-invoices plus digital reporting, which covers touring DJs. Member-state mandates already in force or announced:
- Belgium: Peppol since 2026-01.
- Poland: KSeF in 2026, micro businesses in 2027.
- France: receive from 2026-09; SMEs issue from 2027-09.
- Spain: Verifactu in 2027; B2B dates are uncertain.
- Italy: SdI.
- Croatia: since 2026.

All of these are domestic-only until 2030.

**Domain rules that are *not* invoice fields** but belong to promoter finance:

| Rule | What it means |
|---|---|
| §13b reverse charge for foreign artists | The invoice carries the note and both VAT IDs |
| §50a EStG withholding | 15.825% of gross. €250 per-performance Freigrenze. Quarterly BZSt filing |
| Künstlersozialabgabe | 4.9% in 2026. Paid on fees to self-employed artists |

## 4. Features to bring into KlubHub

Ranked by value divided by effort. "From e-invoice-eu" means the feature, not necessarily its code.

### Tier 1: DJ, the next sprint or two

| # | Feature | Source | Notes |
|---|---|---|---|
| 1 | **PDF download + document storage** (`GET /invoices/{id}/pdf`, wire the Documents handler) | Our own gap | Prerequisite: the renderer exists but is unused |
| 2 | **EN 16931 data model** (fields in §2 table) + **editable multi-line invoices** | Needed for any format | Map `vat_treatment` → category codes: domestic→`S`, reverse_charge→`AE`+`VATEX-EU-AE`, exempt(§19)→`E`, outside_scope→`O`, intra-EU→`K` |
| 3 | **Country legal notes registry** (§19 UStG, §13b wording in DE/EN) | e-invoice-eu samples + BMF | `notes.go` has the hook but no entries |
| 4 | **Structured payment means** (SEPA code 58, IBAN/BIC, remittance = invoice no.) | e-invoice-eu schema | Bonus: an EPC/GiroCode QR on the PDF |
| 5 | **Factur-X EN16931 hybrid PDF** (our fpdf PDF + embedded CII) as the default output, plus **XRechnung** download | e-invoice-eu | fpdf already embeds DejaVu, which avoids the font-embedding gap. Never offer MINIMUM/BASIC-WL |
| 6 | **Validation report stored with the invoice** (XML + report as `documents` rows, sha256, immutable) | BMF recommendation | The documents table already supports versions |
| 7 | **Credit note (381) / corrected invoice (384) mapping** | e-invoice-eu | Check CII 381 with the validator |
| 8 | **Attach e-invoice to the issued-invoice email** | Our Plunk outbox | `attachment_ids` already exists |

### Tier 2: Promoter P5

| # | Feature | Notes |
|---|---|---|
| 9 | **Inbound e-invoice ingestion** (required since 2025): upload or email-in a Factur-X PDF / XRechnung XML → extract the XML → parse into a bill → readable view → archive the original and the validation report | e-invoice-eu **cannot** parse. Use Go: `speedata/einvoice` (BSD, reads and writes CII/UBL with native BR-* checks) or `invopop/gobl.cii`/`gobl.ubl`; pick via a spike. Use pdfcpu (Apache-2.0) to extract PDF attachments [verify] |
| 10 | **DJ → Promoter loop**: a Factur-X invoice from KlubHub DJ is auto-matched in Promoter to the booking/offer | The cross-product feature: one standard file, no shared database needed |
| 11 | **Self-billing Gutschrift** (type 389) from Promoter to artists | Covered by the mandate when both parties are domestic |
| 12 | **§13b / §50a / KSA settlement engine** (artist settlement sheet showing gross, §50a withheld, reverse-charge VAT, KSA base; quarterly §50a and annual KSA reports) | Compliance data, not invoice fields. Store as `*_enc` financial columns |
| 13 | **DATEV export** (P5.8) through `AccountingExporter`, carrying the original e-invoice XML | Already planned |

### Tier 3: later or optional

- **Spreadsheet → invoice mapping** (e-invoice-eu's second headline feature). It could serve bulk import of historical invoices or promoter settlement sheets. It needs the LibreOffice image (675 MB, which also widens the attack surface for uploaded spreadsheets). **Skip** unless users ask for it.
- **Peppol as a bring-your-own access point.** Users connect their own account with a SaaS access point (Peppyrus free tier of 100 messages/month, e-invoice.be at €0.25 per document, Storecove). Self-certifying is unrealistic: OpenPeppol fees are €1.8–6.1k a year. Germany doesn't need Peppol; Belgium does.
- **Expose `GET /schema/invoice` to the frontend** for client-side preflight in the draft editor.
- **ViDA 2030** intra-EU reporting, plus FR/ES/PL connectors when a user base exists there.

## 5. Architecture options

| | A. e-invoice-eu sidecar (recommended v1) | B. Native Go: GOBL | C. Native Go: speedata/einvoice |
|---|---|---|---|
| Formats | All 10, including the Factur-X PDF wrap | XRechnung, ZUGFeRD, Factur-X XML, Peppol, plus FR/ES/IT/PL via addons | CII (all profiles) and UBL 2.1 |
| Who calculates totals | **We do**, so `finance/tax` stays the single source of truth | GOBL calculates them itself; rounding can differ from ours | We do |
| PDF/A-3 | Yes (pdf-lib, "not battle-tested") | No | No |
| Parse incoming | No | Yes | Yes, with BR-* checks |
| Runtime cost | Extra Node container, ~99 MB `slim` | None (in-process) | None |
| Risk | Bus factor 1; no auth, so it must stay internal | Pre-1.0 API churn (v0.507); copyright assignment for contributions | "Work in progress" |
| License | WTFPL | Apache-2.0 | BSD-3 |

**Why A first:**
- It keeps our tax engine authoritative.
- It covers both the XML and the PDF/A-3 wrap that Go lacks.
- It runs fully offline and self-hosted.

Put it behind `finance.Exporter` so B or C can replace it later. Option C, or GOBL, is still needed for Promoter's inbound parsing, so we end up with both regardless.

### Proposed shape

```
┌───────────────── shared Go library: api/internal/billing ─────────────────┐
│ tax · totals · numbering · EN16931 mapping · Exporter · InboundParser    │
│ (extracted from api/internal/finance; owner/org-scoped repos per product)│
└────────────┬──────────────────────────────────────────┬──────────────────┘
        cmd/api (DJ)                              cmd/promoter
   single tenant, Garage                 RLS + envelope enc + OPA + NATS
             │ HTTP multipart (internal network only)   │
             └──────────────►  einvoice sidecar  ◄───────┘
                          gflohr/e-invoice-eu:slim@<digest>
                          + validator (KoSIT/Mustang), optional at runtime,
                            mandatory in CI golden tests
```

**Rules for the sidecar:**
- **Stateless.** No persistence or logging of invoice bodies. Put this in an ADR together with the `data_class` treatment of buyer PII crossing the process boundary.
- **Internal network only.** Never publish its port: the upstream server has no auth.
- **Pinned and gated.** Pin the image by digest. Run golden-file tests in CI that validate each supported profile with KoSIT, plus veraPDF for PDF/A-3b, so an upstream regression fails the build.
- **Same image in both stacks.** Add it to `docker-compose.prod.yml` (DJ) and `docker-compose.promoter.yml`, and add a short note in SELF-HOSTING.md.

**Why not a stateful "finance service" now:**
- DJ has no auth and one tenant. Promoter has RLS, envelope encryption, OPA and an outbox.
- A shared service with its own database would need one of two things: re-implementing all of Promoter's invariants, or exposing promoter financial PII to a weaker boundary.
- Revisit this with the v2 SaaS migration, when DJ gains tenancy too.

## 6. Suggested sequence

1. **Spike (1–2 days).**
   - Render an issued invoice with fpdf.
   - Map it to the e-invoice-eu JSON and call `Factur-X-EN16931` and `XRECHNUNG-CII` on the slim image.
   - Validate with KoSIT and veraPDF.
   - Also try parsing the output with `speedata/einvoice` and `gobl.cii`.
   - Decision gate: does the PDF/A-3 pass veraPDF? If not, try Ghostscript or wait for `pdfa-lab`.
2. **DJ data model** (Tier 1, items 1–4): migration and UI for the EN 16931 fields, editable lines, the notes registry and structured payment means.
3. **Exporter + sidecar** (Tier 1, items 5–8), with CI golden tests and stored validation reports.
4. **Extract `billing` and add owner/org scoping** (Promoter P5 prerequisite).
5. **Promoter inbound + DJ→Promoter matching** (Tier 2, items 9–10). Then self-billing, the settlement engine and DATEV (items 11–13).

## 7. Other countries: a per-country module, US first

Other countries differ less in the invoice file format and more in how the payer collects tax forms and reports at year end. So instead of special-casing Germany, `billing` holds one **module per country**. Each module answers the same questions, and the e-invoice sidecar is just Germany's and the EU's output step.

### What's different in the US

- **No e-invoice mandate and no VAT.** A plain PDF is a valid invoice.
  - A voluntary network exists: DBNAlliance, similar to Peppol, using UBL plus AS4 transport. Its first live invoice went through in April 2024. Nothing requires it ([Avalara](https://www.avalara.com/blog/en/europe/2025/07/us-e-invoicing-fragmentation-and-future-solutions.html), [Storecove](https://www.storecove.com/blog/en/digital-business-network-alliance/)).
- **Sales tax is set by each state.** Most states don't tax a performer's fee; a few tax some services or gross receipts [unverified per state].
  - If exact rates are ever needed, use a rate provider (Avalara, TaxJar) behind an interface rather than writing our own rules.
- **The burden falls on whoever pays the artist,** which in KlubHub means Promoter:

| Situation | What the payer must do |
|---|---|
| US DJ → US promoter | Collect a **W-9** (the DJ's taxpayer ID). File a **1099-NEC** if paid **≥ $2,000** in a year. That limit applies from 2026 (it was $600) and rises with inflation from 2027 ([Patriot](https://www.patriotsoftware.com/blog/accounting/1099-reporting-threshold/), [Avalara](https://www.avalara.com/blog/en/north-america/2025/07/one-big-beautiful-bill-act-1099-reporting-threshold.html)) |
| Foreign DJ → US promoter | Collect a **W-8BEN** and withhold **30% of the gross fee**; report on 1042-S / 1042. The artist can apply for a **Central Withholding Agreement** (Form 13930, or 13930-A under $10k), which bases withholding on net income at graduated rates ([IRS FAQ](https://www.irs.gov/individuals/international-taxpayers/frequently-asked-questions-faqs-about-foreign-artist-and-athlete-withholding), [IRS Pub 519](https://www.irs.gov/publications/p519)). Some states add their own withholding for out-of-state performers [unverified] |
| US DJ → German or EU promoter | Already covered above: §13b / reverse-charge invoice, and §50a withholding on the promoter's side. Treaty relief under the US–Germany treaty Art. 17 may apply [unverified] |

The pattern is the same as in Germany:

| US | German equivalent |
|---|---|
| 30% NRA withholding | §50a withholding |
| 1099 / 1042-S | §50a quarterly filing, KSK annual report |
| W-9 / W-8BEN | Freistellungsbescheinigung |

### What each country module defines

| Part of the module | DE / EU | US |
|---|---|---|
| Tax treatment rules | VAT: standard, reverse charge (`AE`), §19 exemption (`E`), outside scope (`O`) | Sales tax: usually none; optional rate lookup |
| Fields required before issuing | Steuernummer or VAT ID, the EN 16931 set (§2) | Payee legal name, address, EIN/SSN on file (payer side) |
| Legal notes on the invoice | §19 UStG, §13b / Art. 196 | None required |
| Output files | Factur-X EN16931 and XRechnung via the sidecar | PDF (existing fpdf renderer); optional UBL for DBNAlliance later |
| Withholding when paying an artist | §50a: 15.825%, €250 Freigrenze | 30% NRA, or the CWA rate |
| Year-end reporting | §50a filing, KSK report, DATEV | 1099-NEC (≥ $2,000), 1042-S |
| Payee documents | Freistellungsbescheinigung | W-9, W-8BEN |

### Implications

- **Already in the code:** `vat_treatment = us_sales_tax` and `tax_id_kind = ein` exist. What's missing for the US:
  - collecting and storing payee tax forms;
  - year-end income reports per payee;
  - the 30% / CWA withholding calculation.
- **Withholding goes in the module.** The DJ invoice's `withholding_rate_bps` should get its default rate and legal basis from the country module rather than being typed in by hand.
- **Promoter security:** US taxpayer IDs (SSN/EIN) and W-8/W-9 forms are sensitive.
  - Store them encrypted (`*_enc` / `*_bidx`, `data_class` financial), with documents in object storage.
  - Never put them in events, which carry IDs only.
  - Never send them to the e-invoice sidecar.
- **Spike task:** check GOBL's per-country regimes (it already organises tax rules by country) as a source for the module boundaries, even if we keep our own tax engine.
- **Next modules:** UK (VAT, no mandate yet), ES (Verifactu 2027), IT (SdI), FR (2026/27 reform). Add them when there are users there.

## Sources

**e-invoice-eu (repo at commit `dc96ee0`, 2026-09-30):**
- [repo](https://github.com/gflohr/e-invoice-eu): `README.md`, `LICENSE`, `packages/core/src/format/*.ts`, `src/invoice/invoice.service.ts`, `src/schema/invoice.schema.json`, `apps/server/src/**/*.controller.ts`, `apps/server/Dockerfile`, `docs/en/docs/**`, `contrib/`
- [Docs site](https://gflohr.github.io/e-invoice-eu/), including the [internal format](https://gflohr.github.io/e-invoice-eu/en/docs/details/internal-format/) page
- [e-invoice-eu-validator](https://github.com/gflohr/e-invoice-eu-validator)
- [Author's blog post](https://www.guido-flohr.net/creating-electronic-invoices-with-free-and-open-source-software/)
- Issues #11 (computing totals), #52 (versioning), #165 (font embedding), #257 (CII credit notes), #600 (FatturaPA), #624 (PDF metadata)

**German law:**
- [BMF-Schreiben 15 Oct 2025](https://www.bundesfinanzministerium.de/Content/DE/Downloads/BMF_Schreiben/Steuerarten/Umsatzsteuer/Umsatzsteuer-Anwendungserlass/2025-10-15-einfuehrung-obligatorische-e-rechnung.pdf?__blob=publicationFile&v=3) and the [BMF e-invoice FAQ](https://www.bundesfinanzministerium.de/Content/DE/FAQ/e-rechnung.html)
- [§19 UStG](https://www.gesetze-im-internet.de/ustg_1980/__19.html), [§50a EStG](https://www.gesetze-im-internet.de/estg/__50a.html), [BZSt: amount of withholding](https://www.bzst.de/DE/Unternehmen/Abzugsteuern/Abzugsteuer/Hoehe_Steuerabzug/hoehe_steuerabzug.html)
- KSA: [BMAS 2026 rate](https://www.bmas.de/DE/Service/Presse/Pressemitteilungen/2025/kuenstlersozialversicherung-abgabe-sinkt-im-jahr-2026.html), [KSK assessment base](https://www.kuenstlersozialkasse.de/unternehmen-und-verwerter/bemessungsgrundlage), [touring-artists](https://www.touring-artists.info/sozialversicherung/kuenstlersozialabgabe)

**EU:**
- [EU Commission: ViDA adoption](https://taxation-customs.ec.europa.eu/news/adoption-vat-digital-age-package-2025-03-11_en)
- [BDO: Belgium](https://www.bdo.be/en-gb/insights/news-alerts/2025/mandatory-e-invoicing-in-2026-in-belgium-three-month-tolerance-period), [BDO: Spain Verifactu](https://www.bdo.global/en-gb/insights/tax/indirect-tax/spain-veri-factu-obligation-postponed-until-2027), [Sovos: Poland KSeF](https://sovos.com/blog/vat/poland-e-invoicing-via-ksef/) (secondary)

**US:**
- [IRS: foreign artist and athlete withholding FAQ](https://www.irs.gov/individuals/international-taxpayers/frequently-asked-questions-faqs-about-foreign-artist-and-athlete-withholding), [IRS Publication 519](https://www.irs.gov/publications/p519)
- 1099 threshold under OBBBA: [Patriot Software](https://www.patriotsoftware.com/blog/accounting/1099-reporting-threshold/), [Avalara](https://www.avalara.com/blog/en/north-america/2025/07/one-big-beautiful-bill-act-1099-reporting-threshold.html) (secondary)
- DBNAlliance: [Avalara](https://www.avalara.com/blog/en/europe/2025/07/us-e-invoicing-fragmentation-and-future-solutions.html), [Storecove](https://www.storecove.com/blog/en/digital-business-network-alliance/) (secondary)

**Peppol:**
- [OpenPeppol fees](https://peppol.org/join/fees/), [phase4](https://github.com/phax/phase4), [Peppyrus](https://www.peppyrus.be/en) (vendor)

**Tooling:**
- [KoSIT validator](https://github.com/itplr-kosit/validator) + [XRechnung configuration](https://github.com/itplr-kosit/validator-configuration-xrechnung/releases)
- [Mustang](https://www.mustangproject.org/), [veraPDF REST](https://github.com/veraPDF/veraPDF-rest)
- [GOBL](https://github.com/invopop/gobl), [gobl.cii](https://github.com/invopop/gobl.cii), [gobl.ubl](https://github.com/invopop/gobl.ubl), [speedata/einvoice](https://github.com/speedata/einvoice)

**Internal:** `api/internal/finance/**`, `api/internal/platform/migrations/011–023`, `docs/INVOICING.md` §6, `.claude/plans/promoter-app.plan.md` §P5, `api/internal/platform/authz/policies/authz.rego`, ADR 0001 and 0006.

## Methodology

Three parallel research tracks:
1. a source-level read of a clone of e-invoice-eu, plus GitHub, npm and Docker Hub metadata;
2. a web review of the EU and German regulations and of the Go and validator ecosystem, with official sources preferred;
3. a read-only survey of the KlubHub finance code and plans.

Claims marked [unverified] or (secondary) rest on a single or non-official source.
