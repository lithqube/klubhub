import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { reactive } from 'vue'
import EpkPhotosSection from '../EpkPhotosSection.vue'

// Mock store state — use reactive() so computed() in the component reads plain arrays
const mockDeletePhoto = vi.fn().mockResolvedValue(undefined)
const mockUploadPhoto = vi.fn().mockResolvedValue(undefined)

const mockStore = reactive({
  photoPaths: ['photos/a.jpg', 'photos/b.jpg', 'photos/c.jpg'] as string[],
  deletePhoto: mockDeletePhoto,
  uploadPhoto: mockUploadPhoto,
  get canAddPhoto() { return this.photoPaths.length < 20 },
  get photoCount() { return this.photoPaths.length },
})

vi.mock('~/stores/epk', () => ({
  useEpkStore: vi.fn(() => mockStore),
}))

describe('EpkPhotosSection', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockStore.photoPaths = ['photos/a.jpg', 'photos/b.jpg', 'photos/c.jpg']
    mockStore.deletePhoto = mockDeletePhoto
    mockStore.uploadPhoto = mockUploadPhoto
  })

  it('renders correct number of photo items', () => {
    const wrapper = mount(EpkPhotosSection)
    const items = wrapper.findAll('[data-testid="photo-item"]')
    expect(items.length).toBe(3)
  })

  it('photo grid has data-testid="photo-grid"', () => {
    const wrapper = mount(EpkPhotosSection)
    expect(wrapper.find('[data-testid="photo-grid"]').exists()).toBe(true)
  })

  it('delete button calls store.deletePhoto with correct path', async () => {
    const wrapper = mount(EpkPhotosSection)
    const deleteBtn = wrapper.find('[data-testid="photo-delete-0"]')
    expect(deleteBtn.exists()).toBe(true)
    await deleteBtn.trigger('click')
    expect(mockDeletePhoto).toHaveBeenCalledWith('photos/a.jpg')
  })

  it('upload zone is visible when photoCount < 20', () => {
    mockStore.photoPaths = ['photos/a.jpg']
    const wrapper = mount(EpkPhotosSection)
    expect(wrapper.find('[data-testid="photo-upload-zone"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="photo-limit-label"]').exists()).toBe(false)
  })

  it('upload zone is hidden when photoCount >= 20', () => {
    mockStore.photoPaths = Array.from({ length: 20 }, (_, i) => `photos/${i}.jpg`)
    const wrapper = mount(EpkPhotosSection)
    expect(wrapper.find('[data-testid="photo-upload-zone"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="photo-limit-label"]').exists()).toBe(true)
  })

  it('limit label shows correct text at 20/20', () => {
    mockStore.photoPaths = Array.from({ length: 20 }, (_, i) => `photos/${i}.jpg`)
    const wrapper = mount(EpkPhotosSection)
    const label = wrapper.find('[data-testid="photo-limit-label"]')
    expect(label.text()).toContain('20 / 20 PHOTOS')
  })

  it('section is wrapped in glass-panel', () => {
    const wrapper = mount(EpkPhotosSection)
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
  })

  it('renders no photo items when photoPaths is empty', () => {
    mockStore.photoPaths = []
    const wrapper = mount(EpkPhotosSection)
    const items = wrapper.findAll('[data-testid="photo-item"]')
    expect(items.length).toBe(0)
  })
})
