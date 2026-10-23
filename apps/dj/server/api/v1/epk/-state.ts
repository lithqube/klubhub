// In-memory EPK export history for the dev mock (frontend-only dev). The
// leading "-" keeps Nitro from treating this file as a route. The outreach
// mock (server/api/v1/outreach/-mockDb.ts) reads the same rows to validate
// the composer's EPK attach picker, so the two stay consistent.

// Mirrors the Go EPK handler's camelCase export rows (api/internal/epk):
// GET /api/v1/epk/exports answers {"data": [{id, garageObjectKey, createdAt}]}.
export interface MockEpkExport {
  id: string
  garageObjectKey: string
  createdAt: string
}

const iso = (daysAgo: number, hoursAgo = 0): string =>
  new Date(Date.now() - daysAgo * 86400000 - hoursAgo * 3600000).toISOString()

export const epkExportRows: MockEpkExport[] = [
  { id: 'epk-1', garageObjectKey: 'epk/exports/epk-1.pdf', createdAt: iso(2, 5) },
  // Legacy pre-025 export (minio_path row): no garage object key — the send
  // flow refuses to attach it (422 "epk export has no garage object key").
  { id: 'epk-legacy', garageObjectKey: '', createdAt: iso(9) },
]

/** Newest first, like the Go handler's ListExports. */
export function listEpkExports(): MockEpkExport[] {
  return [...epkExportRows]
    .sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))
    .map(e => ({ ...e }))
}

export function findEpkExport(id: string): MockEpkExport | null {
  return epkExportRows.find(e => e.id === id) ?? null
}
