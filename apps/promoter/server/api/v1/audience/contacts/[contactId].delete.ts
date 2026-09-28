import { deleteAudienceContact } from '../../-mockDb'
export default defineEventHandler((event) => {
  deleteAudienceContact(getRouterParam(event, 'contactId')!)
  setResponseStatus(event, 204)
})
