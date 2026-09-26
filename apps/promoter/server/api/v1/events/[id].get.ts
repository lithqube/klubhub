import { findEvent } from '../-mockDb'
export default defineEventHandler(event => findEvent(getRouterParam(event, 'id')!))
