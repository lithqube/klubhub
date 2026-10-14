import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
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

  it('clicking a chip on a non-empty value adds it on its own line (rider text is one requirement per line)', async () => {
    const wrapper = mount(RiderSectionField, {
      props: { section: 'technical', modelValue: 'existing' },
    })
    await wrapper.findAll('.social-chip')[1]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['existing\nTech Chip B'])
  })

  it('keeps the user\'s own line breaks and leading whitespace; drops only trailing blanks', async () => {
    const wrapper = mount(RiderSectionField, {
      props: { section: 'technical', modelValue: '  first\nsecond  \n\n' },
    })
    await wrapper.findAll('.social-chip')[0]!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['  first\nsecond\nTech Chip A'])
  })

  it('the heading is a real label bound to the textarea', () => {
    const wrapper = mount(RiderSectionField, { props: { section: 'technical', modelValue: '' } })
    const label = wrapper.find('label')
    const textarea = wrapper.find('[data-testid="rider-technical-textarea"]')
    expect(label.exists()).toBe(true)
    expect(label.attributes('for')).toBeTruthy()
    expect(textarea.attributes('id')).toBe(label.attributes('for'))
  })

  it('two fields on one page get different ids (so each label points at its own textarea)', () => {
    // One app, like the real page: ids are unique per app, not across apps.
    const Page = defineComponent({
      render: () => h('div', [
        h(RiderSectionField, { section: 'technical', modelValue: '' }),
        h(RiderSectionField, { section: 'backline', modelValue: '' }),
      ]),
    })
    const wrapper = mount(Page)
    const [a, b] = wrapper.findAll('textarea').map(t => t.attributes('id'))
    expect(a).toBeTruthy()
    expect(a).not.toBe(b)
  })
})