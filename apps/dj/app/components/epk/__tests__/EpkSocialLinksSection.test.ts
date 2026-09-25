import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import EpkSocialLinksSection from '../EpkSocialLinksSection.vue'

const mockFetch = vi.fn()

function mountWith(raImport: boolean) {
  vi.stubGlobal('useRuntimeConfig', () => ({ public: { features: { raImport } } }))
  vi.stubGlobal('$fetch', mockFetch)
  return mount(EpkSocialLinksSection)
}

describe('EpkSocialLinksSection edition gating', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockFetch.mockReset()
    mockFetch.mockImplementation((_url: string, opts?: { method?: string }) =>
      Promise.resolve(
        opts?.method
          ? {}
          : {
              social_links: {
                instagram: 'https://instagram.example/test-dj',
                residentAdvisor: 'https://links.example/test-dj',
              },
            },
      ),
    )
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('hides the RA link field when RA import is disabled', async () => {
    const wrapper = mountWith(false)
    await flushPromises()
    expect(wrapper.find('[data-testid="social-input-residentAdvisor"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="social-input-instagram"]').exists()).toBe(true)
    expect(wrapper.text()).not.toMatch(/RESIDENT ADVISOR/i)
  })

  it('keeps manual links working and preserves a saved RA link on save', async () => {
    const wrapper = mountWith(false)
    await flushPromises()

    await wrapper.find('input[data-testid="social-input-instagram"]').setValue('https://instagram.example/renamed-dj')
    vi.advanceTimersByTime(1600)
    await flushPromises()

    const put = mockFetch.mock.calls.find(([, opts]) => opts?.method === 'put')
    expect(put).toBeDefined()
    expect(put![1].body.social_links.instagram).toBe('https://instagram.example/renamed-dj')
    expect(put![1].body.social_links.residentAdvisor).toBe('https://links.example/test-dj')
  })

  it('shows the RA link field when RA import is enabled', async () => {
    const wrapper = mountWith(true)
    await flushPromises()
    expect(wrapper.find('[data-testid="social-input-residentAdvisor"]').exists()).toBe(true)
  })
})
