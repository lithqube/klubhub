import { orgKeyInfo } from '../-mockSealed'
// GET /api/v1/keys/org (account.self): status, active version, the caller's wrap, recovery fingerprint.
export default defineEventHandler(event => orgKeyInfo(event))
