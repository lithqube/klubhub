import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import SocialPostCompose from '../SocialPostCompose.vue'
import SocialTabs from '../SocialTabs.vue'
import { useSocialStore } from '../../../stores/social'

async function chooseFile(wrapper: ReturnType<typeof mount>, file: File) {
  const input = wrapper.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
  await input.trigger('change')
}

function mountCompose() {
  return mount(SocialPostCompose, { props: { prefilledImageId: null } })
}
async function fillForm(wrapper: ReturnType<typeof mount>) {
  await wrapper.get('textarea').setValue('Immutable caption')
  await wrapper.get('input[type="datetime-local"]').setValue('2026-12-01T12:00')
  await chooseFile(wrapper, new File(['bad'], 'bad.jpg', { type: 'image/jpeg' }))
  await wrapper.get('[data-testid="submit-btn"]').trigger('click')
  await flushPromises()
}

describe('compose upload recovery with real shared store', () => {
  let fetchMock: ReturnType<typeof vi.fn>
  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock = vi.fn(async (url: string, options?: { method?: string }) => {
      if (url.endsWith('/image')) throw new Error('422 invalid image')
      if (options?.method === 'POST') return { data: { id: 'same-post' } }
      return { data: [] }
    })
    vi.stubGlobal('$fetch', fetchMock)
  })

  it('retains returned id when creation resolves after route unmount', async () => {
    let resolveCreate!: (value: any) => void
    let rejectUpload!: (error: Error) => void
    let resolveRefresh!: (value: any) => void
    let firstRefresh = true
    fetchMock.mockImplementation((url: string, options?: { method?: string }) => {
      if (url.endsWith('/image')) return new Promise((_resolve, reject) => { rejectUpload = reject })
      if (options?.method === 'POST') return new Promise(resolve => { resolveCreate = resolve })
      if (firstRefresh) {
        firstRefresh = false
        return new Promise(resolve => { resolveRefresh = resolve })
      }
      return Promise.resolve({ data: [] })
    })
    const wrapper = mountCompose()
    await fillForm(wrapper)
    wrapper.unmount()
    const whileCreating = mountCompose()
    expect((whileCreating.get('textarea').element as HTMLTextAreaElement).value).toBe('Immutable caption')
    expect(whileCreating.get('[data-testid="submit-btn"]').attributes('disabled')).toBeDefined()
    await whileCreating.get('[data-testid="submit-btn"]').trigger('click')
    whileCreating.unmount()
    resolveCreate({ data: { id: 'same-post' } })
    await flushPromises()
    expect(useSocialStore().pendingComposeUpload?.postId).toBe('same-post')
    // The ID must be retained even while the list refresh is still unresolved.
    resolveRefresh({ data: [] })
    await flushPromises()
    const during = mountCompose()
    expect(during.get('[data-testid="submit-btn"]').attributes('disabled')).toBeDefined()
    during.unmount()
    rejectUpload(new Error('storage unavailable'))
    await flushPromises()
    const reopened = mountCompose()
    expect(reopened.get('[role="alert"]').text()).toContain('image upload failed')
    expect((reopened.get('textarea').element as HTMLTextAreaElement).value).toBe('Immutable caption')
    fetchMock.mockImplementation(async (url: string) => url.endsWith('/image') ? { data: { path: 'ok' } } : { data: [] })
    await reopened.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.filter(([url, options]) => url === '/api/v1/social/posts' && options?.method === 'POST')).toHaveLength(1)
  })

  it('clears recovery only after deletion succeeds and can compose anew on reopen', async () => {
    const wrapper = mountCompose()
    await fillForm(wrapper)
    wrapper.unmount()
    const store = useSocialStore()
    fetchMock.mockRejectedValueOnce(new Error('delete failed'))
    await store.deletePost('same-post')
    expect(store.pendingComposeUpload?.postId).toBe('same-post')
    await store.deletePost('unrelated')
    expect(store.pendingComposeUpload?.postId).toBe('same-post')
    await store.deletePost('same-post')
    expect(store.pendingComposeUpload).toBeNull()
    const reopened = mountCompose()
    await fillForm(reopened)
    expect(fetchMock.mock.calls.filter(([url, options]) => url === '/api/v1/social/posts' && options?.method === 'POST')).toHaveLength(2)
  })

  it('unfreezes metadata after an upload 404 so the missing post can be scheduled anew', async () => {
    fetchMock.mockImplementation(async (url: string, options?: { method?: string }) => {
      if (url.endsWith('/image')) throw Object.assign(new Error('post gone'), { statusCode: 404 })
      if (options?.method === 'POST') return { data: { id: 'same-post' } }
      return { data: [] }
    })
    const wrapper = mountCompose()
    await fillForm(wrapper)
    expect(useSocialStore().pendingComposeUpload?.postId).toBeNull()
    expect(wrapper.get('[role="alert"]').text()).toContain('no longer exists')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).disabled).toBe(false)
    fetchMock.mockImplementation(async (url: string, options?: { method?: string }) => {
      if (url.endsWith('/image')) return { data: { path: 'ok' } }
      if (options?.method === 'POST') return { data: { id: 'new-post' } }
      return { data: [] }
    })
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.filter(([url, options]) => url === '/api/v1/social/posts' && options?.method === 'POST')).toHaveLength(2)
  })

  it('restores an existing image after creation fails and can edit metadata before retry', async () => {
    fetchMock.mockRejectedValueOnce(new Error('creation failed'))
    // The existing image arrives after mount (the compose panel only applies prefilledImageId when it changes).
    const wrapper = mount(SocialPostCompose, { props: { prefilledImageId: null } })
    await wrapper.setProps({ prefilledImageId: 'existing-image' })
    await wrapper.get('textarea').setValue('First caption')
    await wrapper.get('input[type="datetime-local"]').setValue('2026-12-01T12:00')
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    wrapper.unmount()
    const reopened = mountCompose()
    expect(reopened.text()).toContain('ATTACHED FROM TRACKLIST')
    await reopened.get('textarea').setValue('Revised caption')
    fetchMock.mockImplementation(async (url: string, options?: { method?: string }) => {
      if (options?.method === 'POST') return { data: { id: 'same-post' } }
      return { data: [] }
    })
    await reopened.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    const lastCreate = fetchMock.mock.calls.filter(([, options]) => options?.method === 'POST').at(-1)!
    expect((lastCreate[1] as any).body.get('image_id')).toBe('existing-image')
    expect((lastCreate[1] as any).body.get('caption')).toBe('Revised caption')
    expect(useSocialStore().pendingComposeUpload).toBeNull()
  })

  it('does not silently abandon a created post when a retry has no image', async () => {
    const wrapper = mountCompose()
    await fillForm(wrapper)
    const store = useSocialStore()
    const recovery = store.pendingComposeUpload!
    const saved = await store.submitCompose({
      postType: recovery.postType, caption: recovery.caption,
      scheduledAt: recovery.scheduledAt, timezoneName: recovery.timezoneName,
    }, null)
    expect(saved).toBe(false)
    expect(store.pendingComposeUpload?.postId).toBe('same-post')
  })

  it('allows removing a rejected image and selecting a valid replacement without recreating metadata', async () => {
    const wrapper = mountCompose()
    await fillForm(wrapper)
    expect(wrapper.get('[role="alert"]').text()).toContain('image upload failed')
    const remove = wrapper.findAll('button').find(b => b.text() === '×')!
    expect((remove.element as HTMLButtonElement).matches(':disabled')).toBe(false)
    await remove.trigger('click')
    expect(wrapper.get('[data-testid="submit-btn"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    expect(useSocialStore().pendingComposeUpload?.postId).toBe('same-post')
    expect(wrapper.get('[role="alert"]').text()).toContain('image upload failed')
    const good = new File(['valid'], 'good.png', { type: 'image/png' })
    await chooseFile(wrapper, good)
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).matches(':disabled')).toBe(true)
    fetchMock.mockImplementation(async (url: string) => url.endsWith('/image') ? { data: { path: 'social/same-post/good.png' } } : { data: [] })
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    const creates = fetchMock.mock.calls.filter(([url, options]) => url === '/api/v1/social/posts' && options?.method === 'POST')
    expect(creates).toHaveLength(1)
    const uploads = fetchMock.mock.calls.filter(([url]) => url.endsWith('/image'))
    expect(uploads).toHaveLength(2)
    expect(uploads[1][0]).toBe('/api/v1/social/posts/same-post/image')
    expect((uploads[1][1] as any).body.get('image_file')).toBe(good)
  })

  it('restores pending id, metadata, file and error after actual tabs unmount/remount and list refresh', async () => {
    const store = useSocialStore()
    store.openComposePanel()
    const host = defineComponent({
      components: { SocialTabs }, setup: () => ({ store }),
      template: '<SocialTabs :posts="store.posts" :compose-panel-open="store.composePanelOpen" :prefilled-image-id="store.prefilledImageId" />',
    })
    const wrapper = mount(host)
    await fillForm(wrapper)
    const original = wrapper.getComponent(SocialPostCompose).vm.$.uid
    await wrapper.findAll('button').find(b => b.text() === 'CLOSE ×')!.trigger('click')
    expect(wrapper.findComponent(SocialPostCompose).exists()).toBe(false)
    await store.loadPosts()
    store.openComposePanel('must-not-overwrite-pending')
    await flushPromises()
    expect(wrapper.getComponent(SocialPostCompose).vm.$.uid).not.toBe(original)
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('Immutable caption')
    expect(wrapper.text()).toContain('bad.jpg')
    expect(wrapper.get('[role="alert"]').text()).toContain('image upload failed')
    await wrapper.findAll('button').find(b => b.text() === '×')!.trigger('click')
    const good = new File(['valid'], 'replacement.png', { type: 'image/png' })
    await chooseFile(wrapper, good)
    fetchMock.mockImplementation(async (url: string) => url.endsWith('/image') ? { data: { path: 'social/same-post/replacement.png' } } : { data: [] })
    await wrapper.get('[data-testid="submit-btn"]').trigger('click')
    await flushPromises()
    expect(fetchMock.mock.calls.filter(([url, options]) => url === '/api/v1/social/posts' && options?.method === 'POST')).toHaveLength(1)
    expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/image')).map(([url]) => url)).toEqual([
      '/api/v1/social/posts/same-post/image', '/api/v1/social/posts/same-post/image',
    ])
    expect(store.pendingComposeUpload).toBeNull()
    store.openComposePanel()
    await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('replacement.png')
  })
})
