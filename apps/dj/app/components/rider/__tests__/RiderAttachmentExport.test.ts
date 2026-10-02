import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { useRiderAutosave } from '../../../composables/useRiderAutosave'
import RiderAttachmentEditor from '../RiderAttachmentEditor.vue'
import RiderAttachmentCard from '../RiderAttachmentCard.vue'
import RiderPageHeader from '../RiderPageHeader.vue'
import { useRiderStore } from '../../../stores/rider'
import type { Gig } from '../../../types/gig'
import type { RiderAttachment } from '../../../types/rider'

// The PDF is rendered by the server from what it has STORED. Autosave is
// debounced, so Export must save pending edits first, and must not hand out
// a PDF that silently lacks edits it could not save.

const openPdf = vi.hoisted(() => vi.fn())
vi.mock('~/utils/openPdf', () => ({ openPdf }))

// Autosave queues are module-level, so a conflict left by one test would
// block saves in the next: every test works on its own attachment/gig ids.
let seq = 0
let ID = 'att-0'
let GIG = 'gig-0'
const attachment = (over: Partial<RiderAttachment> = {}): RiderAttachment => ({
  id: ID, gigId: GIG, templateId: null, technical: 'mine', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: 'T0', updatedAt: 'T1', ...over,
})
type Call = [string, { method?: string; body?: Record<string, unknown> }?]
const fetchMock = () => vi.mocked($fetch as unknown as (u: string, o?: Call[1]) => Promise<unknown>)
const calls = () => fetchMock().mock.calls.map(c => `${c[1]?.method ?? 'GET'} ${c[0]}`)
const http = (statusCode: number, data?: unknown) => Object.assign(new Error(`HTTP ${statusCode}`), { statusCode, data })

const Harness = defineComponent({
  setup() {
    const store = useRiderStore()
    return () => h(RiderAttachmentEditor, { attachment: store.attachmentsByGigId[GIG]! })
  },
})
const pdf = () => ({ id: ID, downloadUrl: 'https://garage.example/rider.pdf', createdAt: 'T9' })
const putUrl = () => `PUT /api/v1/rider/attachments/${ID}`
const postUrl = () => `POST /api/v1/rider/attachments/${ID}/pdf`

beforeEach(() => {
  seq += 1
  ID = `att-${seq}`
  GIG = `gig-${seq}`
  openPdf.mockClear()
  setActivePinia(createPinia())
  vi.stubGlobal('$fetch', vi.fn())
})
afterEach(async () => {
  vi.useRealTimers()
  // Forget this test's autosave queue (module-level state), or a conflict or
  // failure it left would colour the global status in every later test.
  await useRiderAutosave().cancel({ kind: 'attachment', id: ID })
  vi.unstubAllGlobals()
})

describe('RiderAttachmentEditor export', () => {
  it('saves edits typed in the last moments BEFORE asking the server for the PDF', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.attachmentsByGigId = { [GIG]: attachment() }
    const w = mount(Harness)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('typed 100ms ago')

    fetchMock()
      .mockResolvedValueOnce({ data: attachment({ technical: 'typed 100ms ago', updatedAt: 'T2' }) }) // the save
      .mockResolvedValueOnce(pdf())                                                                       // the export
    await w.find('[data-testid="rider-attachment-export"]').trigger('click')
    await flushPromises()

    expect(calls()).toEqual([putUrl(), postUrl()])
    expect(openPdf).toHaveBeenCalledWith('https://garage.example/rider.pdf')
    expect(w.find('[data-testid="rider-export-error"]').exists()).toBe(false)
  })

  it('with nothing pending it just exports', async () => {
    const store = useRiderStore()
    store.attachmentsByGigId = { [GIG]: attachment() }
    const w = mount(Harness)
    fetchMock().mockResolvedValueOnce(pdf())
    await w.find('[data-testid="rider-attachment-export"]').trigger('click')
    await flushPromises()
    expect(calls()).toEqual([postUrl()])
    expect(openPdf).toHaveBeenCalledTimes(1)
  })

  it('refuses to export when the pending edits cannot be saved (conflict), and says why', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.attachmentsByGigId = { [GIG]: attachment() }
    const w = mount(Harness)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('mine, edited')

    fetchMock().mockRejectedValueOnce(http(409)) // the save is stale
    await w.find('[data-testid="rider-attachment-export"]').trigger('click')
    await flushPromises()

    expect(calls()).toEqual([putUrl()]) // no POST: a stale PDF is not exported
    expect(openPdf).not.toHaveBeenCalled()
    expect(w.find('[data-testid="rider-export-error"]').text()).toContain('latest edits are not saved')
    expect(w.find('[data-testid="rider-conflict-notice"]').exists()).toBe(true)
  })

  it('refuses to export when the save is rejected (e.g. too long)', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.attachmentsByGigId = { [GIG]: attachment() }
    const w = mount(Harness)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('edit')
    fetchMock().mockRejectedValueOnce(http(422, { error: 'invalid rider input: technical is too long' }))

    await w.find('[data-testid="rider-attachment-export"]').trigger('click')
    await flushPromises()

    expect(calls()).toEqual([putUrl()])
    expect(openPdf).not.toHaveBeenCalled()
    expect(w.find('[data-testid="rider-export-error"]').exists()).toBe(true)
  })

  it('shows the server\'s reason when the export itself fails', async () => {
    const store = useRiderStore()
    store.attachmentsByGigId = { [GIG]: attachment() }
    const w = mount(Harness)
    fetchMock().mockRejectedValueOnce(http(404, { error: 'rider record not found: the gig for this rider no longer exists' }))
    await w.find('[data-testid="rider-attachment-export"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-testid="rider-export-error"]').text()).toContain('the gig for this rider no longer exists')
    expect(openPdf).not.toHaveBeenCalled()
    expect((w.find('[data-testid="rider-attachment-export"]').element as HTMLButtonElement).disabled).toBe(false)
  })
})

describe('RiderAttachmentCard export', () => {
  const gig = () => ({ id: GIG }) as Gig

  it('opens the PDF, and shows a failure instead of swallowing it', async () => {
    const store = useRiderStore()
    store.attachmentsByGigId = { [GIG]: attachment() }
    const w = mount(RiderAttachmentCard, { props: { gig: gig() } })
    await flushPromises()

    fetchMock().mockResolvedValueOnce(pdf())
    await w.find(`[data-testid="rider-export-pdf-${GIG}"]`).trigger('click')
    await flushPromises()
    expect(openPdf).toHaveBeenCalledWith('https://garage.example/rider.pdf')
    expect(w.emitted('exported')).toHaveLength(1)

    openPdf.mockClear()
    fetchMock().mockRejectedValueOnce(http(500))
    await w.find(`[data-testid="rider-export-pdf-${GIG}"]`).trigger('click')
    await flushPromises()
    expect(openPdf).not.toHaveBeenCalled()
    expect(w.find('[data-testid="rider-export-error"]').text()).toBe('Could not export the PDF.')
  })
})

describe('autosave failures explain themselves', () => {
  it('the header shows the API\'s reason next to SAVE FAILED', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.attachmentsByGigId = { [GIG]: attachment() }
    const w = mount({ components: { RiderPageHeader, Harness }, template: '<div><RiderPageHeader /><Harness /></div>' })
    await w.find('[data-testid="rider-technical-textarea"]').setValue('edit')

    fetchMock().mockRejectedValueOnce(http(422, { error: 'invalid rider input: technical is too long (20001 characters; the limit is 20000)' }))
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()

    const status = w.find('[data-testid="rider-save-status"]').text()
    expect(status).toContain('SAVE FAILED')
    expect(status).toContain('technical is too long')
  })

  it('every text area stops at the API limit', () => {
    const store = useRiderStore()
    store.attachmentsByGigId = { [GIG]: attachment() }
    const w = mount(Harness)
    for (const s of ['technical', 'hospitality', 'backline', 'otherNotes']) {
      expect(w.find(`[data-testid="rider-${s}-textarea"]`).attributes('maxlength')).toBe('20000')
    }
  })
})
