import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { ref } from 'vue'
import EpkBioSection from '../EpkBioSection.vue'

// Mock the EPK store
const mockBioShort = ref('Hello world')
const mockBioLong = ref('Long bio text')

vi.mock('~/stores/epk', () => ({
  useEpkStore: vi.fn(() => ({
    bioShort: mockBioShort,
    bioLong: mockBioLong,
  })),
}))

// Mock useEpkAutosave
const mockScheduleSave = vi.fn()
vi.mock('~/composables/useEpkAutosave', () => ({
  useEpkAutosave: vi.fn(() => ({
    scheduleSave: mockScheduleSave,
  })),
}))

describe('EpkBioSection', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockBioShort.value = 'Hello world'
    mockBioLong.value = 'Long bio text'
  })

  it('renders short bio input with correct initial value from store', () => {
    const wrapper = mount(EpkBioSection)
    const input = wrapper.find('[data-testid="bio-short-input"]')
    expect(input.exists()).toBe(true)
    expect((input.element as HTMLInputElement).value).toBe('Hello world')
  })

  it('renders long bio textarea with correct initial value from store', () => {
    const wrapper = mount(EpkBioSection)
    const textarea = wrapper.find('[data-testid="bio-long-textarea"]')
    expect(textarea.exists()).toBe(true)
    expect((textarea.element as HTMLTextAreaElement).value).toBe('Long bio text')
  })

  it('character counter shows correct format N / 280', () => {
    mockBioShort.value = 'Hello world'
    const wrapper = mount(EpkBioSection)
    const counter = wrapper.find('[data-testid="bio-short-counter"]')
    expect(counter.exists()).toBe(true)
    expect(counter.text()).toContain('11 / 280')
  })

  it('character counter shows 0 / 280 when bio is empty', () => {
    mockBioShort.value = ''
    const wrapper = mount(EpkBioSection)
    const counter = wrapper.find('[data-testid="bio-short-counter"]')
    expect(counter.text()).toContain('0 / 280')
  })

  it('@update:modelValue on short bio input calls scheduleSave with bioShort', async () => {
    const wrapper = mount(EpkBioSection)
    const input = wrapper.find('[data-testid="bio-short-input"]')
    await input.setValue('New bio text')
    expect(mockScheduleSave).toHaveBeenCalledWith({ bioShort: 'New bio text' })
  })

  it('@update:modelValue on long bio textarea calls scheduleSave with bioLong', async () => {
    const wrapper = mount(EpkBioSection)
    const textarea = wrapper.find('[data-testid="bio-long-textarea"]')
    await textarea.setValue('New long bio')
    expect(mockScheduleSave).toHaveBeenCalledWith({ bioLong: 'New long bio' })
  })

  it('section is wrapped in glass-panel', () => {
    const wrapper = mount(EpkBioSection)
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
  })

  it('section label is uppercase BIO', () => {
    const wrapper = mount(EpkBioSection)
    expect(wrapper.text()).toContain('BIO')
  })

  it('long bio textarea has font-mono class', () => {
    const wrapper = mount(EpkBioSection)
    const textarea = wrapper.find('[data-testid="bio-long-textarea"]')
    expect(textarea.classes()).toContain('font-mono')
  })
})
