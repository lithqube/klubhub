import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { ref, computed } from 'vue'
import EpkPhotosSection from '../EpkPhotosSection.vue'

// Mock store state
const mockPhotoPaths = ref<string[]>(['photos/a.jpg', 'photos/b.jpg', 'photos/c.jpg'])
const mockDeletePhoto = vi.fn().mockResolvedValue(undefined)
const mockUploadPhoto = vi.fn().mockResolvedValue(undefined)

vi.mock('~/stores/epk', () => ({
  useEpkStore: vi.fn(() => ({
    photoPaths: mockPhotoPaths,
    canAddPhoto: computed(() => mockPhotoPaths.value.length < 20),
    photoCount: computed(() => mockPhotoPaths.value.length),
    deletePhoto: mockDeletePhoto,
    uploadPhoto: mockUploadPhoto,
  })),
}))

describe('EpkPhotosSection', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockPhotoPaths.value = ['photos/a.jpg', 'photos/b.jpg', 'photos/c.jpg']
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
    mockPhotoPaths.value = ['photos/a.jpg']
    const wrapper = mount(EpkPhotosSection)
    expect(wrapper.find('[data-testid="photo-upload-zone"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="photo-limit-label"]').exists()).toBe(false)
  })

  it('upload zone is hidden when photoCount >= 20', () => {
    mockPhotoPaths.value = Array.from({ length: 20 }, (_, i) => `photos/${i}.jpg`)
    const wrapper = mount(EpkPhotosSection)
    expect(wrapper.find('[data-testid="photo-upload-zone"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="photo-limit-label"]').exists()).toBe(true)
  })

  it('limit label shows correct text at 20/20', () => {
    mockPhotoPaths.value = Array.from({ length: 20 }, (_, i) => `photos/${i}.jpg`)
    const wrapper = mount(EpkPhotosSection)
    const label = wrapper.find('[data-testid="photo-limit-label"]')
    expect(label.text()).toContain('20 / 20 PHOTOS')
  })

  it('section is wrapped in glass-panel', () => {
    const wrapper = mount(EpkPhotosSection)
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
  })

  it('renders no photo items when photoPaths is empty', () => {
    mockPhotoPaths.value = []
    const wrapper = mount(EpkPhotosSection)
    const items = wrapper.findAll('[data-testid="photo-item"]')
    expect(items.length).toBe(0)
  })
})
