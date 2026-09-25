<script setup lang="ts">
import { useGigStore } from '../stores/gig'
import { useSocialStore } from '../stores/social'
import { countdownLabel, formatDayLabel, formatMoney } from '../utils/quickPreview'

// Quick-preview readouts: what needs the DJ next, each linking to where the
// number lives. Real data only: there is no invoice or due-date model yet, so
// money comes from each gig's payment status (see utils/quickPreview.ts).
const route = useRoute()
const { preview, refresh } = useQuickPreview()
const gigStore = useGigStore()
const socialStore = useSocialStore()

type Tone = 'default' | 'primary' | 'alert' | 'muted'

interface StatusItem {
  key: string
  label: string
  value: string
  tone: Tone
  to: string
  title: string
  /** Hidden below the xl breakpoint to keep the bar on one line. */
  wide?: boolean
}

const TONE_STYLE: Record<Tone, string | undefined> = {
  default: undefined,
  primary: 'color:var(--color-primary);',
  alert: 'color:var(--color-error);',
  muted: 'color:var(--color-tertiary);',
}

const items = computed<StatusItem[]>(() => {
  const p = preview.value

  // Until the first read lands (and on SSR) every value is a dash, so server and
  // client render the same markup.
  if (!p) {
    return [
      { key: 'gig', label: 'NEXT GIG', value: '—', tone: 'default', to: '/gigs', title: 'Next confirmed gig' },
      { key: 'pay', label: 'PAYMENTS', value: '—', tone: 'default', to: '/gigs', title: 'Overdue and unpaid gigs' },
      { key: 'social', label: 'SOCIAL', value: '—', tone: 'default', to: '/social', title: 'Scheduled and failed posts' },
      { key: 'booked', label: 'BOOKED', value: '—', tone: 'default', to: '/gigs', title: 'Upcoming confirmed gigs', wide: true },
      { key: 'mtd', label: 'EARNED MTD', value: '—', tone: 'default', to: '/gigs', title: 'Played gigs this month', wide: true },
    ]
  }

  // NEXT GIG
  let gig: StatusItem
  if (p.nextGig) {
    const g = p.nextGig.gig
    const place = (g.city || g.venue || '').toUpperCase()
    gig = {
      key: 'gig',
      label: `NEXT GIG · ${countdownLabel(p.nextGig.days)}`,
      value: place ? `${formatDayLabel(p.nextGig.day)} · ${place}` : formatDayLabel(p.nextGig.day),
      // The thing that must be found: lit when it is today or tomorrow.
      tone: p.nextGig.days <= 1 ? 'primary' : 'default',
      to: '/gigs',
      title: [g.event_name, [g.venue, g.city].filter(Boolean).join(', ')].filter(Boolean).join(' — '),
    }
  } else {
    gig = { key: 'gig', label: 'NEXT GIG', value: 'NONE BOOKED', tone: 'muted', to: '/gigs', title: 'No confirmed gig from today on' }
  }

  // PAYMENTS
  const { overdueCount, overdue, unpaidCount, unpaid } = p.payments
  let pay: StatusItem
  if (overdueCount > 0) {
    pay = {
      key: 'pay',
      label: 'PAYMENTS',
      value: `${overdueCount} OVERDUE`,
      tone: 'alert',
      to: '/gigs',
      title: `${overdueCount} overdue${overdue ? ` (${formatMoney(overdue)})` : ''}${unpaidCount > 0 ? `, ${unpaidCount} more unpaid` : ''}`,
    }
  } else if (unpaidCount > 0) {
    pay = {
      key: 'pay',
      label: 'PAYMENTS',
      value: `${formatMoney(unpaid)} UNPAID`,
      tone: 'default',
      to: '/gigs',
      title: `${unpaidCount} played gig${unpaidCount === 1 ? '' : 's'} not paid yet`,
    }
  } else {
    pay = { key: 'pay', label: 'PAYMENTS', value: 'NOTHING DUE', tone: 'muted', to: '/gigs', title: 'No overdue or unpaid gigs' }
  }

  // SOCIAL
  let social: StatusItem
  if (p.social === null) {
    social = { key: 'social', label: 'SOCIAL', value: '—', tone: 'default', to: '/social', title: 'The post queue could not be read' }
  } else if (p.social.failed > 0) {
    social = {
      key: 'social',
      label: 'SOCIAL',
      value: `${p.social.failed} FAILED`,
      tone: 'alert',
      to: '/social',
      title: `${p.social.failed} failed post${p.social.failed === 1 ? '' : 's'}, ${p.social.scheduled} scheduled`,
    }
  } else if (p.social.scheduled > 0) {
    social = {
      key: 'social',
      label: 'SOCIAL',
      value: `${p.social.scheduled} SCHEDULED`,
      tone: 'default',
      to: '/social',
      title: p.social.nextAt ? `Next post ${new Date(p.social.nextAt).toLocaleString('en-US', { weekday: 'short', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}` : 'Scheduled posts',
    }
  } else {
    social = { key: 'social', label: 'SOCIAL', value: 'QUEUE EMPTY', tone: 'muted', to: '/social', title: 'No scheduled posts' }
  }

  return [
    gig,
    pay,
    social,
    { key: 'booked', label: 'BOOKED', value: formatMoney(p.booked), tone: 'default', to: '/gigs', title: 'Upcoming confirmed and advanced gigs', wide: true },
    { key: 'mtd', label: 'EARNED MTD', value: formatMoney(p.earnedMtd), tone: 'default', to: '/gigs', title: 'Played gigs this month', wide: true },
  ]
})

// Keep the readouts fresh: on load, on navigation, when the tab regains focus,
// every few minutes (so a countdown does not go stale past midnight), and soon
// after a gig or post is saved elsewhere in the app.
let timer: ReturnType<typeof setInterval> | undefined
let soon: ReturnType<typeof setTimeout> | undefined
const onFocus = () => refresh()
function refreshSoon() {
  clearTimeout(soon)
  soon = setTimeout(() => refresh(true), 400)
}

onMounted(() => {
  refresh(true)
  window.addEventListener('focus', onFocus)
  timer = setInterval(() => refresh(), 5 * 60_000)
})
onBeforeUnmount(() => {
  window.removeEventListener('focus', onFocus)
  clearInterval(timer)
  clearTimeout(soon)
})

watch(() => route.fullPath, () => refresh())
watch(() => gigStore.gigs, refreshSoon, { deep: true })
watch(() => socialStore.posts, refreshSoon, { deep: true })
</script>

<template>
  <!-- Fixed bottom bar: desktop only — Hick's Law: hide technical metrics on mobile -->
  <!-- TheBottomNav replaces this area on mobile -->
  <footer
    class="fixed bottom-0 left-0 right-0 bg-surface-container border-t border-outline-variant/20 z-10 hidden lg:block"
    style="height:var(--status-bar-h);"
    aria-label="Quick preview"
  >
    <div class="flex items-center gap-6 px-6 h-full">
      <NuxtLink
        v-for="item in items"
        :key="item.key"
        :to="item.to"
        class="status-item"
        :class="item.wide ? 'hidden xl:flex' : 'flex'"
        :title="item.title"
      >
        <span class="font-terminal tracking-terminal text-tertiary text-[8px] uppercase">
          {{ item.label }}
        </span>
        <span
          class="status-value font-command font-bold text-sm tabular-nums"
          :style="TONE_STYLE[item.tone]"
        >
          {{ item.value }}
        </span>
      </NuxtLink>
      <!-- Right-align: version tag -->
      <div class="ml-auto">
        <span class="font-terminal tracking-terminal text-tertiary text-[8px] uppercase">
          KLUBHUB_DJ v1.0
        </span>
      </div>
    </div>
  </footer>
</template>
