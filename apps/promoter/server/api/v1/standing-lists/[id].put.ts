import type { ListInput } from '~/types/guest'
import { standingLists, validateListInput } from '../-mockDb'
export default defineEventHandler(async (event) => {
  const s = standingLists.find(x => x.id === getRouterParam(event, 'id'))
  if (!s) throw createError({ statusCode: 404, data: { error: 'not_found' } })
  Object.assign(s, validateListInput(await readBody<ListInput>(event), true))
  return s
})
