import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import RaEventImport from '../RaEventImport.vue'

const mockFetch = vi.fn()

function mountWith(raImport: boolean) {
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { features: { raImport } } }))
  vi.stubGlobal('$fetch', mockFetch)
  setActivePinia(createPinia())
  return mount(RaEventImport)
}

describe('RaEventImport (licensed feature)', () => {
  beforeEach(() => {
    mockFetch.mockReset()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders nothing when RA import is disabled', () => {
    const wrapper = mountWith(false)
    expect(wrapper.find('.glass-panel').exists()).toBe(false)
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).not.toMatch(/\bRA\b|Resident Advisor|ra\.co/i)
    expect(mockFetch).not.toHaveBeenCalled()
  })

  it('renders the event import panel when RA import is enabled', () => {
    const wrapper = mountWith(true)
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
    expect(wrapper.text()).toContain('IMPORT FROM RA')
  })
})
