// Mock of the audited reveal; like the real policy, it needs a second factor.
import { mockSession } from '../../-mockDb'
export default defineEventHandler(() => {
  if (!mockSession.mfa) throw createError({ statusCode: 403, data: { error: 'mfa_required' } })
  return { address: 'Musterstraße 1, 10999 Berlin', geo: '', contact_name: 'Sam Production', contact_email: 'prod@example.org', contact_phone: '+49 30 0000000' }
})
