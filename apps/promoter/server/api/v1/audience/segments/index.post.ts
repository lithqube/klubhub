import type { SegmentInput } from '~/types/audience'
import { createAudienceSegment } from '../../-mockDb'
export default defineEventHandler(async (event) => {
  const r = createAudienceSegment(await readBody<SegmentInput>(event))
  setResponseStatus(event, 201)
  return r
})
