import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { reactive, nextTick } from 'vue'
import EpkGigHighlightsSection from '../EpkGigHighlightsSection.vue'

const mockStore = reactive({
  gigHighlights: ['Club Alpha 2024', 'Gamma Hall'] as string[],
})

vi.mock('~/stores/epk', () => ({
  useEpkStore: vi.fn(() => mockStore),
}))

const mockScheduleSave = vi.fn()
vi.mock('~/composables/useEpkAutosave', () => ({
  useEpkAutosave: vi.fn(() => ({
    scheduleSave: mockScheduleSave,
  })),
}))

describe('EpkGigHighlightsSection', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockStore.gigHighlights = ['Club Alpha 2024', 'Gamma Hall']
  })

  it('renders existing highlights from store', async () => {
    const wrapper = mount(EpkGigHighlightsSection)
    await nextTick()
    const inputs = wrapper.findAll('[data-testid^="highlight-input-"]')
    expect(inputs.length).toBe(2)
    expect((inputs[0].element as HTMLInputElement).value).toBe('Club Alpha 2024')
    expect((inputs[1].element as HTMLInputElement).value).toBe('Gamma Hall')
  })

  it('renders empty list when gigHighlights is empty', () => {
    mockStore.gigHighlights = []
    const wrapper = mount(EpkGigHighlightsSection)
    const inputs = wrapper.findAll('[data-testid^="highlight-input-"]')
    expect(inputs.length).toBe(0)
  })

  it('add highlight button appends empty entry', async () => {
    const wrapper = mount(EpkGigHighlightsSection)
    await nextTick()
    const addBtn = wrapper.find('[data-testid="add-highlight-btn"]')
    expect(addBtn.exists()).toBe(true)
    await addBtn.trigger('click')
    await nextTick()
    const inputs = wrapper.findAll('[data-testid^="highlight-input-"]')
    expect(inputs.length).toBe(3)
    expect(mockScheduleSave).toHaveBeenCalledWith({ gigHighlights: ['Club Alpha 2024', 'Gamma Hall', ''] })
  })

  it('remove highlight button removes the correct entry', async () => {
    const wrapper = mount(EpkGigHighlightsSection)
    await nextTick()
    const deleteBtn = wrapper.find('[data-testid="highlight-delete-0"]')
    expect(deleteBtn.exists()).toBe(true)
    await deleteBtn.trigger('click')
    await nextTick()
    const inputs = wrapper.findAll('[data-testid^="highlight-input-"]')
    expect(inputs.length).toBe(1)
    expect(mockScheduleSave).toHaveBeenCalledWith({ gigHighlights: ['Gamma Hall'] })
  })

  it('import-from-gigs button is disabled', () => {
    const wrapper = mount(EpkGigHighlightsSection)
    const btn = wrapper.find('[data-testid="import-from-gigs-btn"]')
    expect(btn.exists()).toBe(true)
    expect((btn.element as HTMLButtonElement).disabled).toBe(true)
  })

  it('import-from-gigs button has correct title attribute', () => {
    const wrapper = mount(EpkGigHighlightsSection)
    const btn = wrapper.find('[data-testid="import-from-gigs-btn"]')
    expect(btn.attributes('title')).toBe('Coming Soon — available after Gig Tracker is set up')
  })

  it('section is wrapped in glass-panel', () => {
    const wrapper = mount(EpkGigHighlightsSection)
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
  })

  it('section label is GIG HIGHLIGHTS', () => {
    const wrapper = mount(EpkGigHighlightsSection)
    expect(wrapper.text()).toContain('GIG HIGHLIGHTS')
  })
})
