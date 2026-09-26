<script setup lang="ts">
import type { EventSummary } from '~/types/event'
import { dayLabel, rangeLabel, zoneAbbrev } from '~/utils/datetime'

const props = defineProps<{ event: EventSummary, now: number }>()
const e = computed(() => props.event)

const countdown = computed(() => {
  const ms = Date.parse(e.value.starts_at) - props.now
  if (ms <= 0) return Date.parse(e.value.ends_at) > props.now ? 'LIVE NOW' : 'ENDED'
  const d = Math.floor(ms / 86_400_000)
  const h = Math.floor((ms % 86_400_000) / 3_600_000)
  const m = Math.floor((ms % 3_600_000) / 60_000)
  return d ? `T-${d}D ${String(h).padStart(2, '0')}H` : `T-${h}H ${String(m).padStart(2, '0')}M`
})
const checks = computed(() => [
  { ok: !!e.value.venue_id || e.value.location_mode !== 'venue', label: 'Place set' },
  { ok: e.value.act_count > 0, label: 'Lineup' },
  { ok: e.value.untimed_count === 0 && e.value.act_count > 0, label: 'Every act timed' },
  { ok: e.value.error_count === 0, label: 'No timetable conflicts' },
  { ok: !!e.value.external_ticket_url || !!e.value.cost_text, label: 'Tickets or price' },
  { ok: e.value.status === 'published', label: 'Published' },
])
const done = computed(() => checks.value.filter(c => c.ok).length)
const firstGap = computed(() => checks.value.find(c => !c.ok))
</script>

<template>
  <article class="glass hud-card" style="padding:18px;min-width:0;" aria-labelledby="next-h">
    <div style="display:flex;justify-content:space-between;gap:8px;align-items:center;">
      <span class="section-lbl" style="margin:0;">NEXT EVENT · <span :style="countdown === 'LIVE NOW' ? 'color:var(--color-primary)' : ''">{{ countdown }}</span></span>
      <EventStatusBadge :status="e.status" :past="false" />
    </div>
    <h2 id="next-h" style="font-family:var(--font-command);font-size:22px;font-weight:700;text-transform:uppercase;margin:8px 0 2px;overflow-wrap:anywhere;">{{ e.title }}</h2>
    <div style="font-size:13px;">{{ dayLabel(e.starts_at, e.timezone) }} · {{ rangeLabel(e.starts_at, e.ends_at, e.timezone) }} · {{ zoneAbbrev(e.starts_at, e.timezone) }}</div>
    <div style="font-size:12px;color:var(--color-on-surface-variant);">
      {{ [e.venue_name, e.city].filter(Boolean).join(' · ') }} · {{ e.stage_count }} {{ e.stage_count === 1 ? 'STAGE' : 'STAGES' }} · {{ e.act_count }} {{ e.act_count === 1 ? 'ACT' : 'ACTS' }}
    </div>
    <div style="margin-top:12px;">
      <div style="display:flex;justify-content:space-between;font-size:11px;" class="data-frag">
        <span>PREFLIGHT {{ done }}/{{ checks.length }}</span>
        <span v-if="firstGap">NEXT: {{ firstGap.label.toUpperCase() }}</span>
      </div>
      <div class="prog-track" role="progressbar" :aria-valuenow="done" aria-valuemin="0" :aria-valuemax="checks.length" aria-label="Preflight" style="margin-top:6px;">
        <div class="prog-fill" :style="{ width: `${(done / checks.length) * 100}%` }" />
      </div>
    </div>
    <div style="display:flex;flex-wrap:wrap;gap:6px;margin-top:14px;">
      <NuxtLink :to="`/events/${e.id}/lineup`" class="btn-hud btn-hud-cta" style="min-height:44px;">OPEN TIMETABLE</NuxtLink>
      <NuxtLink :to="`/events/${e.id}/export`" class="btn-hud btn-hud-ghost" style="min-height:44px;">EXPORT PACK</NuxtLink>
    </div>
  </article>
</template>
