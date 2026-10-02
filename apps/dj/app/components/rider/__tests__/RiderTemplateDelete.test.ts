import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import RiderTemplateEditor from '../RiderTemplateEditor.vue'
import RiderAttachmentInlineCard from '../RiderAttachmentInlineCard.vue'
import { useRiderStore } from '../../../stores/rider'
import type { Gig } from '../../../types/gig'
import type { RiderAttachment, RiderTemplate } from '../../../types/rider'

// Deleting a template: pending and in-flight autosaves must not race the
// DELETE; a failed delete must not lose the user's text; and the cached
// attachments must stop pointing at the deleted template.

const tpl = (over: Partial<RiderTemplate> = {}): RiderTemplate => ({
  id: 'tmpl-del', name: 'Club', technical: 'mine', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: 'T0', updatedAt: 'T1', ...over,
})
type Call = [string, { method?: string; body?: Record<string, unknown> }?]
const fetchMock = () => vi.mocked($fetch as unknown as (u: string, o?: Call[1]) => Promise<unknown>)
const methods = () => fetchMock().mock.calls.map(c => c[1]?.method ?? 'GET')
const http = (statusCode: number, data?: unknown) => Object.assign(new Error(`HTTP ${statusCode}`), { statusCode, data })

// The page hands the editor the store's current record.
const Harness = defineComponent({
  setup() {
    const store = useRiderStore()
    return () => {
      const t = store.templates[0]
      return t ? h(RiderTemplateEditor, { template: t }) : null
    }
  },
})
const capture: unknown[] = []
const mountOpts = { global: { config: { errorHandler: (e: unknown) => { capture.push(e) } } } }

beforeEach(() => {
  capture.length = 0
  setActivePinia(createPinia())
  vi.stubGlobal('$fetch', vi.fn())
  vi.stubGlobal('confirm', vi.fn().mockReturnValue(true))
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('RiderTemplateEditor delete', () => {
  it('has a labelled name field and a delete button', () => {
    const store = useRiderStore()
    store.templates = [tpl()]
    const w = mount(Harness)
    const input = w.find('[data-testid="rider-template-name-input"]')
    expect(w.find(`label[for="${input.attributes('id')}"]`).text()).toBe('TEMPLATE NAME')
    expect(w.find('[data-testid="rider-template-delete"]').exists()).toBe(true)
  })

  it('asks first, naming the template and reassuring about existing riders', async () => {
    const store = useRiderStore()
    store.templates = [tpl()]
    const w = mount(Harness)
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false))
    await w.find('[data-testid="rider-template-delete"]').trigger('click')
    const msg = vi.mocked(confirm).mock.calls[0]![0] as string
    expect(msg).toContain('"Club"')
    expect(msg).toContain('keep their own copy')
    expect(fetchMock()).not.toHaveBeenCalled()
    expect(store.templates).toHaveLength(1)
  })

  it('pending edits are dropped: only the DELETE is sent, never a PUT', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.templates = [tpl()]
    const w = mount(Harness)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('typed just before deleting')

    fetchMock().mockResolvedValueOnce(undefined)
    await w.find('[data-testid="rider-template-delete"]').trigger('click')
    await flushPromises()
    w.unmount()
    await vi.advanceTimersByTimeAsync(10_000)

    expect(methods()).toEqual(['DELETE'])
    expect(store.templates).toEqual([])
  })

  it('an in-flight save finishes BEFORE the DELETE is sent', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.templates = [tpl()]
    const w = mount(Harness)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('saving')
    let finishPut!: (v: unknown) => void
    fetchMock().mockReturnValueOnce(new Promise((r) => { finishPut = r }))
    await vi.advanceTimersByTimeAsync(2000)

    fetchMock().mockResolvedValueOnce(undefined)
    await w.find('[data-testid="rider-template-delete"]').trigger('click')
    await flushPromises()
    expect(methods()).toEqual(['PUT'])

    finishPut({ data: tpl({ technical: 'saving', updatedAt: 'T2' }) })
    await flushPromises()
    expect(methods()).toEqual(['PUT', 'DELETE'])
    expect(store.templates).toEqual([]) // the late PUT response did not bring it back
  })

  it('if the DELETE fails it says why and the edits are queued again, not lost', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.templates = [tpl()]
    const w = mount(Harness, mountOpts)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('do not lose me')

    fetchMock().mockRejectedValueOnce(http(500, { error: 'internal_error', message: 'an internal error occurred' }))
    await w.find('[data-testid="rider-template-delete"]').trigger('click')
    await flushPromises()

    expect(w.find('[data-testid="rider-template-delete-error"]').text()).toBe('an internal error occurred')
    expect(store.templates).toHaveLength(1)
    expect((w.find('[data-testid="rider-template-delete"]').element as HTMLButtonElement).disabled).toBe(false)

    fetchMock().mockResolvedValueOnce({ data: tpl({ technical: 'do not lose me', updatedAt: 'T2' }) })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(methods()).toEqual(['DELETE', 'PUT'])
    expect(fetchMock().mock.calls[1]![1]!.body).toMatchObject({ technical: 'do not lose me', updatedAt: 'T1' })
  })
})

describe('store.deleteTemplate', () => {
  const att = (over: Partial<RiderAttachment> = {}): RiderAttachment => ({
    id: 'a1', gigId: 'g1', templateId: 'tmpl-del', technical: 'keep', hospitality: '', backline: '', otherNotes: '',
    createdAt: 'T0', updatedAt: 'T1', ...over,
  })

  it('clears the template reference on cached attachments, leaving text and token alone', async () => {
    const store = useRiderStore()
    store.templates = [tpl(), tpl({ id: 'other', name: 'Other' })]
    store.attachmentsByGigId = {
      g1: att(),
      g2: att({ id: 'a2', gigId: 'g2', templateId: 'other' }),
      g3: null,
    }
    fetchMock().mockResolvedValueOnce(undefined)

    await store.deleteTemplate('tmpl-del')

    expect(store.attachmentsByGigId.g1).toMatchObject({ templateId: null, technical: 'keep', updatedAt: 'T1' })
    expect(store.attachmentsByGigId.g2?.templateId).toBe('other')
    expect(store.attachmentsByGigId.g3).toBeNull()
    expect(store.templates.map(t => t.id)).toEqual(['other'])
  })

  it('changes nothing if the DELETE fails', async () => {
    const store = useRiderStore()
    store.templates = [tpl()]
    store.attachmentsByGigId = { g1: att() }
    fetchMock().mockRejectedValueOnce(http(500))
    await expect(store.deleteTemplate('tmpl-del')).rejects.toBeTruthy()
    expect(store.templates).toHaveLength(1)
    expect(store.attachmentsByGigId.g1?.templateId).toBe('tmpl-del')
  })
})

describe('RiderAttachmentInlineCard (inside the gig form)', () => {
  it('opens the editor in a NEW tab, so the gig dialog keeps its unsaved edits', async () => {
    const navigateTo = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigateTo', navigateTo)
    const store = useRiderStore()
    const gig = { id: 'g1' } as Gig
    store.attachmentsByGigId = { g1: { id: 'a1', gigId: 'g1', templateId: null, technical: 't', hospitality: '', backline: '', otherNotes: '', createdAt: 'T0', updatedAt: 'T1' } }
    const w = mount(RiderAttachmentInlineCard, { props: { gig } })
    await flushPromises()

    await w.find('[data-testid="rider-open-editor-g1"]').trigger('click')

    expect(navigateTo).toHaveBeenCalledTimes(1)
    expect(navigateTo).toHaveBeenCalledWith({ path: '/rider', query: { gig: 'g1' } }, { open: { target: '_blank' } })
  })
})
