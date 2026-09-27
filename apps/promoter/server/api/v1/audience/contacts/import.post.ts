import type { ConsentInput, ImportRow } from '~/types/audience'
import { importAudienceCSV } from '../../-mockDb'
export default defineEventHandler(async (event) => {
  const b = await readBody<{ rows: ImportRow[], consent: ConsentInput }>(event)
  return importAudienceCSV(b.rows, b.consent)
})
