# Phase 1 Plan 4: User Setup

## External Services

### playwright-chromium

**Why:** Playwright Chromium binary must be installed locally for dev screenshot testing

**Setup:**

```bash
cd apps/dj && npx playwright install chromium
```

**Verification:**

```bash
npx playwright install --dry-run chromium
# Should show chromium is installed
```

---

_Phase: 01-tracklist-image-generator_
_Status: Incomplete_
