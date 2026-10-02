import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useRiderAutosave } from '../useRiderAutosave'
import { useRiderStore } from '../../stores/rider'
import { RiderConflictError } from '../../types/rider'
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
const puts = () => fetchMock().mock.calls.filter(c => c[1]?.method === 'PUT')
// The patch each PUT carried, without the concurrency token (asserted separately).
const bodies = () => puts().map((c) => {
  const { updatedAt: _token, ...patch } = c[1].body as Record<string, unknown>
  return patch
})
// The updatedAt token each PUT carried.
const tokens = () => puts().map(c => (c[1].body as Record<string, unknown>).updatedAt)

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
    // Each save carries the token the previous response returned.
    expect(tokens()).toEqual(['T0', 'T1'])
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


// ─── Conflicts (the server rejected a stale updatedAt with 409) ─────────────

const http409 = () => Object.assign(new Error('409 Conflict'), { statusCode: 409 })

describe('useRiderAutosave: conflicts', () => {
  it('on 409 keeps the edits, flags the target and reports a conflict instead of an error', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-c1')]
    const { scheduleSave, flush } = useRiderAutosave()
    fetchMock().mockRejectedValueOnce(http409())

    scheduleSave({ kind: 'template', id: 't-c1' }, { technical: 'mine' })
    await flush({ kind: 'template', id: 't-c1' })

    expect(store.saveStatus).toBe('conflict')
    expect(store.conflicts).toEqual(['template:t-c1'])

    // Clean up the shared module-level queue.
    fetchMock().mockResolvedValue({ data: tmpl('t-c1', { updatedAt: 'T9' }) })
    await useRiderAutosave().discard({ kind: 'template', id: 't-c1' })
  })

  it('does not resend while a conflict is unresolved, however much more is typed', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-c2')]
    const { scheduleSave, flush } = useRiderAutosave()
    fetchMock().mockRejectedValueOnce(http409())

    scheduleSave({ kind: 'template', id: 't-c2' }, { technical: 'first' })
    await flush({ kind: 'template', id: 't-c2' })
    expect(fetchMock()).toHaveBeenCalledTimes(1)

    vi.useFakeTimers()
    scheduleSave({ kind: 'template', id: 't-c2' }, { technical: 'second' })
    await vi.advanceTimersByTimeAsync(10_000) // far past the debounce
    await flush({ kind: 'template', id: 't-c2' })
    expect(fetchMock()).toHaveBeenCalledTimes(1)
    expect(store.saveStatus).toBe('conflict')

    vi.useRealTimers()
    fetchMock().mockResolvedValue({ data: tmpl('t-c2', { updatedAt: 'T9' }) })
    await useRiderAutosave().discard({ kind: 'template', id: 't-c2' })
  })

  it('discard() reloads the latest copy, drops the unsaved edits, clears the conflict and signals editors', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-c3', { technical: 'old' })]
    const { scheduleSave, flush, discard } = useRiderAutosave()
    fetchMock().mockRejectedValueOnce(http409())
    scheduleSave({ kind: 'template', id: 't-c3' }, { technical: 'mine' })
    await flush({ kind: 'template', id: 't-c3' })
    expect(store.conflicts).toHaveLength(1)

    const before = store.reloadVersion
    fetchMock().mockResolvedValueOnce({ data: tmpl('t-c3', { technical: 'theirs', updatedAt: 'T5' }) })
    await discard({ kind: 'template', id: 't-c3' })

    expect(fetchMock().mock.calls.at(-1)?.[0]).toBe('/api/v1/rider/templates/t-c3') // the GET
    expect(store.templates[0]).toMatchObject({ technical: 'theirs', updatedAt: 'T5' })
    expect(store.conflicts).toEqual([])
    expect(store.reloadVersion).toBe(before + 1)
    expect(store.saveStatus).toBe('saved')

    // The next save uses the reloaded token and does not resurrect the dropped edit.
    fetchMock().mockResolvedValueOnce({ data: tmpl('t-c3', { updatedAt: 'T6' }) })
    scheduleSave({ kind: 'template', id: 't-c3' }, { hospitality: 'after reload' })
    await flush({ kind: 'template', id: 't-c3' })
    expect(bodies().at(-1)).toEqual({ hospitality: 'after reload' })
    expect(tokens().at(-1)).toBe('T5')
  })

  it('discard() keeps the conflict (and the edits) if the reload itself fails', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-c4')]
    const { scheduleSave, flush, discard } = useRiderAutosave()
    fetchMock().mockRejectedValueOnce(http409())
    scheduleSave({ kind: 'template', id: 't-c4' }, { technical: 'mine' })
    await flush({ kind: 'template', id: 't-c4' })

    fetchMock().mockRejectedValueOnce(new Error('offline'))
    await expect(discard({ kind: 'template', id: 't-c4' })).rejects.toThrow('offline')
    expect(store.conflicts).toEqual(['template:t-c4'])
    expect(store.saveStatus).toBe('conflict')

    fetchMock().mockResolvedValueOnce({ data: tmpl('t-c4', { updatedAt: 'T7' }) })
    await discard({ kind: 'template', id: 't-c4' })
    expect(store.conflicts).toEqual([])
  })

  it('a 409 on one target does not block another target from saving', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-c5'), tmpl('t-c6')]
    const { scheduleSave, flush, discard } = useRiderAutosave()
    fetchMock().mockRejectedValueOnce(http409()).mockResolvedValueOnce({ data: tmpl('t-c6', { updatedAt: 'T1' }) })

    scheduleSave({ kind: 'template', id: 't-c5' }, { name: 'a' })
    await flush({ kind: 'template', id: 't-c5' })
    scheduleSave({ kind: 'template', id: 't-c6' }, { name: 'b' })
    await flush({ kind: 'template', id: 't-c6' })

    expect(store.templates.find(t => t.id === 't-c6')?.updatedAt).toBe('T1')
    expect(store.saveStatus).toBe('conflict') // t-c5 still needs the user

    fetchMock().mockResolvedValueOnce({ data: tmpl('t-c5', { updatedAt: 'T2' }) })
    await discard({ kind: 'template', id: 't-c5' })
  })

  it('RiderConflictError is what the store throws for a 409', async () => {
    const store = useRiderStore()
    store.templates = [tmpl('t-c7')]
    fetchMock().mockRejectedValueOnce(http409())
    await expect(store.updateTemplate('t-c7', { name: 'x' })).rejects.toBeInstanceOf(RiderConflictError)
  })
})
