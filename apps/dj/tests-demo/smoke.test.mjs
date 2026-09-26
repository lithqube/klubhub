/**
 * Browser smoke test for the static demo build (docs/DEMO.md).
 *
 * Serves dist/demo under /demo/ from a local static server, visits every
 * app page in Chromium, and asserts: no console errors, no requests leaving
 * the local server, and a draft invoice can be created from a played gig.
 *
 *   pnpm exec playwright install chromium     (once)
 *   node --test apps/dj/tests-demo/smoke.test.mjs
 *
 * Builds the demo first when dist/demo is missing (DEMO_REBUILD=1 forces it).
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { existsSync, readFileSync, statSync } from 'node:fs'
import { extname, join, normalize } from 'node:path'
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { chromium } from '@playwright/test'

const root = fileURLToPath(new URL('../../../', import.meta.url))
const dist = join(root, 'dist/demo')
if (process.env.DEMO_REBUILD === '1' || !existsSync(join(dist, 'index.html'))) {
  execFileSync(process.execPath, [join(root, 'apps/dj/scripts/build-demo.mjs')], { stdio: 'inherit' })
}

const TYPES = {
  '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json',
  '.woff2': 'font/woff2', '.ico': 'image/x-icon', '.svg': 'image/svg+xml', '.txt': 'text/plain',
}

/** GitHub Pages-like static server: /demo/<path>, <path>/index.html, 404.html. */
function serve() {
  const server = createServer((req, res) => {
    const url = new URL(req.url, 'http://localhost')
    if (!url.pathname.startsWith('/demo/')) {
      res.writeHead(url.pathname === '/demo' ? 301 : 404, { location: '/demo/' }).end()
      return
    }
    const rel = normalize(decodeURIComponent(url.pathname.slice('/demo/'.length))).replace(/^(\.\.[/\\])+/, '')
    let file = join(dist, rel)
    if (existsSync(file) && statSync(file).isDirectory()) file = join(file, 'index.html')
    if (!existsSync(file) && existsSync(join(file, 'index.html'))) file = join(file, 'index.html')
    const found = existsSync(file) && statSync(file).isFile()
    const body = readFileSync(found ? file : join(dist, '404.html'))
    res.writeHead(found ? 200 : 404, { 'content-type': TYPES[extname(found ? file : '.html')] ?? 'application/octet-stream' })
    res.end(body)
  })
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve(server)))
}

const PAGES = [
  { path: '', text: 'UPCOMING' },
  { path: 'tracklist', text: 'RECENT IMPORTS' },
  { path: 'social', text: 'sam.example' },
  { path: 'epk', text: 'Sam Example' },
  { path: 'gigs', text: 'Zeta Garden' },
  { path: 'finance', text: 'INVOICES' },
]

test('demo build works offline in the browser', async (t) => {
  const server = await serve()
  const base = `http://127.0.0.1:${server.address().port}/demo/`
  const browser = await chromium.launch()
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } })
  const errors = []
  const external = []
  context.on('request', (r) => {
    const u = r.url()
    if (!u.startsWith(`http://127.0.0.1:${server.address().port}/`) && !/^(data|blob):/.test(u)) external.push(u)
  })
  // Never let a request escape, even if the assertion above would catch it.
  await context.route((url) => url.hostname !== '127.0.0.1', (route) => route.abort())
  const page = await context.newPage()
  page.on('console', (m) => { if (m.type() === 'error') errors.push(`${page.url()}: ${m.text()}`) })
  page.on('pageerror', (e) => errors.push(`${page.url()}: ${e.message}`))

  try {
    for (const p of PAGES) {
      await t.test(`/demo/${p.path} renders`, async () => {
        const res = await page.goto(base + p.path, { waitUntil: 'networkidle' })
        assert.equal(res.status(), 200)
        await page.getByTestId('demo-banner').waitFor()
        await page.getByText(p.text, { exact: false }).first().waitFor({ timeout: 10000 })
      })
    }

    await t.test('tracklist sample file parses into the editor', async () => {
      await page.goto(base + 'tracklist', { waitUntil: 'networkidle' })
      await page.getByTestId('demo-sample-tracklist').click()
      await page.getByText('EXPORT_IMAGES').waitFor()
    })

    await t.test('creates a draft invoice from a played gig', async () => {
      await page.goto(base + 'finance', { waitUntil: 'networkidle' })
      await page.getByRole('button', { name: '+ NEW INVOICE' }).click()
      const picker = page.getByRole('combobox')
      await picker.click()
      await picker.fill('Open Air')
      await page.getByRole('option', { name: /Open Air/ }).dispatchEvent('mousedown')
      await page.getByRole('button', { name: 'CREATE DRAFT' }).click()
      await page.waitForFunction(() => {
        const s = JSON.parse(localStorage.getItem('klubhub-demo:v1') ?? '{}')
        return (s.finance?.invoices ?? []).some((i) => i.gig_id === '00000000-0000-4000-8000-000000000106' && i.status === 'draft')
      })
    })

    await t.test('reset restores the seed', async () => {
      await page.goto(base + 'gigs', { waitUntil: 'networkidle' })
      await page.getByTestId('demo-reset').click()
      await Promise.all([page.waitForEvent('load'), page.getByTestId('demo-reset').click()])
      await page.waitForLoadState('networkidle')
      const drafts = await page.evaluate(() => {
        const s = JSON.parse(localStorage.getItem('klubhub-demo:v1') ?? '{}')
        return (s.finance?.invoices ?? []).filter((i) => i.gig_id === '00000000-0000-4000-8000-000000000106').length
      })
      assert.equal(drafts, 0)
    })

    assert.deepEqual(errors, [], 'no console errors')
    assert.deepEqual(external, [], 'no requests leave the local server')
  } finally {
    await browser.close()
    server.close()
  }
})
