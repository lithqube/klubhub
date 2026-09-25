// /api/v1/social/* — simulated Instagram scheduling. Nothing is sent to
// Instagram: due posts flip to "published" when the queue is read.

import { uuid } from '../../../shared/finance-mock/rules'
import { postSvg, svgDataUrl } from '../lib/art'
import { dataUrlToBlob, imageDataUrl, TooLargeError, zonedToUtc } from '../lib/util'
import type { DemoRouter } from '../router'
import { apiError, json, noContent, notFound } from '../router'
import { seedAccount } from '../seed/content'
import type { DemoContext, DemoPost } from '../types'

/** Post image as a data URL: uploaded/exported image or a generated card. */
export function postImage(post: DemoPost): string {
  return post.imageData || svgDataUrl(postSvg(post.caption || 'Scheduled post'))
}

/** Public shape: the stored image data stays internal. */
function view(p: DemoPost) {
  const { imageData: _img, ...rest } = p
  return rest
}

/** Simulated publisher: scheduled posts that are due become published. */
function publishDue(c: DemoContext): void {
  const now = c.now()
  for (const p of c.state.posts) {
    if (p.status === 'scheduled' && Date.parse(p.scheduledAtUtc) <= now.getTime()) {
      p.status = 'published'
      p.updatedAt = now.toISOString()
    }
  }
}

async function imageFrom(c: DemoContext, form: FormData): Promise<string | undefined> {
  const file = form.get('image_file')
  if (file instanceof Blob && file.size > 0) return imageDataUrl(file)
  const id = form.get('image_id')
  if (typeof id === 'string' && id) {
    const blob = c.blobFor(id)
    if (blob) return imageDataUrl(blob)
    if (id.startsWith('data:image/')) return id
  }
  return undefined
}

export function registerSocial(r: DemoRouter): void {
  r.on('GET', '/api/v1/social/accounts', (_req, c) => json({ data: c.state.account }))

  r.on('DELETE', '/api/v1/social/accounts/:id', (req, c) => {
    if (!c.state.account || c.state.account.id !== req.params.id) return notFound('account not found')
    c.state.account = null
    return noContent()
  })

  // "Connect Instagram": connects the fictional account and returns to the page.
  r.on('GET', '/api/v1/social/auth/url', (_req, c) => {
    c.state.account = seedAccount(c.now())
    return json({ url: `${c.baseURL}social?connected=demo` })
  })

  r.on('GET', '/api/v1/social/posts', (_req, c) => {
    publishDue(c)
    const posts = [...c.state.posts].sort((a, b) => a.scheduledAtUtc.localeCompare(b.scheduledAtUtc))
    return json({ data: posts.map(view) })
  })

  r.on('POST', '/api/v1/social/posts', async (req, c) => {
    const form = req.body instanceof FormData ? req.body : null
    if (!form) return apiError(400, 'expected multipart form data')
    const caption = String(form.get('caption') ?? '')
    const scheduledAt = String(form.get('scheduled_at') ?? '')
    const tz = String(form.get('timezone_name') ?? 'UTC')
    const postType = form.get('post_type') === 'story' ? 'story' : 'feed'
    if (!scheduledAt) return apiError(400, 'scheduled_at is required')
    if (postType === 'feed' && caption.length > 2200) return apiError(400, 'caption exceeds 2200 characters')
    let imageData: string | undefined
    try {
      imageData = await imageFrom(c, form)
    } catch (e) {
      return apiError(e instanceof TooLargeError ? 413 : 400, e instanceof Error ? e.message : 'invalid image')
    }
    const now = c.now().toISOString()
    const post: DemoPost = {
      id: uuid(),
      accountId: String(form.get('account_id') ?? c.state.account?.id ?? ''),
      status: 'scheduled',
      postType,
      caption,
      imageMinioPath: `demo/social/${uuid()}`,
      scheduledAtUtc: zonedToUtc(scheduledAt, tz),
      timezoneName: tz,
      retryCount: 0,
      nextRetryAt: null,
      lastError: '',
      createdAt: now,
      updatedAt: now,
      imageData,
    }
    c.state.posts.push(post)
    return json({ data: view(post) }, 201)
  })

  r.on('PUT', '/api/v1/social/posts/:id', (req, c) => {
    const post = c.state.posts.find((p) => p.id === req.params.id)
    if (!post) return notFound('post not found')
    const body = (req.body ?? {}) as Record<string, unknown>
    if (typeof body.caption === 'string') post.caption = body.caption
    if (typeof body.timezoneName === 'string' && body.timezoneName) post.timezoneName = body.timezoneName
    if (typeof body.scheduledAt === 'string' && body.scheduledAt) post.scheduledAtUtc = zonedToUtc(body.scheduledAt, post.timezoneName)
    post.updatedAt = c.now().toISOString()
    return json({ data: view(post) })
  })

  r.on('DELETE', '/api/v1/social/posts/:id', (req, c) => {
    const i = c.state.posts.findIndex((p) => p.id === req.params.id)
    if (i === -1) return notFound('post not found')
    c.state.posts.splice(i, 1)
    return noContent()
  })

  r.on('POST', '/api/v1/social/posts/:id/retry', (req, c) => {
    const post = c.state.posts.find((p) => p.id === req.params.id)
    if (!post) return notFound('post not found')
    post.status = 'published'
    post.lastError = ''
    post.nextRetryAt = null
    post.retryCount += 1
    post.updatedAt = c.now().toISOString()
    return json({ data: view(post) })
  })

  r.on('GET', '/api/v1/social/posts/:id/image', (req, c) => {
    const post = c.state.posts.find((p) => p.id === req.params.id)
    if (!post) return notFound('post not found')
    return { status: 200, blob: dataUrlToBlob(postImage(post)) }
  })
}
