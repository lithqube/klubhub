import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useRiderStore } from '../rider'

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