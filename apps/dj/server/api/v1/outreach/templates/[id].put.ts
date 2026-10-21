import { bodyOf, idOf, send, updateTemplate } from '../-mockDb'

export default defineEventHandler(async (event) => send(event, updateTemplate(idOf(event), await bodyOf(event))))
