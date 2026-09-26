import { venues } from '../-mockDb'
export default defineEventHandler((event) => {
  const v = venues.find(x => x.id === getRouterParam(event, 'id'))
  if (!v) throw createError({ statusCode: 404, data: { error: 'not_found' } })
  return v
})
