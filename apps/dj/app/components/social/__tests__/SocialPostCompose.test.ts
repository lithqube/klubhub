import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { ref } from 'vue'
import SocialPostCompose from '../SocialPostCompose.vue'

// Mock the social store
vi.mock('~/stores/social', () => ({
  useSocialStore: vi.fn(() => ({
    createPost: vi.fn().mockResolvedValue(undefined),
    closeComposePanel: vi.fn(),
    loadPosts: vi.fn().mockResolvedValue(undefined),
  })),
}))

// Mock the tracklist store
vi.mock('~/stores/tracklist', () => ({
  useTracklistStore: vi.fn(() => ({
    tracklist: ref({ title: 'Test Set', trackCount: 20, avgBpm: 140 }),
    tracks: ref([{ title: 'Track 1' }]),
  })),
}))

// Mock useSocialPostForm composable
const mockPostType = ref<'feed' | 'story'>('feed')
const mockCaption = ref('')
const mockScheduledAt = ref('')
const mockTimezoneName = ref('Europe/Berlin')
const mockTimezoneSearch = ref('')
const mockImageId = ref<string | null>(null)
const mockImageFile = ref<File | null>(null)
const mockCharLimit = ref<number | null>(2200)
const mockCharCount = ref(0)
const mockIsOverLimit = ref(false)
const mockTimezoneOptions = ref(['Europe/Berlin', 'America/New_York', 'UTC'])
const mockScheduledAtUTCPreview = ref('')
const mockGenerateCaption = vi.fn()
const mockResetForm = vi.fn()

vi.mock('~/composables/useSocialPostForm', () => ({
  useSocialPostForm: vi.fn(() => ({
    postType: mockPostType,
    caption: mockCaption,
    scheduledAt: mockScheduledAt,
    timezoneName: mockTimezoneName,
    timezoneSearch: mockTimezoneSearch,
    imageId: mockImageId,
    imageFile: mockImageFile,
    charLimit: mockCharLimit,
    charCount: mockCharCount,
    isOverLimit: mockIsOverLimit,
    timezoneOptions: mockTimezoneOptions,
    scheduledAtUTCPreview: mockScheduledAtUTCPreview,
    generateCaption: mockGenerateCaption,
    resetForm: mockResetForm,
  })),
}))

describe('SocialPostCompose', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    // Reset reactive refs
    mockPostType.value = 'feed'
    mockCaption.value = ''
    mockScheduledAt.value = ''
    mockImageId.value = null
    mockImageFile.value = null
    mockCharLimit.value = 2200
    mockCharCount.value = 0
    mockIsOverLimit.value = false
  })

  describe('char counter', () => {
    it('shows "N / 2200" counter for feed type', async () => {
      mockPostType.value = 'feed'
      mockCharCount.value = 482
      mockCharLimit.value = 2200
      const wrapper = mount(SocialPostCompose, {
        props: { prefilledImageId: null },
      })
      expect(wrapper.text()).toContain('482 / 2200')
    })

    it('charLimit is null for story type and note text is visible', async () => {
      mockPostType.value = 'story'
      mockCharLimit.value = null
      const wrapper = mount(SocialPostCompose, {
        props: { prefilledImageId: null },
      })
      // Counter should NOT show for story
      expect(wrapper.text()).not.toContain('/ 2200')
      // Note should be visible for story
      expect(wrapper.text()).toContain('not shown on Stories')
    })
  })

  describe('caption auto-gen', () => {
    it('calls generateCaption when prefilledImageId changes from null to a value', async () => {
      const wrapper = mount(SocialPostCompose, {
        props: { prefilledImageId: null },
      })
      await wrapper.setProps({ prefilledImageId: 'img-123' })
      expect(mockGenerateCaption).toHaveBeenCalled()
    })

    it('does not call generateCaption when prefilledImageId remains null', async () => {
      const wrapper = mount(SocialPostCompose, {
        props: { prefilledImageId: null },
      })
      await wrapper.setProps({ prefilledImageId: null })
      expect(mockGenerateCaption).not.toHaveBeenCalled()
    })
  })

  describe('submit guard', () => {
    it('submit button is disabled when caption is empty', () => {
      mockCaption.value = ''
      mockScheduledAt.value = '2026-10-24T23:45'
      mockImageId.value = 'img-123'
      const wrapper = mount(SocialPostCompose, {
        props: { prefilledImageId: 'img-123' },
      })
      const submitBtn = wrapper.find('[data-testid="submit-btn"]')
      expect(submitBtn.attributes('disabled')).toBeDefined()
    })

    it('submit button is disabled when scheduledAt is empty', () => {
      mockCaption.value = 'Test caption'
      mockScheduledAt.value = ''
      mockImageId.value = 'img-123'
      const wrapper = mount(SocialPostCompose, {
        props: { prefilledImageId: 'img-123' },
      })
      const submitBtn = wrapper.find('[data-testid="submit-btn"]')
      expect(submitBtn.attributes('disabled')).toBeDefined()
    })

    it('submit button is disabled when no image is set', () => {
      mockCaption.value = 'Test caption'
      mockScheduledAt.value = '2026-10-24T23:45'
      mockImageId.value = null
      mockImageFile.value = null
      const wrapper = mount(SocialPostCompose, {
        props: { prefilledImageId: null },
      })
      const submitBtn = wrapper.find('[data-testid="submit-btn"]')
      expect(submitBtn.attributes('disabled')).toBeDefined()
    })

    it('submit button is enabled when all required fields are set', () => {
      mockCaption.value = 'Test caption'
      mockScheduledAt.value = '2026-10-24T23:45'
      mockImageId.value = 'img-123'
      mockIsOverLimit.value = false
      const wrapper = mount(SocialPostCompose, {
        props: { prefilledImageId: 'img-123' },
      })
      const submitBtn = wrapper.find('[data-testid="submit-btn"]')
      expect(submitBtn.attributes('disabled')).toBeUndefined()
    })

    it('submit button is disabled when isOverLimit is true', () => {
      mockCaption.value = 'A'.repeat(2201)
      mockScheduledAt.value = '2026-10-24T23:45'
      mockImageId.value = 'img-123'
      mockIsOverLimit.value = true
      const wrapper = mount(SocialPostCompose, {
        props: { prefilledImageId: 'img-123' },
      })
      const submitBtn = wrapper.find('[data-testid="submit-btn"]')
      expect(submitBtn.attributes('disabled')).toBeDefined()
    })
  })
})
