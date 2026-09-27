import { updateBan } from '../-mockSealed'
// PUT /api/v1/ban-list/{id} (guestlist.write).
export default defineEventHandler(async event => updateBan(event, getRouterParam(event, 'id')!, await readBody(event)))
