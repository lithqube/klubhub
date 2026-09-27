import { recoveryWrap } from '../../-mockSealed'
// GET /api/v1/keys/org/recovery (security.manage): the recovery wrap, public key and fingerprint.
export default defineEventHandler(event => recoveryWrap(event))
