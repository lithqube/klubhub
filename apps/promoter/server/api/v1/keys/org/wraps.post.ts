import { addWraps } from '../../-mockSealed'
// POST /api/v1/keys/org/wraps (security.manage): add wraps for the active version.
export default defineEventHandler(async (event) => {
  addWraps(event, await readBody(event))
  setResponseStatus(event, 204)
  return null
})
