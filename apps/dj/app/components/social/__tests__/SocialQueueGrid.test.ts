import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import SocialQueueGrid from '../SocialQueueGrid.vue'
import type { ScheduledPost } from '../../../types/social'

const mockRetryPost = vi.fn().mockResolvedValue(undefined)
const mockDownloadImage = vi.fn().mockResolvedValue(undefined)
const mockOpenComposePanel = vi.fn()

vi.mock('~/stores/social', () => ({
  useSocialStore: vi.fn(() => ({
    retryPost: mockRetryPost,
    downloadImage: mockDownloadImage,
    openComposePanel: mockOpenComposePanel,
  })),
}))

function makePost(overrides: Partial<ScheduledPost> = {}): ScheduledPost {
  return {
    id: 'post-1',
    accountId: 'acc-1',
    status: 'scheduled',
    postType: 'feed',
    caption: 'Test caption for this post',
    imageMinioPath: 'tracklists/img.png',
    scheduledAtUtc: '2026-10-24T21:45:00Z',
    timezoneName: 'Europe/Berlin',
    retryCount: 0,
    nextRetryAt: null,
    lastError: '',
    createdAt: '2026-10-01T00:00:00Z',
    updatedAt: '2026-10-01T00:00:00Z',
    ...overrides,
  }
}

describe('SocialQueueGrid', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  describe('card routing by status', () => {
    it('renders SocialPostCard for scheduled posts', () => {
      const posts = [makePost({ status: 'scheduled' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      expect(wrapper.find('[data-testid="post-card"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="failed-card"]').exists()).toBe(false)
    })

    it('renders SocialPostCard for published posts', () => {
      const posts = [makePost({ status: 'published' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      expect(wrapper.find('[data-testid="post-card"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="failed-card"]').exists()).toBe(false)
    })

    it('renders SocialPostCard for draft posts', () => {
      const posts = [makePost({ status: 'draft' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      expect(wrapper.find('[data-testid="post-card"]').exists()).toBe(true)
    })

    it('renders SocialPostCardFailed for failed posts', () => {
      const posts = [makePost({ status: 'failed', lastError: 'Upload timeout' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      expect(wrapper.find('[data-testid="failed-card"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="post-card"]').exists()).toBe(false)
    })

    it('renders SocialPostCardFailed for permanently_failed posts', () => {
      const posts = [makePost({ status: 'permanently_failed', lastError: 'Invalid token' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      expect(wrapper.find('[data-testid="failed-card"]').exists()).toBe(true)
    })
  })

  describe('mixed posts', () => {
    it('renders both SocialPostCard and SocialPostCardFailed in same grid', () => {
      const posts = [
        makePost({ id: 'post-1', status: 'scheduled' }),
        makePost({ id: 'post-2', status: 'failed', lastError: 'Error' }),
        makePost({ id: 'post-3', status: 'published' }),
      ]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      expect(wrapper.findAll('[data-testid="post-card"]').length).toBe(2)
      expect(wrapper.findAll('[data-testid="failed-card"]').length).toBe(1)
    })
  })

  describe('SocialNextSlotCard', () => {
    it('always renders SocialNextSlotCard at end of grid (no posts)', () => {
      const wrapper = mount(SocialQueueGrid, { props: { posts: [] } })
      expect(wrapper.find('[data-testid="next-slot-card"]').exists()).toBe(true)
    })

    it('always renders SocialNextSlotCard at end of grid (with posts)', () => {
      const posts = [makePost({ status: 'scheduled' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      expect(wrapper.find('[data-testid="next-slot-card"]').exists()).toBe(true)
    })
  })

  describe('SocialPostCardFailed actions', () => {
    it('calls store.retryPost(id) when RETRY SYNC is clicked', async () => {
      const posts = [makePost({ id: 'post-1', status: 'failed', lastError: 'Error' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      const retryBtn = wrapper.find('[data-testid="retry-btn"]')
      expect(retryBtn.exists()).toBe(true)
      await retryBtn.trigger('click')
      expect(mockRetryPost).toHaveBeenCalledWith('post-1')
    })

    it('calls store.downloadImage(id) when DOWNLOAD IMAGE is clicked', async () => {
      const posts = [makePost({ id: 'post-1', status: 'failed', lastError: 'Error' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      const downloadBtn = wrapper.find('[data-testid="download-btn"]')
      expect(downloadBtn.exists()).toBe(true)
      await downloadBtn.trigger('click')
      expect(mockDownloadImage).toHaveBeenCalledWith('post-1')
    })
  })

  describe('failed card content', () => {
    it('shows ATTENTION REQUIRED banner on failed card', () => {
      const posts = [makePost({ status: 'failed', lastError: 'Upload failed' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      expect(wrapper.text()).toContain('ATTENTION REQUIRED')
    })

    it('shows lastError text on failed card', () => {
      const posts = [makePost({ status: 'failed', lastError: 'Upload timeout' })]
      const wrapper = mount(SocialQueueGrid, { props: { posts } })
      expect(wrapper.text()).toContain('Upload timeout')
    })
  })
})
