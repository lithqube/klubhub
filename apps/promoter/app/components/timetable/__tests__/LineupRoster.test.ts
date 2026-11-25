import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { mount } from '@vue/test-utils'
import type { LineupEntry } from '~/types/event'
import LineupRoster from '../LineupRoster.vue'

const act = (id: string, name: string): LineupEntry => ({
  id, stage_id: null, display_name: name, profile_url: null, billing_order: 0, b2b_group: null, set_start: null, set_end: null,
})

describe('LineupRoster', () => {
  it('removes nobody else when a row that is still animating out is tapped again', async () => {
    vi.stubGlobal('ref', ref) // Nuxt auto-import
    const [a, b, c] = [act('a', 'Kaiser'), act('b', 'Dasha'), act('c', 'Klock')]
    // The real TransitionGroup keeps the leaving row, and its handler, on screen.
    const wrapper = mount(LineupRoster, {
      props: { lineup: [a, b, c], untimedIds: new Set<string>() },
      global: { stubs: { TransitionGroup: false } },
    })
    const button = wrapper.get('[aria-label="Remove Dasha"]').element as HTMLButtonElement
    button.click()
    await wrapper.setProps({ lineup: [a, c] })
    button.click()
    const updates = wrapper.emitted('update') as [LineupEntry[]][]
    expect(updates).toHaveLength(2)
    expect(updates[1]![0].map(l => l.id)).toEqual(['a', 'c'])
    wrapper.unmount()
    vi.unstubAllGlobals()
  })
})
