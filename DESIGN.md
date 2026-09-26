# Design System: KlubHub DJ

Character: tactical instrument panel for an independent DJ's career — mission control, not a SaaS dashboard. Dense telemetry, one radioactive signal color against obsidian, zero-radius angular geometry, dashed schematic lines, corner-bracket targeting reticles.

*(Extracted from the shared layer `libs/ui/app/assets/css/kinetic.css` (product surfaces stay in each app's `styles.css`) — "The Kinetic HUD" / dark default, "Daytime HUD" / light variant. Not invented; this file documents decisions already encoded in the CSS and flags where implementation drifts from them.)*

## Color

- `--color-surface` `#0E0E0F` (dark) / `#F0F0F2` (light): obsidian base / cool near-white. The two themes are a deliberate ink-swap — dark's surface literally becomes light's `--color-on-surface` ink, and vice versa.
- `--color-primary` `#96F8FF` radioactive cyan (dark) / `#006875` deep teal, WCAG AA 5.8:1 (light): the ONE signature. Primary CTA, active nav/tab, focus rings, "ready/scheduled" status. Nowhere else.
- `--color-secondary` `#C8B8FF` violet (dark) / `#5B4BBD` (light): second-tier signal only — "draft/pending" status, EPK accents. Never competes with primary for attention: a selected/active state is primary, never violet, and a neutral metric (an average, a total) is never violet.
- `--color-tertiary` / `--color-on-surface-variant`: muted meta/label text. Dark `--color-tertiary` is `#8897A9` (was `#7A8899`, 4.34:1 on the 8px status-bar labels): about 4.8:1 on `surface-container-highest`, so it clears AA on every surface step.
- `--color-error` `#FF716C` / `#C0302C`: "failed" status only — never decorative. `--color-on-error` (`#0E0E0F` / `#FFFFFF`) is the ink on an error fill: white on the dark-theme red is 2.7:1, the dark ink 7.2:1.
- `--color-status-archived` `#F5C518` / `#A64B08`: amber, "archived" only. Daytime was `#B45309`, 4.05:1 at 8px.
- Neutrals are a cool obsidian ramp (`--color-surface-container-*`, 8 steps), never Tailwind's default gray scale.
- Rule: strip every accent and the UI still reads — hierarchy comes from the type/space system below, not from color.
- Tints and hairlines of a signal colour are `color-mix(in srgb, var(--color-<token>) N%, transparent)`, never an `rgba()` copy of the token: a copy is frozen to the dark theme and reads as nothing (or as mud) in Daytime. Glows and modal scrims are the exception: they are meant to vanish in Daytime, which swaps neon halos for ambient depth (`--shadow-glow-*` already differ per theme, so use those tokens).
- Never dim text with opacity alone: 70% of the Daytime teal or violet is under AA at 8px. Use `.quiet` (70% Kinetic, 95% Daytime; links lift to full on hover) or a lighter token. Already-muted `--color-tertiary` text takes no further dimming.
- Contrast is measured, not assumed: every text node clears 4.5:1 against its composited background in both themes. Audited 2026-09-24 across all six pages and 11 interactive states (RA panels loading/empty/error/selected/result, gig dialog, calendar): 0 failures in 318 text nodes, from 35 (Daytime) and 28 (Kinetic).

## Typography

Three families, three fixed roles — never interchangeable:

- `--font-command` **Space Grotesk**: page titles, big numbers (gig fee, tx amount, calendar day). Uppercase, `tracking-command: -0.02em`. The "display" register.
- `--font-data` **Inter**: body copy, captions, table values, form input text. Normal case, reading register.
- `--font-terminal` **Manrope**: labels, meta, badges, tabs, placeholders. ALWAYS uppercase, tiny (7–9px), `tracking-terminal: 0.05–0.08em`. The "HUD readout" register — this is the system's most distinctive voice and the easiest to drop by accident.
- Extreme range is the point: 7px meta text up to `clamp(28px,5vw,48px)` EPK hero name. A component using an in-between default (13–15px, normal case, no tracking) for a label is off-system.
- The three families only exist if the files load. Until 2026-09-24 `public/fonts/*.woff2` were saved HTML pages (committed that way in `706fea9`), so none of them ever loaded and the whole app rendered in system sans; no CSS or static check could tell. They are now the OFL Latin subsets from `apps/site/public/fonts` (licence texts alongside). If the type ever looks generic again, check `document.fonts` (each face should say `loaded`) before touching any CSS.
- Known drift (2026-09-24, left as is, needs a decision): 13 terminal-register labels run at 10px, not 7–9px (social tabs and badges, input placeholders), and about 90 terminal-register controls use Tailwind `text-xs` (12px), mostly buttons. Either the doc gains a 10–12px "control" step or they shrink. The EPK bio and press-quote editors use `font-mono` on purpose (pinned by `EpkBioSection.test.ts`), which makes the mono a fourth family the system doesn't own.

## Spacing

- Rhythm is dense and compressed, not airy: 2–4px inside tight clusters (theme segmented control, calendar grid gaps), 10–16px inside a component, 20px page gutters, 14px between stacked page-body sections.
- Tell of drift: uniform 16px gap everywhere — the system never does this on purpose.

## Shape & Elevation

- **Radius: `0px`, enforced globally** (`--radius-*` all zero). No rounded corners anywhere. This is a hard rule, not a default — a `rounded-md`/`rounded-lg` anywhere in product UI is off-system. Status dots are included: `.pulse-dot` and the calendar and timeline event dots are 6px squares. The one round thing left is a CSS-ring spinner in `SocialConnectionBanner` (a rotating ring needs the curve).
- **Separation language: dashed 1px borders**, not shadows-as-elevation. `ghost-border`, badges, `nav-item-active/inactive`, `upload-zone`, `social-chip`, `photo-slot` all use dashed borders as the signature "schematic line" motif.
- Glow (`shadow-glow-*`) is reserved for primary CTA / active-state emphasis, not general card elevation.
- Corner brackets (`.hud-card`, `.bracket-box` ::before/::after) are a targeting-reticle accent on key panels — a specific motif, not applied everywhere. `.hud-card` brackets are token-derived (since 2026-09-21): `--hud-bracket-color` picks the signal token (`--color-primary`; `.hud-card-v` → `--color-secondary`) and `--hud-bracket-alpha` the strength (40% dark, 30% light), mixed with `color-mix()`. Add a variant by setting `--hud-bracket-color` — never by pasting an `rgba()` copy of a token.
- Glass: `backdrop-filter: blur(20–24px)` panels at 60–70% opacity (`.glass`, `.page-header`, `.tabs-bar`) for chrome that floats over the `.hud-bg` dot-grid + scanline background.

## Motion

- Standard transition: `.15s`–`.2s`, default easing.
- Live-state signals use keyframe motion: `pulse-dot` (2s) for "live" indicators, `scan` (2s linear) sweeping progress fills.

## Components

- **Button** (`btn-hud`): `cta` (cyan gradient + glow, primary action), `ghost` (dashed border, transparent), `violet` (secondary action). Sizes `sm`/`xs` step height down, never radius.
- **Badge** (`badge-hud` + status modifier): dashed-border + colored text for `ready`/`draft`/`archived` (low-emphasis signal); filled background for `scheduled`/`failed` (needs-attention); muted fill for `published` (done, no longer urgent).
- **Input/Textarea** (`hud-input`/`hud-textarea`): no full border — flat fill, 2px left accent bar that lights up cyan on focus. Placeholder renders in the terminal register (uppercase, tracked, tiny).
- **Card/Panel**: `.glass` or `.hud-card` (corner brackets) over `.hud-bg`; status lists use a left `accent-bar-*` (3px solid, status color) rather than a colored background fill.
- **Table row / list row**: left accent bar for status, dashed row divider (`rgba(46,46,49,.15–.2)`), no zebra striping.
- **Toggle** (`hud-toggle`): square, not pill — consistent with the zero-radius rule.
- **Button, destructive** (`btn-hud-error`, extension added 2026-09-16): solid `--color-error` fill + `shadow-glow-error`, mirroring `btn-hud-cta`/`btn-hud-violet`'s treatment. Added because destructive confirmations (e.g. cancelling a gig) previously faked this with an inline `background` override on bare `.btn-hud`.
- **Bottom nav bar** (`bottom-nav-bar`, extension added 2026-09-16): same glass/blur/hairline recipe as `.page-header`, flipped to `border-top`. Added because the mobile bottom nav previously hardcoded dark-only rgba values inline, which can't be reached by a `[data-theme="light"]` override — it broke in Daytime HUD.

- **EPK hero** (`epk-preview-hero` + `epk-eyebrow` / `epk-dj-name` / `epk-meta`, revised 2026-09-21): flat `--color-surface-container-low` panel, 3px left bar in `--color-secondary` (the EPK accent), dashed `--color-outline-variant` rule underneath. Eyebrow and label/value readouts in the terminal register; the name in the command register at `--tracking-command`, colored `--color-on-surface`. No gradient, glow, or brackets (the BIO card below already carries the reticle). Revised because the previous hero was a hardcoded dark gradient with `#fff` text, a blurred `border-radius:50%` blob and a `text-shadow` glow — the same dark-only-rgba failure as the old bottom nav (it turned muddy grey in Daytime HUD), plus a radius and an off-role glow.

- **Sidenav** (`TheNav`, rail added 2026-09-24): 220px, or a 56px icon rail. The toggle is the last row of the sidebar footer (`side-nav-action`, terminal register); the state lives in a cookie (`klubhub-nav-collapsed`, read on the server, so the first paint is already the right width; localStorage would snap open-then-shut). In the rail every label stays in the DOM as `sr-only` text and gets a `title` tooltip; the theme control becomes an icon column; the header keeps its corner brackets around a "KH" monogram and the status dot. The sidebar is `sticky` (a long page must not scroll the navigation away) and pads its bottom by `--status-bar-h`, the same variable the status bar and the main column use, so nothing sits under the bar (the logout row used to). LOGOUT is hidden behind `showLogout` until the app has authentication.
- **Status bar** (`TheStatusBar`, revised 2026-09-24): quick-preview readouts instead of placeholder metrics, label over value in the terminal/command registers, each a link to where its number lives. NEXT GIG (date, city, countdown; lit primary when today or tomorrow), PAYMENTS (`N OVERDUE` in error red, else the unpaid amount, else `NOTHING DUE`), SOCIAL (`N FAILED` in red, else scheduled count), and at `xl` BOOKED and EARNED MTD. Real data only, from the gig list and the post queue via `utils/quickPreview.ts` (pure, tested). There is no invoice or due-date model yet, so money follows each gig's `payment_status`; a NEXT DUE readout belongs here once invoices exist. The bar is `surface-container`, not `-high`: red text on Daytime's `-high` grey was 4.15:1.
- **Empty state** (dashboard hero, revised 2026-09-24): dashed primary-20% hairline with the `hud-card` brackets, a quiet tertiary icon well, command-register title, data-register hint, and exactly one `btn-hud-cta` with a `Plus` icon so the button is the thing to find. No violet accent bar: violet means draft. Never override a `.btn-hud`'s `display` inline: `inline-block` pinned the label to the top edge.
- **Modal** (a `.glass` panel over a `rgba(0,0,0,.6–.8)` scrim, revised 2026-09-24): the panel sets an opaque `background: var(--color-surface-container)`. The translucent glass fill composites with the scrim, so in Daytime the panel went mid-grey and every form label fell to 3.0:1. Applies to the gig form, its cancel confirmation, and the tracklist delete confirmation.
- **Integration panels** (RA import on `/epk` and `/gigs`, revised 2026-09-24): primary cyan throughout — `btn-hud-cta` for FETCH / APPLY / IMPORT, `--color-primary` for labels and hints, `color-mix(in srgb, var(--color-primary) N%, transparent)` for tints, dividers and glows, `social-chip social-chip-cyan` for handle chips. Violet stays reserved for draft/pending status and the EPK preview itself (plain `social-chip`, `hud-card-v`), where it is the EPK accent. Revised because these panels were the last violet-CTA outliers on pages where every other primary action is cyan; the old inline `rgba(200,184,255,…)` tints were dark-only and could not follow Daytime HUD.

## Voice

- Labels and meta are terse, uppercase, HUD-readout style ("READY", "DRAFT", section labels) — never sentence-case chrome text in the terminal register.
