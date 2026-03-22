import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { reactive, nextTick } from 'vue'
import EpkExportPanel from '../EpkExportPanel.vue'

const mockGenerateExport = vi.fn().mockResolvedValue(undefined)
const mockLoadExports = vi.fn().mockResolvedValue(undefined)
const mockDeleteExport = vi.fn().mockResolvedValue(undefined)

const mockStore = reactive({
  sectionVisibility: {
    bio: true,
    photos: true,
    gigHighlights: true,
    pressQuotes: true,
    techRider: true,
    stagePlot: true,
    socialLinks: true,
    contactInfo: true,
  } as Record<string, boolean>,
  exports: [] as any[],
  saveStatus: 'idle' as string,
  generateExport: mockGenerateExport,
  loadExports: mockLoadExports,
  deleteExport: mockDeleteExport,
})

vi.mock('~/stores/epk', () => ({
  useEpkStore: vi.fn(() => mockStore),
}))

const mockScheduleSave = vi.fn()
vi.mock('~/composables/useEpkAutosave', () => ({
  useEpkAutosave: vi.fn(() => ({
    scheduleSave: mockScheduleSave,
  })),
}))

describe('EpkExportPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockStore.sectionVisibility = {
      bio: true,
      photos: true,
      gigHighlights: true,
      pressQuotes: true,
      techRider: true,
      stagePlot: true,
      socialLinks: true,
      contactInfo: true,
    }
    mockStore.exports = []
    mockStore.saveStatus = 'idle'
  })

  it('renders 8 section toggle rows', () => {
    const wrapper = mount(EpkExportPanel)
    const EXPECTED_KEYS = ['bio', 'photos', 'gigHighlights', 'pressQuotes', 'techRider', 'stagePlot', 'socialLinks', 'contactInfo']
    for (const key of EXPECTED_KEYS) {
      expect(wrapper.find(`[data-testid="section-toggle-row-${key}"]`).exists()).toBe(true)
    }
  })

  it('renders 8 section toggle switches', () => {
    const wrapper = mount(EpkExportPanel)
    const EXPECTED_KEYS = ['bio', 'photos', 'gigHighlights', 'pressQuotes', 'techRider', 'stagePlot', 'socialLinks', 'contactInfo']
    for (const key of EXPECTED_KEYS) {
      expect(wrapper.find(`[data-testid="section-toggle-${key}"]`).exists()).toBe(true)
    }
  })

  it('EXPORT PDF button is present', () => {
    const wrapper = mount(EpkExportPanel)
    const btn = wrapper.find('[data-testid="export-pdf-btn"]')
    expect(btn.exists()).toBe(true)
  })

  it('EXPORT PDF button calls generateExport on click', async () => {
    const wrapper = mount(EpkExportPanel)
    const btn = wrapper.find('[data-testid="export-pdf-btn"]')
    await btn.trigger('click')
    await nextTick()
    expect(mockGenerateExport).toHaveBeenCalledTimes(1)
  })

  it('EXPORT PDF button has gradient-cta class', () => {
    const wrapper = mount(EpkExportPanel)
    const btn = wrapper.find('[data-testid="export-pdf-btn"]')
    expect(btn.classes()).toContain('gradient-cta')
  })

  it('export history list shows correct item count', () => {
    mockStore.exports = [
      { id: 'e1', minioPath: 'p1', downloadUrl: 'http://dl1', createdAt: '2024-01-01T00:00:00Z' },
      { id: 'e2', minioPath: 'p2', downloadUrl: 'http://dl2', createdAt: '2024-01-02T00:00:00Z' },
    ]
    const wrapper = mount(EpkExportPanel)
    const items = wrapper.findAll('[data-testid="export-history-item"]')
    expect(items.length).toBe(2)
  })

  it('delete button calls deleteExport with correct id', async () => {
    mockStore.exports = [
      { id: 'e1', minioPath: 'p1', downloadUrl: 'http://dl1', createdAt: '2024-01-01T00:00:00Z' },
    ]
    const wrapper = mount(EpkExportPanel)
    const deleteBtn = wrapper.find('[data-testid="export-delete-e1"]')
    expect(deleteBtn.exists()).toBe(true)
    await deleteBtn.trigger('click')
    expect(mockDeleteExport).toHaveBeenCalledWith('e1')
  })

  it('shows export history list when exports exist', () => {
    mockStore.exports = [
      { id: 'e1', minioPath: 'p1', downloadUrl: 'http://dl1', createdAt: '2024-01-01T00:00:00Z' },
    ]
    const wrapper = mount(EpkExportPanel)
    expect(wrapper.find('[data-testid="export-history-list"]').exists()).toBe(true)
  })

  it('shows save status indicator when saveStatus is saved', () => {
    mockStore.saveStatus = 'saved'
    const wrapper = mount(EpkExportPanel)
    const indicator = wrapper.find('[data-testid="save-status-indicator"]')
    expect(indicator.exists()).toBe(true)
    expect(indicator.text()).toContain('SAVED')
  })

  it('does not show save status indicator when saveStatus is idle', () => {
    mockStore.saveStatus = 'idle'
    const wrapper = mount(EpkExportPanel)
    const indicator = wrapper.find('[data-testid="save-status-indicator"]')
    expect(indicator.exists()).toBe(false)
  })

  it('section is wrapped in glass-panel', () => {
    const wrapper = mount(EpkExportPanel)
    expect(wrapper.find('.glass-panel').exists()).toBe(true)
  })
})
