import { findEvent, orgProfile } from '../../../-mockDb'
export default defineEventHandler((event) => {
  const e = findEvent(getRouterParam(event, 'id')!)
  if (e.visibility === 'private') throw createError({ statusCode: 409, data: { error: 'private_event' } })
  return {
    '@context': 'https://schema.org', '@type': 'MusicEvent', name: e.title, startDate: e.starts_at, endDate: e.ends_at,
    eventStatus: 'https://schema.org/EventScheduled', eventAttendanceMode: 'https://schema.org/OfflineEventAttendanceMode',
    location: { '@type': 'Place', name: e.city, address: { '@type': 'PostalAddress', addressLocality: e.city } },
    performer: [...e.lineup].sort((a, b) => a.billing_order - b.billing_order).map(l => ({ '@type': 'Person', name: l.display_name })),
    organizer: {
      '@type': 'Organization', name: 'Nachtwerk Collective', url: orgProfile.website_url ?? undefined,
      sameAs: [orgProfile.instagram_url, orgProfile.soundcloud_url, orgProfile.ra_url].filter(Boolean),
    },
  }
})
