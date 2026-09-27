import { banList } from '../-mockSealed'
// GET /api/v1/ban-list (guestlist.read): ciphertext only, expired entries excluded.
export default defineEventHandler(event => banList(event))
