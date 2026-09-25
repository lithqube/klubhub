import type { ExportData } from '~/types/event'
import { dayLabel, rangeLabel } from '~/utils/datetime'

/** Plain text of light Markdown (mirrors the Go PlainText). */
export function plainText(md: string): string {
  return md.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/[*_`#>]+/g, '').trim()
}

/** Only http(s) links reach generated HTML. */
const safeUrl = (u: string | null): string | null => (u && /^https?:\/\//i.test(u) ? u : null)

const esc = (s: string) => s.replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', '\'': '&#39;' }[c]!))

export interface CopyField { label: string, value: string, withheld?: boolean }

/** Copy-paste fields per platform; withheld values say so instead of vanishing. */
export function platformFields(x: ExportData, platform: 'ra' | 'facebook' | 'dice' | 'generic'): CopyField[] {
  const e = x.event
  const when = `${dayLabel(e.starts_at, e.timezone)} · ${rangeLabel(e.starts_at, e.ends_at, e.timezone)} (${e.timezone})`
  const lineup = x.lineup.map(l => l.display_name).join('\n')
  const venue: CopyField = x.location.withheld
    ? { label: 'VENUE', value: x.location.name, withheld: true }
    : { label: 'VENUE', value: [x.location.name, x.location.address].filter(Boolean).join(', ') }
  const base: CopyField[] = [
    { label: 'TITLE', value: e.title },
    { label: 'DATE & TIME', value: when },
    venue,
    { label: 'LINEUP', value: lineup },
    { label: 'DESCRIPTION', value: plainText(e.description_md) },
  ]
  const extra: CopyField[] = [
    ...(e.cost_text ? [{ label: 'COST', value: e.cost_text }] : []),
    ...(e.external_ticket_url ? [{ label: 'TICKETS', value: e.external_ticket_url }] : []),
    ...(e.min_age ? [{ label: 'MIN AGE', value: `${e.min_age}+` }] : []),
  ]
  if (platform === 'ra') return [...base, ...(e.genres.length ? [{ label: 'GENRES', value: e.genres.join(', ') }] : []), ...extra]
  if (platform === 'facebook') {
    return [{ label: 'EVENT NAME', value: e.title }, { label: 'DATE & TIME', value: when }, venue,
      { label: 'DETAILS', value: [plainText(e.description_md), lineup && `LINEUP\n${lineup}`, e.external_ticket_url && `Tickets: ${e.external_ticket_url}`].filter(Boolean).join('\n\n') }]
  }
  if (platform === 'dice') return [...base, ...extra]
  return [...base, ...extra, { label: 'PRESENTED BY', value: x.organizer }]
}

/** Standalone event page for the collective's own site (UX §4.6). */
export function staticPage(x: ExportData, jsonld: Record<string, unknown> | null, ogImage = 'og-image.png'): string {
  const e = x.event
  const when = `${dayLabel(e.starts_at, e.timezone)} · ${rangeLabel(e.starts_at, e.ends_at, e.timezone)}`
  const desc = plainText(e.description_md)
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${esc(e.title)} — ${esc(x.organizer)}</title>
${e.visibility === 'unlisted' ? '<meta name="robots" content="noindex">\n' : ''}<meta name="description" content="${esc(desc.slice(0, 160))}">
<meta property="og:type" content="website">
<meta property="og:title" content="${esc(e.title)}">
<meta property="og:description" content="${esc(`${when} · ${x.location.name}`)}">
<meta property="og:image" content="${ogImage}">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta name="twitter:card" content="summary_large_image">
${jsonld ? `<script type="application/ld+json">${JSON.stringify(jsonld).replace(/</g, '\\u003c')}</script>` : ''}
<style>
body{margin:0;background:#0e0e0f;color:#e0e0e2;font:16px/1.5 Inter,system-ui,sans-serif}
main{max-width:720px;margin:0 auto;padding:48px 20px}
h1{font:700 40px/1.1 "Space Grotesk",system-ui,sans-serif;text-transform:uppercase;margin:0 0 8px}
.meta{color:#96f8ff;text-transform:uppercase;letter-spacing:.06em;font-size:13px}
ul{list-style:none;padding:0}li{padding:6px 0;border-bottom:1px dashed #2a2a2d;text-transform:uppercase;font-weight:600}
a.cta{display:inline-block;margin-top:20px;padding:12px 20px;background:#96f8ff;color:#0e0e0f;text-decoration:none;font-weight:700;text-transform:uppercase}
</style>
</head>
<body>
<main>
<div class="meta">${esc(x.organizer)} presents</div>
<h1>${esc(e.title)}</h1>
<div class="meta">${esc(when)} · ${esc(x.location.name)}</div>
${desc ? `<p>${esc(desc).replace(/\n/g, '<br>')}</p>` : ''}
${x.lineup.length ? `<ul>${x.lineup.map(l => `<li>${esc(l.display_name)}</li>`).join('')}</ul>` : ''}
${e.cost_text ? `<p>${esc(e.cost_text)}</p>` : ''}
${safeUrl(e.external_ticket_url) ? `<a class="cta" href="${esc(e.external_ticket_url!)}" rel="noopener">Tickets</a>` : ''}
</main>
</body>
</html>
`
}

/** Self-contained HTML card to paste into another site. */
export function embedSnippet(x: ExportData): string {
  const e = x.event
  const when = `${dayLabel(e.starts_at, e.timezone)} · ${rangeLabel(e.starts_at, e.ends_at, e.timezone)}`
  return `<div style="font:15px/1.4 system-ui,sans-serif;background:#0e0e0f;color:#e0e0e2;padding:16px;max-width:420px;border-left:3px solid #96f8ff">
  <div style="font-weight:700;text-transform:uppercase;font-size:18px">${esc(e.title)}</div>
  <div style="color:#96f8ff;font-size:12px;text-transform:uppercase;letter-spacing:.06em">${esc(when)} · ${esc(x.location.name)}</div>
  ${x.lineup.length ? `<div style="margin-top:8px">${x.lineup.map(l => esc(l.display_name)).join(' · ')}</div>` : ''}
  ${safeUrl(e.external_ticket_url) ? `<a href="${esc(e.external_ticket_url!)}" rel="noopener" style="display:inline-block;margin-top:10px;color:#96f8ff">Tickets →</a>` : ''}
</div>
`
}
