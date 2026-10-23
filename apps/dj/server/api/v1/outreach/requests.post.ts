import { bodyOf, send, sendRequest } from './-mockDb'

export default defineEventHandler(async (event) => send(event, sendRequest(await bodyOf(event))))
