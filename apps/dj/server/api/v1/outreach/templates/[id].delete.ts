import { deleteTemplate, idOf, send } from '../-mockDb'

export default defineEventHandler((event) => send(event, deleteTemplate(idOf(event))))
