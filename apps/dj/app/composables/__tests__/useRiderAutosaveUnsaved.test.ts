import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { hasAnyUnsavedRiderEdits, useRiderAutosave } from '../useRiderAutosave'
import { useRiderStore } from '../../stores/rider'
import type { RiderTemplate } from '../../types/rider'

// Separate file so the module-level queues start empty: this asserts a
// global ("is anything unsaved?"), which other tests' leftovers would colour.

const tpl = (id: string): RiderTemplate => ({
  id, name: 'n', technical: '', hospitality: '', backline: '', otherNotes: '', createdAt: 'T0', updatedAt: 'T0',
})
const fetchMock = () => vi.mocked($fetch as unknown as (u: string, o?: unknown) => Promise<unknown>)

beforeEach(() => {
  setActivePinia(createPinia())
  vi.stubGlobal('$fetch', vi.fn())
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('hasAnyUnsavedRiderEdits', () => {
  it('walks through the lifecycle: clean -> queued -> in flight -> saved', async () => {
    const store = useRiderStore()
    store.templates = [tpl('u1')]
    const { scheduleSave, flush } = useRiderAutosave()
    expect(hasAnyUnsavedRiderEdits()).toBe(false)

    scheduleSave({ kind: 'template', id: 'u1' }, { technical: 'x' })
    expect(hasAnyUnsavedRiderEdits()).toBe(true) // queued behind the debounce

    let finish!: (v: unknown) => void
    fetchMock().mockReturnValueOnce(new Promise((r) => { finish = r }))
    const done = flush({ kind: 'template', id: 'u1' })
    await vi.waitFor(() => expect(fetchMock()).toHaveBeenCalledTimes(1))
    expect(hasAnyUnsavedRiderEdits()).toBe(true) // in flight

    finish({ data: { ...tpl('u1'), technical: 'x', updatedAt: 'T1' } })
    await done
    expect(hasAnyUnsavedRiderEdits()).toBe(false)
  })

  it('stays true after a failed save and after a conflict, until resolved', async () => {
    const store = useRiderStore()
    store.templates = [tpl('u2'), tpl('u3')]
    const { scheduleSave, flush, discard } = useRiderAutosave()

    fetchMock().mockRejectedValueOnce(Object.assign(new Error('offline'), { statusCode: 503 }))
    scheduleSave({ kind: 'template', id: 'u2' }, { technical: 'x' })
    await flush({ kind: 'template', id: 'u2' })
    expect(hasAnyUnsavedRiderEdits()).toBe(true) // kept for retry

    fetchMock().mockResolvedValueOnce({ data: tpl('u2') })
    scheduleSave({ kind: 'template', id: 'u2' }, { technical: 'y' })
    await flush({ kind: 'template', id: 'u2' })
    expect(hasAnyUnsavedRiderEdits()).toBe(false)

    fetchMock().mockRejectedValueOnce(Object.assign(new Error('409'), { statusCode: 409 }))
    scheduleSave({ kind: 'template', id: 'u3' }, { technical: 'z' })
    await flush({ kind: 'template', id: 'u3' })
    expect(hasAnyUnsavedRiderEdits()).toBe(true) // a conflict blocks the save

    fetchMock().mockResolvedValueOnce({ data: tpl('u3') })
    await discard({ kind: 'template', id: 'u3' })
    expect(hasAnyUnsavedRiderEdits()).toBe(false)
  })

  it('is false again once a target is cancelled', async () => {
    const store = useRiderStore()
    store.templates = [tpl('u4')]
    const { scheduleSave, cancel } = useRiderAutosave()
    scheduleSave({ kind: 'template', id: 'u4' }, { technical: 'x' })
    expect(hasAnyUnsavedRiderEdits()).toBe(true)
    await cancel({ kind: 'template', id: 'u4' })
    expect(hasAnyUnsavedRiderEdits()).toBe(false)
  })
})
