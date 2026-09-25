// Mock of the audited reveal; the real API requires a second factor.
export default defineEventHandler(() => ({
  address: 'Musterstraße 1, 10999 Berlin', geo: '', contact_name: 'Sam Production', contact_email: 'prod@example.org', contact_phone: '+49 30 0000000',
}))
