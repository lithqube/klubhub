import { audiencePage } from '../../-mockDb'
import { toCsv } from '~/utils/guests'
export default defineEventHandler((event) => {
  const q = getQuery(event)
  const page = audiencePage({ status: q.status as string | undefined, source: q.source as string | undefined })
  const rows = page.contacts.map(c => [c.name, c.email, c.phone, c.status, c.source, c.consent_basis, c.consent_recorded_at])
  setHeader(event, 'Content-Type', 'text/csv; charset=utf-8')
  setHeader(event, 'Content-Disposition', 'attachment; filename="audience.csv"')
  return '﻿' + toCsv([['name', 'email', 'phone', 'status', 'source', 'consent_basis', 'consent_recorded_at'], ...rows])
})
