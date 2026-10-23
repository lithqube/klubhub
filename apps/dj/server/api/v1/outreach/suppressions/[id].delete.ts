import { deleteSuppression, idOf, send } from '../-mockDb'

export default defineEventHandler((event) => send(event, deleteSuppression(idOf(event))))
