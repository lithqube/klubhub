import { listViews } from '../../../-mockDb'
export default defineEventHandler(event => listViews(getRouterParam(event, 'id')!))
