import { deleteSlot, idOf, send } from '../../-mockDb'

export default defineEventHandler((event) => send(event, deleteSlot(idOf(event))))
