import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import RiderAttachmentEditor from '../RiderAttachmentEditor.vue'
import { useRiderStore } from '../../../stores/rider'
import type { RiderAttachment } from '../../../types/rider'

// Detaching deletes the attachment. Pending or in-flight autosaves must not
// reach the server after (or race) that DELETE: a PUT to a deleted record
// 404s and leaves a sticky "save failed", and a late response can resurrect
// the record in the store cache.

const attachment = (over: Partial<RiderAttachment> = {}): RiderAttachment => ({
  id: 'att-d', gigId: 'gig-d', templateId: null, technical: 'mine', hospitality: 'h', backline: 'b', otherNotes: 'o',
  createdAt: 'T0', updatedAt: 'T1', ...over,
})
type Call = [string, { method?: string; body?: Record<string, unknown> }?]
const fetchMock = () => vi.mocked($fetch as unknown as (u: string, o?: Call[1]) => Promise<unknown>)
const methods = () => fetchMock().mock.calls.map(c => c[1]?.method ?? 'GET')

const Harness = defineComponent({
  setup() {
    const store = useRiderStore()
    // Like the page: nothing to edit once the attachment is gone.
    return () => {
      const a = store.attachmentsByGigId['gig-d']
      return a ? h(RiderAttachmentEditor, { attachment: a }) : null
    }
  },
})
// A failed detach rethrows by design; capture it instead of letting Vue log it.
const surfaced: unknown[] = []
const capture = { global: { config: { errorHandler: (e: unknown) => { surfaced.push(e) } } } }

let navigateTo: ReturnType<typeof vi.fn>
beforeEach(() => {
  surfaced.length = 0
  setActivePinia(createPinia())
  vi.stubGlobal('$fetch', vi.fn())
  navigateTo = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigateTo', navigateTo)
  vi.stubGlobal('confirm', vi.fn().mockReturnValue(true))
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('RiderAttachmentEditor detach', () => {
  it('pending edits are dropped: only the DELETE is sent, never a PUT', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.attachmentsByGigId = { 'gig-d': attachment() }
    const w = mount(Harness)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('typed just before detaching')

    fetchMock().mockResolvedValueOnce(undefined) // DELETE
    await w.find('[data-testid="rider-attachment-detach"]').trigger('click')
    await flushPromises()
    w.unmount() // the page leaves; the unmount flush must stay quiet
    await vi.advanceTimersByTimeAsync(10_000)

    expect(methods()).toEqual(['DELETE'])
    expect(store.attachmentsByGigId['gig-d']).toBeNull()
    expect(navigateTo).toHaveBeenCalledWith('/rider')
  })

  it('an in-flight save finishes BEFORE the DELETE is sent', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.attachmentsByGigId = { 'gig-d': attachment() }
    const w = mount(Harness)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('saving')
    let finishPut!: (v: unknown) => void
    fetchMock().mockReturnValueOnce(new Promise((r) => { finishPut = r }))
    await vi.advanceTimersByTimeAsync(2000) // the autosave PUT is now in flight

    fetchMock().mockResolvedValueOnce(undefined) // DELETE
    await w.find('[data-testid="rider-attachment-detach"]').trigger('click')
    await flushPromises()
    expect(methods()).toEqual(['PUT']) // DELETE is waiting for it

    finishPut({ data: attachment({ technical: 'saving', updatedAt: 'T2' }) })
    await flushPromises()

    expect(methods()).toEqual(['PUT', 'DELETE'])
    expect(store.attachmentsByGigId['gig-d']).toBeNull() // the late PUT response did not resurrect it
  })

  it('declining the confirm does nothing and keeps autosave working', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false))
    const store = useRiderStore()
    store.attachmentsByGigId = { 'gig-d': attachment() }
    const w = mount(Harness)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('still editing')
    await w.find('[data-testid="rider-attachment-detach"]').trigger('click')
    fetchMock().mockResolvedValueOnce({ data: attachment({ technical: 'still editing', updatedAt: 'T2' }) })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()

    expect(methods()).toEqual(['PUT'])
    expect(navigateTo).not.toHaveBeenCalled()
  })

  it('if the DELETE fails the edits are queued again, not lost', async () => {
    vi.useFakeTimers()
    const store = useRiderStore()
    store.attachmentsByGigId = { 'gig-d': attachment() }
    const w = mount(Harness, capture)
    await w.find('[data-testid="rider-technical-textarea"]').setValue('do not lose me')

    fetchMock().mockRejectedValueOnce(Object.assign(new Error('offline'), { statusCode: 503 })) // DELETE fails
    await w.find('[data-testid="rider-attachment-detach"]').trigger('click')
    await flushPromises()
    expect(navigateTo).not.toHaveBeenCalled()
    expect(store.attachmentsByGigId['gig-d']).not.toBeNull()
    expect(surfaced).toHaveLength(1) // the failure is reported, not swallowed

    fetchMock().mockResolvedValueOnce({ data: attachment({ technical: 'do not lose me', updatedAt: 'T2' }) })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()

    expect(methods()).toEqual(['DELETE', 'PUT'])
    const put = fetchMock().mock.calls[1]![1]!
    expect(put.body).toMatchObject({ technical: 'do not lose me', updatedAt: 'T1' })
  })
})
