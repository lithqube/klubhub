import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import RiderConflictNotice from '../RiderConflictNotice.vue'
import RiderTemplateEditor from '../RiderTemplateEditor.vue'
import RiderAttachmentEditor from '../RiderAttachmentEditor.vue'
import RiderPageHeader from '../RiderPageHeader.vue'
import { useRiderStore } from '../../../stores/rider'
import type { RiderAttachment, RiderTemplate } from '../../../types/rider'

// Real Pinia store + stubbed transport: a save is rejected as stale, the user
// is told, and only an explicit reload replaces what they were editing.

const template = (over: Partial<RiderTemplate> = {}): RiderTemplate => ({
  id: 'tmpl-ui', name: 'Standard club', technical: 'mine', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: 'T0', updatedAt: 'T1', ...over,
})
const attachment = (over: Partial<RiderAttachment> = {}): RiderAttachment => ({
  id: 'att-ui', gigId: 'gig-ui', templateId: null, technical: 'mine', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: 'T0', updatedAt: 'T1', ...over,
})
const http = (statusCode: number) => Object.assign(new Error(`HTTP ${statusCode}`), { statusCode })
const fetchMock = () => vi.mocked($fetch as unknown as (u: string, o?: { method?: string }) => Promise<unknown>)

beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('$fetch', vi.fn())
})
afterEach(() => { vi.unstubAllGlobals() })

describe('RiderConflictNotice', () => {
  it('is hidden until its own target is in conflict', async () => {
    const store = useRiderStore()
    const w = mount(RiderConflictNotice, { props: { kind: 'template', id: 'tmpl-ui' } })
    expect(w.find('[data-testid="rider-conflict-notice"]').exists()).toBe(false)

    store.markConflict('attachment:something-else')
    await w.vm.$nextTick()
    expect(w.find('[data-testid="rider-conflict-notice"]').exists()).toBe(false)

    store.markConflict('template:tmpl-ui')
    await w.vm.$nextTick()
    const notice = w.find('[data-testid="rider-conflict-notice"]')
    expect(notice.exists()).toBe(true)
    expect(notice.attributes('role')).toBe('alert')
    expect(notice.text()).toContain('not saved')
    expect(notice.text()).toContain('discards your unsaved edits')
  })

  it('the button reloads the latest copy and clears the conflict', async () => {
    const store = useRiderStore()
    store.templates = [template()]
    store.markConflict('template:tmpl-ui')
    fetchMock().mockResolvedValueOnce({ data: template({ technical: 'theirs', updatedAt: 'T9' }) })
    const w = mount(RiderConflictNotice, { props: { kind: 'template', id: 'tmpl-ui' } })

    await w.find('[data-testid="rider-conflict-reload"]').trigger('click')
    await flushPromises()

    expect(store.templates[0]).toMatchObject({ technical: 'theirs', updatedAt: 'T9' })
    expect(store.conflicts).toEqual([])
    expect(w.find('[data-testid="rider-conflict-notice"]').exists()).toBe(false)
  })

  it('stays up with a message if the reload fails, so the user can retry', async () => {
    const store = useRiderStore()
    store.templates = [template()]
    store.markConflict('template:tmpl-ui')
    fetchMock().mockRejectedValueOnce(http(500))
    const w = mount(RiderConflictNotice, { props: { kind: 'template', id: 'tmpl-ui' } })

    await w.find('[data-testid="rider-conflict-reload"]').trigger('click')
    await flushPromises()

    expect(w.find('[data-testid="rider-conflict-error"]').exists()).toBe(true)
    expect(store.conflicts).toEqual(['template:tmpl-ui'])
    expect((w.find('[data-testid="rider-conflict-reload"]').element as HTMLButtonElement).disabled).toBe(false)
  })
})

// The pages hand each editor the store's current record, so the prop is
// replaced after every save/reload. Mount through the same wiring.
const TemplateHarness = defineComponent({
  setup() {
    const store = useRiderStore()
    return () => h(RiderTemplateEditor, { template: store.templates[0]! })
  },
})
const AttachmentHarness = defineComponent({
  setup() {
    const store = useRiderStore()
    return () => h(RiderAttachmentEditor, { attachment: store.attachmentsByGigId['gig-ui']! })
  },
})

describe('editors on a stale save', () => {
  it('template editor: typing -> 409 -> notice, then reload reseeds the text from the server', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.templates = [template()]
    const w = mount(TemplateHarness)
    const input = () => w.find('[data-testid="rider-technical-textarea"]')
    const field = () => input().element as HTMLTextAreaElement

    expect(field().value).toBe('mine')
    await input().setValue('mine, edited')
    fetchMock().mockRejectedValueOnce(http(409))
    await vi.advanceTimersByTimeAsync(2000) // autosave fires, server says stale
    await flushPromises()

    expect(w.find('[data-testid="rider-conflict-notice"]').exists()).toBe(true)
    expect(field().value).toBe('mine, edited') // nothing is overwritten behind the user's back

    fetchMock().mockResolvedValueOnce({ data: template({ technical: 'theirs', updatedAt: 'T9' }) })
    await w.find('[data-testid="rider-conflict-reload"]').trigger('click')
    await flushPromises()

    expect(w.find('[data-testid="rider-conflict-notice"]').exists()).toBe(false)
    expect(field().value).toBe('theirs')
    vi.useRealTimers()
  })

  it('does NOT reseed from the store when its own save returns (text typed during the request survives)', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.templates = [template()]
    const w = mount(TemplateHarness)
    const input = () => w.find('[data-testid="rider-technical-textarea"]')
    const field = () => input().element as HTMLTextAreaElement

    await input().setValue('first')
    let respond!: (v: unknown) => void
    fetchMock().mockReturnValueOnce(new Promise((r) => { respond = r }))
    await vi.advanceTimersByTimeAsync(2000) // the save for "first" is now in flight

    await input().setValue('first, then more') // typed while the request is out
    respond({ data: template({ technical: 'first', updatedAt: 'T2' }) })
    await flushPromises()

    expect(store.templates[0]?.updatedAt).toBe('T2') // the parent was handed the new record
    expect(field().value).toBe('first, then more')
    vi.useRealTimers()
  })

  it('attachment editor: shows the notice for its own attachment and reseeds on reload', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.attachmentsByGigId = { 'gig-ui': attachment() }
    const w = mount(AttachmentHarness)
    const input = () => w.find('[data-testid="rider-technical-textarea"]')
    const field = () => input().element as HTMLTextAreaElement

    await input().setValue('mine, edited')
    fetchMock().mockRejectedValueOnce(http(409))
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(w.find('[data-testid="rider-conflict-notice"]').exists()).toBe(true)

    fetchMock().mockResolvedValueOnce({ data: attachment({ technical: 'theirs', updatedAt: 'T9' }) })
    await w.find('[data-testid="rider-conflict-reload"]').trigger('click')
    await flushPromises()

    expect(field().value).toBe('theirs')
    vi.useRealTimers()
  })

  it('attachment editor: text typed while its own save is in flight survives the response', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.attachmentsByGigId = { 'gig-ui': attachment() }
    const w = mount(AttachmentHarness)
    const input = () => w.find('[data-testid="rider-technical-textarea"]')

    await input().setValue('first')
    let respond!: (v: unknown) => void
    fetchMock().mockReturnValueOnce(new Promise((r) => { respond = r }))
    await vi.advanceTimersByTimeAsync(2000)
    await input().setValue('first, then more')
    respond({ data: attachment({ technical: 'first', updatedAt: 'T2' }) })
    await flushPromises()

    expect((input().element as HTMLTextAreaElement).value).toBe('first, then more')
    vi.useRealTimers()
  })
})

describe('RiderPageHeader', () => {
  it('says CHANGED ELSEWHERE for a conflict (not SAVE FAILED) and announces status changes', async () => {
    const store = useRiderStore()
    const w = mount(RiderPageHeader)
    store.saveStatus = 'conflict'
    await w.vm.$nextTick()
    const status = w.find('[data-testid="rider-save-status"]')
    expect(status.text()).toBe('CHANGED ELSEWHERE')
    expect(status.attributes('role')).toBe('status')
    expect(status.attributes('aria-live')).toBe('polite')

    store.saveStatus = 'error'
    await w.vm.$nextTick()
    expect(status.text()).toBe('SAVE FAILED')
  })
})
