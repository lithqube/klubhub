// Mock state shared across all rider/* Nitro endpoints so CRUD appears
// to persist for the lifetime of the dev server. Reset on dev restart.
// Mirrors the EPK mock data pattern (apps/dj/server/api/v1/epk/content.get.ts).

interface MockTemplate {
  id: string
  name: string
  technical: string
  hospitality: string
  backline: string
  otherNotes: string
  createdAt: string
  updatedAt: string
}

interface MockAttachment {
  id: string
  gigId: string
  templateId: string | null
  technical: string
  hospitality: string
  backline: string
  otherNotes: string
  createdAt: string
  updatedAt: string
}

const now = () => new Date().toISOString()

// updatedAt is the optimistic-concurrency token, so every write must produce
// a value different from the one it replaces (two writes inside one
// millisecond would otherwise share a token).
function stampAfter(prev: string): string {
  const t = now()
  return t > prev ? t : new Date(new Date(prev).getTime() + 1).toISOString()
}

// Input rules, mirroring the Go service (rider.MaxNameChars / MaxSectionChars
// and its NUL / blank-name checks) so frontend-only dev shows the same 422s.
export const MAX_NAME_CHARS = 200
export const MAX_SECTION_CHARS = 20000

/**
 * Returns the 422 message for invalid rider text in `body`, or null if it is
 * acceptable. `nameRequired` is true on create; on update a name is only
 * checked when present. Lengths count characters (code points), not bytes.
 */
export function validateRiderText(body: Record<string, unknown>, nameRequired: boolean): string | null {
  const check = (field: string, value: unknown, max: number): string | null => {
    if (typeof value !== 'string') return null
    if (value.includes('\u0000')) return `invalid rider input: ${field} contains a NUL character`
    const n = [...value].length
    if (n > max) return `invalid rider input: ${field} is too long (${n} characters; the limit is ${max})`
    return null
  }
  if (nameRequired && (typeof body.name !== 'string' || body.name.trim() === '')) {
    return 'invalid rider input: name is required'
  }
  if (typeof body.name === 'string') {
    if (body.name.trim() === '') return 'invalid rider input: name cannot be blank'
    const bad = check('name', body.name.trim(), MAX_NAME_CHARS)
    if (bad) return bad
  }
  for (const f of ['technical', 'hospitality', 'backline', 'otherNotes']) {
    const bad = check(f, body[f], MAX_SECTION_CHARS)
    if (bad) return bad
  }
  return null
}

/** Outcome of a compare-and-set update, mirroring the Go API's 404 / 409. */
export type UpdateResult<T> =
  | { ok: true; value: T }
  | { ok: false; reason: 'not_found' | 'conflict' }

// Module-level state — survives across requests within one dev process.
const state: {
  templates: MockTemplate[]
  attachmentsByGigId: Record<string, MockAttachment>
} = {
  templates: [
    {
      id: 'tmpl-1',
      name: 'Standard club',
      technical: '2× CDJ-3000 (or equivalent)\n1× 4-channel club mixer (DJM-900NXS2 or equivalent)\nMonitor: full-range booth monitor\nDI box for laptop (if used)',
      hospitality: '4× bottled water (room temp)\n2× hot meals (vegetarian option for 1)\nTowels ×2\nGreen room access from load-in',
      backline: 'DJM-900NXS2 mixer (or equivalent)\nXLR line out to FOH',
      otherNotes: 'Soundcheck: minimum 30 minutes before doors\nArrival: 2 hours before doors\nNo flash photography on stage',
      createdAt: '2025-09-01T10:00:00Z',
      updatedAt: '2025-09-01T10:00:00Z',
    },
    {
      id: 'tmpl-2',
      name: 'Festival heavy',
      technical: '2× CDJ-3000 + 1× DJM-A9 (preferred)\nBackup: DVS setup with 2× Technics SL-1200\nXLR + headphone monitor wedge\nDI box',
      hospitality: '5× bottled water\n2× meals + snacks\nArtist lounge access\nTowels',
      backline: 'DJM-A9 or better\nPioneer XDJ setup OK\nNo rotary mixers (please confirm in advance)',
      otherNotes: 'Load-in 4 hours before set\nSoundcheck window: 60 min before set\nGenerator backup preferred',
      createdAt: '2025-09-15T12:00:00Z',
      updatedAt: '2025-09-15T12:00:00Z',
    },
    {
      id: 'tmpl-3',
      name: 'Boilerplate',
      technical: '2× CDJ + 1× mixer (any reasonable club standard)\nHeadphone monitor wedge\nXLR line out',
      hospitality: 'Bottled water\nCoffee/tea backstage',
      backline: '',
      otherNotes: 'Flexible — happy to work with what you have',
      createdAt: '2025-10-01T08:00:00Z',
      updatedAt: '2025-10-01T08:00:00Z',
    },
  ],
  attachmentsByGigId: {},
}

export function listTemplates(): MockTemplate[] {
  return state.templates
}

export function getTemplate(id: string): MockTemplate | null {
  return state.templates.find(t => t.id === id) ?? null
}

export function createTemplate(input: {
  name?: string
  technical?: string
  hospitality?: string
  backline?: string
  otherNotes?: string
}): MockTemplate {
  const t: MockTemplate = {
    id: 'tmpl-' + Math.random().toString(36).slice(2, 10),
    name: input.name ?? 'Untitled template',
    technical: input.technical ?? '',
    hospitality: input.hospitality ?? '',
    backline: input.backline ?? '',
    otherNotes: input.otherNotes ?? '',
    createdAt: now(),
    updatedAt: now(),
  }
  state.templates.push(t)
  return t
}

export function updateTemplate(
  id: string,
  patch: Partial<Pick<MockTemplate, 'name' | 'technical' | 'hospitality' | 'backline' | 'otherNotes'>>,
  expectedUpdatedAt: string,
): UpdateResult<MockTemplate> {
  const t = state.templates.find(x => x.id === id)
  if (!t) return { ok: false, reason: 'not_found' }
  if (t.updatedAt !== expectedUpdatedAt) return { ok: false, reason: 'conflict' }
  if (patch.name !== undefined) t.name = patch.name
  if (patch.technical !== undefined) t.technical = patch.technical
  if (patch.hospitality !== undefined) t.hospitality = patch.hospitality
  if (patch.backline !== undefined) t.backline = patch.backline
  if (patch.otherNotes !== undefined) t.otherNotes = patch.otherNotes
  t.updatedAt = stampAfter(t.updatedAt)
  return { ok: true, value: t }
}

export function deleteTemplate(id: string): boolean {
  const idx = state.templates.findIndex(x => x.id === id)
  if (idx < 0) return false
  state.templates.splice(idx, 1)
  return true
}

export function getAttachmentByGig(gigId: string): MockAttachment | null {
  return state.attachmentsByGigId[gigId] ?? null
}

export function getAttachment(id: string): MockAttachment | null {
  for (const a of Object.values(state.attachmentsByGigId)) {
    if (a.id === id) return a
  }
  return null
}

export function createAttachment(input: {
  gigId: string
  templateId?: string | null
  technical?: string
  hospitality?: string
  backline?: string
  otherNotes?: string
}): MockAttachment {
  // If a template is referenced, copy its values into the new row.
  let technical = input.technical ?? ''
  let hospitality = input.hospitality ?? ''
  let backline = input.backline ?? ''
  let otherNotes = input.otherNotes ?? ''
  if (input.templateId) {
    const t = state.templates.find(x => x.id === input.templateId)
    if (t) {
      technical = t.technical
      hospitality = t.hospitality
      backline = t.backline
      otherNotes = t.otherNotes
    }
  }
  const att: MockAttachment = {
    id: 'att-' + Math.random().toString(36).slice(2, 10),
    gigId: input.gigId,
    templateId: input.templateId ?? null,
    technical,
    hospitality,
    backline,
    otherNotes,
    createdAt: now(),
    updatedAt: now(),
  }
  state.attachmentsByGigId[att.gigId] = att
  return att
}

export function updateAttachment(
  id: string,
  patch: Partial<Pick<MockAttachment, 'technical' | 'hospitality' | 'backline' | 'otherNotes'>>,
  expectedUpdatedAt: string,
): UpdateResult<MockAttachment> {
  for (const a of Object.values(state.attachmentsByGigId)) {
    if (a.id === id) {
      if (a.updatedAt !== expectedUpdatedAt) return { ok: false, reason: 'conflict' }
      if (patch.technical !== undefined) a.technical = patch.technical
      if (patch.hospitality !== undefined) a.hospitality = patch.hospitality
      if (patch.backline !== undefined) a.backline = patch.backline
      if (patch.otherNotes !== undefined) a.otherNotes = patch.otherNotes
      a.updatedAt = stampAfter(a.updatedAt)
      return { ok: true, value: a }
    }
  }
  return { ok: false, reason: 'not_found' }
}

export function deleteAttachment(id: string): boolean {
  for (const [gigId, a] of Object.entries(state.attachmentsByGigId)) {
    if (a.id === id) {
      state.attachmentsByGigId = Object.fromEntries(
        Object.entries(state.attachmentsByGigId).filter(([k]) => k !== gigId),
      )
      return true
    }
  }
  return false
}