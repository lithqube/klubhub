# Design System: KlubHub DJ

Character: tactical instrument panel for an independent DJ's career — mission control, not a SaaS dashboard. Dense telemetry, one radioactive signal color against obsidian, zero-radius angular geometry, dashed schematic lines, corner-bracket targeting reticles.

*(Extracted from `apps/dj/app/assets/css/styles.css` — "The Kinetic HUD" / dark default, "Daytime HUD" / light variant. Not invented; this file documents decisions already encoded in the CSS and flags where implementation drifts from them.)*

## Color

- `--color-surface` `#0E0E0F` (dark) / `#F0F0F2` (light): obsidian base / cool near-white. The two themes are a deliberate ink-swap — dark's surface literally becomes light's `--color-on-surface` ink, and vice versa.
- `--color-primary` `#96F8FF` radioactive cyan (dark) / `#006875` deep teal, WCAG AA 5.8:1 (light): the ONE signature. Primary CTA, active nav/tab, focus rings, "ready/scheduled" status. Nowhere else.
- `--color-secondary` `#C8B8FF` violet (dark) / `#5B4BBD` (light): second-tier signal only — "draft/pending" status, EPK accents. Never competes with primary for attention.
- `--color-tertiary` / `--color-on-surface-variant`: muted meta/label text.
- `--color-error` `#FF716C` / `#C0302C`: "failed" status only — never decorative.
- `--color-status-archived` `#F5C518` / `#B45309`: amber, "archived" only.
- Neutrals are a cool obsidian ramp (`--color-surface-container-*`, 8 steps), never Tailwind's default gray scale.
- Rule: strip every accent and the UI still reads — hierarchy comes from the type/space system below, not from color.

## Typography

Three families, three fixed roles — never interchangeable:

- `--font-command` **Space Grotesk**: page titles, big numbers (gig fee, tx amount, calendar day). Uppercase, `tracking-command: -0.02em`. The "display" register.
- `--font-data` **Inter**: body copy, captions, table values, form input text. Normal case, reading register.
- `--font-terminal` **Manrope**: labels, meta, badges, tabs, placeholders. ALWAYS uppercase, tiny (7–9px), `tracking-terminal: 0.05–0.08em`. The "HUD readout" register — this is the system's most distinctive voice and the easiest to drop by accident.
- Extreme range is the point: 7px meta text up to `clamp(28px,5vw,48px)` EPK hero name. A component using an in-between default (13–15px, normal case, no tracking) for a label is off-system.

## Spacing

- Rhythm is dense and compressed, not airy: 2–4px inside tight clusters (theme segmented control, calendar grid gaps), 10–16px inside a component, 20px page gutters, 14px between stacked page-body sections.
- Tell of drift: uniform 16px gap everywhere — the system never does this on purpose.

## Shape & Elevation

- **Radius: `0px`, enforced globally** (`--radius-*` all zero). No rounded corners anywhere. This is a hard rule, not a default — a `rounded-md`/`rounded-lg` anywhere in product UI is off-system.
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

## Voice

- Labels and meta are terse, uppercase, HUD-readout style ("READY", "DRAFT", section labels) — never sentence-case chrome text in the terminal register.
