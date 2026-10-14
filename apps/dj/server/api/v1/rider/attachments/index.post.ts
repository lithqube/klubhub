import { db, unwrap } from '../_state'

export default defineEventHandler(async (event) => {
  // The dev server cannot see the gig store, so every gig counts as live.
  const created = unwrap(db.createAttachment((await readBody(event)) ?? {}, true))
  setResponseStatus(event, 201)
  return { data: created }
})
