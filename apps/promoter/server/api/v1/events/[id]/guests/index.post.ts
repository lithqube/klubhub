import type { AddGuestsInput } from '~/types/guest'
import { addGuests } from '../../../-mockDb'
export default defineEventHandler(async (event) => {
  const r = addGuests(getRouterParam(event, 'id')!, await readBody<AddGuestsInput>(event))
  setResponseStatus(event, 201)
  return r
})
