import { db } from '../_state'

export default defineEventHandler(() => ({ data: db.listTemplates() }))
