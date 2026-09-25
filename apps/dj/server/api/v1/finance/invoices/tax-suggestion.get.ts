import { suggest } from '../-mockDb'

export default defineEventHandler((event) => {
  const q = getQuery(event)
  return {
    data: suggest({
      country: String(q.customer_country ?? '').toUpperCase(),
      vat_id: String(q.customer_vat_id ?? ''),
      is_business: String(q.customer_is_business ?? 'false') === 'true',
    }),
  }
})
