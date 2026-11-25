import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SocialPostCard from '../SocialPostCard.vue'
import type { ScheduledPost } from '../../../types/social'

const post = {
  id: 'p1', accountId: 'a1', status: 'scheduled', postType: 'feed', caption: 'A', imageStorageKey: '',
  scheduledAtUtc: '2030-07-01T18:00:00Z', timezoneName: 'Europe/Berlin', retryCount: 0, nextRetryAt: null,
  lastError: null, containerId: null, createdAt: '', updatedAt: '',
} as unknown as ScheduledPost

describe('SocialPostCard', () => {
  beforeEach(() => setActivePinia(createPinia()))
  afterEach(() => vi.unstubAllGlobals())

  it('locks the editor while a save is pending, so nothing typed meanwhile is lost', async () => {
    vi.stubGlobal('$fetch', vi.fn(() => new Promise<never>(() => undefined)))
    const card = mount(SocialPostCard, { props: { post } })
    await card.trigger('click')
    await card.find('textarea').setValue('B')
    await card.findAll('button').find(b => b.text() === 'SAVE')!.trigger('click')
    await flushPromises()
    for (const el of card.findAll('textarea, input, button')) expect(el.attributes('disabled')).toBeDefined()
  })
})
