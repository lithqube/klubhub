// In-memory rider store implementing the Go API's contract
// (api/internal/rider). One instance backs the Nitro dev mocks, another the
// browser demo (which persists it with toJSON()/load()). Keeping the rules
// here, once, is what stops frontend-only dev from hiding errors the real
// backend returns:
//   - updates need the updatedAt the caller last saw: missing -> 422,
//     stale -> 409, nothing applied
//   - text limits (name 200, each section 20000 characters), no NUL, no
//     blank name, and template names unique case-insensitively -> 422
//   - deleting a template clears the reference on its attachments
//   - an attachment needs a live gig (404) and there is one per gig (409)

import type { RiderAttachment, RiderTemplate } from '../../app/types/rider'

export const MAX_NAME_CHARS = 200
export const MAX_SECTION_CHARS = 20000

/** A failed operation, with the HTTP status the Go API answers with. */
export interface Failure {
  ok: false
  status: 400 | 404 | 409 | 422
  message: string
}
export type Result<T> = { ok: true; value: T } | Failure

export interface RiderSnapshot {
  templates: RiderTemplate[]
  attachments: RiderAttachment[]
  /** Last timestamp handed out (ms since epoch); keeps updatedAt strictly increasing. */
  clock: number
}

const SECTIONS = ['technical', 'hospitality', 'backline', 'otherNotes'] as const
type Section = (typeof SECTIONS)[number]
type SectionPatch = Partial<Record<Section, unknown>>

const fail = (status: Failure['status'], message: string): Failure => ({ ok: false, status, message })
const invalid = (detail: string): Failure => fail(422, `invalid rider input: ${detail}`)
const NOT_FOUND = 'rider record not found'
const STALE = 'rider record was changed since it was read'
const ALREADY_ATTACHED = 'rider attachment already exists for this gig'

const normName = (s: string): string => s.trim().toLowerCase()

function uuid(): string {
  const c = globalThis.crypto as Crypto | undefined
  if (c?.randomUUID) return c.randomUUID()
  return 'xxxxxxxx-xxxx-4xxx-8xxx-xxxxxxxxxxxx'.replace(/x/g, () => Math.floor(Math.random() * 16).toString(16))
}

/**
 * Returns the 422 message for invalid rider text in `body`, or null if it is
 * acceptable. `nameRequired` is true when creating a template; on update a
 * name is only checked when present. Lengths count characters (code points).
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
  for (const f of SECTIONS) {
    const bad = check(f, body[f], MAX_SECTION_CHARS)
    if (bad) return bad
  }
  return null
}

function seedTemplates(): RiderTemplate[] {
  return [
    {
      id: 'tmpl-1',
      name: 'Standard club',
      technical: '2× CDJ-3000 (or equivalent)\n1× 4-channel club mixer (DJM-900NXS2 or equivalent)\nMonitor: full-range booth monitor\nDI box for laptop (if used)',
      hospitality: '4× bottled water (room temp)\n2× hot meals (vegetarian option for 1)\nTowels ×2\nGreen room access from load-in',
      backline: 'DJM-900NXS2 mixer (or equivalent)\nXLR line out to FOH',
      otherNotes: 'Soundcheck: minimum 30 minutes before doors\nArrival: 2 hours before doors\nNo flash photography on stage',
      createdAt: '2025-09-01T10:00:00.000Z',
      updatedAt: '2025-09-01T10:00:00.000Z',
    },
    {
      id: 'tmpl-2',
      name: 'Festival heavy',
      technical: '2× CDJ-3000 + 1× DJM-A9 (preferred)\nBackup: DVS setup with 2× Technics SL-1200\nXLR + headphone monitor wedge\nDI box',
      hospitality: '5× bottled water\n2× meals + snacks\nArtist lounge access\nTowels',
      backline: 'DJM-A9 or better\nPioneer XDJ setup OK\nNo rotary mixers (please confirm in advance)',
      otherNotes: 'Load-in 4 hours before set\nSoundcheck window: 60 min before set\nGenerator backup preferred',
      createdAt: '2025-09-15T12:00:00.000Z',
      updatedAt: '2025-09-15T12:00:00.000Z',
    },
    {
      id: 'tmpl-3',
      name: 'Boilerplate',
      technical: '2× CDJ + 1× mixer (any reasonable club standard)\nHeadphone monitor wedge\nXLR line out',
      hospitality: 'Bottled water\nCoffee/tea backstage',
      backline: '',
      otherNotes: 'Flexible — happy to work with what you have',
      createdAt: '2025-10-01T08:00:00.000Z',
      updatedAt: '2025-10-01T08:00:00.000Z',
    },
  ]
}

export class RiderMockDb {
  private templates: RiderTemplate[] = []
  private attachments: RiderAttachment[] = []
  private clock = 0
  private readonly now: () => Date

  constructor(opts: { now?: () => Date; seed?: boolean } = {}) {
    this.now = opts.now ?? (() => new Date())
    if (opts.seed) this.load(undefined)
  }

  /** Replaces the contents with `snapshot`, or the seed data when omitted. */
  load(snapshot: RiderSnapshot | undefined): void {
    if (!snapshot) {
      this.templates = seedTemplates()
      this.attachments = []
      this.clock = 0
      return
    }
    this.templates = snapshot.templates.map((t) => ({ ...t }))
    this.attachments = snapshot.attachments.map((a) => ({ ...a }))
    this.clock = snapshot.clock
  }

  toJSON(): RiderSnapshot {
    return {
      templates: this.templates.map((t) => ({ ...t })),
      attachments: this.attachments.map((a) => ({ ...a })),
      clock: this.clock,
    }
  }

  /** A timestamp later than `prev` and than anything issued before (the updatedAt token). */
  private stamp(prev?: string): string {
    const floor = Math.max(this.clock, prev ? Date.parse(prev) : 0) + 1
    this.clock = Math.max(this.now().getTime(), floor)
    return new Date(this.clock).toISOString()
  }

  // ── templates ──

  listTemplates(): RiderTemplate[] {
    return [...this.templates]
      .sort((a, b) => a.name.localeCompare(b.name))
      .map((t) => ({ ...t }))
  }

  getTemplate(id: string): RiderTemplate | null {
    const t = this.templates.find((x) => x.id === id)
    return t ? { ...t } : null
  }

  private nameTaken(name: string, exceptId?: string): boolean {
    const key = normName(name)
    return this.templates.some((t) => t.id !== exceptId && normName(t.name) === key)
  }

  createTemplate(input: Record<string, unknown>): Result<RiderTemplate> {
    const bad = validateRiderText(input, true)
    if (bad) return fail(422, bad)
    const name = (input.name as string).trim()
    if (this.nameTaken(name)) return invalid(`a template named "${name}" already exists`)
    const at = this.stamp()
    const t: RiderTemplate = {
      id: uuid(),
      name,
      technical: typeof input.technical === 'string' ? input.technical : '',
      hospitality: typeof input.hospitality === 'string' ? input.hospitality : '',
      backline: typeof input.backline === 'string' ? input.backline : '',
      otherNotes: typeof input.otherNotes === 'string' ? input.otherNotes : '',
      createdAt: at,
      updatedAt: at,
    }
    this.templates.push(t)
    return { ok: true, value: { ...t } }
  }

  /** `patch` is the body without updatedAt; `token` is the updatedAt the caller last saw. */
  updateTemplate(id: string, patch: Record<string, unknown>, token: unknown): Result<RiderTemplate> {
    if (typeof token !== 'string' || token === '') return invalid('updatedAt is required')
    const bad = validateRiderText(patch, false)
    if (bad) return fail(422, bad)
    const t = this.templates.find((x) => x.id === id)
    if (!t) return fail(404, NOT_FOUND)
    if (t.updatedAt !== token) return fail(409, STALE)
    if (typeof patch.name === 'string') {
      const name = patch.name.trim()
      if (this.nameTaken(name, id)) return invalid(`a template named "${name}" already exists`)
      t.name = name
    }
    this.applySections(t, patch)
    t.updatedAt = this.stamp(t.updatedAt)
    return { ok: true, value: { ...t } }
  }

  /** Detaches the template from its attachments (their content and tokens are untouched). */
  deleteTemplate(id: string): Result<null> {
    const i = this.templates.findIndex((x) => x.id === id)
    if (i < 0) return fail(404, NOT_FOUND)
    this.templates.splice(i, 1)
    for (const a of this.attachments) if (a.templateId === id) a.templateId = null
    return { ok: true, value: null }
  }

  // ── attachments ──

  getAttachmentByGig(gigId: string): RiderAttachment | null {
    const a = this.attachments.find((x) => x.gigId === gigId)
    return a ? { ...a } : null
  }

  getAttachment(id: string): RiderAttachment | null {
    const a = this.attachments.find((x) => x.id === id)
    return a ? { ...a } : null
  }

  /** `gigExists`: whether the gig is live (the real API refuses deleted or unknown gigs). */
  createAttachment(input: Record<string, unknown>, gigExists: boolean): Result<RiderAttachment> {
    const gigId = typeof input.gigId === 'string' ? input.gigId : ''
    if (gigId === '') return fail(400, 'invalid gigId')
    const templateId = typeof input.templateId === 'string' && input.templateId !== '' ? input.templateId : null

    let sections: Record<Section, string> = { technical: '', hospitality: '', backline: '', otherNotes: '' }
    if (templateId === null) {
      const bad = validateRiderText(input, false)
      if (bad) return fail(422, bad)
      for (const f of SECTIONS) if (typeof input[f] === 'string') sections[f] = input[f] as string
    } else {
      // A per-gig copy, not a live link: the template's current text is snapshotted.
      const tpl = this.templates.find((x) => x.id === templateId)
      if (!tpl) return fail(404, NOT_FOUND)
      sections = { technical: tpl.technical, hospitality: tpl.hospitality, backline: tpl.backline, otherNotes: tpl.otherNotes }
    }
    if (!gigExists) return fail(404, NOT_FOUND)
    if (this.attachments.some((x) => x.gigId === gigId)) return fail(409, ALREADY_ATTACHED)

    const at = this.stamp()
    const a: RiderAttachment = { id: uuid(), gigId, templateId, ...sections, createdAt: at, updatedAt: at }
    this.attachments.push(a)
    return { ok: true, value: { ...a } }
  }

  updateAttachment(id: string, patch: Record<string, unknown>, token: unknown): Result<RiderAttachment> {
    if (typeof token !== 'string' || token === '') return invalid('updatedAt is required')
    const bad = validateRiderText(patch, false)
    if (bad) return fail(422, bad)
    const a = this.attachments.find((x) => x.id === id)
    if (!a) return fail(404, NOT_FOUND)
    if (a.updatedAt !== token) return fail(409, STALE)
    this.applySections(a, patch)
    a.updatedAt = this.stamp(a.updatedAt)
    return { ok: true, value: { ...a } }
  }

  deleteAttachment(id: string): Result<null> {
    const i = this.attachments.findIndex((x) => x.id === id)
    if (i < 0) return fail(404, NOT_FOUND)
    this.attachments.splice(i, 1)
    return { ok: true, value: null }
  }

  /** Per-field partial update: a section that is absent is left alone, "" clears it. */
  private applySections(target: SectionPatch & Record<Section, string>, patch: SectionPatch): void {
    for (const f of SECTIONS) if (typeof patch[f] === 'string') target[f] = patch[f] as string
  }
}
