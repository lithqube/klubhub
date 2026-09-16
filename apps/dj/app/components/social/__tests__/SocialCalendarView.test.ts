import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import SocialCalendarView from '../SocialCalendarView.vue'
import { dotColorForStatus } from '../socialCalendarUtils'
import type { ScheduledPost } from '../../../types/social'

function makePost(overrides: Partial<ScheduledPost> = {}): ScheduledPost {
  return {
    id: 'post-1',
    accountId: 'acc-1',
    status: 'scheduled',
    postType: 'feed',
    caption: 'Test caption',
    imageMinioPath: 'path/to/image.jpg',
    scheduledAtUtc: '2026-10-05T14:00:00Z',
    timezoneName: 'UTC',
    retryCount: 0,
    nextRetryAt: null,
    lastError: '',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('dotColorForStatus', () => {
  it('returns bg-error for failed status', () => {
    expect(dotColorForStatus('failed')).toBe('bg-error')
  })

  it('returns bg-error for permanently_failed status', () => {
    expect(dotColorForStatus('permanently_failed')).toBe('bg-error')
  })

  it('returns bg-primary for scheduled status', () => {
    expect(dotColorForStatus('scheduled')).toBe('bg-primary')
  })

  it('returns bg-secondary for draft status', () => {
    expect(dotColorForStatus('draft')).toBe('bg-secondary')
  })

  it('returns bg-tertiary for published status', () => {
    expect(dotColorForStatus('published')).toBe('bg-tertiary')
  })

  it('returns bg-tertiary for publishing status', () => {
    expect(dotColorForStatus('publishing')).toBe('bg-tertiary')
  })
})

describe('SocialCalendarView', () => {
  it('postsByDate groups posts by date key', () => {
    const posts = [
      makePost({ scheduledAtUtc: '2026-10-05T14:00:00Z', id: 'p1' }),
      makePost({ scheduledAtUtc: '2026-10-05T18:00:00Z', id: 'p2' }),
    ]
    const wrapper = mount(SocialCalendarView, {
      props: { posts, month: new Date(2026, 9, 1) },
    })
    // Access the component's exposed postsByDate
    const vm = wrapper.vm as any
    const map = vm.postsByDate
    expect(map.get('2026-10-05')).toHaveLength(2)
    expect(map.get('2026-10-05').map((p: ScheduledPost) => p.id)).toContain('p1')
    expect(map.get('2026-10-05').map((p: ScheduledPost) => p.id)).toContain('p2')
  })

  it('emits day-selected with date string when a day cell with posts is clicked', async () => {
    const posts = [
      makePost({ scheduledAtUtc: '2026-10-05T14:00:00Z', id: 'p1' }),
    ]
    const wrapper = mount(SocialCalendarView, {
      props: { posts, month: new Date(2026, 9, 1) },
    })
    // Find the day cell for October 5
    const dayCells = wrapper.findAll('[data-date]')
    const oct5 = dayCells.find(cell => cell.attributes('data-date') === '2026-10-05')
    expect(oct5).toBeDefined()
    await oct5!.trigger('click')
    expect(wrapper.emitted('day-selected')).toBeTruthy()
    expect(wrapper.emitted('day-selected')![0]).toEqual(['2026-10-05'])
  })

  it('emits day-selected with null when same day is clicked again', async () => {
    const posts = [
      makePost({ scheduledAtUtc: '2026-10-05T14:00:00Z', id: 'p1' }),
    ]
    const wrapper = mount(SocialCalendarView, {
      props: { posts, month: new Date(2026, 9, 1) },
    })
    const dayCells = wrapper.findAll('[data-date]')
    const oct5 = dayCells.find(cell => cell.attributes('data-date') === '2026-10-05')
    await oct5!.trigger('click')
    await oct5!.trigger('click')
    const emitted = wrapper.emitted('day-selected')!
    expect(emitted).toHaveLength(2)
    expect(emitted[1]).toEqual([null])
  })
})
