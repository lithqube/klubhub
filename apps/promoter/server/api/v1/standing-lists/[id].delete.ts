import { guestLists, standingLists } from '../-mockDb'
export default defineEventHandler((event) => {
  const i = standingLists.findIndex(x => x.id === getRouterParam(event, 'id'))
  if (i < 0) throw createError({ statusCode: 404, data: { error: 'not_found' } })
  const [s] = standingLists.splice(i, 1)
  for (const l of guestLists) if (l.standing_template_id === s!.id) l.standing_template_id = null
  setResponseStatus(event, 204)
  return null
})
