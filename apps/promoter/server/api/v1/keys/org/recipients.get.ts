import { recipients } from '../../-mockSealed'
// GET /api/v1/keys/org/recipients (security.manage).
export default defineEventHandler(event => recipients(event))
