import type { ImportField, ImportPreset } from '~/types/guest'
import { type CsvTable, decodeCsv, type ImportProblem, MAX_IMPORT_BYTES, MAX_IMPORT_ROWS, parseCsv } from '~/utils/attendeeImport'
import { importAttendees } from '../../../-mockDb'

// Same contract as the Go API: multipart (file, preset, mapping JSON) or
// JSON rows; dry run unless ?dry_run=false.
export default defineEventHandler(async (event) => {
  const id = getRouterParam(event, 'id')!
  const dry = getQuery(event).dry_run !== 'false'
  const fail = (status: number, data: Record<string, unknown>) => createError({ statusCode: status, data })
  const type = getHeader(event, 'content-type') ?? ''
  let preset: ImportPreset
  let mapping: Partial<Record<ImportField, string>> = {}
  let table: CsvTable
  try {
    if (type.startsWith('multipart/form-data')) {
      const parts = (await readMultipartFormData(event)) ?? []
      const field = (n: string) => parts.find(p => p.name === n && !p.filename)?.data.toString('utf8') ?? ''
      const file = parts.find(p => p.name === 'file')
      if (!file) throw { error: 'invalid', field: 'file', problem: 'attach the CSV export as "file"' } satisfies ImportProblem
      if (file.data.length > MAX_IMPORT_BYTES) throw fail(413, { error: 'too_large', max_bytes: MAX_IMPORT_BYTES })
      preset = field('preset') as ImportPreset
      if (field('mapping')) mapping = JSON.parse(field('mapping'))
      const { text, encoding } = decodeCsv(new Uint8Array(file.data))
      table = parseCsv(text, encoding)
    } else if (type.startsWith('application/json')) {
      const body = await readBody<{ preset: ImportPreset, mapping?: Partial<Record<ImportField, string>>, rows: Record<string, string>[] }>(event)
      if (!body?.rows?.length) throw { error: 'invalid', field: 'rows', problem: 'no rows' } satisfies ImportProblem
      if (body.rows.length > MAX_IMPORT_ROWS) throw { error: 'invalid', field: 'rows', problem: `at most ${MAX_IMPORT_ROWS} rows per import` } satisfies ImportProblem
      const headers = [...new Set(body.rows.flatMap(r => Object.keys(r)))].sort()
      preset = body.preset
      mapping = body.mapping ?? {}
      table = { headers, encoding: 'utf-8', rows: body.rows.map((r, i) => ({ line: i + 1, cells: headers.map(h => r[h] ?? '') })) }
    } else {
      throw fail(415, { error: 'unsupported_media_type' })
    }
  } catch (e) {
    if ((e as { statusCode?: number }).statusCode) throw e
    throw fail(422, e as Record<string, unknown>)
  }
  return importAttendees(id, preset, mapping, table, dry)
})
