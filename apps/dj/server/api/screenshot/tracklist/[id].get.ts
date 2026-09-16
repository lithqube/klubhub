import type { Browser, BrowserContext } from 'playwright-core';

// Plan B.8 renderer hardening:
//   - try/finally context cleanup so failed screenshots do not leak contexts.
//   - Bounded render concurrency via a semaphore so a flood of
//     /api/screenshot requests cannot exhaust Chromium's per-process
//     context limit.
//   - The render URL is hard-coded to localhost; do NOT trust any
//     caller-supplied host. This blocks an SSRF redirect that points
//     the renderer at internal services.
//
// The semaphore is intentionally tiny (2 concurrent renders). It is a
// stateless map keyed by Playwright instance; the same Browser is
// shared across requests, which keeps startup fast but means we MUST
// cap how many contexts exist at once.

const RENDER_CONCURRENCY = 2

interface BrowserHandle {
  acquire(): Promise<() => void>
}

function makeSemaphore(max: number): BrowserHandle {
  let inFlight = 0
  const waiters: Array<() => void> = []
  return {
    acquire: () =>
      new Promise<() => void>((resolve) => {
        const tryAcquire = () => {
          if (inFlight < max) {
            inFlight++
            resolve(() => {
              inFlight--
              const next = waiters.shift()
              if (next) next()
            })
          } else {
            waiters.push(tryAcquire)
          }
        }
        tryAcquire()
      }),
  }
}

const semaphore = makeSemaphore(RENDER_CONCURRENCY)

export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')
  if (!id) throw createError({ statusCode: 400, message: 'id required' })

  const format = (getQuery(event).format as string) ?? 'story'
  const { width, height } =
    format === 'square'
      ? { width: 1080, height: 1080 }
      : { width: 1080, height: 1920 }

  const nitroApp = useNitroApp() as any
  const browser: Browser | null | undefined = nitroApp._playwright
  if (!browser) {
    throw createError({ statusCode: 503, message: 'Playwright not ready' })
  }

  // Acquire a render slot. If saturation is reached, fail fast with
  // 503 rather than queueing indefinitely — a self-hosted user can
  // retry, and queueing a slow request behind a slow request is the
  // exact way contexts pile up.
  const release = await semaphore.acquire()

  let context: BrowserContext | null = null
  try {
    context = await browser.newContext({
      viewport: { width, height },
      deviceScaleFactor: 2,
    })
    const page = await context.newPage()

    // The render URL is hard-coded to localhost — DO NOT derive it
    // from a request header. This is an SSRF defense.
    const port = process.env.PORT ?? '3000'
    const renderURL = `http://localhost:${port}/render/tracklist/${id}`

    await page.goto(renderURL, { waitUntil: 'networkidle', timeout: 15_000 })

    const buffer = await page.screenshot({ type: 'png', fullPage: false })

    setResponseHeader(event, 'Content-Type', 'image/png')
    return buffer
  } catch (err: any) {
    // Surface as a clean 502 rather than leaking the Playwright
    // stack trace to the caller. The browser context is closed in
    // the finally block regardless of success or failure.
    throw createError({
      statusCode: 502,
      statusMessage: 'render failed',
      message: (err && err.message) || 'render failed',
    })
  } finally {
    if (context) {
      try {
        await context.close()
      } catch {
        // best-effort: the browser is shut down on hook 'close'
      }
    }
    release()
  }
})
