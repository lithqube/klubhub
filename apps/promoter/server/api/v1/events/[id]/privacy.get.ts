import { eventPrivacy } from '../../-mockDb'
export default defineEventHandler(event => eventPrivacy(getRouterParam(event, 'id')!))
