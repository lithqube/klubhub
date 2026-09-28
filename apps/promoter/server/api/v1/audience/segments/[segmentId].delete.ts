import { deleteAudienceSegment } from '../../-mockDb'
export default defineEventHandler((event) => {
  deleteAudienceSegment(getRouterParam(event, 'segmentId')!)
  setResponseStatus(event, 204)
})
