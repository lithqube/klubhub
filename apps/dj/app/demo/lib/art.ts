// Generated SVG artwork for the demo: track covers, press photos, post
// images and exported tracklist cards. Deterministic per seed string.

export function hashString(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** Data URL safe inside an unquoted CSS url(): parens and quotes are escaped too. */
export function svgDataUrl(svg: string): string {
  const encoded = encodeURIComponent(svg).replace(/[()']/g, (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase())
  return 'data:image/svg+xml;charset=utf-8,' + encoded
}

export function escapeXml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

const PALETTE = ['#96F8FF', '#00F1FD', '#C8B8FF', '#7B68EE', '#4CCAD4', '#FF716C', '#F5C518']
const pick = (h: number, shift: number) => PALETTE[(h >>> shift) % PALETTE.length]!

/** Square abstract cover art (200×200). */
export function coverSvg(seed: string): string {
  const h = hashString(seed)
  const a = pick(h, 0)
  const b = pick(h, 5)
  const r = 30 + (h % 50)
  const cx = 40 + ((h >>> 8) % 120)
  const cy = 40 + ((h >>> 16) % 120)
  return `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200">`
    + `<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#0E0E0F"/><stop offset="1" stop-color="${b}" stop-opacity=".55"/></linearGradient></defs>`
    + `<rect width="200" height="200" fill="url(#g)"/>`
    + `<circle cx="${cx}" cy="${cy}" r="${r}" fill="none" stroke="${a}" stroke-width="6"/>`
    + `<rect x="0" y="${150 + (h % 30)}" width="200" height="3" fill="${a}" opacity=".7"/></svg>`
}

export const coverDataUrl = (seed: string) => svgDataUrl(coverSvg(seed))

/** Portrait placeholder used as a press photo (600×800). */
export function photoSvg(label: string, seed: string): string {
  const h = hashString(seed)
  const a = pick(h, 0)
  const b = pick(h, 7)
  return `<svg xmlns="http://www.w3.org/2000/svg" width="600" height="800" viewBox="0 0 600 800">`
    + `<defs><radialGradient id="g" cx=".5" cy=".35" r=".8"><stop offset="0" stop-color="${a}" stop-opacity=".55"/><stop offset="1" stop-color="#0E0E0F"/></radialGradient></defs>`
    + `<rect width="600" height="800" fill="url(#g)"/>`
    + `<circle cx="300" cy="300" r="120" fill="#161618" stroke="${b}" stroke-width="4"/>`
    + `<rect x="170" y="440" width="260" height="300" rx="120" fill="#161618" stroke="${b}" stroke-width="4"/>`
    + `<text x="300" y="770" fill="#E0E0E2" font-family="sans-serif" font-size="22" text-anchor="middle" letter-spacing="3">${escapeXml(label)}</text></svg>`
}

/** Square image for a social post, built from its caption. */
export function postSvg(caption: string): string {
  const h = hashString(caption)
  const a = pick(h, 0)
  const words = caption.replace(/#\S+/g, '').trim().split(/\s+/).slice(0, 6).join(' ').toUpperCase()
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1080" height="1080" viewBox="0 0 1080 1080">`
    + `<rect width="1080" height="1080" fill="#0E0E0F"/>`
    + `<rect x="60" y="60" width="960" height="960" fill="none" stroke="${a}" stroke-width="4" stroke-dasharray="18 12"/>`
    + `<text x="540" y="520" fill="${a}" font-family="sans-serif" font-size="54" font-weight="700" text-anchor="middle">${escapeXml(words.slice(0, 32))}</text>`
    + `<text x="540" y="600" fill="#8897A9" font-family="sans-serif" font-size="30" text-anchor="middle" letter-spacing="6">SAM EXAMPLE · DEMO</text></svg>`
}

export interface CardInput {
  title: string
  djName: string
  format: 'story' | 'square'
  tracks: { artist: string; title: string; bpm?: number }[]
}

/** Exported tracklist image (1080×1920 story or 1080×1080 square). */
export function trackcardSvg({ title, djName, format, tracks }: CardInput): string {
  const W = 1080
  const H = format === 'story' ? 1920 : 1080
  const max = format === 'story' ? 18 : 9
  const rowH = format === 'story' ? 80 : 72
  const top = format === 'story' ? 420 : 300
  const rows = tracks.slice(0, max).map((t, i) => {
    const y = top + i * rowH
    const line = `${String(i + 1).padStart(2, '0')}  ${t.artist} — ${t.title}`
    return `<text x="90" y="${y}" fill="#E0E0E2" font-family="sans-serif" font-size="34">${escapeXml(line.slice(0, 48))}</text>`
      + (t.bpm ? `<text x="990" y="${y}" fill="#8897A9" font-family="sans-serif" font-size="26" text-anchor="end">${Math.round(t.bpm)}</text>` : '')
  }).join('')
  const more = tracks.length > max ? `<text x="90" y="${top + max * rowH}" fill="#8897A9" font-family="sans-serif" font-size="28">+ ${tracks.length - max} MORE</text>` : ''
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">`
    + `<defs><linearGradient id="g" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#161618"/><stop offset="1" stop-color="#0E0E0F"/></linearGradient></defs>`
    + `<rect width="${W}" height="${H}" fill="url(#g)"/>`
    + `<rect x="0" y="0" width="${W}" height="12" fill="#96F8FF"/>`
    + `<text x="90" y="${top - 200}" fill="#8897A9" font-family="sans-serif" font-size="28" letter-spacing="8">${escapeXml(djName.toUpperCase())}</text>`
    + `<text x="90" y="${top - 110}" fill="#96F8FF" font-family="sans-serif" font-size="64" font-weight="700">${escapeXml(title.toUpperCase().slice(0, 26))}</text>`
    + rows + more
    + `<text x="${W / 2}" y="${H - 60}" fill="#8897A9" font-family="sans-serif" font-size="24" text-anchor="middle" letter-spacing="6">KLUBHUB DJ · DEMO EXPORT</text></svg>`
}
