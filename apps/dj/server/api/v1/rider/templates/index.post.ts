import { db, unwrap } from '../_state'

export default defineEventHandler(async (event) => {
  const created = unwrap(db.createTemplate((await readBody(event)) ?? {}))
  setResponseStatus(event, 201)
  return { data: created }
})
