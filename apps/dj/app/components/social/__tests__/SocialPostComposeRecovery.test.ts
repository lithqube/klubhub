import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SocialPostCompose from '../SocialPostCompose.vue'
import { useSocialStore } from '~/stores/social'

const fetchMock = vi.fn()
const wrappers: ReturnType<typeof mount>[] = []
function openComposer() {
  useSocialStore().openComposePanel()
  const wrapper = mount(SocialPostCompose, { props: { prefilledImageId: null } })
  wrappers.push(wrapper)
  return wrapper
}
async function fillComposer(wrapper: ReturnType<typeof mount>, file: File) {
  await wrapper.get('textarea').setValue('Selected image caption')
  await wrapper.get('input[type="datetime-local"]').setValue('2026-12-24T12:00')
  const input = wrapper.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
}
beforeEach(() => {
  setActivePinia(createPinia())
  fetchMock.mockReset()
  vi.stubGlobal('$fetch', fetchMock)
})
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()); vi.unstubAllGlobals() })

describe('mounted compose image submission', () => {
  it('retains failed upload metadata and selected file across reopen and retries without creating again', async () => {
    let uploads = 0
    fetchMock.mockImplementation(async (url, options) => {
      if (url === '/api/v1/social/posts' && options?.method === 'POST') return { data: { id: 'created-post' } }
      if (url.endsWith('/image')) {
        if (++uploads === 1) throw new Error('upload unavailable')
        return { data: { path: 'social/image.png' } }
      }
      return { data: [] }
    })
    const file = new File(['image'], 'retained.png', { type: 'image/png' })
    let wrapper = openComposer()
    await fillComposer(wrapper, file)
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('image upload failed')
    expect(useSocialStore().composePanelOpen).toBe(true)
    await wrapper.get('button').trigger('click')
    wrapper.unmount()
    wrapper = openComposer()
    expect(wrapper.get('textarea').element.value).toBe('Selected image caption')
    expect(wrapper.text()).toContain('retained.png')
    expect(wrapper.get('textarea').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.filter(([url, options]) => url === '/api/v1/social/posts' && options?.method === 'POST')).toHaveLength(1)
    expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/image'))).toHaveLength(2)
    expect(useSocialStore().pendingComposeUpload).toBeNull()
    expect(useSocialStore().composePanelOpen).toBe(false)
  })
  it('creates metadata then uploads the selected file before closing', async () => {
    fetchMock.mockImplementation(async (url, options) => {
      if (url === '/api/v1/social/posts' && options?.method === 'POST') return { data: { id: 'created-post' } }
      if (url.endsWith('/image')) return { data: { path: 'social/image.png' } }
      return { data: [] }
    })
    const file = new File(['image'], 'selected.png', { type: 'image/png' })
    const wrapper = openComposer()
    await fillComposer(wrapper, file)
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    const writes = fetchMock.mock.calls.filter(([, options]) => options?.method === 'POST')
    expect(writes.map(([url]) => url)).toEqual(['/api/v1/social/posts', '/api/v1/social/posts/created-post/image'])
    expect(writes[0]![1].body.get('caption')).toBe('Selected image caption')
    expect(writes[0]![1].body.has('image_file')).toBe(false)
    expect(writes[1]![1].body.get('image_file')).toBe(file)
    expect(useSocialStore().composePanelOpen).toBe(false)
  })
})
