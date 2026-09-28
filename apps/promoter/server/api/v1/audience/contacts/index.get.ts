import { audiencePage } from '../../-mockDb'
export default defineEventHandler((event) => {
  const q = getQuery(event)
  return audiencePage({ status: q.status as string | undefined, source: q.source as string | undefined, q: q.q as string | undefined })
})
