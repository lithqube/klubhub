import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import type { ImportField, ImportPreset } from '~/types/guest'
import {
  decodeCsv, guessMapping, headerKey, importErrorText, mapRow, maskEmail, maskName, parseCsv, resolveMapping, ticketStatus,
} from '../attendeeImport'

// The Go tests use the same fixtures, so both sides map exports alike.
const FIXTURES = resolve(import.meta.dirname, '../../../../../api/internal/promoter/guest/testdata/import')
const fixture = (name: string) => {
  const { text, encoding } = decodeCsv(new Uint8Array(readFileSync(resolve(FIXTURES, name))))
  return parseCsv(text, encoding)
}

function mapAll(file: string, preset: ImportPreset, mapping?: Partial<Record<ImportField, string>>) {
  const t = fixture(file)
  const m = resolveMapping(preset, t.headers, mapping)
  const accepted: string[] = []
  const rejected: Record<number, string> = {}
  for (const row of t.rows) {
    const r = mapRow(m, row)
    if (r.reason !== undefined) rejected[row.line] = r.reason
    else accepted.push([r.row.line, r.row.name, r.row.email, r.row.order_ref, r.row.ticket_ref, r.row.secret, r.row.ticket_type, r.row.status].join('|'))
  }
  return { accepted, rejected, mapping: m.header }
}

describe('platform presets (shared fixtures with the Go importer)', () => {
  it('pretix: order positions with attendee vs buyer email', () => {
    const r = mapAll('pretix.csv', 'pretix')
    expect(r.mapping).toMatchObject({ email: 'Attendee email', buyer_email: 'Email', ticket_type: 'Product', secret: 'Secret' })
    expect(r.accepted).toEqual([
      '2|Mara Weiss|mara@example.org|AB12C|1|k2x9fvq8mzq7hs4p|Early bird|valid',
      '3|José Müller|buyer@example.org|AB12C|2|p7wd3nn2c8x4ra1z|Early bird|valid',
      '4|Tomasz Nowak|tom@example.org|XY34Z|3|q9ee4bb1v6m2kd8t|Regular|cancelled',
      '5|Ines "Nes" Duarte|ines@example.org|QQ55R|4|z1aa2bb3cc4dd5ee|Regular|pending',
    ])
  })

  it('RA: semicolons, first + last name, rows without a name are refused', () => {
    const r = mapAll('ra.csv', 'ra')
    expect(r.accepted).toEqual([
      '2|Lena Vogt|lena@example.org|90001|T-1|RA-0001-AAA|Tier 1|valid',
      '3|Sam Oduya|lena@example.org|90001|T-2|RA-0001-AAB|Tier 1|valid',
      '4|Kofi Mensah|kofi@example.org|90002|T-3|RA-0002-AAA|Tier 2|refunded',
    ])
    expect(r.rejected).toEqual({ 5: 'no name or email' })
  })

  it('DICE: bad emails are refused by line, masked', () => {
    const r = mapAll('dice.csv', 'dice')
    expect(r.accepted).toHaveLength(3)
    expect(r.rejected).toEqual({ 5: '"n…" is not an email address' })
  })

  it('Shotgun: unknown statuses are refused, not guessed', () => {
    const r = mapAll('shotgun.csv', 'shotgun')
    expect(r.accepted.map(a => a.split('|').at(-1))).toEqual(['valid', 'cancelled'])
    expect(r.rejected).toEqual({ 4: 'unknown status "resold elsewhere"' })
  })

  it('Luma: api_id is the order, the QR url the barcode, approval the status', () => {
    const r = mapAll('luma.csv', 'luma')
    expect(r.accepted).toEqual([
      '2|Mara Weiss|mara@example.org|gst-a1||https://lu.ma/check-in/evt-1?pk=g-a1|Free RSVP|valid',
      '3|Tom Novak|tom@example.org|gst-b2||https://lu.ma/check-in/evt-1?pk=g-b2|Free RSVP|pending',
      '4|Ines Duarte|ines@example.org|gst-c3||https://lu.ma/check-in/evt-1?pk=g-c3|VIP|cancelled',
    ])
  })

  it('generic: explicit mapping, tolerant header matching, multi-line cells keep line numbers', () => {
    const r = mapAll('generic.csv', 'generic', { name: 'guest', email: ' MAIL ', order_ref: 'Ref', ticket_type: 'Kind' })
    expect(r.accepted).toEqual([
      '2|Anna Berg|anna@example.org|G-1|||Door list|valid',
      '5|Ben Roth||G-2|||Door list|valid',
    ])
    expect(r.rejected).toEqual({ 6: 'no name or email' })
  })
})

describe('parseCsv', () => {
  it.each([
    ['BOM and CRLF', '\uFEFFName,Email\r\nA,a@x.io\r\n', ['Name', 'Email'], 1],
    ['semicolons', 'Name;Email;Order\nA;a@x.io;1\n', ['Name', 'Email', 'Order'], 1],
    ['excel sep line', 'sep=|\nName|Order\nA,B|1\n', ['Name', 'Order'], 1],
    ['tabs', 'Name\tOrder\nA\t1\n', ['Name', 'Order'], 1],
    ['quoted delimiters do not count', '"a;b",c,d\n1,2,3\n', ['a;b', 'c', 'd'], 1],
    ['blank lines and ragged rows', '\n\nName,Order\n\nA\n,,\nB,2,extra\n', ['Name', 'Order'], 2],
  ])('%s', (_, text, headers, rows) => {
    const t = parseCsv(text)
    expect(t.headers).toEqual(headers)
    expect(t.rows).toHaveLength(rows)
  })

  it('counts lines like the server (sep line, quoted newlines)', () => {
    const t = parseCsv('sep=;\nName;Note\nA;"x\ny"\nB;z\n')
    expect(t.rows.map(r => r.line)).toEqual([3, 5])
    expect(t.rows[0]!.cells[1]).toBe('x\ny')
  })

  it.each([
    ['', 'the file is empty'],
    ['Name,Order\n', 'no rows below the header line'],
    ['Name\0,Order\n1,2', 'not a CSV file'],
  ])('refuses %j', (text, problem) => {
    expect(() => parseCsv(text)).toThrow(expect.objectContaining({ problem }) as unknown as Error)
  })

  it('refuses more than 20 000 rows', () => {
    const text = `Name,Order\n${Array.from({ length: 20_001 }, (_, i) => `N${i},${i}`).join('\n')}\n`
    expect(() => parseCsv(text)).toThrow(expect.objectContaining({ problem: 'at most 20000 rows per import; split the file' }) as unknown as Error)
  })
})

describe('decodeCsv', () => {
  it('reads UTF-8 with a BOM and falls back to Windows-1252', () => {
    expect(decodeCsv(new Uint8Array([0xEF, 0xBB, 0xBF, 0x41]))).toEqual({ text: 'A', encoding: 'utf-8' })
    expect(decodeCsv(new Uint8Array([0x4A, 0x6F, 0x73, 0xE9]))).toEqual({ text: 'José', encoding: 'windows-1252' })
    expect(() => decodeCsv(new Uint8Array([0xFF, 0xFE, 0x41, 0]))).toThrow(expect.objectContaining({ field: 'file' }) as unknown as Error)
  })
})

describe('resolveMapping', () => {
  it('names what is missing and returns the headers', () => {
    expect(() => resolveMapping('ra', ['Name', 'Email'])).toThrow(expect.objectContaining({
      error: 'mapping_incomplete', missing: ['order number, ticket id or barcode'], headers: ['Name', 'Email'],
    }) as unknown as Error)
    expect(() => resolveMapping('generic', ['A'], { name: 'A', order_ref: 'a' })).toThrow(expect.objectContaining({ field: 'mapping' }) as unknown as Error)
    expect(() => resolveMapping('generic', ['A'], { name: 'A', order_ref: 'C' })).toThrow(expect.objectContaining({ problem: 'no column "C" in the file' }) as unknown as Error)
  })

  it('matches headers whatever their case and punctuation', () => {
    expect(resolveMapping('shotgun', [' ORDER-ID ', 'first_name', 'LAST NAME']).header).toEqual({ order_ref: ' ORDER-ID ', first_name: 'first_name', last_name: 'LAST NAME' })
    expect(headerKey('Order code')).toBe(headerKey('order_code'))
  })

  it('guesses a generic mapping from common header names', () => {
    expect(guessMapping(['Barcode', 'Full name', 'E-mail address', 'Notes'])).toEqual({ secret: 'Barcode', name: 'Full name', email: 'E-mail address' })
  })
})

describe('ticketStatus', () => {
  it.each([
    ['', 'valid'], ['Paid', 'valid'], ['p', 'valid'], [' Checked in ', 'valid'], ['pending_approval', 'pending'], ['n', 'pending'],
    ['Partially refunded', 'refunded'], ['c', 'cancelled'], ['CANCELED', 'cancelled'], ['Cancelled by organiser', 'cancelled'],
    ['resold elsewhere', null],
  ])('%j → %s', (raw, want) => {
    expect(ticketStatus(raw)).toBe(want)
  })
})

describe('masking', () => {
  it.each([['John Doe', 'Jo… D…'], ['José Müller-Lüdenscheidt', 'Jo… M…'], ['Al', 'Al'], ['Ines D', 'In… D'], ['Élodie', 'Él…']])('%s', (n, want) => {
    expect(maskName(n)).toBe(want)
  })
  it('masks emails', () => {
    expect(maskEmail('mara@label.example')).toBe('m…@l…')
    expect(maskEmail('')).toBe('')
  })
})

describe('importErrorText', () => {
  it('explains mapping and size problems', () => {
    expect(importErrorText({ error: 'mapping_incomplete', detail: { missing: ['name or email'] } })).toContain('no column for name or email')
    expect(importErrorText({ error: 'too_large' })).toContain('8 MB')
    expect(importErrorText({ error: 'invalid', field: 'file', problem: 'the file is empty' })).toBe('File: the file is empty.')
  })
})
