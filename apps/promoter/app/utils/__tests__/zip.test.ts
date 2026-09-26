import { describe, expect, it } from 'vitest'
import { crc32, zip } from '../zip'

describe('zip', () => {
  it('computes CRC-32 like zlib', () => {
    expect(crc32(new TextEncoder().encode('The quick brown fox jumps over the lazy dog'))).toBe(0x414FA339)
    expect(crc32(new Uint8Array())).toBe(0)
  })

  it('writes a readable archive structure', () => {
    const out = zip([{ name: 'a.txt', data: 'hello' }, { name: 'ü.ics', data: 'BEGIN:VCALENDAR' }])
    const view = new DataView(out.buffer)
    expect(view.getUint32(0, true)).toBe(0x04034B50)
    const eocd = out.length - 22
    expect(view.getUint32(eocd, true)).toBe(0x06054B50)
    expect(view.getUint16(eocd + 10, true)).toBe(2)
    const cdOffset = view.getUint32(eocd + 16, true)
    expect(view.getUint32(cdOffset, true)).toBe(0x02014B50)
  })
})
