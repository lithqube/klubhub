import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useRiderStore } from '../rider'
import { RiderConflictError } from '../../types/rider'

// Mirror stores/__tests__/epk.test.ts shape: vi.mock $fetch to assert
// the store's wire-up without touching a real server.

describe('useRiderStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  it('loadTemplates populates templates from API', async () => {
    const seed = [{ id: 'tmpl-1', name: 'A', technical: '', hospitality: '', backline: '', otherNotes: '', createdAt: '', updatedAt: '' }]
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: seed }))

    const store = useRiderStore()
    await store.loadTemplates()

    expect(store.templates).toHaveLength(1)
    expect(store.templates[0]?.name).toBe('A')
  })

  it('createTemplate appends to local state', async () => {
    const created = { id: 'tmpl-new', name: 'New', technical: '', hospitality: '', backline: '', otherNotes: '', createdAt: '', updatedAt: '' }
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: created }))

    const store = useRiderStore()
    const out = await store.createTemplate({ name: 'New' })

    expect(store.templates).toHaveLength(1)
    expect(out.id).toBe('tmpl-new')
  })

  it('deleteTemplate removes from local state', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue(undefined))
    const store = useRiderStore()
    store.templates.push({
      id: 'tmpl-1', name: 'X', technical: '', hospitality: '', backline: '', otherNotes: '', createdAt: '', updatedAt: '',
    })

    await store.deleteTemplate('tmpl-1')

    expect(store.templates).toHaveLength(0)
  })

  it('loadAttachmentByGig returns cached value on second call without re-fetching', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ data: { id: 'att-1', gigId: 'gig-1', templateId: null, technical: '', hospitality: '', backline: '', otherNotes: '', createdAt: '', updatedAt: '' } })
    vi.stubGlobal('$fetch', fetchMock)

    const store = useRiderStore()
    await store.loadAttachmentByGig('gig-1')
    expect(fetchMock).toHaveBeenCalledTimes(1)

    await store.loadAttachmentByGig('gig-1')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('loadAttachmentByGig handles null response', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: null }))

    const store = useRiderStore()
    const out = await store.loadAttachmentByGig('gig-1')

    expect(out).toBeNull()
    expect(store.attachmentsByGigId['gig-1']).toBeNull()
  })
})

// ─── updatedAt token (optimistic concurrency) ───────────────────────────────

const tpl = (over: Record<string, unknown> = {}) => ({
  id: 'tmpl-1', name: 'A', technical: 't', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: 'T0', updatedAt: 'T1', ...over,
})
const att = (over: Record<string, unknown> = {}) => ({
  id: 'att-1', gigId: 'gig-1', templateId: null, technical: 't', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: 'T0', updatedAt: 'T1', ...over,
})
const http = (statusCode: number) => Object.assign(new Error(`HTTP ${statusCode}`), { statusCode })
type PutCall = [string, { method?: string; body?: Record<string, unknown> }?]
const calls = () => vi.mocked($fetch as unknown as (u: string, o?: PutCall[1]) => Promise<unknown>).mock.calls as PutCall[]

describe('useRiderStore: updatedAt token', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  it('updateTemplate sends the cached updatedAt with the patch', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: tpl({ name: 'B', updatedAt: 'T2' }) }))
    const store = useRiderStore()
    store.templates = [tpl() as never]

    await store.updateTemplate('tmpl-1', { name: 'B' })

    expect(calls()[0]?.[1]).toMatchObject({ method: 'PUT', body: { name: 'B', updatedAt: 'T1' } })
    expect(store.templates[0]?.updatedAt).toBe('T2') // the next save will carry the new token
  })

  it('updateAttachment sends the cached updatedAt, found by attachment id', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: att({ technical: 'x', updatedAt: 'T2' }) }))
    const store = useRiderStore()
    store.attachmentsByGigId = { 'gig-0': null, 'gig-1': att() as never }

    await store.updateAttachment('att-1', { technical: 'x' })

    expect(calls()[0]?.[1]).toMatchObject({ method: 'PUT', body: { technical: 'x', updatedAt: 'T1' } })
    expect(store.attachmentsByGigId['gig-1']?.updatedAt).toBe('T2')
  })

  it('a 409 becomes RiderConflictError and leaves the cache untouched', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockRejectedValue(http(409)))
    const store = useRiderStore()
    store.templates = [tpl() as never]
    store.attachmentsByGigId = { 'gig-1': att() as never }

    await expect(store.updateTemplate('tmpl-1', { name: 'B' })).rejects.toBeInstanceOf(RiderConflictError)
    await expect(store.updateAttachment('att-1', { technical: 'x' })).rejects.toBeInstanceOf(RiderConflictError)
    expect(store.templates[0]?.name).toBe('A')
    expect(store.attachmentsByGigId['gig-1']?.technical).toBe('t')
  })

  it('other failures pass through unchanged (they are retried, not treated as conflicts)', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockRejectedValue(http(500)))
    const store = useRiderStore()
    store.templates = [tpl() as never]
    const err = await store.updateTemplate('tmpl-1', { name: 'B' }).catch(e => e)
    expect(err).not.toBeInstanceOf(RiderConflictError)
    expect((err as { statusCode: number }).statusCode).toBe(500)
  })

  it('refuses to save a record it has not loaded (there is no token to send)', async () => {
    vi.stubGlobal('$fetch', vi.fn())
    const store = useRiderStore()
    await expect(store.updateTemplate('nope', { name: 'x' })).rejects.toThrow(/not loaded/)
    await expect(store.updateAttachment('nope', { technical: 'x' })).rejects.toThrow(/not loaded/)
    expect(calls()).toHaveLength(0)
  })

  it('reloadTemplate / reloadAttachment replace the cached copy and its token', async () => {
    const store = useRiderStore()
    store.templates = [tpl() as never]
    store.attachmentsByGigId = { 'gig-1': att() as never }

    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: tpl({ name: 'theirs', updatedAt: 'T9' }) }))
    await store.reloadTemplate('tmpl-1')
    expect(store.templates[0]).toMatchObject({ name: 'theirs', updatedAt: 'T9' })

    vi.stubGlobal('$fetch', vi.fn().mockResolvedValue({ data: att({ technical: 'theirs', updatedAt: 'T9' }) }))
    await store.reloadAttachment('att-1')
    expect(store.attachmentsByGigId['gig-1']).toMatchObject({ technical: 'theirs', updatedAt: 'T9' })
  })

  it('a 404 on reload means it was deleted elsewhere: drop it from the cache', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockRejectedValue(http(404)))
    const store = useRiderStore()
    store.templates = [tpl() as never, tpl({ id: 'tmpl-2' }) as never]
    store.attachmentsByGigId = { 'gig-1': att() as never }

    await store.reloadTemplate('tmpl-1')
    await store.reloadAttachment('att-1')

    expect(store.templates.map(t => t.id)).toEqual(['tmpl-2'])
    expect(store.attachmentsByGigId['gig-1']).toBeNull()
  })

  it('a reload failure other than 404 is surfaced, not swallowed', async () => {
    vi.stubGlobal('$fetch', vi.fn().mockRejectedValue(http(500)))
    const store = useRiderStore()
    store.templates = [tpl() as never]
    await expect(store.reloadTemplate('tmpl-1')).rejects.toMatchObject({ statusCode: 500 })
    expect(store.templates).toHaveLength(1)
  })
})
