import type { SegmentInput } from '~/types/audience'
import { updateAudienceSegment } from '../../-mockDb'
export default defineEventHandler(async event =>
  updateAudienceSegment(getRouterParam(event, 'segmentId')!, await readBody<SegmentInput>(event)))
