# KlubHub email design system

The KlubHub emails and the static site (`apps/site/`) share the same design tokens. The token source of truth is `apps/dj/app/assets/css/styles.css` (the application's `@theme` block). Email templates here use the same color values, font stacks, and visual idioms — the email reads like an extension of the site, not a separate brand.

This document is the operating manual for adding a new email to `apps/site/emails/`. It is intentionally short.

## What's in scope

- Static HTML email templates (`onboarding.html`, `welcome.html`) using inline styles only.
- The Plunk dashboard as the sending surface. Templates ship as static HTML; an operator pastes the rendered HTML into Plunk's template editor, or uploads it as a custom template.
- A read-only `design-tokens.css` excerpt (this file) for reference.
- Documentation of the Plunk **workflow** contracts that consume these templates (lifecycle automations like `klubhub.subscribed` → welcome email).

## Templates in this directory

| Template | Type | Trigger | Audience |
|---|---|---|---|
| `onboarding.html` | Marketing (broadcast) | Manual campaign send | Subscribed contacts, one-time announcement |
| `welcome.html` | Marketing (lifecycle) | Workflow trigger on `klubhub.subscribed` | New subscribers, automatic |

### Welcome template — lifecycle contract

The `welcome.html` template is delivered through a Plunk workflow, not a manual campaign. The contract:

- **Trigger:** Custom event `klubhub.subscribed`, fired by `apps/site/public/newsletter.js` via `POST /v1/track` with `{ email, event: "klubhub.subscribed" }`.

  **Why custom, not system:** Plunk's documented `contact.subscribed` system event is listed as a workflow trigger option, but in practice it does **not** fire on contact upserts created via `/v1/track`. The Plunk waitlist recipe uses a namespaced custom event for exactly this reason. Custom events fire reliably; system events on `/v1/track` upserts do not.

- **Workflow name (suggested):** `klubhub · welcome onboard`.
- **Single step:** `SEND_EMAIL` with `templateId` pointing at the welcome template. No delay, no follow-up, no exit — the workflow completes after one email.
- **`enabled: true`.** Workflows ship disabled; until the toggle is on, no executions fire. This is the most common reason "my workflow isn't firing."
- **`allowReentry: false`.** Each contact enters once. Re-entry is for re-engagement sequences, not first-touch welcome.

The welcome template includes `{{ manageUrl }}` (Plunk's preferences page) but **not** `{{ unsubscribeUrl }}`. Welcome is a 1:1 lifecycle message, not a marketing broadcast — Plunk auto-injects an unsubscribe footer for Marketing templates, but the explicit `{{ manageUrl }}` is the more polite pointer for a transactional-style email.

### How to create the workflow in Plunk

1. Sign in to https://next-app.useplunk.com with the `sk_*` for the relevant project (`klubhub-prod`).
2. **Workflows → New workflow.**
3. **Trigger:** `EVENT` on `klubhub.subscribed`. **Critical:** Plunk does not let you rename a trigger event after the first execution fires — pick the right name up front. If you ever need to rename, create a new workflow and disable the old one.
4. **Add step:** `SEND_EMAIL` pointing at the `KlubHub DJ · Welcome` template.
5. **Save** with `enabled: false` initially.
6. **Test:** in Plunk, run a manual test execution against your own contact (you can create one under **Contacts** with `subscribed: true`). Verify the email arrives with the merge tags resolved.
7. **Flip `enabled: true`.** Welcome emails start firing for every new signup.

### Reusing the workflow for staging / preview

Each Plunk project (`klubhub-prod`, `klubhub-staging`, `klubhub-preview`) needs its own copy of this workflow, all pointing at the same template. The static site fires `event: 'klubhub.subscribed'` to whichever project the `pk_*` is configured for — so a signup against the preview project will trigger the preview workflow automatically.

## What's out of scope

- Any build step. There is no bundler, no template engine. The email is plain HTML + inline styles. This is the only reliable cross-client strategy for emails in 2025 — every ESP rewrites or strips CSS classes, but inline styles survive.
- Any `sk_*` Plunk key. This directory is in version control; secrets are in your password manager. See `SECURITY.md` for the project's Plunk-keys handling rules.
- Any JavaScript. Email clients do not run scripts. No tracking pixels either, until/unless the operator enables Plunk's marketing-only tracking mode.

## Design tokens (copy these into inline styles)

The email templates use **literal hex values**, not CSS variables, because email clients strip `<style>` blocks. The values below mirror `apps/dj/app/assets/css/styles.css` exactly. If the application design system changes, update the tokens here too.

### Colors

| Token | Hex | Use |
|---|---|---|
| surface | `#0E0E0F` | email body background |
| surface-container-lowest | `#121214` | container card background |
| surface-variant | `#1A1A1C` | inner panel background (e.g. two-path box) |
| on-surface | `#E0E0E2` | body text |
| on-surface-variant | `#C4C4C7` | secondary text |
| tertiary | `#7A8899` | muted labels, footers |
| outline | `#3A3A3D` | dashed borders |
| primary | `#96F8FF` | brand accent, CTA gradient start |
| primary-container | `#00F1FD` | CTA gradient end |
| secondary | `#C8B8FF` | secondary accent (path 02, EPK, etc.) |

### Gradients

```css
background: linear-gradient(135deg, #96F8FF, #00F1FD);  /* primary CTA */
```

### Typography

| Role | Stack | Size |
|---|---|---|
| Headlines (`h1`, `h2`, `.brand`) | `'Space Grotesk', sans-serif` | 36px / 22px / 20px |
| Body (`p`) | `'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif` | 16px (body), 15px (panels) |
| Labels (eyebrow, footer, small caps) | `'Manrope', sans-serif` | 11px / 10px / 9px |

Tracking: `-0.02em` on headlines (Space Grotesk), `0.08em` on uppercase labels (Manrope).

### Visual idioms

- **Dashed borders.** Every container uses `border:1px dashed #3A3A3D`. Solid borders feel heavy; dashed matches the Kinetic HUD aesthetic.
- **Corner brackets are skipped in email.** The site uses pseudo-element corner brackets; email clients strip pseudo-elements. The dashed border is the substitute.
- **Background dot grid.** Implemented as a CSS gradient on the outer `<table>`:
  ```css
  background-color: #0E0E0F;
  background-image:
    radial-gradient(circle, rgba(150,248,255,0.06) 1px, transparent 1px),
    radial-gradient(circle, rgba(200,184,255,0.03) 1px, transparent 1px);
  background-size: 32px 32px, 64px 64px;
  background-position: 0 0, 16px 16px;
  ```
  Not all email clients honor multi-layer gradients in `background-image` (Outlook on Windows falls back to solid). That's fine; the fallback is the obsidian surface.

## Anatomy of a KlubHub email

Every email follows this structure (matches `onboarding.html`):

```
<table width="100%" background-color:#0E0E0F; dot-grid>
  <tr><td align=center>
    <table width=600 max-width:600 background:#121214 border:1px dashed #3A3A3D>
      <!-- 1. Brand bar (KlubHub mark + section eyebrow) -->
      <tr><td padding:24px 32px border-bottom:1px dashed>
        K KLUBHUB          01 / SHIPPING NOTICE
      </td></tr>

      <!-- 2. Hero (eyebrow + headline + intro) -->
      <tr><td padding:48px 32px 32px>
        // KLUBHUB DJ · V1.0.0
        YOUR SCENE. YOUR TOOLS.
        Body paragraph.
      </td></tr>

      <!-- 3. Action panels (one or more .surface-variant cards) -->
      <tr><td padding:0 32px 32px>
        <table background:#1A1A1C border:1px dashed>
          [Path 01 panel] [hr] [Path 02 panel]
        </table>
      </td></tr>

      <!-- 4. Optional feature strip (3-column) -->
      <tr><td padding:0 32px 32px>
        01 Feature · 02 Feature · 03 Feature
      </td></tr>

      <!-- 5. Footer -->
      <tr><td padding:24px 32px border-top:1px dashed background:#121214>
        KLUBHUB // INDEPENDENT TOOLS FOR INDEPENDENT MUSIC
        Signup context + unsubscribe links
        klubhub.io · GitHub · Unsubscribe
      </td></tr>
    </table>
    <p>MIT license · No telemetry · No vendor lock-in</p>
  </td></tr>
</table>
```

Maximum width is **600px**. Padding rhythm is `32px` outer, `24px-28px` inside cards, `8px-14px` between sub-blocks. Vertical rhythm between sections is `32px`.

## How to add a new email

1. Copy `onboarding.html` to `apps/site/emails/<name>.html`.
2. Replace the body content. Keep the brand bar, hero structure, and footer.
3. Use literal hex values for every color (no `var(--color-*)` — those don't survive email clients).
4. Use table-based layout for everything. No flexbox, no grid, no `<div>` for layout. Email clients are stuck in 2007.
5. Use `role="presentation"` on every `<table>`. Accessibility readers should ignore the structural tables.
6. Keep all text and images inside table cells. `<img>` tags must use `width` and `height` attributes (not CSS) and include `alt` text.
7. Test in [Litmus](https://litmus.com/) or [Email on Acid](https://www.emailonacid.com/) if you have access. At minimum, test in:
   - Gmail (web + iOS + Android)
   - Apple Mail (macOS + iOS)
   - Outlook (Windows desktop — the worst email client)
   - Yahoo Mail
8. Add a section to this README documenting what the email is for, who it goes to, and which Plunk campaign/event triggers it.

## How to send an email via Plunk

This directory does not contain a sender. The flow is:

1. Render the template locally (or copy the static HTML).
2. In Plunk: **Campaigns** → **Create campaign**.
3. Paste the rendered HTML into the campaign composer (or upload as a custom template).
4. Set the audience:
   - Production: the `klubhub-prod` project, contacts where `subscribed === true` and `event:klubhub.subscribed` was fired recently.
   - Staging / preview: the corresponding Plunk projects (see `SECURITY.md`).
5. Set tracking:
   - For a one-time announcement: tracking **Disabled** (the safest default — no opens, no clicks, no PII to anyone).
   - For a product-update series: tracking **Enabled** or **Marketing only** (depends on your preference).
6. Schedule or send.
7. Archive this file in `apps/site/emails/sent/<date>-<name>.html` so the next person can see what was actually delivered.

The `{{ unsubscribeUrl }}` and `{{ manageUrl }}` placeholders are Plunk's standard merge tags — they resolve to per-recipient signed URLs (one-click unsubscribe page and preferences management page, respectively). Plunk uses Liquid syntax; spaces inside the braces are tolerated and ignored.

If the email needs personalization beyond these (e.g. `{{ firstName }}`), add the merge tag and verify the fallback (contact without the field) renders cleanly.

## When to escalate

If a new email needs:

- **Tracked open/click pixels**: enable Plunk tracking **Marketing only** or **Enabled** per Plunk's docs. The base template ships with tracking off.
- **A different audience (segment, exclude, etc.)**: Plunk segments can be referenced from the campaign composer. Document the segment definition in this file when you add the campaign.
- **Personalization beyond `{{ manageUrl }}` / `{{ unsubscribeUrl }}`**: Plunk supports `{{ firstName }}`, etc. The base templates do not include them. Add them as needed and test that the fallback (no merge value) renders cleanly.
- **A second visual variant (e.g. text-only fallback)**: Plunk can auto-generate a text version from the HTML. Verify the auto-generated text reads well; if not, write a hand-crafted text version.

## Reference

- Plunk API for campaigns: https://docs.useplunk.com/api-reference/campaigns
- Plunk tracking modes: https://docs.useplunk.com/guides/tracking
- Kinetic-HUD design system source of truth: `apps/dj/app/assets/css/styles.css`
