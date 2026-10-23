import { listTemplates, ok, send } from './-mockDb'

export default defineEventHandler((event) => send(event, ok(listTemplates())))
