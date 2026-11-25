import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useEpkStore } from '../../stores/epk'
import { useEpkAutosave } from '../useEpkAutosave'

// The server only knows what was successfully saved.
const server: Record<string, unknown> = {}
let failNext = false

describe('useEpkAutosave', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.assign(server, { id: 'epk-1', bioShort: '', bioLong: 'old bio', techRider: 'old rider' })
    failNext = false
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn(async (_url: string, opts: { body: Record<string, unknown> }) => {
      if (failNext) {
        failNext = false
        throw new Error('offline')
      }
      Object.assign(server, opts.body)
      return { data: { ...server } }
    })
  })
  afterEach(() => vi.restoreAllMocks())

  it('keeps a failed edit and sends it with the next one', async () => {
    const store = useEpkStore()
    const { scheduleSave, flush } = useEpkAutosave()

    store.bioLong = 'typed bio'
    scheduleSave({ bioLong: 'typed bio' })
    failNext = true
    await flush()
    expect(store.saveStatus).toBe('error')
    expect(store.bioLong).toBe('typed bio')

    store.techRider = 'new rider'
    scheduleSave({ techRider: 'new rider' })
    await flush()
    expect(server).toMatchObject({ bioLong: 'typed bio', techRider: 'new rider' })
    expect(store.bioLong).toBe('typed bio')
    expect(store.saveStatus).toBe('saved')
  })

  it('does not overwrite text typed while a save is in flight', async () => {
    const store = useEpkStore()
    let respond!: (value: unknown) => void
    // @ts-expect-error - mocking global $fetch
    global.$fetch = vi.fn(() => new Promise(resolve => { respond = resolve }))

    store.bioLong = 'first'
    const saving = store.updateContent({ bioLong: 'first' })
    store.bioLong = 'first and more'
    respond({ data: { bioLong: 'first', techRider: 'stale' } })
    await saving

    expect(store.bioLong).toBe('first and more')
    expect(store.techRider).toBe('')
  })
})
