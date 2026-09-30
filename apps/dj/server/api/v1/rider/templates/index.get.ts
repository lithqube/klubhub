import { listTemplates } from '../_state'

export default defineEventHandler(() => ({ data: listTemplates() }))