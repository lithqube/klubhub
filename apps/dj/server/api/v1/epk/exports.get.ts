import { listEpkExports } from './-state'

// Mirrors the Go EPK handler: GET /api/v1/epk/exports answers
// {"data": [{id, garageObjectKey, createdAt}]}, newest first.
export default defineEventHandler(() => ({ data: listEpkExports() }))
