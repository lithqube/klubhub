import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { ref } from 'vue'
import SocialPostCompose from '../SocialPostCompose.vue'
import { useSocialStore } from '../../../stores/social'

const mockCreatePost = vi.fn().mockResolvedValue({ id: 'created-post' })
const mockUploadImage = vi.fn().mockResolvedValue('social/created-post/image.jpg')
const mockLoadPosts = vi.fn().mockResolvedValue(undefined)


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
    mockCreatePost.mockReset().mockResolvedValue({ id: 'created-post' })
    mockUploadImage.mockReset().mockResolvedValue('social/created-post/image.jpg')
    mockResetForm.mockImplementation(() => { mockImageFile.value = null })
    useSocialStore().openComposePanel()
    vi.stubGlobal('$fetch', vi.fn(async (url: string, options?: any) => {
      if (url.endsWith('/image')) {
        const id = url.split('/').at(-2)!
        return { data: { path: await mockUploadImage(id, options.body.get('image_file')) } }
      }
      if (options?.method === 'POST') {
        const f = options.body as FormData
        return { data: await mockCreatePost({
          postType: f.get('post_type'), caption: f.get('caption'),
          scheduledAt: f.get('scheduled_at'), timezoneName: f.get('timezone_name'),
          imageId: f.get('image_id') ?? undefined,
        }) }
      }
      await mockLoadPosts()
      return { data: [] }
    }))
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

  describe('create then upload', () => {
    function readyForm(file: File | null = null) {
      mockCaption.value = 'Test caption'
      mockScheduledAt.value = '2026-12-01T12:00'
      mockImageFile.value = file
      mockImageId.value = file ? null : 'tracklists/existing.jpg'
      return mount(SocialPostCompose, { props: { prefilledImageId: null } })
    }

    it('uploads the selected file using the returned post id before closing and clearing', async () => {
      const file = new File(['image'], 'image.jpg', { type: 'image/jpeg' })
      let completeUpload!: (path: string) => void
      mockUploadImage.mockImplementationOnce(() => new Promise<string>((resolve) => { completeUpload = resolve }))
      const wrapper = readyForm(file)
      await wrapper.get('[data-testid="submit-btn"]').trigger('click')
      await flushPromises()
      expect(mockCreatePost).toHaveBeenCalledWith({
        postType: 'feed', caption: 'Test caption', scheduledAt: '2026-12-01T12:00',
        timezoneName: 'Europe/Berlin', imageId: undefined,
      })
      expect(mockUploadImage).toHaveBeenCalledWith('created-post', file)
      expect(useSocialStore().composePanelOpen).toBe(true)
      expect(mockResetForm).not.toHaveBeenCalled()
      expect(mockImageFile.value).toBe(file)
      completeUpload('social/created-post/image.jpg')
      await flushPromises()
      expect(useSocialStore().composePanelOpen).toBe(false)
      expect(mockResetForm).toHaveBeenCalledOnce()
      expect(mockImageFile.value).toBeNull()
    })

    it('uses an existing image without uploading a file', async () => {
      const wrapper = readyForm()
      await wrapper.get('[data-testid="submit-btn"]').trigger('click')
      await flushPromises()
      expect(mockCreatePost.mock.calls[0][0]).not.toHaveProperty('imageFile')
      expect(mockCreatePost.mock.calls[0][0].imageId).toBe('tracklists/existing.jpg')
      expect(mockUploadImage).not.toHaveBeenCalled()
      expect(useSocialStore().composePanelOpen).toBe(false)
      expect(mockResetForm).toHaveBeenCalledOnce()
    })

    it('shows partial failure, retains the file and retries upload without recreating the post', async () => {
      const file = new File(['image'], 'image.jpg')
      mockUploadImage.mockRejectedValueOnce(new Error('storage unavailable'))
      const wrapper = readyForm(file)
      await wrapper.get('[data-testid="submit-btn"]').trigger('click')
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toMatch(/post.*created.*image upload failed/i)
      expect(useSocialStore().composePanelOpen).toBe(true)
      expect(mockResetForm).not.toHaveBeenCalled()
      expect(mockImageFile.value).toBe(file)
      await wrapper.get('[data-testid="submit-btn"]').trigger('click')
      await flushPromises()
      expect(mockCreatePost).toHaveBeenCalledOnce()
      expect(mockUploadImage).toHaveBeenCalledTimes(2)
      expect(mockUploadImage).toHaveBeenLastCalledWith('created-post', file)
      expect(useSocialStore().composePanelOpen).toBe(false)
      expect(mockImageFile.value).toBeNull()
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
