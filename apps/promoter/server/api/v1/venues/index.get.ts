import { venues } from '../-mockDb'
export default defineEventHandler(() => venues.filter(v => !v.archived_at))
