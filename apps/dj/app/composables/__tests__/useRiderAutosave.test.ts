import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useRiderAutosave } from '../useRiderAutosave'
import { useRiderStore } from '../../stores/rider'
import type { RiderTemplate } from '../../types/rider'

// Real Pinia store + stubbed transport. The autosave queues are module-level,
// so every test uses its own template ids and leaves its queues clean.

const tmpl = (id: string, over: Partial<RiderTemplate> = {}): RiderTemplate => ({
  id, name: 'n', technical: '', hospitality: '', backline: '', otherNotes: '',
  createdAt: 'T0', updatedAt: 'T0', ...over,
})
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((a, b) => { resolve = a; reject = b })
  return { promise, resolve, reject }
}
type Call = [string, { method?: string; body?: Record<string, unknown> }]
const fetchMock = () => vi.mocked($fetch as unknown as (url: string, o: Call[1]) => Promise<unknown>)
const bodies = () => fetchMock().mock.calls.map(c => c[1].body)

beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('$fetch', vi.fn())
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('useRiderAutosave', () => {
  it('sends saves for one target strictly one after another, so a stale response cannot land last', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-order')]
    const { scheduleSave, flush } = useRiderAutosave()
    const first = deferred<unknown>()
    const second = deferred<unknown>()
    fetchMock().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)

    scheduleSave({ kind: 'template', id: 't-order' }, { technical: 'one' })
    const f1 = flush({ kind: 'template', id: 't-order' })
    await vi.waitFor(() => expect(fetchMock()).toHaveBeenCalledTimes(1))
    // Typed while the first PUT is still in flight:
    scheduleSave({ kind: 'template', id: 't-order' }, { technical: 'two' })
    const f2 = flush({ kind: 'template', id: 't-order' })
    await new Promise(r => setTimeout(r, 0))
    expect(fetchMock()).toHaveBeenCalledTimes(1) // the second waits its turn

    first.resolve({ data: tmpl('t-order', { technical: 'one', updatedAt: 'T1' }) })
    await f1
    expect(fetchMock()).toHaveBeenCalledTimes(2)
    second.resolve({ data: tmpl('t-order', { technical: 'two', updatedAt: 'T2' }) })
    await f2

    expect(bodies()).toEqual([{ technical: 'one' }, { technical: 'two' }])
    expect(store.templates[0]?.technical).toBe('two')
  })

  it('coalesces edits made while a save is in flight into the next save', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-coalesce')]
    const { scheduleSave, flush } = useRiderAutosave()
    const first = deferred<unknown>()
    fetchMock().mockReturnValueOnce(first.promise).mockResolvedValueOnce({ data: tmpl('t-coalesce') })

    scheduleSave({ kind: 'template', id: 't-coalesce' }, { name: 'A' })
    const f1 = flush({ kind: 'template', id: 't-coalesce' })
    await vi.waitFor(() => expect(fetchMock()).toHaveBeenCalledTimes(1))
    scheduleSave({ kind: 'template', id: 't-coalesce' }, { technical: 'x' })
    scheduleSave({ kind: 'template', id: 't-coalesce' }, { hospitality: 'y' })
    const f2 = flush({ kind: 'template', id: 't-coalesce' })
    first.resolve({ data: tmpl('t-coalesce') })
    await Promise.all([f1, f2])

    expect(bodies()).toEqual([{ name: 'A' }, { technical: 'x', hospitality: 'y' }])
  })

  it('keeps a failed edit and resends it with the next save instead of dropping it', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-retry')]
    const { scheduleSave, flush } = useRiderAutosave()
    fetchMock().mockRejectedValueOnce(new Error('422')).mockResolvedValueOnce({ data: tmpl('t-retry') })

    scheduleSave({ kind: 'template', id: 't-retry' }, { name: 'New name' })
    await flush({ kind: 'template', id: 't-retry' })
    expect(store.saveStatus).toBe('error')

    scheduleSave({ kind: 'template', id: 't-retry' }, { technical: 'later edit' })
    await flush({ kind: 'template', id: 't-retry' })

    expect(bodies()).toEqual([{ name: 'New name' }, { name: 'New name', technical: 'later edit' }])
    expect(store.saveStatus).toBe('saved')
  })

  it('a newer edit to the same field wins over the failed one', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-newer')]
    const { scheduleSave, flush } = useRiderAutosave()
    fetchMock().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce({ data: tmpl('t-newer') })

    scheduleSave({ kind: 'template', id: 't-newer' }, { technical: 'old' })
    await flush({ kind: 'template', id: 't-newer' })
    scheduleSave({ kind: 'template', id: 't-newer' }, { technical: 'new' })
    await flush({ kind: 'template', id: 't-newer' })

    expect(bodies()[1]).toEqual({ technical: 'new' })
  })

  it('does not report "saved" while another target failed or edits are still waiting', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-bad'), tmpl('t-good')]
    const { scheduleSave, flush, flushAll } = useRiderAutosave()
    fetchMock()
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValueOnce({ data: tmpl('t-good') })

    scheduleSave({ kind: 'template', id: 't-bad' }, { name: 'x' })
    await flush({ kind: 'template', id: 't-bad' })
    scheduleSave({ kind: 'template', id: 't-good' }, { name: 'y' })
    await flush({ kind: 'template', id: 't-good' })
    // t-good succeeded, but t-bad's edit is still unsaved.
    expect(store.saveStatus).toBe('error')

    // A pending (timer-armed) edit reads as saving, not saved.
    vi.useFakeTimers()
    scheduleSave({ kind: 'template', id: 't-good' }, { name: 'z' })
    expect(store.saveStatus).toBe('saving')

    // Leave the shared module-level queues clean for the next test.
    fetchMock().mockResolvedValue({ data: tmpl('t-bad') })
    await flushAll()
    expect(store.saveStatus).toBe('saved')
  })

  it('an empty flush does not claim anything was saved', async () => {
    const store = useRiderStore()
    const { scheduleSave, flush } = useRiderAutosave()
    scheduleSave({ kind: 'template', id: 't-empty' }, {})
    store.saveStatus = 'idle'
    await flush({ kind: 'template', id: 't-empty' })
    expect(fetchMock()).not.toHaveBeenCalled()
    expect(store.saveStatus).toBe('idle')
  })
})
