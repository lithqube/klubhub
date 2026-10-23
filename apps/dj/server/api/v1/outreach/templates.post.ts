import { bodyOf, createTemplate, send } from './-mockDb'

export default defineEventHandler(async (event) => send(event, createTemplate(await bodyOf(event))))
