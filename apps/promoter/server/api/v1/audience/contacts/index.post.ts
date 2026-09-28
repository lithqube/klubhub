import type { ContactInput } from '~/types/audience'
import { createAudienceContact } from '../../-mockDb'
export default defineEventHandler(async (event) => {
  const r = createAudienceContact(await readBody<ContactInput>(event))
  setResponseStatus(event, 201)
  return r
})
