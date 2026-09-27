import { getMemberKey } from '../-mockSealed'
// GET /api/v1/keys/me (account.self): the caller's member key, or 404 no_member_key.
export default defineEventHandler(event => getMemberKey(event))
