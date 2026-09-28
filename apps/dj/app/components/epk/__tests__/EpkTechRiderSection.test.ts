import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { reactive, nextTick } from 'vue'
import EpkTechRiderSection from '../EpkTechRiderSection.vue'

// Mock the EPK store — use reactive() so computed() accessors in the component
// read a plain string value (Pinia stores are reactive objects, not ref wrappers).
const mockStore = reactive({
  techRider: '',
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

describe('EpkTechRiderSection', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockStore.techRider = ''
  })

  it('renders the textarea with the store value', async () => {
    mockStore.techRider = 'Existing rider'
    const wrapper = mount(EpkTechRiderSection)
    await nextTick()
    const textarea = wrapper.find('[data-testid="tech-rider-textarea"]')
    expect((textarea.element as HTMLTextAreaElement).value).toBe('Existing rider')
  })

  it('renders one quick-insert chip per entry', () => {
    const wrapper = mount(EpkTechRiderSection)
    const chips = wrapper.findAll('.social-chip')
    expect(chips.length).toBeGreaterThan(0)
    expect(chips[0]!.text()).toContain('2× CDJ-3000')
  })

  it('clicking a chip on an empty rider sets it to just that text and saves', async () => {
    const wrapper = mount(EpkTechRiderSection)
    await wrapper.findAll('.social-chip')[0]!.trigger('click')
    expect(mockStore.techRider).toBe('2× CDJ-3000')
    expect(mockScheduleSave).toHaveBeenCalledWith({ techRider: '2× CDJ-3000' })
  })

  it('clicking a chip preserves existing whitespace/line breaks instead of trimming them', async () => {
    mockStore.techRider = 'Line one\nLine two  '
    const wrapper = mount(EpkTechRiderSection)
    await nextTick()
    await wrapper.findAll('.social-chip')[0]!.trigger('click')
    expect(mockStore.techRider).toBe('Line one\nLine two  , 2× CDJ-3000')
  })

  it('clicking a chip on a non-empty rider appends it with a comma', async () => {
    mockStore.techRider = 'DJM-A9 or better'
    const wrapper = mount(EpkTechRiderSection)
    await nextTick()
    const wedges = wrapper.findAll('.social-chip').find(c => c.text().includes('Monitor wedges'))!
    await wedges.trigger('click')
    expect(mockStore.techRider).toBe('DJM-A9 or better, Monitor wedges ×2')
    expect(mockScheduleSave).toHaveBeenCalledWith({ techRider: 'DJM-A9 or better, Monitor wedges ×2' })
  })

  it('typing in the textarea still saves directly (chips are additive, not exclusive)', async () => {
    const wrapper = mount(EpkTechRiderSection)
    const textarea = wrapper.find('[data-testid="tech-rider-textarea"]')
    await textarea.setValue('Custom note')
    expect(mockScheduleSave).toHaveBeenCalledWith({ techRider: 'Custom note' })
  })
})
