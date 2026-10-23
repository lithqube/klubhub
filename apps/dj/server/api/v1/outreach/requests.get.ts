import { listRequests, ok, send } from './-mockDb'

export default defineEventHandler((event) => send(event, ok(listRequests())))
