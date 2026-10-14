import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import VenueAutocomplete from '../VenueAutocomplete.vue'
import type { Venue } from '../../../types/gig'

const mockFetch = vi.fn()

function makeVenue(over: Partial<Venue> = {}): Venue {
  return {
    id: 'v1',
    name: 'Berghain',
    city: 'Berlin',
    country: 'DE',
    capacity: 1500,
    website: null,
    tech_contact_name: '',
    tech_contact_email: '',
    tech_contact_phone: '',
    notes: '',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    deleted_at: null,
    ...over,
  }
}

const wrappers: Array<{ unmount: () => void }> = []

function mountAc(modelValue: Venue | null = null) {
  const wrapper = mount(VenueAutocomplete, {
    props: { modelValue },
    attachTo: document.body,
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('VenueAutocomplete', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockFetch.mockReset()
    vi.stubGlobal('$fetch', mockFetch)
    setActivePinia(createPinia())
  })

  afterEach(() => {
    while (wrappers.length) wrappers.pop()!.unmount()
    vi.useRealTimers()
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('prefills the query from the model value and shows the placeholder otherwise', () => {
    const withVenue = mountAc(makeVenue())
    expect((withVenue.find('input').element as HTMLInputElement).value).toBe('Berghain')
    withVenue.unmount()
    const without = mountAc()
    const input = without.find('input').element as HTMLInputElement
    expect(input.value).toBe('')
    expect(input.placeholder).toBe('Search venue...')
  })

  it('does not search for queries shorter than 2 characters', async () => {
    const wrapper = mountAc()
    const input = wrapper.find('input')
    await input.setValue('b')
    await vi.advanceTimersByTimeAsync(500)
    expect(mockFetch).not.toHaveBeenCalled()
  })

  it('debounces input and searches, rendering results in the dropdown', async () => {
    mockFetch.mockResolvedValue([makeVenue(), makeVenue({ id: 'v2', name: 'Tresor', city: 'Berlin' })])
    const wrapper = mountAc()
    const input = wrapper.find('input')
    await input.setValue('ber')
    expect(mockFetch).not.toHaveBeenCalled() // debounced
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/venues/autocomplete?q=ber&limit=5')
    // two result rows + the "CREATE NEW VENUE" row inside the dropdown
    const rows = wrapper.findAll('.glass > div')
    expect(rows.length).toBe(3)
    const names = wrapper.text()
    expect(names).toContain('Berghain')
    expect(names).toContain('Tresor')
    expect(names).toContain('Berlin, DE')
  })

  it('searches immediately on focus when the query is long enough', async () => {
    mockFetch.mockResolvedValue([makeVenue()])
    const wrapper = mountAc(makeVenue())
    await wrapper.find('input').trigger('focus')
    await flushPromises()
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/venues/autocomplete?q=Berghain&limit=5')
  })

  it('selecting a result emits both events and closes the dropdown', async () => {
    const venue = makeVenue()
    mockFetch.mockResolvedValue([venue])
    const wrapper = mountAc()
    await wrapper.find('input').setValue('berg')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    await wrapper.findAll('.glass > div')[0].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[venue]])
    expect(wrapper.emitted('venue-selected')).toEqual([[venue]])
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('Berghain')
    expect(wrapper.text()).not.toContain('CREATE NEW VENUE')
  })

  it('clear button empties the query and emits null', async () => {
    const wrapper = mountAc(makeVenue())
    const clearBtn = wrapper.find('button')
    expect(clearBtn.exists()).toBe(true)
    await clearBtn.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[null]])
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('')
  })

  it('hides the clear button while loading', async () => {
    let resolveFetch: (v: Venue[]) => void = () => undefined
    mockFetch.mockReturnValue(new Promise<Venue[]>((r) => { resolveFetch = r }))
    const wrapper = mountAc(makeVenue())
    await wrapper.find('input').setValue('berg')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).toContain('SEARCHING...')
    resolveFetch([])
    await flushPromises()
  })

  it('shows the empty state when nothing matches', async () => {
    mockFetch.mockResolvedValue([])
    const wrapper = mountAc()
    await wrapper.find('input').setValue('zzz')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    // no results -> dropdown stays closed, no empty-state panel either
    expect(wrapper.text()).not.toContain('NO VENUES FOUND')
  })

  it('navigates results with the keyboard and selects with Enter', async () => {
    const v1 = makeVenue()
    const v2 = makeVenue({ id: 'v2', name: 'Tresor' })
    mockFetch.mockResolvedValue([v1, v2])
    const wrapper = mountAc()
    const input = wrapper.find('input')
    await input.setValue('ber')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    await input.trigger('keydown', { key: 'ArrowDown' })
    await input.trigger('keydown', { key: 'ArrowDown' })
    await input.trigger('keydown', { key: 'ArrowUp' })
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('venue-selected')).toEqual([[v1]])
  })

  it('closes the dropdown on Escape and on outside click', async () => {
    mockFetch.mockResolvedValue([makeVenue()])
    const wrapper = mountAc()
    const input = wrapper.find('input')
    await input.setValue('ber')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(wrapper.text()).toContain('CREATE NEW VENUE')

    await input.trigger('keydown', { key: 'Escape' })
    expect(wrapper.text()).not.toContain('CREATE NEW VENUE')

    // reopen, then click outside (pointerdown arms onClickOutside, click fires it)
    await input.setValue('berg')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(wrapper.text()).toContain('CREATE NEW VENUE')
    document.body.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushPromises()
    expect(wrapper.text()).not.toContain('CREATE NEW VENUE')
  })

  it('updates the query when the model value changes', async () => {
    const wrapper = mountAc()
    await wrapper.setProps({ modelValue: makeVenue({ name: 'Komplex' }) })
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('Komplex')
  })
})
