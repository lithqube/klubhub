import { listSuppressions, ok, send } from './-mockDb'

export default defineEventHandler((event) => send(event, ok(listSuppressions())))
