import { addSuppression, bodyOf, send } from './-mockDb'

export default defineEventHandler(async (event) => send(event, addSuppression(await bodyOf(event))))
