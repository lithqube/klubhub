<script setup lang="ts">
import { ChartNoAxesColumn, Radio, RefreshCw } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useEventStore } from '~/stores/event'
import { useReportStore } from '~/stores/report'
import type { ApiError } from '~/types/event'
import type { ReportSubmitterRow } from '~/types/report'
import { dayLabel, timeLabel } from '~/utils/datetime'
import { hasActivity, listBackFilename, reportPhase } from '~/utils/report'

/**
 * Event REPORT tab (P2.4): who came, when and through which list. Works
 * during the night too (LIVE badge, refresh). Aggregates only; names leave
 * solely through a per-allocation LIST BACK CSV.
 */
const { current } = storeToRefs(useEventStore())
const store = useReportStore()
const { report, loading, error } = storeToRefs(store)

const id = computed(() => current.value?.id ?? '')
await useAsyncData(() => `report-${id.value}`, () => (id.value ? store.load(id.value).then(() => true) : Promise.resolve(null)), { watch: [id] })

// A report left over from another event is never shown.
const shown = computed(() => (report.value && report.value.event.id === id.value ? report.value : null))
const tz = computed(() => shown.value?.event.timezone ?? current.value?.timezone ?? 'UTC')
const phase = computed(() => (shown.value ? reportPhase(shown.value) : null))
const active = computed(() => (shown.value ? hasActivity(shown.value) : false))
const updated = computed(() => (shown.value ? `${dayLabel(shown.value.generated_at, tz.value)} ${timeLabel(shown.value.generated_at, tz.value)}` : ''))

const busy = ref<string | null>(null)
const notice = ref('')

async function listBack(row: ReportSubmitterRow) {
  if (!current.value) return
  busy.value = row.allocation_id
  notice.value = ''
  try {
    const csv = await store.listBackCsv(current.value.id, row.allocation_id)
    const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }))
    const a = document.createElement('a')
    a.href = url
    a.download = listBackFilename(current.value.slug, row.submitter)
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (e) {
    const err = e as ApiError
    notice.value = err.error === 'not_found' || err.error === 'invalid'
      ? 'That allocation no longer exists. Refresh the report.'
      : err.error === 'forbidden' || err.error === 'no_role_grant'
        ? 'Your role cannot export guest names.'
        : 'Could not prepare the list-back CSV. Check your connection and try again.'
  } finally {
    busy.value = null
  }
}

const reload = () => (current.value ? store.load(current.value.id) : Promise.resolve())
</script>

<template>
  <div v-if="current" class="space-y-3">
    <div style="display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px;">
      <div style="display:flex;flex-wrap:wrap;align-items:center;gap:8px;">
        <h2 class="section-lbl" style="margin:0;">POST-EVENT REPORT</h2>
        <span v-if="phase === 'live'" class="badge-hud badge-scheduled" data-testid="report-live">
          <Radio style="width:10px;height:10px;" aria-hidden="true" /> LIVE
        </span>
        <span v-if="shown" class="data-frag" style="font-size:8px;">UPDATED {{ updated }} · {{ tz.toUpperCase() }}</span>
      </div>
      <button v-if="shown" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" :disabled="loading" @click="reload">
        <RefreshCw style="width:14px;height:14px;" aria-hidden="true" /> {{ loading ? 'REFRESHING…' : 'REFRESH' }}
      </button>
    </div>

    <p v-if="notice" role="alert" class="glass" style="padding:10px 14px;border-left:3px solid var(--color-error);font-size:13px;margin:0;">{{ notice }}</p>
    <p v-if="error" role="alert" class="glass" style="padding:10px 14px;border-left:3px solid var(--color-error);font-size:13px;margin:0;">
      {{ shown ? 'COULD NOT REFRESH THE REPORT. SHOWING THE LAST NUMBERS.' : 'COULD NOT LOAD THE REPORT.' }}
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" :disabled="loading" @click="reload">RETRY</button>
    </p>
    <p v-else-if="loading && !shown" role="status" class="data-frag" style="font-size:9px;">LOADING REPORT…</p>

    <template v-if="shown">
      <KhEmptyState
        v-if="!active" :icon="ChartNoAxesColumn" title="NOTHING TO REPORT YET"
        :hint="phase === 'past'
          ? 'No check-ins, walk-ups or scans were recorded for this night.'
          : 'The report fills in once the door starts checking people in: arrivals, no-shows, the crowd curve and list-backs for each submitter.'"
      />
      <template v-else>
        <ReportKpiTiles :totals="shown.totals" :capacity="shown.event.capacity" :tz="tz" />
        <ReportCheckinCurve v-if="shown.curve.length" :curve="shown.curve" :tz="tz" />
        <div class="report-grid">
          <ReportSubmitterTable :rows="shown.by_submitter" :busy="busy" @list-back="listBack" />
          <ReportListTable :rows="shown.by_list" />
        </div>
        <ReportTicketTypes v-if="shown.tickets_by_type.length" :rows="shown.tickets_by_type" />
      </template>
    </template>
  </div>
</template>

<style scoped>
.report-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}
@media (min-width: 1200px) {
  .report-grid { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
}
</style>
