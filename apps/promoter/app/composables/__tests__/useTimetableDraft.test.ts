import { describe, expect, it } from 'vitest'
import { defineComponent, h, nextTick, ref, type Ref } from 'vue'
import { mount } from '@vue/test-utils'
import type { EventDetail, LineupEntry } from '~/types/event'
import { useTimetableDraft } from '../useTimetableDraft'

const act = (id: string, name: string): LineupEntry => ({
  id, stage_id: null, display_name: name, profile_url: null, billing_order: 0, b2b_group: null, set_start: null, set_end: null,
})
const night = (lineup: LineupEntry[], version = 1) => ({
  id: 'e1', version, lineup, stages: [], timezone: 'Europe/Berlin',
  starts_at: '2026-12-01T21:00:00Z', ends_at: '2026-12-02T05:00:00Z', doors_at: null,
}) as unknown as EventDetail

function setup(event: Ref<EventDetail | null>) {
  let api!: ReturnType<typeof useTimetableDraft>
  mount(defineComponent({ setup() { api = useTimetableDraft(event); return () => h('div') } }))
  return api
}

describe('useTimetableDraft', () => {
  it('stays saveable when only the event version changes', () => {
    const event = ref<EventDetail | null>(night([act('a', 'Kaiser')]))
    const draft = setup(event)
    draft.update([act('a', 'Kaiser'), act('b', 'Dasha')])
    event.value = night([act('a', 'Kaiser')], 2)
    expect(draft.dirtyCount.value).toBeGreaterThan(0)
    expect(draft.stale.value).toBe(false)
  })

  it('goes stale when the saved lineup changes underneath it, until the user chooses', () => {
    const event = ref<EventDetail | null>(night([act('a', 'Kaiser')]))
    const draft = setup(event)
    draft.update([act('a', 'Kaiser'), act('b', 'Dasha')])
    event.value = night([act('a', 'Kaiser'), act('c', 'Klock')], 2)
    expect(draft.stale.value).toBe(true)

    // Comparing shows theirs read-only and never changes what would be saved.
    draft.viewSaved.value = true
    expect(draft.lineup.value.map(l => l.display_name)).toEqual(['Kaiser', 'Klock'])
    draft.update([])
    expect(draft.toInput().map(l => l.display_name)).toEqual(['Kaiser', 'Dasha'])

    draft.keepMine()
    expect(draft.stale.value).toBe(false)
    expect(draft.lineup.value.map(l => l.display_name)).toEqual(['Kaiser', 'Dasha'])
  })

  it('leaves the saved view when the conflict goes away', async () => {
    for (const saved of [[act('a', 'Kaiser'), act('b', 'Dasha')], [act('a', 'Kaiser')]]) {
      const event = ref<EventDetail | null>(night([act('a', 'Kaiser')]))
      const draft = setup(event)
      draft.update([act('a', 'Kaiser'), act('b', 'Dasha')])
      event.value = night([act('c', 'Klock')], 2)
      draft.viewSaved.value = true
      await nextTick()
      event.value = night(saved, 3) // equal to mine, or back to where I started
      await nextTick()
      expect(draft.stale.value).toBe(false)
      expect(draft.viewSaved.value).toBe(false)
      expect(draft.lineup.value.map(l => l.display_name)).toEqual(['Kaiser', 'Dasha'])
    }
  })

  it('drops back to the saved lineup on discard', () => {
    const event = ref<EventDetail | null>(night([act('a', 'Kaiser')]))
    const draft = setup(event)
    draft.update([])
    event.value = night([act('c', 'Klock')], 2)
    draft.discard()
    expect(draft.stale.value).toBe(false)
    expect(draft.lineup.value.map(l => l.display_name)).toEqual(['Klock'])
  })
})
