import type { ContactInput } from '~/types/audience'
import { updateAudienceContact } from '../../-mockDb'
export default defineEventHandler(async event =>
  updateAudienceContact(getRouterParam(event, 'contactId')!, await readBody<ContactInput>(event)))
