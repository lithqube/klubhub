import type { GuestInput } from '~/types/guest'
import { updateGuest } from '../../../-mockDb'
export default defineEventHandler(async event =>
  updateGuest(getRouterParam(event, 'id')!, getRouterParam(event, 'guestId')!, await readBody<GuestInput>(event)))
