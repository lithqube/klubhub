import { rotate } from '../../-mockSealed'
// POST /api/v1/keys/org/rotate (security.manage): atomic version switch with every ban entry re-encrypted.
export default defineEventHandler(async (event) => {
  rotate(event, await readBody(event))
  setResponseStatus(event, 204)
  return null
})
