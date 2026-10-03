import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import GigFormDialog from '../GigFormDialog.vue'
import { useGigStore } from '../../../stores/gig'
import type { Gig } from '../../../types/gig'

const t0 = '2026-09-01T00:00:00.123456Z'
const t1 = '2026-09-01T00:00:01.234567Z'
const t2 = '2026-09-01T00:00:02.345678Z'
const original: Gig = {
  id: 'gig-1', date: '2026-10-01T00:00:00Z', venue: 'Original venue', city: 'Berlin', country: 'DE',
  event_name: 'Event', promoter_name: 'Promoter', promoter_email: '', promoter_phone: '',
  fee_amount: 500, fee_currency: 'EUR', set_length_minutes: 90, notes: 'Original notes',
  status: 'confirmed', payment_status: 'unpaid', gig_reader_venue_id: null, gig_reader_contact_id: null,
  created_at: t0, updated_at: t0, deleted_at: null,
}
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const stubs = {
  teleport: true, transition: false, EntryReconciliationDialog: true,
  VenueAutocomplete: { props: ['modelValue'], template: '<input data-testid="venue" :value="modelValue?.name" @input="$emit(\'venue-selected\', {name: $event.target.value})" />' },
  ContactAutocomplete: true,
}

function setup() {
  const fresh = { ...original, venue: 'Other venue', notes: 'Other writer notes', updated_at: t1 }
  let server = fresh
  const puts: Array<Record<string, unknown>> = []
  const fetch = vi.fn(async (url: string, options?: { method?: string; body?: Record<string, unknown> }) => {
    if (url === '/api/v1/gigs/gig-1' && options?.method === 'PUT') {
      puts.push(options.body!)
      if (options.body!.updated_at !== server.updated_at) throw { response: { status: 409 } }
      server = { ...server, ...options.body, updated_at: t2 } as Gig
      return { data: server }
    }
    if (url === '/api/v1/gigs/gig-1') return { data: server }
    if (url.endsWith('/detail')) return { data: original }
    return { data: [] }
  })
  vi.stubGlobal('$fetch', fetch)
  const store = useGigStore()
  store.gigs = [original]
  const wrapper = mount(GigFormDialog, { props: { open: false, gig: original }, global: { stubs } })
  const button = (label: string) => wrapper.findAll('button').find(b => b.text() === label)!
  return { wrapper, store, fresh, puts, fetch, button }
}

describe('mounted gig dialog snapshot concurrency', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
  })
  afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

  it.each(['cache-only', 'same-id prop refresh'])('retains original snapshot on conflict, blocks blind retry, and explicitly reloads fields plus version (%s)', async (refresh) => {
    const { wrapper, store, fresh, puts, button } = setup()
    await wrapper.setProps({ open: true })
    await flushPromises()
    // The page still holds its selected t0 object while the list cache is t1.
    store.gigs = [fresh]
    if (refresh === 'same-id prop refresh') await wrapper.setProps({ gig: fresh })
    await wrapper.get('[data-testid="venue"]').setValue('My venue')
    await button('SAVE').trigger('click')
    await flushPromises()
    expect(puts[0]).toMatchObject({ venue: 'My venue', notes: 'Original notes', updated_at: t0 })
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.get('[role="alert"]').text()).toContain('changed by another writer')
    expect(wrapper.get('[role="alert"]').text()).toContain('discard')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Original notes')
    expect((wrapper.get('[data-testid="venue"]').element as HTMLInputElement).value).toBe('My venue')
    expect(button('SAVE').attributes('disabled')).toBeDefined()
    await button('SAVE').trigger('click')
    expect(puts).toHaveLength(1)
    // A same-id prop/cache replacement cannot silently clear the warning or edits.
    await wrapper.setProps({ gig: { ...fresh } })
    expect(button('SAVE').attributes('disabled')).toBeDefined()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Original notes')
    await button('RELOAD LATEST (DISCARD MY EDITS)').trigger('click')
    await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Other writer notes')
    expect((wrapper.get('[data-testid="venue"]').element as HTMLInputElement).value).toBe('Other venue')
    await wrapper.get('[data-testid="venue"]').setValue('Reapplied venue')
    await button('SAVE').trigger('click')
    await flushPromises()
    expect(puts[1]).toMatchObject({ venue: 'Reapplied venue', notes: 'Other writer notes', updated_at: t1 })
    expect(store.gigs[0]).toMatchObject({ notes: 'Other writer notes', updated_at: t2 })
    expect(wrapper.emitted('saved')).toHaveLength(1)
    wrapper.unmount()
  })

  it('keeps edits and Save disabled when conflict refresh and explicit reload fail, then recovers on reload', async () => {
    const { wrapper, puts, fetch, button } = setup()
    const realFetch = fetch.getMockImplementation()!
    let offline = true
    fetch.mockImplementation(async (url, options) => {
      if (url === '/api/v1/gigs/gig-1' && !options && offline) throw new Error('offline')
      return realFetch(url, options)
    })
    await wrapper.setProps({ open: true })
    await flushPromises()
    await wrapper.get('textarea').setValue('Unsaved notes')
    await button('SAVE').trigger('click')
    await flushPromises()
    expect(button('SAVE').attributes('disabled')).toBeDefined()
    await button('RELOAD LATEST (DISCARD MY EDITS)').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Could not reload')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Unsaved notes')
    expect(button('SAVE').attributes('disabled')).toBeDefined()
    expect(puts).toHaveLength(1)
    offline = false
    await button('RELOAD LATEST (DISCARD MY EDITS)').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('button.btn-hud-cta').at(-1)!.attributes('disabled')).toBeUndefined()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Other writer notes')
    wrapper.unmount()
  })

  it.each([
    ['switch-id', 'success'], ['switch-id', 'conflict'], ['switch-id', 'error'],
    ['reopen-same-id', 'success'], ['reopen-same-id', 'conflict'], ['reopen-same-id', 'error'],
  ] as const)('ignores late save %s / %s without replacing edits or closing the new session', async (transition, outcome) => {
    const pending = deferred<{ data: Gig }>()
    const puts: Array<{ url: string; body: Record<string, unknown> }> = []
    vi.stubGlobal('$fetch', vi.fn(async (url: string, options?: { method?: string; body?: Record<string, unknown> }) => {
      if (options?.method === 'PUT') {
        puts.push({ url, body: options.body! })
        return pending.promise
      }
      if (url === '/api/v1/gigs/gig-1') return { data: { ...original, updated_at: t1 } }
      if (url.endsWith('/detail')) return { data: original }
      return { data: [] }
    }))
    const store = useGigStore()
    store.gigs = [original]
    const wrapper = mount(GigFormDialog, { props: { open: true, gig: original }, global: { stubs } })
    const button = (label: string) => wrapper.findAll('button').find(b => b.text() === label)!
    await flushPromises()
    await wrapper.get('textarea').setValue('Old session edit')
    await button('SAVE').trigger('click')
    if (transition === 'switch-id') await wrapper.setProps({ gig: { ...original, id: 'gig-2' } })
    else {
      await button('CANCEL').trigger('click')
      await wrapper.setProps({ open: false })
      await wrapper.setProps({ open: true })
    }
    await wrapper.get('textarea').setValue('New session edit')
    const closes = wrapper.emitted('update:open')?.length ?? 0
    if (outcome === 'success') pending.resolve({ data: { ...original, notes: 'Old saved notes', updated_at: t1 } })
    else pending.reject({ status: outcome === 'conflict' ? 409 : 500 })
    await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('New session edit')
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.emitted('update:open')?.length ?? 0).toBe(closes)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(puts[0]).toMatchObject({ url: '/api/v1/gigs/gig-1', body: { notes: 'Old session edit', updated_at: t0 } })
    expect(store.gigs.find(g => g.id === original.id)?.updated_at).toBe(outcome === 'error' ? t0 : t1)
    expect(wrapper.findAll('button.btn-hud-cta').at(-1)!.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it.each([
    ['switch-id', 'success'], ['switch-id', 'conflict'], ['switch-id', 'error'],
    ['reopen-same-id', 'success'], ['reopen-same-id', 'conflict'], ['reopen-same-id', 'error'],
  ] as const)('keeps a new save busy when obsolete %s save completes with %s', async (transition, outcome) => {
    const oldSave = deferred<{ data: Gig }>()
    const newSave = deferred<{ data: Gig }>()
    let putCount = 0
    vi.stubGlobal('$fetch', vi.fn(async (url: string, options?: { method?: string; body?: Record<string, unknown> }) => {
      if (options?.method === 'PUT') return ++putCount === 1 ? oldSave.promise : newSave.promise
      if (url === '/api/v1/gigs/gig-1') return { data: { ...original, updated_at: t1 } }
      if (url.endsWith('/detail')) return { data: original }
      return { data: [] }
    }))
    const wrapper = mount(GigFormDialog, { props: { open: true, gig: original }, global: { stubs } })
    const button = (label: string) => wrapper.findAll('button').find(b => b.text() === label)!
    await flushPromises()
    await button('SAVE').trigger('click')
    const nextGig = transition === 'switch-id' ? { ...original, id: 'gig-2' } : original
    if (transition === 'switch-id') await wrapper.setProps({ gig: nextGig })
    else { await wrapper.setProps({ open: false }); await wrapper.setProps({ open: true }) }
    await wrapper.get('textarea').setValue('New session edit')
    expect(wrapper.findAll('button.btn-hud-cta').at(-1)!.attributes('disabled')).toBeUndefined()
    await button('SAVE').trigger('click')
    expect(putCount).toBe(2)
    if (outcome === 'success') oldSave.resolve({ data: { ...original, notes: 'Old saved', updated_at: t1 } })
    else oldSave.reject({ status: outcome === 'conflict' ? 409 : 500 })
    await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('New session edit')
    expect(button('SAVING...').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.emitted('update:open')).toBeUndefined()
    newSave.resolve({ data: { ...nextGig, notes: 'New saved', updated_at: t2 } })
    await flushPromises()
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('update:open')).toEqual([[false]])
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('New saved')
    wrapper.unmount()
  })

  it.each([
    ['switch-id', 'success'], ['switch-id', 'error'],
    ['reopen-same-id', 'success'], ['reopen-same-id', 'error'],
  ] as const)('does not install/show errors from an obsolete explicit reload after %s (%s)', async (transition, outcome) => {
    const { wrapper, fetch, button } = setup()
    await wrapper.setProps({ open: true })
    await flushPromises()
    await button('SAVE').trigger('click')
    await flushPromises()
    const reload = deferred<{ data: Gig }>()
    fetch.mockReturnValueOnce(reload.promise)
    await button('RELOAD LATEST (DISCARD MY EDITS)').trigger('click')
    if (transition === 'switch-id') await wrapper.setProps({ gig: { ...original, id: 'gig-2' } })
    else { await wrapper.setProps({ open: false }); await wrapper.setProps({ open: true }) }
    await wrapper.get('textarea').setValue('New session edit')
    expect(wrapper.findAll('button.btn-hud-cta').at(-1)!.attributes('disabled')).toBeUndefined()
    if (outcome === 'success') reload.resolve({ data: { ...original, notes: 'Old reloaded', updated_at: t2 } })
    else reload.reject(new Error('offline'))
    await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('New session edit')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it.each(['switch-id', 'reopen-same-id'] as const)('keeps the current reload busy when an obsolete reload finishes (%s)', async (transition) => {
    const oldReload = deferred<{ data: Gig }>()
    const newReload = deferred<{ data: Gig }>()
    let getCount = 0
    vi.stubGlobal('$fetch', vi.fn(async (url: string, options?: { method?: string }) => {
      if (options?.method === 'PUT') throw { status: 409 }
      if (url === '/api/v1/gigs/gig-1' || url === '/api/v1/gigs/gig-2') {
        getCount++
        if (getCount === 2) return oldReload.promise
        if (getCount === 4) return newReload.promise
        return { data: { ...original, id: url.split('/').pop(), updated_at: t1 } }
      }
      if (url.endsWith('/detail')) return { data: original }
      return { data: [] }
    }))
    const wrapper = mount(GigFormDialog, { props: { open: true, gig: original }, global: { stubs } })
    const button = (label: string) => wrapper.findAll('button').find(b => b.text() === label)!
    await flushPromises()
    await button('SAVE').trigger('click')
    await flushPromises()
    await button('RELOAD LATEST (DISCARD MY EDITS)').trigger('click')
    const nextGig = transition === 'switch-id' ? { ...original, id: 'gig-2' } : original
    if (transition === 'switch-id') await wrapper.setProps({ gig: nextGig })
    else { await wrapper.setProps({ open: false }); await wrapper.setProps({ open: true }) }
    await button('SAVE').trigger('click')
    await flushPromises()
    await wrapper.get('textarea').setValue('New session edit')
    await button('RELOAD LATEST (DISCARD MY EDITS)').trigger('click')
    expect(getCount).toBe(4)
    oldReload.resolve({ data: { ...original, notes: 'Obsolete reload', updated_at: t1 } })
    await flushPromises()
    expect(button('RELOADING...').attributes('disabled')).toBeDefined()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('New session edit')
    newReload.resolve({ data: { ...nextGig, notes: 'Current reload', updated_at: t2 } })
    await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Current reload')
    expect(button('SAVE').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it.each(['switch-id', 'reopen-same-id'] as const)('ignores save completion awaiting finance reconciliation after %s', async (transition) => {
    const finance = deferred<{ data: null }>()
    const fetch = vi.fn(async (url: string, options?: { method?: string }) => {
      if (options?.method === 'PUT') return { data: { ...original, notes: 'Saved', updated_at: t1 } }
      if (url === '/api/v1/finance/reconciliations') return finance.promise
      if (url.endsWith('/detail')) return { data: original }
      return { data: [] }
    })
    vi.stubGlobal('$fetch', fetch)
    const wrapper = mount(GigFormDialog, { props: { open: true, gig: original }, global: { stubs } })
    await flushPromises()
    await wrapper.findAll('button.btn-hud-cta').at(-1)!.trigger('click')
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith('/api/v1/finance/reconciliations', expect.objectContaining({ params: { gig_id: original.id } }))
    if (transition === 'switch-id') await wrapper.setProps({ gig: { ...original, id: 'gig-2' } })
    else { await wrapper.setProps({ open: false }); await wrapper.setProps({ open: true }) }
    await wrapper.get('textarea').setValue('New session edit')
    finance.resolve({ data: null })
    await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('New session edit')
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.emitted('update:open')).toBeUndefined()
    expect(wrapper.findAll('button.btn-hud-cta').at(-1)!.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('adopts successful server-normalized fields and returned version together while the parent still holds t0', async () => {
    const puts: Array<Record<string, unknown>> = []
    vi.stubGlobal('$fetch', vi.fn(async (url: string, options?: { method?: string; body?: Record<string, unknown> }) => {
      if (options?.method === 'PUT') {
        puts.push(options.body!)
        return { data: { ...original, ...options.body, notes: 'Server-normalized notes', venue: 'Server venue', updated_at: puts.length === 1 ? t1 : t2 } }
      }
      if (url.endsWith('/detail')) return { data: original }
      return { data: [] }
    }))
    const store = useGigStore()
    store.gigs = [original]
    const wrapper = mount(GigFormDialog, { props: { open: true, gig: original }, global: { stubs } })
    const save = () => wrapper.findAll('button').find(b => b.text() === 'SAVE')!
    await flushPromises()
    await wrapper.get('textarea').setValue('My edit')
    await save().trigger('click')
    await flushPromises()
    expect(puts[0]?.updated_at).toBe(t0)
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Server-normalized notes')
    expect((wrapper.get('[data-testid="venue"]').element as HTMLInputElement).value).toBe('Server venue')
    expect(wrapper.props('gig')?.updated_at).toBe(t0)
    await wrapper.get('input[placeholder="Saturday Night"]').setValue('Next edit')
    await save().trigger('click')
    await flushPromises()
    expect(puts[1]).toMatchObject({ notes: 'Server-normalized notes', venue: 'Server venue', event_name: 'Next edit', updated_at: t1 })
    expect(store.gigs[0]?.updated_at).toBe(t2)
    wrapper.unmount()
  })
})
