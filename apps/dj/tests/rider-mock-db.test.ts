// @vitest-environment node
// The shared rider mock (shared/rider-mock/db.ts) implements the Go API's
// contract (api/internal/rider) for the Nitro dev mocks and the browser demo.
// These tests pin that contract, so a mock cannot drift from the real API.
import { describe, expect, it } from 'vitest'
import { MAX_NAME_CHARS, MAX_SECTION_CHARS, RiderMockDb, validateRiderText, type Failure, type Result } from '../shared/rider-mock/db'

const NOW = new Date('2026-10-02T12:00:00.000Z')
const fresh = (seed = false) => new RiderMockDb({ now: () => NOW, seed })

function ok<T>(r: Result<T>): T {
  if (!r.ok) throw new Error(`expected success, got ${r.status} ${r.message}`)
  return r.value
}
function bad(r: Result<unknown>, status: Failure['status'], message?: string): void {
  expect(r.ok).toBe(false)
  if (r.ok) return
  expect(r.status).toBe(status)
  if (message) expect(r.message).toContain(message)
}

describe('validateRiderText', () => {
  it('accepts values at the limits, counting characters rather than bytes', () => {
    expect(validateRiderText({ name: 'é'.repeat(MAX_NAME_CHARS), technical: 'é'.repeat(MAX_SECTION_CHARS) }, true)).toBeNull()
    expect(validateRiderText({ name: '🎧'.repeat(MAX_NAME_CHARS) }, true)).toBeNull() // an emoji is one character
  })
  it('rejects blank / missing names, over-long text and NUL with the Go API\'s wording', () => {
    expect(validateRiderText({}, true)).toBe('invalid rider input: name is required')
    expect(validateRiderText({ name: '   ' }, true)).toBe('invalid rider input: name is required')
    expect(validateRiderText({ name: '   ' }, false)).toBe('invalid rider input: name cannot be blank')
    expect(validateRiderText({ name: 'n'.repeat(201) }, true)).toContain('name is too long (201 characters; the limit is 200)')
    expect(validateRiderText({ name: 'n', backline: 'b'.repeat(20001) }, true)).toContain('backline is too long')
    expect(validateRiderText({ name: 'n', otherNotes: 'a\u0000b' }, true)).toContain('otherNotes contains a NUL')
  })
  it('on update only inspects what is present', () => {
    expect(validateRiderText({}, false)).toBeNull()
    expect(validateRiderText({ technical: 'ok' }, false)).toBeNull()
  })
})

describe('RiderMockDb templates', () => {
  it('seeds three templates, listed by name', () => {
    expect(fresh(true).listTemplates().map((t) => t.name)).toEqual(['Boilerplate', 'Festival heavy', 'Standard club'])
    expect(fresh(false).listTemplates()).toEqual([])
  })

  it('create: trims the name, snapshots sections, gives createdAt === updatedAt', () => {
    const t = ok(fresh().createTemplate({ name: '  Club  ', technical: 't' }))
    expect(t).toMatchObject({ name: 'Club', technical: 't', hospitality: '', backline: '', otherNotes: '' })
    expect(t.createdAt).toBe(t.updatedAt)
  })

  it('create / update refuse a name already used by another template, case-insensitively', () => {
    const db = fresh()
    const a = ok(db.createTemplate({ name: 'Standard club' }))
    bad(db.createTemplate({ name: 'STANDARD CLUB' }), 422, 'already exists')
    const b = ok(db.createTemplate({ name: 'Festival' }))
    bad(db.updateTemplate(b.id, { name: 'standard Club' }, b.updatedAt), 422, 'already exists')
    expect(db.getTemplate(b.id)?.name).toBe('Festival') // nothing applied
    ok(db.updateTemplate(a.id, { name: 'STANDARD club' }, a.updatedAt)) // keeping your own name is fine
  })

  it('a deleted template frees its name', () => {
    const db = fresh()
    const a = ok(db.createTemplate({ name: 'Reusable' }))
    ok(db.deleteTemplate(a.id))
    ok(db.createTemplate({ name: 'reusable' }))
  })

  it('update: token required (422), stale (409), gone (404); success advances the token', () => {
    const db = fresh()
    const t = ok(db.createTemplate({ name: 'T' }))
    bad(db.updateTemplate(t.id, { technical: 'x' }, undefined), 422, 'updatedAt is required')
    bad(db.updateTemplate(t.id, { technical: 'x' }, ''), 422, 'updatedAt is required')
    bad(db.updateTemplate(t.id, { technical: 'x' }, '1999-01-01T00:00:00.000Z'), 409, 'changed since it was read')
    bad(db.updateTemplate('nope', { technical: 'x' }, t.updatedAt), 404)

    const next = ok(db.updateTemplate(t.id, { technical: 'x' }, t.updatedAt))
    expect(next.updatedAt > t.updatedAt).toBe(true)
    bad(db.updateTemplate(t.id, { technical: 'y' }, t.updatedAt), 409) // the old token is now stale
    ok(db.updateTemplate(t.id, { technical: 'y' }, next.updatedAt))
  })

  it('validation runs before the lookup, like the service layer (422 beats 404/409)', () => {
    const db = fresh()
    bad(db.updateTemplate('nope', { technical: 'x'.repeat(MAX_SECTION_CHARS + 1) }, 'whatever'), 422, 'too long')
  })

  it('updates are per-field: absent sections are untouched, "" clears one', () => {
    const db = fresh()
    const t = ok(db.createTemplate({ name: 'T', technical: 't', hospitality: 'h', backline: 'b', otherNotes: 'o' }))
    const a = ok(db.updateTemplate(t.id, { technical: 't2' }, t.updatedAt))
    expect(a).toMatchObject({ technical: 't2', hospitality: 'h', backline: 'b', otherNotes: 'o', name: 'T' })
    const b = ok(db.updateTemplate(t.id, { hospitality: '' }, a.updatedAt))
    expect(b).toMatchObject({ technical: 't2', hospitality: '', backline: 'b' })
  })

  it('every write yields a strictly newer token, even inside one millisecond', () => {
    const db = fresh() // the clock is frozen at NOW
    const t = ok(db.createTemplate({ name: 'T' }))
    const a = ok(db.updateTemplate(t.id, { technical: '1' }, t.updatedAt))
    const b = ok(db.updateTemplate(t.id, { technical: '2' }, a.updatedAt))
    expect(new Set([t.updatedAt, a.updatedAt, b.updatedAt]).size).toBe(3)
    expect(a.updatedAt > t.updatedAt && b.updatedAt > a.updatedAt).toBe(true)
  })

  it('returns copies, so callers cannot mutate the store through a result', () => {
    const db = fresh()
    const t = ok(db.createTemplate({ name: 'T' }))
    t.name = 'tampered'
    expect(db.getTemplate(t.id)?.name).toBe('T')
  })
})

describe('RiderMockDb attachments', () => {
  const GIG = 'gig-1'

  it('create from a template snapshots its text; later template edits do not reach the copy', () => {
    const db = fresh()
    const tpl = ok(db.createTemplate({ name: 'Src', technical: 'original' }))
    const att = ok(db.createAttachment({ gigId: GIG, templateId: tpl.id }, true))
    expect(att).toMatchObject({ gigId: GIG, templateId: tpl.id, technical: 'original' })
    ok(db.updateTemplate(tpl.id, { technical: 'changed' }, tpl.updatedAt))
    expect(db.getAttachment(att.id)?.technical).toBe('original')
  })

  it('create: unknown template 404, unknown gig 404, one per gig 409, missing gigId 400, limits 422', () => {
    const db = fresh()
    bad(db.createAttachment({ gigId: GIG, templateId: 'no-such' }, true), 404)
    bad(db.createAttachment({ gigId: 'gone' }, false), 404)
    bad(db.createAttachment({}, true), 400)
    bad(db.createAttachment({ gigId: GIG, technical: 'x'.repeat(MAX_SECTION_CHARS + 1) }, true), 422, 'too long')
    expect(db.getAttachmentByGig(GIG)).toBeNull() // nothing was stored by the refusals
    ok(db.createAttachment({ gigId: GIG }, true))
    bad(db.createAttachment({ gigId: GIG }, true), 409, 'already exists')
  })

  it('update: token rules and per-field sections', () => {
    const db = fresh()
    const att = ok(db.createAttachment({ gigId: GIG, technical: 't', backline: 'b' }, true))
    bad(db.updateAttachment(att.id, { technical: 'x' }, undefined), 422, 'updatedAt is required')
    bad(db.updateAttachment(att.id, { technical: 'x' }, '1999-01-01T00:00:00.000Z'), 409)
    bad(db.updateAttachment('nope', { technical: 'x' }, att.updatedAt), 404)
    const next = ok(db.updateAttachment(att.id, { backline: 'b2' }, att.updatedAt))
    expect(next).toMatchObject({ technical: 't', backline: 'b2' })
    expect(next.updatedAt > att.updatedAt).toBe(true)
  })

  it('deleting a template clears the reference on its attachments, keeping content and token', () => {
    const db = fresh()
    const tpl = ok(db.createTemplate({ name: 'Src', technical: 'keep me' }))
    const other = ok(db.createTemplate({ name: 'Other' }))
    const att = ok(db.createAttachment({ gigId: GIG, templateId: tpl.id }, true))
    const unrelated = ok(db.createAttachment({ gigId: 'gig-2', templateId: other.id }, true))

    ok(db.deleteTemplate(tpl.id))

    const got = db.getAttachment(att.id)!
    expect(got.templateId).toBeNull()
    expect(got.technical).toBe('keep me')
    expect(got.updatedAt).toBe(att.updatedAt) // open editors keep a valid token
    expect(db.getAttachment(unrelated.id)?.templateId).toBe(other.id)
    bad(db.deleteTemplate(tpl.id), 404)
    ok(db.updateAttachment(att.id, { technical: 'edited' }, att.updatedAt))
  })

  it('delete: removes it, 404 the second time; by-gig is null when there is none', () => {
    const db = fresh()
    const att = ok(db.createAttachment({ gigId: GIG }, true))
    expect(db.getAttachmentByGig(GIG)?.id).toBe(att.id)
    ok(db.deleteAttachment(att.id))
    expect(db.getAttachmentByGig(GIG)).toBeNull()
    bad(db.deleteAttachment(att.id), 404)
    ok(db.createAttachment({ gigId: GIG }, true)) // the gig can be attached again
  })
})

describe('RiderMockDb persistence', () => {
  it('toJSON / load round-trips everything, including the token clock', () => {
    const a = fresh(true)
    const tpl = ok(a.createTemplate({ name: 'Persisted' }))
    const att = ok(a.createAttachment({ gigId: 'g', templateId: tpl.id }, true))

    const b = new RiderMockDb({ now: () => NOW })
    b.load(JSON.parse(JSON.stringify(a.toJSON())))

    expect(b.listTemplates()).toEqual(a.listTemplates())
    expect(b.getAttachment(att.id)).toEqual(att)
    const next = ok(b.updateAttachment(att.id, { technical: 'x' }, att.updatedAt))
    expect(next.updatedAt > att.updatedAt).toBe(true) // the clock survived the round trip
  })

  it('load(undefined) reseeds, e.g. for a stored demo state from before rider existed', () => {
    const db = fresh()
    db.load(undefined)
    expect(db.listTemplates()).toHaveLength(3)
  })

  it('a snapshot is independent of the live db', () => {
    const db = fresh(true)
    const snap = db.toJSON()
    ok(db.createTemplate({ name: 'Later' }))
    expect(snap.templates).toHaveLength(3)
  })
})
