/**
 * Browser-only demo mode (docs/DEMO.md): `isDemo` is true in builds made
 * with NUXT_DEMO=1. `reset` wipes this browser's demo data and re-seeds it.
 */
interface DemoApi {
  reset: () => void
  sampleTracklist: string
}

/**
 * The demo has no server to host a live calendar feed, so the iCal buttons
 * download the generated .ics file instead of copying a subscription URL.
 */
async function downloadCalendar(): Promise<void> {
  const blob = await $fetch<Blob>('/api/v1/gigs/calendar.ics', { responseType: 'blob' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'klubhub-gigs.ics'
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export const DEMO_ICAL_HINT = 'Download your gigs as a calendar file (.ics). Live feed URLs need the full app.'

export function useDemo(): {
  isDemo: boolean
  reset: () => void
  sampleTracklist: string
  downloadCalendar: () => Promise<void>
} {
  const isDemo = !!(useRuntimeConfig().public as Record<string, unknown>).demo
  const api = isDemo ? (useNuxtApp().$demo as DemoApi | null) : undefined
  return {
    isDemo,
    reset: () => api?.reset(),
    sampleTracklist: api?.sampleTracklist ?? '',
    downloadCalendar,
  }
}
