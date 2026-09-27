import { setupOrg } from '../../-mockSealed'
// POST /api/v1/keys/org/setup (security.manage): version 1, recovery wrap, wraps incl. the caller; 409 when set up.
export default defineEventHandler(async (event) => {
  setupOrg(event, await readBody(event))
  setResponseStatus(event, 201)
  return null
})
