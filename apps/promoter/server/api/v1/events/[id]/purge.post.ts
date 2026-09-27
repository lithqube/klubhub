import { purgeNow } from '../../-mockDb'

// Mirrors POST /api/v1/events/{id}/purge: {confirm: "<event title>"}, ended
// events only, recent sign-in (403 reauthentication_required otherwise).
export default defineEventHandler(async event => purgeNow(getRouterParam(event, 'id')!, await readBody(event)))
