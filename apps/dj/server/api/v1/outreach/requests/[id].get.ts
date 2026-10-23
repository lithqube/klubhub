import { getRequest, idOf, send } from '../-mockDb'

export default defineEventHandler((event) => send(event, getRequest(idOf(event))))
