import { eventReport } from '../../-mockDb'
export default defineEventHandler(event => eventReport(getRouterParam(event, 'id')!))
