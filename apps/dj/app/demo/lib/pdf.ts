// Tiny text-only PDF writer (Helvetica, A4) for demo exports. Output is
// ASCII so string length equals byte offsets in the xref table.

export interface PdfLine {
  text: string
  size?: number
  bold?: boolean
}

const PAGE_H = 842
const MARGIN = 56
const WRAP = 92

/** Maps text to printable ASCII (accents stripped, dashes/quotes simplified). */
export function toAscii(s: string): string {
  return s
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[\u2012-\u2015]/g, '-')
    .replace(/[\u2018\u2019]/g, "'")
    .replace(/[\u201c\u201d]/g, '"')
    .replace(/\u2026/g, '...')
    .replace(/[\u00b7\u2022]/g, '-')
    .replace(/\u20ac/g, 'EUR ')
    .replace(/[^\x20-\x7e]/g, '?')
}


const esc = (s: string) => s.replace(/\\/g, '\\\\').replace(/\(/g, '\\(').replace(/\)/g, '\\)')

function wrap(text: string, width: number): string[] {
  const out: string[] = []
  for (const para of text.split('\n')) {
    let line = ''
    for (const word of para.split(' ')) {
      if ((line + ' ' + word).trim().length > width && line) {
        out.push(line)
        line = word
      } else line = (line ? line + ' ' : '') + word
    }
    out.push(line)
  }
  return out
}

/** Builds a PDF document from lines; paginates automatically. */
export function buildPdf(lines: PdfLine[]): string {
  const pages: string[][] = [[]]
  let y = PAGE_H - MARGIN
  for (const l of lines) {
    const size = l.size ?? 11
    const lead = Math.round(size * 1.45)
    const font = l.bold ? 'F2' : 'F1'
    for (const part of wrap(toAscii(l.text), Math.floor(WRAP * 11 / size))) {
      if (y - lead < MARGIN) {
        pages.push([])
        y = PAGE_H - MARGIN
      }
      y -= lead
      pages[pages.length - 1]!.push(`BT /${font} ${size} Tf ${MARGIN} ${y} Td (${esc(part)}) Tj ET`)
    }
  }

  const objs: string[] = []
  const pageIds = pages.map((_, i) => 5 + i * 2)
  objs[1] = '<< /Type /Catalog /Pages 2 0 R >>'
  objs[2] = `<< /Type /Pages /Kids [${pageIds.map((id) => `${id} 0 R`).join(' ')}] /Count ${pages.length} >>`
  objs[3] = '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>'
  objs[4] = '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>'
  pages.forEach((ops, i) => {
    const stream = ops.join('\n')
    objs[5 + i * 2] = `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 ${PAGE_H}] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents ${6 + i * 2} 0 R >>`
    objs[6 + i * 2] = `<< /Length ${stream.length} >>\nstream\n${stream}\nendstream`
  })

  let out = '%PDF-1.4\n'
  const offsets: number[] = []
  for (let id = 1; id < objs.length; id++) {
    offsets[id] = out.length
    out += `${id} 0 obj\n${objs[id]}\nendobj\n`
  }
  const xref = out.length
  out += `xref\n0 ${objs.length}\n0000000000 65535 f \n`
  for (let id = 1; id < objs.length; id++) out += `${String(offsets[id]).padStart(10, '0')} 00000 n \n`
  out += `trailer\n<< /Size ${objs.length} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`
  return out
}

export function pdfBlob(lines: PdfLine[]): Blob {
  return new Blob([buildPdf(lines)], { type: 'application/pdf' })
}
