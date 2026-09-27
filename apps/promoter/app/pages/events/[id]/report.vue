<script setup lang="ts">
import { useDocumentVisibility, useIntervalFn, useNow } from '@vueuse/core'
import { ChartNoAxesColumn, Radio, RefreshCw } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useEventStore } from '~/stores/event'
import { usePrivacyStore } from '~/stores/privacy'
import { useReportStore } from '~/stores/report'
import type { ApiError } from '~/types/event'
import type { ReportSubmitterRow } from '~/types/report'
import { dayLabel, timeLabel } from '~/utils/datetime'
import { hasActivity, listBackCounts, listBackFilename, loadErrorMessage, reportPhase, updatedAgo } from '~/utils/report'

/**
 * Event REPORT tab (P2.4): who came, when and through which list. Works
 * during the night too (LIVE badge, refreshed every minute while visible).
 * Aggregates only; names leave solely through a per-allocation LIST BACK CSV.
 */
const POLL_MS = 60_000

const { current } = storeToRefs(useEventStore())
const store = useReportStore()
const { report, loading, error, busy } = storeToRefs(store)

const id = computed(() => current.value?.id ?? '')
await useAsyncData(() => `report-${id.value}`, () => (id.value ? store.load(id.value).then(() => true) : Promise.resolve(null)), { watch: [id] })

// Retention (P2.5): the report survives an erase (counts only); list-backs need names, so they stop.
const privacy = usePrivacyStore()
const { events: privacyByEvent } = storeToRefs(privacy)
await useAsyncData(() => `privacy-${id.value}`, () => (id.value ? privacy.fetchEvent(id.value).then(() => true) : Promise.resolve(null)), { watch: [id] })
const purgedAt = computed(() => privacyByEvent.value[id.value]?.purged_at ?? null)

// A report left over from another event is never shown.
const shown = computed(() => (report.value && report.value.event.id === id.value ? report.value : null))
const tz = computed(() => shown.value?.event.timezone ?? current.value?.timezone ?? 'UTC')
const phase = computed(() => (shown.value ? reportPhase(shown.value) : null))
const active = computed(() => (shown.value ? hasActivity(shown.value) : false))
const heading = computed(() => (phase.value === 'past' || !phase.value ? 'POST-EVENT REPORT' : 'LIVE REPORT'))

const now = useNow({ interval: 30_000 })
const updated = computed(() => {
  if (!shown.value) return ''
  if (phase.value === 'live') return `UPDATED ${updatedAgo(shown.value.generated_at, now.value.getTime())}`
  return `UPDATED ${dayLabel(shown.value.generated_at, tz.value)} ${timeLabel(shown.value.generated_at, tz.value)} · ${tz.value.toUpperCase()}`
})
const loadError = computed(() => (error.value ? loadErrorMessage(error.value, Boolean(shown.value)) : null))

// Allocations with someone to list back, for the header shortcut.
const listBacks = computed(() => (purgedAt.value ? 0 : shown.value?.by_submitter.filter(r => r.going > 0).length ?? 0))

// ---------------------------------------------------------------- live polling
const visibility = useDocumentVisibility()
const poll = useIntervalFn(() => {
  if (current.value && !loading.value) void store.load(current.value.id)
}, POLL_MS, { immediate: false })
if (import.meta.client) {
  watch([phase, visibility], ([p, v], prev) => {
    if (p === 'live' && v === 'visible') {
      // Back to a hidden tab: catch up at once, then every minute.
      if (prev?.[1] === 'hidden' && current.value) void store.load(current.value.id)
      poll.resume()
    } else {
      poll.pause()
    }
  }, { immediate: true })
}

// ---------------------------------------------------------------- list back
const notice = ref('')
const noticeKind = ref<'success' | 'error'>('success')

function say(kind: 'success' | 'error', text: string) {
  noticeKind.value = kind
  notice.value = text
}

/** Share sheet on phones (Web Share with files), a download everywhere else. */
async function deliver(file: File): Promise<'shared' | 'downloaded' | 'cancelled'> {
  const coarse = typeof matchMedia === 'function' && matchMedia('(pointer: coarse)').matches
  if (coarse && typeof navigator.canShare === 'function' && navigator.canShare({ files: [file] })) {
    try {
      await navigator.share({ files: [file], title: file.name })
      return 'shared'
    } catch (e) {
      if ((e as DOMException)?.name === 'AbortError') return 'cancelled'
      // Share refused (permissions, unsupported type): fall back to a download.
    }
  }
  const url = URL.createObjectURL(file)
  const a = document.createElement('a')
  a.href = url
  a.download = file.name
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
  return 'downloaded'
}

async function listBack(row: ReportSubmitterRow) {
  if (!current.value) return
  notice.value = ''
  let csv: string
  try {
    csv = await store.listBackCsv(current.value.id, row.allocation_id)
  } catch (e) {
    const err = e as ApiError
    say('error', err.error === 'not_found' || err.error === 'invalid'
      ? 'That allocation no longer exists. Refresh the report.'
      : err.error === 'forbidden' || err.error === 'no_role_grant'
        ? 'Your role cannot export guest names.'
        : err.error === 'event_purged'
          ? 'Guest names for this event were erased, so there is nothing to list back.'
          : 'Could not prepare the list-back CSV. Check your connection and try again.')
    if (err.error === 'event_purged') await privacy.fetchEvent(current.value.id)
    return
  }
  const file = new File([csv], listBackFilename(current.value.slug, row.submitter), { type: 'text/csv;charset=utf-8' })
  const how = await deliver(file)
  if (how === 'cancelled') return
  const { guests, arrived } = listBackCounts(csv)
  say('success', `List back for ${row.submitter} ${how}: ${guests} ${guests === 1 ? 'guest' : 'guests'}, ${arrived} arrived. Send it to them.`)
}

function reload() {
  notice.value = ''
  return current.value ? store.load(current.value.id) : Promise.resolve()
}

function onPurged() {
  notice.value = ''
  return reload()
}
</script>

<template>
  <div v-if="current" class="space-y-3">
    <EventPrivacyBanner :event="current" @purged="onPurged" />
    <div style="display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px;">
      <div style="display:flex;flex-wrap:wrap;align-items:center;gap:8px;">
        <h2 class="section-lbl" style="margin:0;" data-testid="report-heading">{{ heading }}</h2>
        <span v-if="phase === 'live'" class="badge-hud badge-scheduled" data-testid="report-live">
          <Radio style="width:10px;height:10px;" aria-hidden="true" /> LIVE
        </span>
        <span v-if="shown" class="data-frag" style="font-size:10px;" data-testid="report-updated">{{ updated }}</span>
      </div>
      <div style="display:flex;flex-wrap:wrap;align-items:center;gap:8px;">
        <a v-if="shown && active && listBacks" href="#report-submitters" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" data-testid="report-list-backs-link">
          LIST BACKS ({{ listBacks }}) ↓
        </a>
        <button v-if="shown" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" :disabled="loading" @click="reload">
          <RefreshCw style="width:14px;height:14px;" aria-hidden="true" /> {{ loading ? 'REFRESHING…' : 'REFRESH' }}
        </button>
      </div>
    </div>

    <p
      v-if="notice" :role="noticeKind === 'error' ? 'alert' : 'status'" class="glass notice" :class="`notice-${noticeKind}`"
      data-testid="report-notice"
    >
      {{ notice }}
    </p>
    <p v-if="loadError" role="alert" class="glass notice notice-error" data-testid="report-error">
      {{ loadError.text }}
      <button v-if="loadError.retry" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" :disabled="loading" @click="reload">RETRY</button>
    </p>
    <p v-else-if="loading && !shown" role="status" class="data-frag" style="font-size:11px;">LOADING REPORT…</p>

    <template v-if="shown">
      <KhEmptyState
        v-if="!active" :icon="ChartNoAxesColumn" title="NOTHING TO REPORT YET"
        :hint="phase === 'past'
          ? 'No check-ins, walk-ups or scans were recorded for this night.'
          : 'The report fills in once the door starts checking people in: arrivals, no-shows, the crowd curve and list-backs for each submitter.'"
        :action-label="phase === 'past' ? undefined : 'OPEN DOOR SETUP'"
        :action-to="phase === 'past' ? undefined : `/events/${current.id}/door`"
      />
      <template v-else>
        <ReportKpiTiles :totals="shown.totals" :curve="shown.curve" :capacity="shown.event.capacity" :tz="tz" :event-id="current.id" />
        <ReportCheckinCurve v-if="shown.curve.length" :curve="shown.curve" :tz="tz" />
        <div class="report-grid">
          <ReportSubmitterTable :rows="shown.by_submitter" :busy="busy" :purged="!!purgedAt" @list-back="listBack" />
          <ReportListTable :rows="shown.by_list" />
        </div>
        <ReportTicketTypes v-if="shown.tickets_by_type.length" :rows="shown.tickets_by_type" />
      </template>
    </template>
  </div>
</template>

<style scoped>
.notice {
  padding: 10px 14px;
  font-size: 13px;
  margin: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.notice-error { border-left: 3px solid var(--color-error); }
.notice-success { border-left: 3px solid var(--color-status-ready); }
.report-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}
@media (min-width: 1400px) {
  .report-grid { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
}
</style>
