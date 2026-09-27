import { doorPinStatus } from '../../../-mockDb'
export default defineEventHandler(event => doorPinStatus(getRouterParam(event, 'id')!))
