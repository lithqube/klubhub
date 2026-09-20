---
phase: 04-gig-tracker
plan: 05
subsystem: api + frontend
tags: [gig-tracker, tracklist, linking]
status: complete
completed: 2026-09-20
---

# Phase 04 Plan 05: Tracklist ↔ Gig Linking

## Goal
Allow DJs to link tracklists to gigs bidirectionally — from the tracklist view and from the gig form — so booked gigs carry their actual tracklist references.

## Scope
- Backend: `LinkTracklist` / `UnlinkTracklist` repository + service ops, `POST /{id}/tracklists/{tracklistId}` and `DELETE /{id}/tracklists/{tracklistId}` handler routes, `ServiceIface` extended
- Nuxt: proxy routes `[id]/tracklists/[tracklistId].post.ts` and `.delete.ts`
- Frontend: `TracklistGigLinker.vue` component, tracklist detail page wiring, GigFormDialog linked-tracklists section, GigListRow tracklist badge, `useTracklistStore.linkedGigs` state

## Files touched
- `api/internal/gig/repository.go` — LinkTracklist / UnlinkTracklist
- `api/internal/gig/service.go` — LinkTracklist / UnlinkTracklist service ops + Exists on tracklistRepoIface
- `api/internal/gig/handler.go` — handleLinkTracklist / handleUnlinkTracklist + routes
- `api/internal/tracklist/repository.go` — Exists method
- `apps/dj/server/api/v1/gigs/[id]/tracklists/[tracklistId].post.ts` — Nuxt proxy route
- `apps/dj/server/api/v1/gigs/[id]/tracklists/[tracklistId].delete.ts` — Nuxt proxy route
- `apps/dj/app/components/tracklist/TracklistGigLinker.vue` — NEW component
- `apps/dj/app/components/gig/GigFormDialog.vue` — linked-tracklists section + script wiring
- `apps/dj/app/components/gig/GigListRow.vue` — tracklist badge
- `apps/dj/app/pages/render/tracklist/[id].vue` — TracklistGigLinker integration
- `apps/dj/app/stores/gig.ts` — linkTracklist / unlinkTracklist
- `apps/dj/app/stores/tracklist.ts` — linkedGigs state + fetchTracklist
- `.planning/ROADMAP.md` — plan 04-05 marked complete

## Verification
- `go build ./...` passes (full API including cmd/api)
- `go test ./internal/gig/... ./internal/tracklist/...` passes
- `nuxi build` succeeds with both proxy routes compiled
