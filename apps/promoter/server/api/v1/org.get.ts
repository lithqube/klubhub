import type { Organization } from '~/types/org'

// Mock for frontend-only development (no NUXT_PUBLIC_API_BASE).
export default defineEventHandler((): Organization => ({
  id: '0190f1d2-7c1a-7a00-9f00-000000000001',
  name: 'Nachtwerk Collective',
  slug: 'nachtwerk',
  timezone: 'Europe/Berlin',
  currency: 'EUR',
}))
