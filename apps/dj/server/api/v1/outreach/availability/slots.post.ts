import { bodyOf, createSlot, send } from '../-mockDb'

export default defineEventHandler(async (event) => send(event, createSlot(await bodyOf(event))))
