<script setup lang="ts">
import type { ExportData } from '~/types/event'
import { dayLabel, timeLabel, zoneAbbrev } from '~/utils/datetime'

const props = defineProps<{ data: ExportData, currentVersion: number, unsavedDraft: boolean }>()
defineEmits<{ rebuild: [] }>()

const e = computed(() => props.data.event)
const at = (iso: string) => `${dayLabel(iso, e.value.timezone)} ${timeLabel(iso, e.value.timezone)} ${zoneAbbrev(iso, e.value.timezone)}`
const venueLine = computed(() => {
  const l = props.data.location
  if (!l.withheld) return null
  if (e.value.location_mode === 'city_only') return `Venue name & address — CITY ONLY, never exported. Pack says "${l.name}".`
  return l.reveal_at
    ? `Venue name & address — SECRET until ${at(l.reveal_at)}. Pack says "${l.name}". Rebuild after the reveal.`
    : `Venue name & address — SECRET with no reveal time; hidden until you set one. Pack says "${l.name}".`
})
const embargoLine = computed(() => {
  if (!props.data.embargoed) return null
  if (e.value.status === 'draft') return 'DRAFT — this event is not published. Don\'t post the pack yet.'
  return `EMBARGO — goes live ${at(e.value.publish_at!)}. Don't post before then.`
})
const stale = computed(() => props.currentVersion !== e.value.version)
</script>

<template>
  <section class="glass" aria-labelledby="withheld-h" style="padding:14px 16px;border-left:3px solid var(--color-secondary);">
    <h2 id="withheld-h" class="section-lbl" style="margin:0 0 8px;">WITHHELD FROM THIS PACK</h2>
    <ul style="list-style:none;margin:0;padding:0;display:grid;gap:6px;font-size:13px;">
      <li v-if="venueLine"><span aria-hidden="true">⊘ </span>{{ venueLine }}</li>
      <li><span aria-hidden="true">⊘ </span>Venue contacts, tech notes and capacity — internal, never exported.</li>
      <li v-if="e.visibility === 'private'"><span aria-hidden="true">⊘ </span>PRIVATE event — only the calendar file and copy fields. No page, embed or structured data.</li>
      <li v-if="embargoLine" style="color:var(--color-secondary);font-weight:600;"><span aria-hidden="true">◷ </span>{{ embargoLine }}</li>
      <li v-if="unsavedDraft"><span aria-hidden="true">△ </span>Unsaved timetable changes on this device are not in the pack. <NuxtLink :to="`/events/${e.id}/lineup`" style="color:var(--color-primary);">Save them first →</NuxtLink></li>
    </ul>
    <div style="display:flex;flex-wrap:wrap;align-items:center;gap:8px;margin-top:10px;">
      <span class="data-frag" style="font-size:9px;">BUILT FROM VERSION {{ e.version }} · {{ stale ? 'EVENT CHANGED' : 'UP TO DATE' }}</span>
      <button v-if="stale" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="$emit('rebuild')">REBUILD</button>
    </div>
  </section>
</template>
