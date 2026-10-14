// Frontend-only mock of GET /api/v1/tracklists/:id/gigs. The Nitro mocks keep no
// link state, so no tracklist has linked gigs; the in-browser demo backend does.
export default defineEventHandler(() => [])
