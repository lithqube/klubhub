import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RiderSectionField from '../RiderSectionField.vue'

// Mirror the EPK test pattern: vi.mock the composable / store so the
// component reads from a reactive() object instead of the real Pinia
// store (which doesn't play well with vi.mock in unit tests).

vi.mock('../chips', () => ({
  SECTION_CHIPS: {
    technical: ['Tech Chip A', 'Tech Chip B'],
    hospitality: [],
    backline: [],
    otherNotes: [],
  },
}))

describe('RiderSectionField', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders label + textarea + N chip buttons', () => {
    const wrapper = mount(RiderSectionField, {
      props: { section: 'technical', modelValue: '' },
    })
    const chips = wrapper.findAll('.social-chip')
    expect(chips.length).toBe(2)
    expect(wrapper.find('[data-testid="rider-technical-textarea"]').exists()).toBe(true)
  })

  it('emits update:modelValue when typing', async () => {
    const wrapper = mount(RiderSectionField, {
      props: { section: 'technical', modelValue: '' },
    })
    const ta = wrapper.find('[data-testid="rider-technical-textarea"]')
    await ta.setValue('hello')
    // Exactly once per edit: a duplicate listener doubles every keystroke's
    // emit and so every autosave schedule in the parent editor.
    expect(wrapper.emitted('update:modelValue')).toEqual([['hello']])
  })

  it('clicking a chip on empty value sets it to just that chip', async () => {
    const wrapper = mount(RiderSectionField, {
      props: { section: 'technical', modelValue: '' },
    })
    await wrapper.findAll('.social-chip')[0]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['Tech Chip A'])
  })

  it('clicking a chip on non-empty value appends with comma', async () => {
    const wrapper = mount(RiderSectionField, {
      props: { section: 'technical', modelValue: 'existing' },
    })
    await wrapper.findAll('.social-chip')[1]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['existing, Tech Chip B'])
  })
})