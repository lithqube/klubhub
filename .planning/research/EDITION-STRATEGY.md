# Edition strategy: what to keep open, what to charge for

**Researched:** 2026-10-03
**Status:** Recommendation. Decisions needing the owner are listed in §8.
**Owner decision (2026-10-03):** everything built so far stays in the open-source edition, including Resident Advisor import, which is now on by default (it can be switched off). §6.3 and the "default-off integrations" rows below were written before this and no longer apply to RA; they still describe the planned SoundCloud and Spotify integrations.
**Updates:** `MONETIZATION.md` (2026-03-20), which predates KlubHub Promoter and the e-invoicing work, and `docs/EDITIONS.md` (the flag mechanism).
**Method:** desk research only (web sources below plus the repo's own plans). No customer interviews, no survey data of our own. Every claim is tagged with how far it can be trusted; the weak ones are called out in §7.

---

## 1. Answer in one page

1. **Keep the existing rule. It is supported by the evidence:** *own your data free, rent infrastructure paid.* Successful open-source products charge for **hosting, convenience, shared infrastructure and teams**, not for the user's own records. Ghost, Plausible and Immich all follow it.
2. **The main product to sell is "we run it for you", not feature unlocks.** Ghost(Pro) has no free tier and funds the whole project; self-hosters pay nothing. Plausible's self-hosters give about $300/month in donations against 12,000+ paying cloud subscribers. Plan revenue from a hosted offer, and treat self-hosters as adoption and goodwill.
3. **E-invoicing must stay free.** In Germany it is a legal obligation (receive since 2025, issue from 2027/2028) and free tools already include it. Charging for compliance would lose to a free competitor. Use it as the reason to choose KlubHub (gig → invoice → compliant e-invoice, no retyping), and charge only for the *hosted* parts around it.
4. **"Licensed self-hosted" cannot be a lock under MIT.** Flags are an honour system. If it is to earn money, copy Immich: a voluntary licence that unlocks nothing.
5. **The best-supported paywalls are the ones that need your servers or your approvals:** multi-platform posting (TikTok's API keeps unaudited apps private-only), hosted public pages, receive-by-email, managed outbound email, AI with your keys, multi-user.
6. **Promoter makes money on ticket fees and seats, not on a $9 subscription.** Open-source ticketing competitors (pretix, Hi.Events) charge 1.25–2.5% per ticket; B2B booking tools charge far more than artist tools.
7. **Three plan details look wrong or risky** (§6.4): a permanent free *hosted* tier, a $25 TEAM price aimed at collectives, and treating the gated integrations (RA, SoundCloud, Spotify) as a revenue line.

---

## 2. What the precedent says

| Finding | Source | Confidence |
|---|---|---|
| The paywall line is the critical decision: too much paid starves adoption; too little lets everyone run it free. Typical paid items: SSO, roles, audit logs, hosting, support, multi-tenant. Rules of thumb: make the open version genuinely good; never move a free feature into paid (the reverse is fine); don't withhold security fixes. | [Open Core Ventures handbook](https://handbook.opencoreventures.com/how-we-work/open-core), [OneUptime](https://oneuptime.com/blog/post/2026-02-14-open-source-vs-open-core/view) | Medium (practitioner opinion, consistent across sources) |
| Hiding the most compelling features behind the paywall hurts conversion, because trial users never see what justifies the price. | [Open Core Ventures, pricing](https://handbook.opencoreventures.com/pricing/), [OneUptime](https://oneuptime.com/blog/post/2026-03-03-open-source-vs-open-core-whats-the-difference/view) | Medium |
| Generic freemium signup-to-paid is about 2.6% (≈3.5 payers per 1,000 visitors). Not specific to DJs or open source. | [Bruin](https://getbruin.com/use-cases/saas/open-source-to-paid-conversion/) | Low–medium (generic benchmark) |
| **Ghost:** self-hosting is free and pays nothing; the managed Ghost(Pro) service is "the commercial engine", no free tier, from $15/month; about 18K licensees. | [Everything PR](https://everything-pr.com/ghost-the-open-source-publishing-platform-that-chose-nonprofit-over-venture), [Ghost pricing summary](https://thatmarketingbuddy.com/pricing/ghost) | Medium (secondary sources) |
| **Plausible:** self-hosted Community Edition is AGPL and free; self-hosters donate about $300/month versus 12,000+ cloud subscribers funding the company. | [Plausible CE](https://plausible.io/blog/community-edition), [Seline](https://seline.com/blog/plausible-analytics-pricing) | Low–medium (one figure from a secondary page, possibly dated) |
| **Immich:** sells a licence with **no functional difference** ($99.99 per server, $24.99 per person, lifetime) and promises "there will never be any paywalled features". Reported: the first month of sales beat all prior donations combined, about 5×. | [Immich discussion #11186](https://github.com/immich-app/immich/discussions/11186), [Linuxiac](https://linuxiac.com/immich-team-goes-full-time/) | Medium (maintainer statement; no revenue figures published) |

**What this means for us:** self-hosting grows the community; money comes from a hosted offer and, secondarily, from people who pay to support a project they like.

---

## 3. What the market already charges (price anchors)

| Area | Anchor | Source | Confidence |
|---|---|---|---|
| Link-in-bio / pages | Linktree: free, $8 Starter, $15 Pro, $35 Premium per month (monthly billing) | [Unilink summary](https://app.unilink.us/blog/linktree-pricing-2026) | Medium |
| EPK pages | Bandzoogle EPK plan $6.95/month; Sonicbids EPK Pro $5/month | [Bandzoogle](https://bandzoogle.com/help/articles/363-bandzoogle-pricing), [Sonicbids](https://artistdata.sonicbids.com/pricing/) | Medium–high (vendor pages) |
| German freelancer invoicing/bookkeeping | Lexware Office from €7.90 (S), €12.90 (M), €21.90 (L); sevDesk free (3 invoices/month), invoicing from €11.90, bookkeeping from €25.90; Accountable and Papierkram have free tiers with unlimited e-invoices | [fastlancer comparison](https://www.fastlancer.org/fastlancer-blog/lexware-vs-sevdesk/), [thenetworkschool](https://thenetworkschool.de/blog/buchhaltungssoftware-freelancer-2026-vergleich) | Medium (comparison sites) |
| Artist booking management | Gigwell: no free plan; agency and venue products start around $100/month. **Sources disagreed on the artist-plan price ($49 vs $250), so no exact artist figure is used.** | [Software Advice](https://www.softwareadvice.com/event-booking/gigwell-profile/), [Gigwell pricing](https://www.gigwell.com/pricing) | Low on exact numbers; fine for "B2B costs more than B2C" |
| Ticketing (per paid ticket, excluding card fees where stated) | pretix 2.5%; Hi.Events 1.25% + $0.60; Eventbrite 3.7% + $1.79 + 2.9% processing; Luma 5% (or 0% on $59/month); Posh 10% + $0.99 | [Hi.Events comparison](https://hi.events/best/venue-ticketing-platforms), [SimpleTix](https://www.simpletix.com/eventbrite-fees-explained-2026/) | Medium (one source is an interested vendor) |

**Reading:** a solo DJ's willingness to pay for a tool sits around **€8–15/month**, and for pages around **$5–15**. The existing $9 PRO price is inside that band. It is not a premium price, so it cannot carry a lot of cost.

---

## 4. Constraints that decide individual features

| Constraint | Effect | Source |
|---|---|---|
| **E-invoice mandate (Germany):** every business must be able to *receive* e-invoices since 1 Jan 2025. *Issuing* is mandatory from 1 Jan 2027 for businesses over €800k turnover and from 1 Jan 2028 for all domestic B2B, small businesses included (small businesses are exempt from issuing until end of 2027). Formats: XRechnung or ZUGFeRD (EN 16931). | Compliance is a must-have, not a premium feature. Free tools already cover it, so paywalling it loses. | [BMF FAQ](https://www.bundesfinanzministerium.de/Content/DE/FAQ/e-rechnung.html), [IHK München](https://www.ihk-muenchen.de/ratgeber/steuern/steuerliche-sonderthemen/elektronische-rechnungen/), [kostenlose-erechnung.de](https://kostenlose-erechnung.de/ratgeber/e-rechnungspflicht-leitfaden/) |
| **TikTok posting:** unaudited API clients can only post privately (`SELF_ONLY`), for up to 5 users per 24 hours. Public posting needs an audit of the API client. | A self-hoster cannot realistically get audited. One audited, pooled app is a genuine reason to pay. | [TikTok developer docs](https://developers.tiktok.com/docs/en/content-posting-api-get-started), [Outstand](https://www.outstand.so/blog/tiktok-content-posting-api) |
| **Spotify (from our own plan):** development-mode apps are capped at 5 allowlisted users; unlimited users need 250,000+ monthly users. | Personal-data Spotify can only be bring-your-own-keys self-hosted; it cannot be sold as a hosted feature. | `.claude/plans/spotify-integration.plan.md` §1.2 |
| **SoundCloud (from our own plan):** registering an API app needs an Artist Pro subscription. | Same: bring-your-own credentials, hard to sell. | `.claude/plans/soundcloud-integration.plan.md` (risk table) |
| **Resident Advisor:** the import uses RA's undocumented public GraphQL; RA offers no public data API. **I could not retrieve RA's terms of use**, so whether automated access is allowed is unverified. | This is a legal-risk question first and a pricing question second. | search results only; **needs a legal read** |
| **Hosted public pages** need servers, TLS, CDN and domains. | Natural paywall; cannot be given away at scale. | `docs/v2-saas-migration-plan.md` M4 |

---

## 5. Licensing: what an MIT repo can and cannot protect

- **MIT means anyone may run a competing hosted KlubHub.** The repo's own risk register already says so. The mitigation in the plans is sound: keep *billing, tenancy, pooled OAuth and the public edge* in a separate private repo, so a competitor can only host the free core.
- **Flags enforce nothing.** `docs/EDITIONS.md` says it directly. Anyone can set `FEATURE_RA_IMPORT=true`. So "licensed" is a default-off safety switch plus a support boundary, not a lock.
- **Relicensing a successful MIT project later is costly.** Elastic (2021), HashiCorp (2023) and Redis (2024) moved to source-available licences; HashiCorp's was forked as OpenTofu, and Redis and Elastic both reversed course (Elastic added an open option in 2024, Redis in 2025). Lesson: decide before launch, not after traction. Sources: [SoftwareSeni timeline](https://www.softwareseni.com/the-open-source-license-change-pattern-mongodb-to-redis-timeline-2018-to-2026-and-what-comes-next/), [Flowverify](https://www.flowverify.co/blog/open-source-relicensing-2026-what-happened).
- **Options that exist:** keep MIT (maximum adoption, most exposure); AGPL (what Plausible, pretix and Hi.Events use; deters hosted clones, but it would end the "MIT, free forever" promise, and the Promoter plan relies on being MIT so it can reimplement AGPL ideas without copying code); or the [Functional Source License](https://fsl.software/) for *new* modules (free to self-host, no competing hosted service for 2 years, then MIT/Apache; not OSI open source in the meantime).
- **Not retroactive:** the existing MIT code stays MIT whatever is chosen for new modules.

---

## 6. Recommendation

### 6.1 Edition model

| Edition | What it is | Price logic |
|---|---|---|
| **Open source (MIT)** | Everything that is the user's own data and runs on their machine, plus compliance. | Free forever. |
| **Supporter licence** (optional) | A voluntary licence that unlocks **nothing**, as Immich does. A badge and priority issues at most. | One-time, roughly the Immich range ($25 personal, $100 per server) as a starting hypothesis. A small line, not the plan. |
| **KlubHub Cloud (solo)** | We run it: updates, backups, no Docker, phone-friendly, plus the pooled-infrastructure features. | About €9–12/month. 14-day trial. |
| **KlubHub Cloud (team)** | Collectives, agencies, small labels: seats, shared tours, shared templates, custom domain. | Higher than the current $25; see §6.4. |
| **Promoter Cloud** | Hosted public edge: event pages, RSVP, secret submission links, unsubscribe pages, short links. | Per-ticket or per-event fee plus a flat monthly for venues; validate before building. |

### 6.2 Feature placement

| Feature | Edition | Why |
|---|---|---|
| Tracklist generator, text export, cover-art chain | **OSS** | Flagship and funnel. |
| Instagram scheduling | **OSS** | Finishes the flagship workflow. (Self-hosters need their own Meta app; document it.) |
| Gig tracker, venues, contacts, iCal, booking PDF, rider templates | **OSS** | The user's data. |
| Finance: ledger, receipts, invoices, payments, agreements | **OSS** | Highly sensitive; DJs won't put money records somewhere that can be paywalled. |
| **E-invoice: Factur-X/XRechnung export, validation, archive, backfill, send by email** | **OSS** | Legal obligation; free competitors. |
| E-invoice *upload* ingestion (Promoter P5) | **OSS** | Same. |
| EPK builder and PDF | **OSS** | No hosting needed. |
| Release planner, basic tours, dashboard, analytics | **OSS** | Stickiness; the user's data. |
| Promoter: events, venues, lineup, guest lists, offline door app, ban list, export pack, built-in login | **OSS** | Admin tool on a private instance. |
| Multi-platform posting (TikTok, X, Facebook) | **Cloud** | Audited, pooled app; self-hosters can't replicate. |
| Hosted EPK / event page, custom domain | **Cloud** | Needs hosting. |
| AI captions | **Cloud** | Cost per use; our keys. |
| **Receive e-invoices by email** (a hosted address) | **Cloud** | Needs inbound mail. |
| **Managed outbound email** (no Plunk to set up) | **Cloud** | Rented infrastructure. |
| Multi-user, shared tours, shared libraries | **Cloud team** | Always-on collaboration. |
| Promoter public edge, Zitadel identity | **Promoter Cloud** | Already decided. |
| RA import, SoundCloud, Spotify personal data | **Default-off integrations** | Third-party terms and bring-your-own credentials; see §6.3. |
| DATEV export | **Decision needed (§8)** | Accounting tools charge for it (sevDesk lists it on paid tiers); it is also export of the user's own data. |
| Peppol | **Defer** | Tier 3; Germany doesn't need it. |

### 6.3 The gated integrations

RA, SoundCloud and Spotify each need credentials or risk the user must carry (Meta-style app registration, an Artist Pro subscription, a 5-user cap, unverified RA terms). They are a **safety and support boundary**, not a product to sell. Recommend: keep them default-off and clearly labelled, include them in the supporter licence as "supported" rather than "unlocked", and do not put revenue against them.

### 6.4 Changes to the March plan

1. **Drop the permanent free hosted tier** (500 MB, "everything in self-hosted free"). It gives away the exact thing that costs money (running the app) and competes with the free self-hosted option. Ghost has no free tier; Plausible offers a trial. Use a 14-day trial, and keep a small free hosted allowance only for the pooled features if you want a taste of them (the "4 posts a month" idea is fine).
2. **Rethink the $25 TEAM price for collectives and agencies.** B2B buyers pay more (Gigwell's agency and venue products start around $100 a month). $25 for 10 people looks like a gift. Test a higher price, or per-seat.
3. **Do not size the business on self-hosters.** Plausible and Ghost both show self-hosting contributes little revenue.
4. **Add the missing editions:** Promoter Cloud and the e-invoice cloud pieces, which the March document does not cover.

### 6.5 Reality check on revenue

Illustrative arithmetic only (my assumptions, not data): at €9/month, 100 payers is about €10.8k a year, 500 is about €54k, and 2,000 is about €216k. Using the generic 2.6% signup-to-paid benchmark, **500 payers needs roughly 19,000 signups**. Sources on the number of DJs range from 450,000 to 3 million depending on definition, so the market is large, but reaching and converting it is the hard part, not the size.

---

## 7. How much to trust this

- **Strong:** the e-invoice dates (BMF and two corroborating sources); TikTok's audit restriction (platform docs); the pricing of Linktree, Bandzoogle, Sonicbids and the German invoicing tools (vendor and comparison pages); the open-core "lessons" (consistent across several sources).
- **Medium:** Ghost and Immich business facts (secondary sources and maintainer statements, no audited numbers).
- **Weak, treat as hypotheses:** Plausible's $300/month donation figure (one secondary page); all Gigwell prices (sources conflict); the "DJs spend $150–600/month on 4–6 tools" claim (it comes from a vendor blog); the number of DJs (wide range); the 2.6% conversion (generic SaaS).
- **Not verified at all:** Resident Advisor's terms of use; whether Meta app review is workable for a hosted multi-tenant Instagram scheduler; any DJ's actual willingness to pay for *this* product. No customer was asked.

---

## 8. Decisions for the owner

1. **Licence for Promoter and any new hosted-only module,** before publishing: MIT, AGPL or FSL (§5)? The existing DJ core stays MIT either way.
2. **Supporter licence:** yes or no, and at what price? (An Immich-style, unlock-nothing licence.)
3. **DATEV export:** open source (your data) or a Cloud feature (market precedent)?
4. **Is there a free hosted tier at all,** or only a trial?
5. **Team pricing:** keep $25, raise it, or go per-seat?
6. **Resident Advisor:** get a legal read on their terms before offering the import to anyone beyond yourself.

## 9. Cheapest ways to reduce the uncertainty

- Interview about 8–10 working DJs and 3–5 promoters about what they pay for today and what they'd pay for "we run it for you".
- Put up a pricing page with a waitlist and count sign-ups before building the cloud tier.
- Check TikTok's audit requirements and Meta's app review for a hosted multi-tenant app, since both gate the Cloud tier's headline feature.
- Read RA's terms.
