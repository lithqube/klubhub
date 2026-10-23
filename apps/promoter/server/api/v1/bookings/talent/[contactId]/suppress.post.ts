import { suppressTalent } from '../../../-mockDb'
// POST /api/v1/bookings/talent/{contactId}/suppress (talent.write): one-click
// opt-out — the contact's email joins the do-not-contact list.
export default defineEventHandler((event) => {
  const r = suppressTalent(getRouterParam(event, 'contactId')!)
  setResponseStatus(event, 201)
  return r
})
