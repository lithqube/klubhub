import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { ref } from 'vue'
import SocialPostCompose from '../SocialPostCompose.vue'
import { useSocialStore } from '../../../stores/social'

// Spy on useUiStore().showError — this is the toast path that fires
// when social.ts's createPost/uploadImage catch blocks run. With the
// fix, those calls must be removed because submitCompose already
// surfaces the failure in-form via pending.error, producing a duplicate
// toast + in-form error pair for a single failure today.
const { showErrorSpy } = vi.hoisted(() => ({ showErrorSpy: vi.fn() }))

vi.mock('../../../stores/ui', async () => {
  const actual = await vi.importActual<typeof import('../../../stores/ui')>('../../../stores/ui')
  return {
    ...actual,
    useUiStore: () => ({
      ...actual.useUiStore(),
      showError: showErrorSpy,
    }),
  }
})

// Mock the tracklist store the component reads on mount.
vi.mock('~/stores/tracklist', () => ({
  useTracklistStore: () => ({
    tracklist: ref({ title: 'Test Set' }),
    tracks: ref([{ bpm: 140 }]),
  }),
}))

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
const mockTimezoneOptions = ref(['Europe/Berlin'])
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

function readyForm(file: File | null = null) {
  mockCaption.value = 'Test caption'
  mockScheduledAt.value = '2026-12-01T12:00'
  mockImageFile.value = file
  mockImageId.value = file ? null : 'tracklists/existing.jpg'
  return mount(SocialPostCompose, { props: { prefilledImageId: null } })
}

describe('submitCompose surfaces failure exactly once', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    showErrorSpy.mockClear()
    mockPostType.value = 'feed'
    mockCaption.value = ''
    mockScheduledAt.value = ''
    mockImageId.value = null
    mockImageFile.value = null
    mockCharLimit.value = 2200
    mockCharCount.value = 0
    mockIsOverLimit.value = false
    useSocialStore().openComposePanel()
  })

  it('does not call showError (toast) when post creation rejects (in-form alert is the single source)', async () => {
    vi.stubGlobal('$fetch', vi.fn(async (_url: string, options?: { method?: string }) => {
      if (options?.method === 'POST') throw new Error('creation rejected')
      return { data: [] }
    }))
    const wrapper = readyForm()
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    const alert = wrapper.get('[role="alert"]').text()
    expect(alert).toMatch(/post creation failed/i)
    expect(showErrorSpy).not.toHaveBeenCalled()
  })

  it('does not call showError (toast) when image upload rejects after post creation', async () => {
    vi.stubGlobal('$fetch', vi.fn(async (url: string, options?: { method?: string }) => {
      if (url.endsWith('/image')) throw new Error('storage down')
      if (options?.method === 'POST') return { data: { id: 'created-post' } }
      return { data: [] }
    }))
    const file = new File(['img'], 'a.jpg', { type: 'image/jpeg' })
    const wrapper = readyForm(file)
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    const alert = wrapper.get('[role="alert"]').text()
    expect(alert).toMatch(/image upload failed/i)
    expect(showErrorSpy).not.toHaveBeenCalled()
  })
})
