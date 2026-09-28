import type { ContactStatus } from '~/types/audience'
import { setAudienceStatus } from '../../../-mockDb'
export default defineEventHandler(async (event) => {
  const b = await readBody<{ status: ContactStatus }>(event)
  setAudienceStatus(getRouterParam(event, 'contactId')!, b.status)
})
