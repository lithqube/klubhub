// /api/v1/settings — profile and tracklist-card preferences.

import type { DemoRouter } from '../router'
import { apiError, json } from '../router'
import { imageDataUrl, TooLargeError } from '../lib/util'

// Nested card preferences (TracklistCustomizer) are stored flat, the way
// GET /settings returns them.
const PREF_KEYS = ['preset', 'bg_mode', 'visible_fields', 'max_tracks', 'track_range_start', 'track_range_end', 'logo_position']

export function registerSettings(r: DemoRouter): void {
  r.on('GET', '/api/v1/settings', (_req, c) => json(c.state.settings))

  r.on('PUT', '/api/v1/settings', (req, c) => {
    const body = (req.body && typeof req.body === 'object' ? req.body : null) as Record<string, unknown> | null
    if (!body) return apiError(400, 'invalid JSON body')
    const s = c.state.settings
    // Same optimistic-concurrency rule as the API when a token is sent.
    if (typeof body.updated_at === 'string' && body.updated_at !== s.updated_at) {
      return apiError(409, 'settings were updated elsewhere; reload and try again')
    }
    const prefs = body.tracklist_preferences as Record<string, unknown> | undefined
    const { updated_at: _token, tracklist_preferences: _prefs, ...rest } = body
    Object.assign(s, rest)
    if (prefs && typeof prefs === 'object') {
      for (const k of PREF_KEYS) if (k in prefs) s[k] = prefs[k]
    }
    s.updated_at = c.now().toISOString()
    return json(s)
  })

  r.on('POST', '/api/v1/settings/logo', async (req) => {
    const form = req.body instanceof FormData ? req.body : null
    try {
      // The card renders logo_path directly, so the "path" is the data URL.
      return json({ path: await imageDataUrl(form?.get('file')) })
    } catch (e) {
      return apiError(e instanceof TooLargeError ? 413 : 400, e instanceof Error ? e.message : 'upload failed')
    }
  })
}
