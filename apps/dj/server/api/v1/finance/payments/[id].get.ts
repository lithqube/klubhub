import { fail, payments } from '../-mockDb'

export default defineEventHandler((event) => {
  const p = payments.get(getRouterParam(event, 'id') ?? '')
  if (!p) return fail(event, 404, 'not_found', 'payment not found')
  return { data: p }
})
