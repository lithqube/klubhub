# Screenshots

This folder holds UI screenshots referenced from the root [README](../../README.md)
and any docs that need visuals.

## Conventions

- **Format:** PNG for crisp UI captures, JPG only for photo-realistic content.
  Aim for **≤ 300 KB** per image — prefer tightly cropped views over full
  viewport screenshots.
- **Resolution:** capture at 2× pixel density if your tool supports it
  (Retina / HiDPI); the README will render them at a comfortable size.
- **Theme:** when a screenshot is theme-dependent, capture it in **dark** mode
  by default (KlubHub DJ's primary theme). Add a `-light` suffix for a light-mode
  variant, e.g. `dashboard-light.png`.
- **Naming:** `<module>-<view>[-<variant>].png`
  - `dashboard-overview.png`
  - `tracklist-upload.png`
  - `social-calendar-dark.png`
  - `epk-pdf-preview.png`

## Adding a screenshot

1. Drop the image into this folder using the naming above.
2. Reference it from the README with the relative path
   (`docs/screenshots/<file>.png`) so the link works on GitHub.
3. Keep alt text descriptive of the *user value*, not just the UI:
   - ✅ `![Tracklist generator showing parsed track list and cover art](docs/screenshots/tracklist-upload.png)`
   - ❌ `![screenshot](docs/screenshots/img1.png)`

## Why a separate folder?

Keeping screenshots in one place makes it easy to:

- audit coverage of the docs (each module has at least one screenshot)
- regenerate visuals during a design-system refresh without hunting across
  multiple `*.md` files
- keep the repo root clean — `docs/screenshots/` is the single convention
  referenced from `README.md`, `docs/INDEX.md`, and future marketing pages.
