import { listSlots, ok, send } from '../-mockDb'

export default defineEventHandler((event) => send(event, ok(listSlots())))
