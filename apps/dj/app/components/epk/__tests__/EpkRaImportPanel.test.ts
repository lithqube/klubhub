import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import EpkRaImportPanel from '../EpkRaImportPanel.vue'

vi.mock('~/stores/epk', () => ({
  useEpkStore: vi.fn(() => ({ updateContent: vi.fn() })),
}))
vi.mock('~/stores/settings', () => ({
  useSettingsStore: vi.fn(() => ({ socialLinks: {}, save: vi.fn() })),
}))

const mockFetch = vi.fn()

function mountWith(raImport: boolean) {
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { features: { raImport } } }))
  vi.stubGlobal('$fetch', mockFetch)
  setActivePinia(createPinia())
  return mount(EpkRaImportPanel)
}

describe('EpkRaImportPanel (licensed feature)', () => {
  beforeEach(() => {
    mockFetch.mockReset()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders nothing when RA import is disabled', () => {
    const wrapper = mountWith(false)
    expect(wrapper.find('.glass-panel').exists()).toBe(false)
    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.text()).not.toMatch(/\bRA\b|Resident Advisor|ra\.co/i)
    expect(mockFetch).not.toHaveBeenCalled()
  })

  it('renders the import panel when RA import is enabled', () => {
    const wrapper = mountWith(true)
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
    expect(wrapper.text()).toContain('IMPORT FROM RA')
  })
})
