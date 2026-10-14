// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { createDemoBackend } from '../app/demo/backend'

// Genuine 320x320 PNG encoded by Pillow (1-bit black image).
const pngBytes = Uint8Array.from(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAUAAAAFAAQAAAADl65gHAAAAI0lEQVR4nO3BAQ0AAADCoPdPbQ8HFAAAAAAAAAAAAAAAAPwaM0AAASn9scsAAAAASUVORK5CYII=', 'base64'))
function validPng() { return new Blob([pngBytes], { type: 'image/png' }) }

function metadata() {
  const form = new FormData()
  form.set('caption', 'Two request flow')
  form.set('post_type', 'feed')
  form.set('scheduled_at', '2026-12-01T12:00')
  form.set('timezone_name', 'UTC')
  return form
}
function backend() {
  return createDemoBackend({ storage: null, baseURL: '/demo/', now: () => new Date('2026-10-01T12:00:00Z'), createObjectUrl: () => 'blob:test' })
}
async function call(b: ReturnType<typeof backend>, method: string, path: string, body: unknown = null) {
  const res = await b.handle({ method, path, body, query: new URLSearchParams() })
  return { ...res, body: res.body as any }
}

describe('registered social demo image contract', () => {
  it('creates metadata then uploads image_file to the same post and reads the uploaded bytes', async () => {
    const b = backend()
    const count = b.state.posts.length
    const created = await call(b, 'POST', '/api/v1/social/posts', metadata())
    expect(created.status).toBe(201)
    expect(created.body.data.imageStorageKey).toBe('')
    const id = created.body.data.id
    const file = validPng()
    const form = new FormData()
    form.set('image_file', file, 'image.png')
    const upload = await call(b, 'POST', `/api/v1/social/posts/${id}/image`, form)
    expect(upload.status).toBe(200)
    expect(upload.body).toEqual({ data: { path: expect.stringContaining(`demo/social/${id}/`) } })
    expect(b.state.posts).toHaveLength(count + 1)
    const list = await call(b, 'GET', '/api/v1/social/posts')
    expect(list.body.data.find((p: any) => p.id === id).imageStorageKey).toBe(upload.body.data.path)
    const image = await call(b, 'GET', `/api/v1/social/posts/${id}/image`)
    expect(new Uint8Array(await image.blob!.arrayBuffer())).toEqual(pngBytes)
    expect(image.blob!.type).toBe('image/png')
  })

  it('rejects retired inline file creation without scheduling a post', async () => {
    const b = backend()
    const count = b.state.posts.length
    const form = metadata()
    form.set('image_file', new Blob(['image']), 'inline.png')
    expect((await call(b, 'POST', '/api/v1/social/posts', form)).status).toBe(422)
    expect(b.state.posts).toHaveLength(count)
  })

  it('rejects text disguised as PNG and unsupported images without mutation', async () => {
    const b = backend()
    const created = await call(b, 'POST', '/api/v1/social/posts', metadata())
    const id = created.body.data.id
    for (const file of [new Blob(['text'], { type: 'image/png' }), new Blob(['<svg/>'], { type: 'image/svg+xml' })]) {
      const form = new FormData()
      form.set('image_file', file, 'image.png')
      expect((await call(b, 'POST', `/api/v1/social/posts/${id}/image`, form)).status).toBe(422)
      expect(b.state.posts.find(p => p.id === id)?.imageStorageKey).toBe('')
    }
  })

  it('blocks image changes outside scheduled lifecycle', async () => {
    const b = backend()
    const created = await call(b, 'POST', '/api/v1/social/posts', metadata())
    const post = b.state.posts.find(p => p.id === created.body.data.id)!
    for (const status of ['publishing', 'published', 'failed', 'draft'] as const) {
      post.status = status
      const form = new FormData()
      form.set('image_file', validPng(), 'valid.png')
      expect((await call(b, 'POST', `/api/v1/social/posts/${post.id}/image`, form)).status).toBe(409)
      expect(post.imageStorageKey).toBe('')
    }
  })

  it('rejects missing and empty uploads and permits valid retry without changing metadata', async () => {
    const b = backend()
    const created = await call(b, 'POST', '/api/v1/social/posts', metadata())
    const id = created.body.data.id
    const endpoint = `/api/v1/social/posts/${id}/image`
    expect((await call(b, 'POST', endpoint, new FormData())).status).toBe(400)
    const form = new FormData()
    form.set('image_file', new Blob([]), 'empty.png')
    expect((await call(b, 'POST', endpoint, form)).status).toBe(422)
    form.set('image_file', validPng(), 'valid.png')
    expect((await call(b, 'POST', endpoint, form)).status).toBe(200)
    expect(b.state.posts.find(p => p.id === id)?.caption).toBe('Two request flow')
    expect((await call(b, 'POST', '/api/v1/social/posts/missing/image', form)).status).toBe(404)
  })
})
