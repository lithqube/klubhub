/**
 * Attendee import (P2.2): CSV parsing, platform column presets, row
 * mapping and masking. Pure — no Vue, no fetch. Mirrors
 * api/internal/promoter/guest/import_parse.go, which is the source of
 * truth for real imports; the browser uses this to read headers for the
 * mapping step and the dev mocks use it to answer like the server.
 */
import type { ImportField, ImportPreset, TicketStatus } from '~/types/guest'

export const MAX_IMPORT_BYTES = 8 << 20
export const MAX_IMPORT_ROWS = 20_000
export const DEFAULT_TICKET_TYPE = 'General admission'
const MAX_NAME = 120
const MAX_EMAIL = 254
const MAX_REF = 120
const MAX_SECRET = 512

export const IMPORT_PRESETS: { id: ImportPreset, label: string, hint: string }[] = [
  { id: 'ra', label: 'RA', hint: 'Resident Advisor ticket export' },
  { id: 'dice', label: 'DICE', hint: 'DICE (MIO) ticket holder export' },
  { id: 'shotgun', label: 'SHOTGUN', hint: 'Shotgun ticket export' },
  { id: 'pretix', label: 'PRETIX', hint: 'Order data export → Order positions (CSV)' },
  { id: 'luma', label: 'LUMA', hint: 'Guests → Download as CSV' },
  { id: 'generic', label: 'OTHER CSV', hint: 'Any CSV: pick the columns yourself' },
]

export const IMPORT_FIELDS: { id: ImportField, label: string }[] = [
  { id: 'order_ref', label: 'ORDER NUMBER' },
  { id: 'ticket_ref', label: 'TICKET ID' },
  { id: 'secret', label: 'BARCODE / QR' },
  { id: 'name', label: 'FULL NAME' },
  { id: 'first_name', label: 'FIRST NAME' },
  { id: 'last_name', label: 'LAST NAME' },
  { id: 'email', label: 'EMAIL' },
  { id: 'buyer_name', label: 'BUYER NAME' },
  { id: 'buyer_email', label: 'BUYER EMAIL' },
  { id: 'ticket_type', label: 'TICKET TYPE' },
  { id: 'ticket_type_ref', label: 'TICKET TYPE ID' },
  { id: 'status', label: 'STATUS' },
]
const FIELDS = IMPORT_FIELDS.map(f => f.id)

export const TICKET_STATUS_LABEL: Record<TicketStatus, string> = {
  valid: 'VALID', pending: 'UNPAID', cancelled: 'CANCELLED', refunded: 'REFUNDED',
}

/** Header folded for matching: case, spaces and punctuation ignored. */
export function headerKey(h: string): string {
  return h.toLowerCase().replace(/[^\p{L}\p{N}]/gu, '')
}

type Aliases = Partial<Record<ImportField, string[]>>

// Inferred from each platform's documented or commonly seen exports; every
// non-generic preset falls back to COMMON (same tables as the Go side).
const PRESET_ALIASES: Record<Exclude<ImportPreset, 'generic'>, Aliases> = {
  pretix: {
    order_ref: ['ordercode', 'order'], ticket_ref: ['positionid', 'orderpositionid'], secret: ['ticketsecret', 'secret'],
    name: ['attendeename'], first_name: ['attendeenamegivenname', 'attendeenamefirstname'],
    last_name: ['attendeenamefamilyname', 'attendeenamelastname'], email: ['attendeeemail'], buyer_name: ['invoiceaddressname'],
    buyer_email: ['email', 'orderemail'], ticket_type: ['product', 'item'], ticket_type_ref: ['productid', 'itemid'], status: ['status', 'orderstatus'],
  },
  ra: {
    order_ref: ['orderid', 'ordernumber', 'orderref', 'bookingreference', 'bookingref'], ticket_ref: ['ticketid', 'ticketnumber'],
    secret: ['barcode', 'ticketbarcode', 'ticketcode'], name: ['name', 'fullname', 'ticketholder', 'ticketholdername'],
    first_name: ['firstname', 'forename'], last_name: ['lastname', 'surname'], email: ['email', 'emailaddress'],
    ticket_type: ['tickettype', 'ticket', 'tickettier', 'tier'], status: ['status', 'ticketstatus'],
  },
  dice: {
    order_ref: ['orderid', 'purchaseid', 'orderreference', 'order'], ticket_ref: ['ticketid'], secret: ['barcode', 'ticketcode', 'code'],
    name: ['fanname', 'name', 'fullname'], first_name: ['firstname', 'fanfirstname'], last_name: ['lastname', 'fanlastname'],
    email: ['email', 'fanemail'], ticket_type: ['tickettype', 'ticketname', 'ticket'], status: ['status', 'ticketstatus'],
  },
  shotgun: {
    order_ref: ['orderid', 'ordernumber', 'order'], ticket_ref: ['ticketid'], secret: ['barcode', 'ticketbarcode', 'qrcode'],
    name: ['name', 'fullname', 'holdername'], first_name: ['firstname', 'holderfirstname'], last_name: ['lastname', 'holderlastname'],
    email: ['email', 'holderemail', 'contactemail'], buyer_email: ['buyeremail'],
    ticket_type: ['ticket', 'tickettitle', 'tickettype', 'dealtitle', 'ticketname'], status: ['ticketstatus', 'status', 'state'],
  },
  luma: {
    order_ref: ['apiid', 'guestapiid'], secret: ['qrcodeurl', 'qrcode'], name: ['name'], first_name: ['firstname'], last_name: ['lastname'],
    email: ['email'], ticket_type: ['ticketname', 'tickettypename', 'tickettype'], ticket_type_ref: ['tickettypeid'], status: ['approvalstatus', 'status'],
  },
}

const COMMON: Aliases = {
  order_ref: ['orderid', 'ordercode', 'ordernumber', 'orderref', 'orderreference', 'order', 'bookingreference', 'purchaseid', 'transactionid'],
  ticket_ref: ['ticketid', 'ticketnumber', 'positionid'],
  secret: ['barcode', 'ticketbarcode', 'secret', 'ticketsecret', 'qrcode', 'ticketcode'],
  name: ['attendeename', 'name', 'fullname', 'ticketholder', 'holdername', 'guestname'],
  first_name: ['firstname', 'givenname', 'forename'],
  last_name: ['lastname', 'familyname', 'surname'],
  email: ['attendeeemail', 'email', 'emailaddress'],
  buyer_name: ['buyername', 'purchasername', 'customername'],
  buyer_email: ['buyeremail', 'purchaseremail', 'customeremail'],
  ticket_type: ['tickettype', 'ticketname', 'ticket', 'product', 'tier'],
  ticket_type_ref: ['tickettypeid', 'productid'],
  status: ['status', 'ticketstatus', 'orderstatus'],
}

/** A problem shaped like the API's 422 body. */
export interface ImportProblem {
  error: 'invalid' | 'mapping_incomplete'
  field?: string
  problem: string
  missing?: string[]
  headers?: string[]
}

const invalid = (field: string, problem: string): ImportProblem => ({ error: 'invalid', field, problem })

export interface CsvTable {
  headers: string[]
  rows: { line: number, cells: string[] }[]
  encoding: 'utf-8' | 'windows-1252'
}

/** Bytes → text: UTF-8 (BOM dropped), else Windows-1252; UTF-16 is refused. */
export function decodeCsv(bytes: Uint8Array): { text: string, encoding: CsvTable['encoding'] } {
  if (bytes.length > MAX_IMPORT_BYTES) throw invalid('file', `larger than ${MAX_IMPORT_BYTES >> 20} MB`)
  if ((bytes[0] === 0xFF && bytes[1] === 0xFE) || (bytes[0] === 0xFE && bytes[1] === 0xFF)) throw invalid('file', 'UTF-16 text; export or save as CSV (UTF-8)')
  let b = bytes
  if (b[0] === 0xEF && b[1] === 0xBB && b[2] === 0xBF) b = b.subarray(3)
  try {
    return { text: new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(b), encoding: 'utf-8' }
  } catch {
    return { text: new TextDecoder('windows-1252').decode(b), encoding: 'windows-1252' }
  }
}

function sniffDelimiter(text: string): string {
  const head = text.split('\n').find(l => l.trim() !== '') ?? ''
  const n: Record<string, number> = { ',': 0, ';': 0, '\t': 0 }
  let quoted = false
  for (const c of head) {
    if (c === '"') quoted = !quoted
    else if (!quoted && c in n) n[c]!++
  }
  return [';', '\t'].reduce((best, c) => (n[c]! > n[best]! ? c : best), ',')
}

/**
 * Parse CSV text: `,`, `;` or tab (sniffed, or an Excel "sep=;" line),
 * quoted fields across lines, stray quotes kept (lazy), blank rows skipped.
 */
export function parseCsv(input: string, encoding: CsvTable['encoding'] = 'utf-8'): CsvTable {
  let text = input.replace(/^\uFEFF/, '')
  if (text.includes('\0')) throw invalid('file', 'not a CSV file')
  let lineOffset = 0
  let delim = ''
  const first = text.split('\n', 1)[0]!.trim()
  if (/^sep=.$/i.test(first)) {
    delim = first[4]!
    text = text.slice(text.indexOf('\n') + 1)
    lineOffset = 1
  }
  if (!delim) delim = sniffDelimiter(text)

  const records: { line: number, cells: string[] }[] = []
  let cells: string[] = []
  let cell = ''
  let quoted = false
  let atStart = true
  let line = 1
  let recLine = 1
  const endCell = () => { cells.push(cell); cell = ''; atStart = true }
  const endRecord = () => {
    endCell()
    records.push({ line: recLine + lineOffset, cells })
    cells = []
  }
  for (let i = 0; i < text.length; i++) {
    const c = text[i]!
    if (quoted) {
      if (c === '"') {
        if (text[i + 1] === '"') { cell += '"'; i++ } else quoted = false
      } else {
        if (c === '\n') line++
        cell += c
      }
      continue
    }
    if (c === '"' && atStart && cell.trim() === '') { quoted = true; cell = ''; atStart = false; continue }
    if (c === delim) { endCell(); continue }
    if (c === '\r' && text[i + 1] === '\n') continue
    if (c === '\n' || c === '\r') {
      endRecord()
      line++
      recLine = line
      continue
    }
    atStart = false
    cell += c
  }
  if (cell !== '' || cells.length) endRecord()

  const nonBlank = records.filter(r => r.cells.some(c => c.trim() !== ''))
  if (!nonBlank.length) throw invalid('file', 'the file is empty')
  const [head, ...rows] = nonBlank
  if (!rows.length) throw invalid('file', 'no rows below the header line')
  if (rows.length > MAX_IMPORT_ROWS) throw invalid('file', `at most ${MAX_IMPORT_ROWS} rows per import; split the file`)
  return { headers: head!.cells.map(h => h.trim()), rows, encoding }
}

export interface Mapping {
  index: Partial<Record<ImportField, number>>
  header: Partial<Record<ImportField, string>>
}

/**
 * Match a preset's aliases to the headers, or check an explicit mapping for
 * "generic". Throws an ImportProblem when name/email or a key column
 * (order number, ticket id or barcode) is missing.
 */
export function resolveMapping(preset: ImportPreset, headers: string[], explicit: Partial<Record<ImportField, string>> = {}): Mapping {
  const m: Mapping = { index: {}, header: {} }
  if (!IMPORT_PRESETS.some(p => p.id === preset)) throw invalid('preset', IMPORT_PRESETS.map(p => p.id).join(', '))
  const keys = headers.map(headerKey)
  const used = new Set<number>()
  const take = (f: ImportField, i: number) => { m.index[f] = i; m.header[f] = headers[i]; used.add(i) }
  if (preset === 'generic') {
    for (const [field, h] of Object.entries(explicit) as [ImportField, string | undefined][]) {
      if (!h) continue
      if (!FIELDS.includes(field)) throw invalid('mapping', `unknown field ${field}`)
      const i = keys.indexOf(headerKey(h))
      if (i < 0) throw { error: 'mapping_incomplete', missing: [field], headers, problem: `no column "${h}" in the file` } satisfies ImportProblem
      if (used.has(i)) throw invalid('mapping', `column "${h}" is mapped twice`)
      take(field, i)
    }
  } else {
    for (const f of FIELDS) {
      for (const alias of [...(PRESET_ALIASES[preset][f] ?? []), ...(COMMON[f] ?? [])]) {
        const i = keys.indexOf(alias)
        if (i >= 0 && !used.has(i)) { take(f, i); break }
      }
    }
  }
  const has = (...fs: ImportField[]) => fs.some(f => m.index[f] !== undefined)
  const missing: string[] = []
  if (!has('name', 'first_name', 'last_name', 'email', 'buyer_name', 'buyer_email')) missing.push('name or email')
  if (!has('order_ref', 'ticket_ref', 'secret')) missing.push('order number, ticket id or barcode')
  if (missing.length) throw { error: 'mapping_incomplete', missing, headers, problem: `no column for ${missing.join(' and ')}` } satisfies ImportProblem
  return m
}

/** Prefill for the generic mapping step: the common aliases, first match wins. */
export function guessMapping(headers: string[]): Partial<Record<ImportField, string>> {
  const keys = headers.map(headerKey)
  const out: Partial<Record<ImportField, string>> = {}
  const used = new Set<number>()
  for (const f of FIELDS) {
    for (const alias of COMMON[f] ?? []) {
      const i = keys.indexOf(alias)
      if (i >= 0 && !used.has(i)) { out[f] = headers[i]; used.add(i); break }
    }
  }
  return out
}

const STATUS: Record<string, TicketStatus> = Object.fromEntries([
  ...['', 'valid', 'paid', 'p', 'completed', 'complete', 'confirmed', 'approved', 'active', 'issued', 'sold', 'purchased', 'going', 'ok',
    'checkedin', 'attended', 'scanned', 'used', 'free', 'registered'].map(k => [k, 'valid']),
  ...['pending', 'n', 'unpaid', 'reserved', 'pendingapproval', 'waitlist', 'invited', 'awaitingpayment', 'requested'].map(k => [k, 'pending']),
  ...['refunded', 'r', 'refund', 'chargeback'].map(k => [k, 'refunded']),
  ...['cancelled', 'canceled', 'c', 'e', 'expired', 'void', 'voided', 'declined', 'rejected', 'revoked', 'transferred', 'deleted', 'invalid',
    'notgoing', 'notattending'].map(k => [k, 'cancelled']),
])

/** Platform status text → ticket status; null for text we do not know (never guessed). */
export function ticketStatus(raw: string): TicketStatus | null {
  const k = headerKey(raw)
  if (k in STATUS) return STATUS[k]!
  if (k.includes('refund')) return 'refunded'
  if (k.includes('cancel')) return 'cancelled'
  return null
}

export interface ImportRow {
  line: number
  order_ref: string
  ticket_ref: string
  secret: string
  name: string
  email: string
  buyer_name: string
  buyer_email: string
  ticket_type: string
  ticket_type_ref: string
  status: TicketStatus
}

const EMAIL_RE = /^[^\s@<>(),;:"]+@[^\s@<>(),;:"]+\.[^\s@<>(),;:"]+$/

/** Apply a mapping to one row: the normalised row, or why it cannot be imported. */
export function mapRow(m: Mapping, row: { line: number, cells: string[] }): { row: ImportRow, reason?: undefined } | { row?: undefined, reason: string } {
  const raw = (f: ImportField) => { const i = m.index[f]; return i === undefined ? '' : (row.cells[i] ?? '') }
  const cell = (f: ImportField) => raw(f).replace(/\s+/g, ' ').trim()
  let name = cell('name') || `${cell('first_name')} ${cell('last_name')}`.trim()
  let email = cell('email') || cell('buyer_email')
  const buyerEmail = cell('buyer_email') || email
  if (!name) name = cell('buyer_name')
  const buyerName = cell('buyer_name') || name
  for (const e of [email, buyerEmail]) {
    if (e && !EMAIL_RE.test(e)) return { reason: `"${maskEmail(e)}" is not an email address` }
  }
  if (!name) name = email
  const status = ticketStatus(cell('status'))
  if (!status) return { reason: `unknown status "${cell('status')}"` }
  const r: ImportRow = {
    line: row.line, order_ref: cell('order_ref'), ticket_ref: cell('ticket_ref'), secret: raw('secret').trim(), name, email,
    buyer_name: buyerName, buyer_email: buyerEmail, ticket_type: cell('ticket_type') || DEFAULT_TICKET_TYPE,
    ticket_type_ref: cell('ticket_type_ref'), status,
  }
  email = r.email
  if (!r.name) return { reason: 'no name or email' }
  if (!r.order_ref && !r.ticket_ref && !r.secret) return { reason: 'no order number, ticket id or barcode' }
  if (r.name.length > MAX_NAME || r.buyer_name.length > MAX_NAME) return { reason: `name longer than ${MAX_NAME} characters` }
  if (email.length > MAX_EMAIL || r.buyer_email.length > MAX_EMAIL) return { reason: 'email too long' }
  if ([r.order_ref, r.ticket_ref, r.ticket_type_ref].some(x => x.length > MAX_REF)) return { reason: `order or ticket id longer than ${MAX_REF} characters` }
  if (r.ticket_ref.startsWith('#')) return { reason: 'ticket id must not start with #' }
  if (r.secret.length > MAX_SECRET) return { reason: `barcode longer than ${MAX_SECRET} characters` }
  if (r.ticket_type.length > MAX_NAME) return { reason: `ticket type longer than ${MAX_NAME} characters` }
  return { row: r }
}

/** "John Doe" → "Jo… D…" (dry-run preview). */
export function maskName(name: string): string {
  return name.split(/\s+/).filter(Boolean).map((w, i) => {
    const chars = [...w]
    const keep = i === 0 ? 2 : 1
    return chars.length > keep ? `${chars.slice(0, keep).join('')}…` : w
  }).join(' ')
}

/** "mara@label.example" → "m…@l…". */
export function maskEmail(email: string): string {
  if (!email) return ''
  const first = (s: string) => (s ? `${[...s][0]}…` : '')
  const at = email.indexOf('@')
  return at < 0 ? first(email) : `${first(email.slice(0, at))}@${first(email.slice(at + 1))}`
}

/** Plain-language copy for import errors (mapping, size, file). */
export function importErrorText(err: { error: string, field?: string, problem?: string, detail?: Record<string, unknown> }): string {
  const d = err.detail ?? {}
  switch (err.error) {
    case 'mapping_incomplete': {
      const missing = Array.isArray(d.missing) ? (d.missing as string[]).join(' and ') : ''
      return `This file has no column for ${missing || 'a required field'}. Check the platform, or choose OTHER CSV and pick the columns.`
    }
    case 'too_large': return `The file is larger than ${MAX_IMPORT_BYTES >> 20} MB. Split it and import the parts one after the other.`
    case 'unsupported_media_type': return 'Upload the export as a CSV file.'
    case 'invalid': return err.problem ? `${err.field === 'file' ? 'File' : (err.field ?? '').replace(/_/g, ' ')}: ${err.problem}.` : 'Check the file.'
    case 'not_found': return 'This event no longer exists.'
    case 'forbidden': case 'no_role_grant': return 'Your role cannot import attendees.'
    default: return 'Could not import. Check your connection and try again.'
  }
}
