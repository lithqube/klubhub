# KlubHub Promoter — P1 UX design (IA, dashboard, events, timetable, export, venues, door entry)

Produced with the ux-engine `ux-designer` agent (2026-09-26) before implementation. Source of truth for P1 frontend work; open questions at the end are resolved in the "Decisions" section as they are answered.

## 1. Intent brief
- **User:** independent promoters, collectives (2–10), club-night crews, open-air organisers; roles owner/admin/booker/finance/marketing/door; many are DJs already on KlubHub DJ.
- **Primary action:** create an event and get lineup + timetable right in under 10 minutes.
- **Secondary:** publish through the export pack (RA, Facebook, DICE, own site); manage venues; run the door (P2).
- **Context:** planning on a laptop in the evening; timetable tweaks backstage on a phone (dark, one hand, bad signal); door on a phone (very dark, stress, offline).
- **Worst case:** a timetable conflict reaches the public; a secret location or venue contact leaks into an export; the export is posted before the embargo.

## 2. Rationale
- **Dashboard = attention queue**, not metrics: one ranked list, one verb per row, "ALL CLEAR" when nothing is due.
- **Duplicate last event + paste lineup** are the shortest path to the 10-minute goal.
- **Local timetable draft, server decides:** edits persist on the device, save as a batch (`PUT /lineup`); client checks give instant feedback, only the server accepts. Nothing lost backstage, nothing invalid saved.
- **Conflicts impossible to miss:** failed accent bar + hatch + icon + text + conflict panel with jump links; curfew is a labelled dashed line; never colour alone.
- **Export leads with what is withheld** (secret location, embargo), before any copy button.
- **Protected is a visible state:** encrypted venue fields in one locked, masked panel; tech notes warn when they look like contacts.
- **Unbuilt sections honest but quiet:** grouped with a phase tag on desktop; mobile bottom nav holds only working destinations.
- **Nested routes, not ARIA tabs:** `/events/:id/lineup` is deep-linkable.

## 3. Information architecture
```
/                      DASHBOARD
/events                EVENTS (?view=upcoming|drafts|past)
/events/new
/events/:id            → /events/:id/details | /lineup | /export   (P2 adds /guests, /door; P3 /promo)
/venues, /venues/new, /venues/:id
/guests P2   /door P2 (manager: door setup + launch)   /door/run (bare, forced dark)
/audience P3  /campaigns P3  /bookings P4
/settings → COLLECTIVE (P1.5) · MEMBERS · DOOR DEVICES · SECURITY (/account/security)
```
Desktop sidebar groups: PLAN (Dashboard, Events, Venues) · RUN (Guests, Door) · GROW (Audience, Campaigns) · DEAL (Bookings); Settings last. The door role never gets the shell: straight to `/door/run` for its event. Mobile bottom nav (P1): DASH · EVENTS · NOW · VENUES · MORE (NOW = timetable of the live or next event within 7 days; MORE = Sheet of everything else). In P2 DOOR replaces VENUES.

## 4. Wireframes

### 4.1 Shell + dashboard (desktop)
```
┌───────────────────┬──────────────────────────────────────────────────────────────────────┐
│ KLUBHUB PROMOTER  │ DASHBOARD                                              [+ NEW EVENT] │
│ ■ NACHTSCHICHT    │ NACHTSCHICHT COLLECTIVE · EUROPE/BERLIN                              │
├───────────────────┼──────────────────────────────────────────────────────────────────────┤
│ PLAN              │ ┌ NEXT EVENT ─────────────────────────────────┐ ┌ UP NEXT ──────────┐│
│ ▣ DASHBOARD    4  │ │ T-7D 03H                        [PUBLISHED] │ │ SAT 03 KLUBNACHT 03││
│ ▢ EVENTS          │ │ KLUBNACHT 03                                │ │ SAT 10 WAREHOUSE   ││
│ ▢ VENUES          │ │ SAT 03 OCT · 23:00 → 07:00 +1 · CEST        │ │ SAT 24 OPEN AIR    ││
│ RUN               │ │ TRESOR.WEST · BERLIN · 2 STAGES · 9 ACTS    │ │ [ALL EVENTS →]     ││
│ ▢ GUESTS      P2  │ │ PREFLIGHT ▰▰▰▰▰▱ 5/6 · 1 ACT UNTIMED        │ └────────────────────┘│
│ ▢ DOOR        P2  │ │ [OPEN TIMETABLE]  [EXPORT PACK]             │                       │
│ GROW              │ └─────────────────────────────────────────────┘                       │
│ ▢ AUDIENCE    P3  │ NEEDS YOUR ATTENTION · 4                                              │
│ ▢ CAMPAIGNS   P3  │ ┃ ⚠ UNTIMED   Klubnacht 03 · Oscar Mulero has no set time [SCHEDULE →]│
│ DEAL              │ ┃ ◷ GOES LIVE Warehouse 10 Oct · public in 22 h           [REVIEW →]  │
│ ▢ BOOKINGS    P4  │ ┃ ⊘ REVEAL    Warehouse 10 Oct · address unlocks SAT 12:00 [REBUILD →]│
│───────────────────│ ┃ ✎ DRAFT     Halloween 31 Oct · no venue, no lineup      [CONTINUE →]│
│ ▢ SETTINGS        │ ┃ ⚿ SECURITY  Add an authenticator app for members & money [SET UP] [LATER]
└───────────────────┴──────────────────────────────────────────────────────────────────────┘
```
Queue order: unsaved conflict (failed) → untimed/dead air (pending) → goes live <48 h, reveal due (scheduled) → draft <21 days out (draft) → security (pending). More than 3 of one kind collapse ("+2 MORE DRAFTS"). Nav badge = queue length, red only with a conflict.

### 4.2 Events list
```
│ EVENTS                                         [Search events…]  [VENUE ▾]  [+ NEW EVENT] │
│ UPCOMING 5 · DRAFTS 2 · PAST 31                                                           │
│┃ SAT │ KLUBNACHT 03                          [PUBLISHED] ◉PUBLIC                      ⋯  │
│┃ 03  │ Tresor.West, Berlin · 23:00→07:00 +1 · 9 ACTS · 2 STAGES · 1 UNTIMED              │
│┃ SAT │ WAREHOUSE 10                          [SCHEDULED] ◉PUBLIC ◷ LIVE FRI 18:00     ⋯  │
│┃ 10  │ Berlin · SECRET · REVEAL SAT 12:00 · 6 ACTS                                        │
│┃ SAT │ SUNDAY OPEN AIR                       [POSTPONED] ⛓UNLISTED                    ⋯  │
```
Overflow: OPEN · DUPLICATE… · EXPORT PACK · CHANGE STATUS… · CANCEL EVENT. Duplicate asks for a date (default same weekday +7), with LINEUP / STAGES / SET TIMES (shifted) / DESCRIPTION toggles; the copy is always a draft.

### 4.3 Create / edit event
```
│ NEW EVENT                                               [CANCEL] [CREATE & ADD LINEUP →]  │
│ ┌ ESSENTIALS ────────────────────────────────────────┐ ┌ PUBLIC SEES ──────────────────┐ │
│ │ TITLE*        [Klubnacht 04                      ] │ │ KLUBNACHT 04                  │ │
│ │ DATE*         [SAT 10 OCT 2026 ▾]                  │ │ SAT 10 OCT · 23:00–07:00      │ │
│ │ STARTS* [23:00]  ENDS* [07:00]  → +1 SUN · 8H      │ │ BERLIN — LOCATION TBA         │ │
│ │ DOORS   [22:30]                                    │ │ until SAT 10 OCT 12:00        │ │
│ │ VENUE   [Tresor.West ▾]  or [CITY ONLY]            │ │ then: Tresor.West, Berlin     │ │
│ │ TIMES IN EUROPE/BERLIN · CEST · UTC+2  [CHANGE]    │ │ NOT PUBLIC YET · DRAFT        │ │
│ │ ⓘ Your device is in Lisbon (1 h behind).           │ └───────────────────────────────┘ │
│ └────────────────────────────────────────────────────┘                                   │
│ ▸ LOCATION PRIVACY    VENUE · [CITY ONLY] · [SECRET]  REVEAL AT [SAT 10 OCT 12:00]       │
│ ▸ RELEASE             VISIBILITY PUBLIC · GO LIVE: WHEN I PUBLISH                         │
│ ▸ DETAILS             GENRES · MIN AGE · COST · TICKET LINK · DESCRIPTION                 │
│ ▸ CAPACITY & SLUG     1,200 · klubnacht-04                                                │
```
End before start = next day ("+1 SUN", duration), never an error. DST notice when the night crosses a change. Timezone from venue, overridable. Validation on blur; on submit an error summary takes focus; server 422 maps to fields. Edits that would cut sets are pre-checked and blocked with the list of affected sets.

### 4.4 Lineup & timetable (desktop)
```
│ KLUBNACHT 03  [PUBLISHED] ◉PUBLIC                         [EXPORT PACK]  [⋯]              │
│ DETAILS · LINEUP & TIMETABLE · EXPORT                                                     │
├───────────────────────────┬───────────────────────────────────────────────────────────────┤
│ LINEUP · BILLING ORDER    │ [GRID | LIST]  SNAP [15′ ▾]  STAGES [+ STAGE]  ⚠ 1 CONFLICT   │
│ [Add acts — paste a list] │ ┌ ⚠ OVERLAP · MAIN ROOM: Dasha Rush ends 02:30, Ben Klock     │
│ ⠿ 1 Ben Klock    ▲▼  ⋯    │ │   starts 02:00. Move one set or make them B2B. [SHOW]        │
│ ⠿ 2 Dasha Rush   ▲▼  ⋯    │ └──────────────────────────────────────────────────────────── │
│ ⠿ 3 Oscar Mulero ▲▼  ⋯    │        MAIN ROOM · CO 15′        │ GARDEN · CO 10′            │
│ ⠿ 4 ┌ B2B ─────────┐      │ 23:00 ┊ Kaiser 23:00–01:00       │ ┊ Lena W 23:00–01:30       │
│     │ Nene H / Rrose│     │ 01:15 ┃ Dasha Rush 00:30–02:30 ⚠  │ ▨ CO 10′                   │
│ UNTIMED · 1               │ 02:00 ┃▒▒ OVERLAP 30′ ▒▒▒▒▒▒▒▒▒▒ │ ┃ B2B NENE H × RROSE       │
│ ◌ Oscar Mulero [SCHEDULE] │       ┃ Ben Klock 02:00–05:00 ⚠  │ ┃ 01:40–04:00              │
│                           │ 03:00 ░ EMPTY 30′ (dead air)     │                            │
│                           │ 05:00 ━━ CURFEW 05:00 ━━━━━━━━━━ │ ━━ CURFEW 04:00 ━━━━━━━━━━ │
├───────────────────────────┴───────────────────────────────────────────────────────────────┤
│ UNSAVED · 3 CHANGES · SAVED ON THIS DEVICE        [DISCARD]  [SAVE TIMETABLE] (⌘S)        │
```
Day marker at midnight. Drag body = move, bottom edge = resize, roster → column = schedule. Stage settings in a popover from the column header. LIST view = mobile layout and the full keyboard / screen-reader route.

### 4.5 Timetable backstage (mobile)
```
┌──────────────────────────────┐
│ KLUBNACHT 03   ⊘ OFFLINE · 2 │
│ [MAIN ROOM] [GARDEN] [+]     │
│ TIMES IN BERLIN · CURFEW 05:00│
│┃ 23:00–01:00  Kaiser        ›│
│  ▨ changeover 15′            │
│┃ 01:15–02:30  Dasha Rush ⚠  ›│
│┃ ● ON NOW                     │
│┃ 02:00–05:00  Ben Klock  ⚠  ›│
│ ━━ CURFEW 05:00 ━━━━━━━━━━━━ │
│ UNTIMED: Oscar Mulero      › │
├──────────────────────────────┤
│ 2 CHANGES WAITING [SAVE NOW] │
│ DASH EVENTS [NOW] VENUES MORE│
└──────────────────────────────┘
 Sheet: START/END steppers [−15][−5] 02:30 [+5][+15] (48 px), ▢ SHIFT ALL LATER SETS,
 curfew impact preview, STAGE ▾, B2B WITH ▾, [CANCEL] [APPLY → local draft]
```

### 4.6 Export pack
```
│ DETAILS · LINEUP & TIMETABLE · EXPORT                                    [DOWNLOAD ZIP]   │
│ ┌ WITHHELD FROM THIS PACK ──────────────────────────────────────────────────────────────┐ │
│ │ ⊘ Venue name & address — SECRET until SAT 10 OCT 12:00 CEST. Pack says "Berlin — TBA". │ │
│ │ ⊘ Venue contacts, tech notes, capacity — internal, never exported.                     │ │
│ │ ◷ EMBARGO — goes live FRI 09 OCT 18:00 CEST. Don't post before then.                   │ │
│ │ BUILT FROM VERSION 14 · UP TO DATE        (or: EVENT CHANGED · [REBUILD])               │ │
│ └───────────────────────────────────────────────────────────────────────────────────────┘ │
│ ┌ OG IMAGE 1200×630 ───────────┐ ┌ COPY FIELDS  [RA] FACEBOOK  DICE  GENERIC ────────────┐ │
│ │ [ 1.91:1 preview ]           │ │ TITLE       Klubnacht 03                  12/100 [COPY]│ │
│ │ [DOWNLOAD PNG]               │ │ LINEUP      Ben Klock, Dasha Rush…        4 LINES [COPY]│ │
│ └──────────────────────────────┘ │ VENUE       TBA (secret)                  WITHHELD      │ │
│ ┌ STATIC PAGE ─────────────────┐ │ [COPY ALL FIELDS]                                      │ │
│ │ [thumbnail] [PREVIEW] [HTML] │ └────────────────────────────────────────────────────────┘ │
│ └──────────────────────────────┘ ┌ EMBED ─┐ ┌ JSON-LD · REQUIRED 5/5 · RECOMMENDED 6/8 ┐ ┌ ICS ┐│
```
Withheld fields appear as WITHHELD rows, never silently missing. Drafts / unsaved timetables get a pending banner explaining what the pack contains.

### 4.7 Venues list + editor
```
│ VENUES                                                 [Search…]        [+ NEW VENUE]     │
│ NAME          CITY        ROOMS  CAP    CURFEW  TZ             NEXT EVENT                 │
│ Tresor.West   Berlin, DE  2      1,200  05:00   Europe/Berlin  SAT 03 OCT              ⋯  │

│ TRESOR.WEST                                            [ARCHIVE]  [SAVE VENUE]           │
│ ┌ PUBLIC ───────────────────┐ ┌ ROOMS · STAGE TEMPLATES ──────────────────────────────┐ │
│ │ NAME*  CITY*  COUNTRY ▾   │ │ ⠿ Main Room   CAP [800]  ▲▼ ✕                          │ │
│ │ TIMEZONE [Europe/Berlin ▾]│ │ ⠿ Garden      CAP [400]  ▲▼ ✕     [+ ROOM]             │ │
│ └───────────────────────────┘ └────────────────────────────────────────────────────────┘ │
│ ┌ OPERATIONS: CAPACITY · CURFEW (local) · TECH NOTES                                     │ │
│ │ ⓘ Not encrypted. Keep names, phones and emails in PROTECTED.                          │ │
│ │ ⚠ LOOKS LIKE A PHONE NUMBER (+49 30…) — [MOVE TO PROTECTED CONTACT]                   │ │
│ ┌ ⚿ PROTECTED · ENCRYPTED AT REST ──────────────────────────────────────────────────────┐ │
│ │ ADDRESS ••••  [SHOW] Exported only when an event's location is revealed.               │ │
│ │ MAP PIN ••••  [SHOW]   CONTACT NAME / EMAIL / PHONE ••••  [SHOW]  Never exported.       │ │
```

### 4.8 Door entry (mobile, forced dark, `/door/run`)
```
│ DOOR · KLUBNACHT 03 · DEVICE: FRONT DOOR 1 ● ONLINE (⊘ OFFLINE · LIST FROM 21:40) │
│ ENTER EVENT PIN  ■ ■ ■ ▢ ▢ ▢   · 72 px keypad · WRONG PIN · 3 TRIES LEFT           │
```
Unregistered device: "THIS PHONE ISN'T A DOOR DEVICE. A manager registers it in SETTINGS → DOOR DEVICES." Managers see `/door` in the shell with PIN status, devices and [LAUNCH DOOR MODE].

## 5. Components
Reuse: `page-header`/`page-title`/`page-sub`, `page-body`, `tabs-bar` (NuxtLinks), `glass`, `hud-card`, `bracket-box`, `section-lbl`, `data-frag`, `hud-input`, `hud-textarea`, `btn-hud-cta/-ghost/-error`, `badge-hud-*`, `accent-bar-*`, `pulse-dot`, `prog-track/fill`, `hud-toggle`; shadcn Dialog, Sheet, Popover, DropdownMenu, Select, Table, Toaster.

Status → badge: draft `badge-draft`; scheduled `badge-scheduled`; published (upcoming) `badge-ready`; past `badge-published` "PAST"; cancelled `badge-failed`; postponed: see decisions. Visibility: `data-frag` + icon (Globe PUBLIC, Link UNLISTED, Lock PRIVATE).

`libs/ui` changes: `KhNavItem` + `group`, `tag`, `count`, `countTone`; `KhSideNav` renders groups; `KhMobileHeader` status from real connectivity and 44 px theme hit areas; new `KhEmptyState`, `KhSegmented` (`role="radiogroup"`).

Promoter components: `AttentionQueue`/`AttentionItem`, `NextEventCard`, `EventRow`, `EventStatusBadge`, `EventFlags`, `EventForm`, `DateTimeRange`, `TimezoneCombobox`, `LocationModeField`, `PublicPreview`, `FormErrorSummary`, `PreflightChecklist`, `LineupRoster` (paste split on newline/comma), `TimetableGrid`, `SetBlock`, `ChangeoverBlock`, `DeadAirBlock`, `CurfewLine`, `DayMarker`, `TimetableList`, `SetTimeSheet`, `ConflictPanel` (`role="status"`), `SaveBar`, `WithheldPanel`, `OgImagePreview`, `CopyField`, `PlatformFields`, `CodeBlock`, `VenueForm`, `RoomsEditor`, `ProtectedField` (reveal audit-logged), `TechNotesField` (PII heuristic), `DoorPinPad`, `ReadOnlyNotice`.

## 6. States checklist
- **Dashboard:** loading skeleton; ALL CLEAR; no events (empty state); error + retry; MFA snoozed 7 days then inline gate; offline.
- **Events list:** skeleton; empty per tab; filtered empty; error; past-dated draft chip; duplicate in flight.
- **Create/edit:** pristine; field errors + summary; slug taken (suggest -2); overnight; DST; device tz ≠ event tz; secret without reveal time; edit breaks timetable (blocking pre-check); version conflict 409 ("SOMEONE ELSE SAVED THIS EVENT. [RELOAD] [KEEP MINE]"); save failed keeps form.
- **Timetable:** no stages; no acts; all untimed; client conflict; server rejection (errors pinned, draft kept); dead air; past curfew; offline queued; reconnect with server change (diff dialog); live (NOW line); past (read-only).
- **Export:** generating; up to date / stale → rebuild; embargoed; withheld; reveal passed; draft; unsaved timetable; private; missing recommended JSON-LD; clipboard blocked.
- **Venues:** empty; loading; error; protected masked / revealing / failed; PII warning; archive blocked while upcoming events use it.
- **Door entry:** unregistered; PIN; wrong PIN; locked; downloading; offline cached; event not today.
- **Roles:** read-only users see values plus a `ReadOnlyNotice`, never a disabled form.

## 7. Accessibility
- Keyboard DnD: every `SetBlock` is a button; Enter = time popover; Space = pick up → ↑/↓ snap, Shift+↑/↓ 5′, ←/→ stage, Alt+↑/↓ resize end, Space drop, Esc cancel; each step announced politely. Roster ▲▼ buttons.
- Stage = `<section aria-label="Main Room, curfew 05:00, changeover 15 minutes">` with an `<ol>` of sets; no `role=grid`. Block name includes time, stage and conflict.
- Focus order: header → tabs → toolbar → conflict panel → roster → stages → SaveBar.
- Live regions: conflict count `status`; rejected save `alert` + focus; copy announcements polite; countdown not live.
- Never colour alone; tokens only in Daytime HUD, measure hatch/curfew/dead-air contrast; reduced motion: static pulse, no inertia.
- Targets 44 px (sm/xs buttons only with 44 px hit area), set blocks ≥ 44 px, door keys 72 px, mobile rows 56 px.
- Real labels, `aria-describedby` hints/errors, `aria-invalid`; "+1" hint read out.
- Selection = primary outline; fix `.row-selected` violet drift in kinetic.css.

## 8. Copy and voice
Terminal labels uppercase; sentences in data register. Examples: "NO EVENTS YET · Title, date and a place is enough to start." · "OVERLAP · MAIN ROOM · Dasha Rush ends 02:30 but Ben Klock starts 02:00. Move one set or make them B2B." · "PAST CURFEW 05:00 · Last set ends 05:30, 30 min after the Main Room curfew." · "DEAD AIR · 03:00–03:30 · Nobody is playing on Main Room." · "NOT SAVED · 2 PROBLEMS · Your changes are still here." · "OFFLINE · 2 CHANGES WAITING · Saved on this phone." · "WITHHELD UNTIL SAT 10 OCT 12:00 CEST · The pack says 'Berlin — location TBA'." · "EMBARGO · Goes live FRI 18:00 CEST. Don't post before then." · "ENCRYPTED AT REST · Contacts never leave this instance." · "Tech notes aren't encrypted. Keep names, phone numbers and emails in PROTECTED."

## 9. Open questions (designer's recommendation first)
1. POSTPONED colour: widen amber to "on hold" (archived + postponed) in DESIGN.md, or violet.
2. `city_only` = city always, venue never exported; `secret` = city shown, venue withheld until `location_reveal_at` (no reveal time = hidden until changed). Server export must implement exactly this.
3. Conflicted timetables: server rejects all invalid saves, UI keeps a device draft (plan) — or drafts may save with conflicts but cannot publish/export.
4. Role matrix: bookers edit; marketing read + export; owner/admin/booker reveal protected venue fields (audited).
5. Event edits that would cut sets: reject with pre-check (vs auto-trim).
6. Unbuilt nav sections: grouped with phase tags on desktop, hidden on mobile (vs hidden everywhere).
7. Private events export: ICS + copy fields only.
8. Pack before `publish_at`: allowed with EMBARGOED banner and `-embargoed` ZIP suffix.
9. Promoter status bar: defer to P2.
10. Design-system fixes: 44 px vs `btn-hud-sm/-xs`; MobileHeader theme buttons; `.row-selected` drift; `accent-bar-*` hex → tokens.
11. Timetable snap: 15′ default, 5′ with Shift.

## Decisions
_To be filled as the product owner answers the open questions._
