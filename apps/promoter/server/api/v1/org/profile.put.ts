import type { OrgProfile } from '~/types/org'
import { mockOrg, orgProfile } from '../-mockDb'

// Mirrors identity.Profile.normalise in the Go API.
export default defineEventHandler(async (event) => {
  const b = await readBody<OrgProfile>(event)
  const invalid = (field: string, problem: string) => createError({ statusCode: 422, data: { error: 'invalid', field, problem } })
  const bio = (b.bio ?? '').trim()
  if ([...bio].length > 2000) throw invalid('bio', 'at most 2000 characters')
  const links: Partial<OrgProfile> = {}
  for (const k of ['website_url', 'instagram_url', 'soundcloud_url', 'ra_url'] as const) {
    const v = (b[k] ?? '').trim()
    if (v && (!/^https:\/\/[^/@\s]+/.test(v) || v.length > 300)) throw invalid(k, 'an https:// link, at most 300 characters')
    links[k] = v || null
  }
  const accent = (b.accent_color ?? '').trim().toLowerCase()
  if (accent && !/^#[0-9a-f]{6}$/.test(accent)) throw invalid('accent_color', 'a hex colour like #96f8ff')
  Object.assign(orgProfile, { bio, ...links, accent_color: accent || null })
  return mockOrg()
})
