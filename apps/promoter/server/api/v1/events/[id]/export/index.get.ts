import { hasErrors } from '~/utils/timetable'
import { findEvent, orgProfile, venues } from '../../../-mockDb'
export default defineEventHandler((event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  if (hasErrors(e.issues)) throw createError({ statusCode: 409, data: { error: 'timetable_errors', issues: e.issues } })
  const venue = venues.find(v => v.id === e.venue_id)
  const now = Date.now()
  const hidden = e.location_mode === 'city_only' || (e.location_mode === 'secret' && !(e.location_reveal_at && now >= Date.parse(e.location_reveal_at)))
  const location = hidden
    ? { name: e.location_mode === 'city_only' ? e.city : `${e.city} — location TBA`, city: e.city, withheld: true, reveal_at: e.location_reveal_at ?? undefined }
    : { name: venue?.name ?? e.city, city: e.city, country: venue?.country ?? undefined, address: venue ? 'Musterstraße 1, 10999 Berlin' : undefined, withheld: false }
  const { venue: _v, stages, lineup, issues: _i, ...ev } = e
  return {
    event: ev, stages, lineup: [...lineup].sort((a, b) => a.billing_order - b.billing_order), location,
    organizer: 'Nachtwerk Collective',
    organizer_profile: {
      name: 'Nachtwerk Collective', url: orgProfile.website_url ?? undefined, bio: orgProfile.bio || undefined,
      same_as: [orgProfile.instagram_url, orgProfile.soundcloud_url, orgProfile.ra_url].filter((l): l is string => !!l),
      accent_color: orgProfile.accent_color ?? undefined,
    },
    embargoed: e.status === 'draft' || (!!e.publish_at && now < Date.parse(e.publish_at)),
  }
})
