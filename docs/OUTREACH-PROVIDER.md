# Outreach email: provider terms and the `OUTREACH_PROVIDER_PERMITS_OUTREACH` gate

Phase 04.10 (Availability & Outreach) sends **cold B2B outreach** in both
directions: a DJ emails a promoter contact to ask for a booking, a promoter
emails a talent contact with an offer. In both cases the recipient is a
contact in the operator's own database who **has never opted in**.

That is a different legal category from the transactional mail this product
already sends (invoice receipts, agreements, password resets). It also runs
straight into most email providers' acceptable-use terms.

**Outreach is therefore disabled by default.** This is deliberate. See
[Why the gate exists](#why-the-gate-exists) and
[Enabling outreach](#enabling-outreach).

---

## Why the gate exists

The application's default email provider is Plunk
([useplunk.com](https://www.useplunk.com)), the open-source platform we
already use for transactional mail. Its Terms of Service §5 is titled
**"NO COLD EMAILING POLICY"** (last updated October 11, 2026) and reads:

> Plunk strictly prohibits cold emailing. All emails must be sent to
> recipients who have either explicitly opted in to receive emails from you,
> or have an existing business relationship with you (for transactional
> emails only).

Among its **Prohibited Activities** it lists, verbatim:

- ✗ Sending emails without documented consent
- ✗ **"One-time" cold outreach emails**

Its §4 Acceptable Use Policy separately bars *"unsolicited bulk emails"*,
and §10 lets Plunk suspend or terminate the account for AUP violations,
with repeat violations permanent.

**Using the transactional endpoint does not exempt you.** Plunk's own guide
states: *"If you are unsure whether to treat something as transactional or
marketing, treat it as marketing: get consent, include an unsubscribe link
and follow the marketing rules."* Its legal table reserves legitimate
interest for genuine service email and requires **explicit opt-in** for
marketing.

So: sending booking requests through Plunk would breach Plunk's terms from
the first send. The gate stops that from happening by accident.

---

## What the gate does

`api/internal/platform/email/outreach_gate.go` defines a fail-closed
capability check, wired into both send paths:

| Endpoint | Without attestation | With attestation |
|---|---|---|
| `POST /api/v1/outreach/requests` (DJ → promoter) | `422` `outreach_not_permitted` | normal send path |
| `POST /api/v1/bookings/offers` (promoter → DJ) | `422` `outreach_not_permitted` | normal send path |
| All outreach **reads** (slots, matches, templates, sent log, suppressions) | always work | always work |
| Finance email (invoices, agreements, receipts) | always works — **not gated** | always works |

Two properties worth stating explicitly:

- **It is not derived from "is a provider configured?"** A configured
  sender says nothing about whether its terms permit cold outreach, so the
  gate never infers permission from configuration.
- **It fails closed.** `nil`, unset, `"false"`, or any value other than
  `"true"` refuses the send. A new deployment cannot accidentally send.

---

## Enabling outreach

Set the attestation **only after** confirming your provider's terms permit
this use:

```bash
# DJ API (.env / docker-compose)
OUTREACH_PROVIDER_PERMITS_OUTREACH=true

# Promoter API (.env / docker-compose) — same variable, separate file
OUTREACH_PROVIDER_PERMITS_OUTREACH=true
```

### Self-hosted Plunk

The AGPL-3.0 software is self-hostable, and you become the data controller
for your own instance (ToS §7/§14):

```bash
PLUNK_BASE_URL=https://plunk.your-domain.example
PLUNK_API_KEY=...
PLUNK_FROM_EMAIL=bookings@your-domain.example
OUTREACH_PROVIDER_PERMITS_OUTREACH=true
```

Note the residual obligation: ToS §14 says §4 and §5 *still apply* to
self-hosted deployments. Self-hosting removes Plunk's suspension and
termination power and puts you in control of your own policy — it is not a
blanket exemption, and you take on the compliance responsibility directly.

### A different provider (API key)

The send path is provider-agnostic; the gate is not Plunk-specific. Any
provider works as long as its terms permit unsolicited B2B outreach and you
hold a documented lawful basis. Providers commonly considered for this
shape of send include Amazon SES (which permits opted-out B2B under a
defined policy), Postmark, and Mailgun — **verify the current terms of
whichever you choose**; they change, and this document does not track them.

---

## What the attestation does *not* do

Setting the flag records an operator decision. It is **not** a compliance
certificate and it transfers no legal responsibility. After enabling it,
you remain responsible for:

- **A lawful basis** for the sends. GDPR Art. 6(1)(f) (legitimate
  interest) requires a documented balancing test — keep the LIA on file.
  Plunk's ToS §4 also asks you to keep consent records where consent *is*
  the basis you rely on.
- **Honouring opt-outs.** The app enforces its own suppression list and
  exposes a one-click "Do not contact" action in the sent log, but
  **inbound reply handling is not implemented** — a promoter replying
  "stop" is not automatically suppressed. Until it is, treat manual
  suppression as a required operational step.
- **Recipient-jurisdiction law.** CAN-SPAM (physical address, working
  unsubscribe), GDPR, and CASL obligations differ by recipient location,
  not sender location.
- **Rate limits and reputation.** `OUTREACH_MAX_PER_DAY` (default 50)
  bounds volume; Plunk monitors bounce/complaint rates and may throttle or
  suspend a project regardless of your attestation (§5).

---

## Related

- `SECURITY.md` — the DJ API has no application-layer authentication; put
  `/api/v1/outreach/*` behind reverse-proxy auth if the stack is reachable
  from the internet. The daily cap bounds the blast radius but is not
  authentication.
- `docs/SELF-HOSTING.md` — deployment and proxy guidance.
- `.planning/phases/04.10-availability-outreach/04.10-SPEC.md` §9 — the
  compliance design (suppressions, sender identity, ReplyTo, opt-out
  footer, per-day cap).