import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import ContactAutocomplete from '../ContactAutocomplete.vue'
import type { Contact } from '../../../types/gig'

const mockFetch = vi.fn()

function makeContact(over: Partial<Contact> = {}): Contact {
  return {
    id: 'c1',
    name: 'Promoter Pat',
    company: 'Agency',
    email: 'pat@example.com',
    phone: '+49 111',
    type: 'promoter',
    notes: '',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    deleted_at: null,
    ...over,
  }
}

const wrappers: Array<{ unmount: () => void }> = []

function mountAc(modelValue: Contact | null = null) {
  const wrapper = mount(ContactAutocomplete, {
    props: { modelValue, placeholder: 'Search contact...' },
    attachTo: document.body,
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('ContactAutocomplete', () => {
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

  it('prefills the query from the model value', () => {
    const wrapper = mountAc(makeContact())
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('Promoter Pat')
    expect(wrapper.find('input').attributes('placeholder')).toBe('Search contact...')
  })

  it('does not search for queries shorter than 2 characters', async () => {
    const wrapper = mountAc()
    await wrapper.find('input').setValue('p')
    await vi.advanceTimersByTimeAsync(500)
    expect(mockFetch).not.toHaveBeenCalled()
  })

  it('debounces input and searches, rendering results', async () => {
    mockFetch.mockResolvedValue([makeContact(), makeContact({ id: 'c2', name: 'Agent Amy', company: null })])
    const wrapper = mountAc()
    await wrapper.find('input').setValue('pr')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/contacts/autocomplete?q=pr&limit=5')
    expect(wrapper.text()).toContain('Promoter Pat')
    expect(wrapper.text()).toContain('Agent Amy')
    expect(wrapper.text()).toContain('Agency')
  })

  it('selecting a result emits both events and closes the dropdown', async () => {
    const contact = makeContact()
    mockFetch.mockResolvedValue([contact])
    const wrapper = mountAc()
    await wrapper.find('input').setValue('pat')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    await wrapper.findAll('.glass > div')[0].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[contact]])
    expect(wrapper.emitted('contact-selected')).toEqual([[contact]])
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('Promoter Pat')
  })

  it('clear button empties the query and emits null', async () => {
    const wrapper = mountAc(makeContact())
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[null]])
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('')
  })

  it('navigates results with the keyboard and selects with Enter', async () => {
    const c1 = makeContact()
    const c2 = makeContact({ id: 'c2', name: 'Agent Amy' })
    mockFetch.mockResolvedValue([c1, c2])
    const wrapper = mountAc()
    const input = wrapper.find('input')
    await input.setValue('a')
    await input.setValue('ag')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    await input.trigger('keydown', { key: 'ArrowDown' })
    await input.trigger('keydown', { key: 'ArrowDown' })
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('contact-selected')).toEqual([[c2]])
  })

  it('closes the dropdown on Escape', async () => {
    mockFetch.mockResolvedValue([makeContact()])
    const wrapper = mountAc()
    const input = wrapper.find('input')
    await input.setValue('pat')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(wrapper.text()).toContain('CREATE NEW CONTACT')
    await input.trigger('keydown', { key: 'Escape' })
    expect(wrapper.text()).not.toContain('CREATE NEW CONTACT')
  })

  it('updates the query when the model value changes', async () => {
    const wrapper = mountAc()
    await wrapper.setProps({ modelValue: makeContact({ name: 'Booker Bob' }) })
    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('Booker Bob')
  })
})
