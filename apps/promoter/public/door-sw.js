/*
 * KlubHub Door service worker (P2.3). Registered from /door in production
 * only, with scope /door, so it never controls the admin pages.
 *
 * - /_nuxt/* (hashed build assets): cache first — they never change.
 * - The /door document: network first, cached copy when offline.
 * - /api/* and everything else: not touched (the door keeps its own
 *   encrypted data in IndexedDB; API responses are never cached here).
 *
 * The page posts {type: 'door-cache', urls} after load so assets fetched
 * before this worker took control (and the lazy jsQR chunk) are cached too.
 */
const CACHE = 'klubhub-door-v1'

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then(c => c.add('/door')).catch(() => undefined).then(() => self.skipWaiting()))
})

self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    for (const k of await caches.keys()) if (k.startsWith('klubhub-door-') && k !== CACHE) await caches.delete(k)
    await self.clients.claim()
  })())
})

const isAsset = url => url.origin === self.location.origin && url.pathname.startsWith('/_nuxt/')
const isDoorPage = url => url.origin === self.location.origin && (url.pathname === '/door' || url.pathname === '/door/')

self.addEventListener('message', (event) => {
  const d = event.data
  if (!d || d.type !== 'door-cache' || !Array.isArray(d.urls)) return
  const urls = d.urls.map(u => new URL(u, self.location.origin)).filter(u => isAsset(u) || isDoorPage(u))
  event.waitUntil(caches.open(CACHE).then(c => Promise.all(urls.map(u => c.match(u).then(hit => hit || c.add(u).catch(() => undefined))))))
})

self.addEventListener('fetch', (event) => {
  const req = event.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  if (url.pathname.startsWith('/api/')) return

  if (isAsset(url)) {
    event.respondWith(caches.open(CACHE).then(async (c) => {
      const hit = await c.match(req)
      if (hit) return hit
      const res = await fetch(req)
      if (res.ok) c.put(req, res.clone())
      return res
    }))
    return
  }

  if (req.mode === 'navigate' && isDoorPage(url)) {
    event.respondWith((async () => {
      const c = await caches.open(CACHE)
      try {
        const res = await fetch(req)
        if (res.ok) c.put('/door', res.clone())
        return res
      } catch {
        return (await c.match('/door')) || Response.error()
      }
    })())
  }
})
