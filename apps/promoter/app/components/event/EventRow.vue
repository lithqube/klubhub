<script setup lang="ts">
import { Globe, Link2, Lock, EyeOff, Clock } from 'lucide-vue-next'
import type { EventSummary } from '~/types/event'
import { dayLabel, rangeLabel } from '~/utils/datetime'

const props = defineProps<{ event: EventSummary }>()

const past = computed(() => Date.parse(props.event.ends_at) <= Date.now())
const accent = computed(() => ({
  draft: 'accent-bar-draft', scheduled: 'accent-bar-scheduled', published: 'accent-bar-ready',
  cancelled: 'accent-bar-failed', postponed: 'accent-bar-draft',
}[props.event.status]))
const day = computed(() => dayLabel(props.event.starts_at, props.event.timezone).split(' '))
const visibility = computed(() => ({
  public: { icon: Globe, label: 'PUBLIC' }, unlisted: { icon: Link2, label: 'UNLISTED' }, private: { icon: Lock, label: 'PRIVATE' },
}[props.event.visibility]))
const where = computed(() => {
  const e = props.event
  if (e.location_mode === 'secret') return `${e.city} · SECRET`
  if (e.location_mode === 'city_only') return e.city
  return e.venue_name ? `${e.venue_name}, ${e.city}` : e.city
})
const embargo = computed(() => props.event.publish_at && Date.parse(props.event.publish_at) > Date.now() ? props.event.publish_at : null)
</script>

<template>
  <NuxtLink
    :to="`/events/${event.id}/details`"
    class="glass"
    :class="accent"
    style="display:grid;grid-template-columns:64px 1fr;gap:14px;align-items:center;padding:12px 16px;text-decoration:none;color:inherit;border-left-width:3px;border-left-style:solid;min-height:64px;"
  >
    <div style="text-align:center;font-family:var(--font-command);line-height:1.05;">
      <div style="font-size:10px;color:var(--color-tertiary);">{{ day[0] }}</div>
      <div style="font-size:22px;font-weight:700;">{{ day[1] }}</div>
      <div style="font-size:10px;color:var(--color-tertiary);">{{ day[2] }}</div>
    </div>
    <div style="min-width:0;">
      <div style="display:flex;flex-wrap:wrap;align-items:center;gap:8px;">
        <span style="font-family:var(--font-command);font-size:15px;font-weight:700;text-transform:uppercase;">{{ event.title }}</span>
        <EventStatusBadge :status="event.status" :past="past" />
        <span class="data-frag" style="display:inline-flex;align-items:center;gap:4px;font-size:8px;">
          <component :is="visibility.icon" style="width:10px;height:10px;" aria-hidden="true" />{{ visibility.label }}
        </span>
        <span v-if="embargo" class="data-frag" style="display:inline-flex;align-items:center;gap:4px;font-size:8px;">
          <Clock style="width:10px;height:10px;" aria-hidden="true" />LIVE {{ dayLabel(embargo, event.timezone) }}
        </span>
        <span v-if="event.location_mode === 'secret'" class="data-frag" style="display:inline-flex;align-items:center;gap:4px;font-size:8px;">
          <EyeOff style="width:10px;height:10px;" aria-hidden="true" />SECRET
        </span>
      </div>
      <div style="margin-top:4px;font-family:var(--font-data);font-size:12px;color:var(--color-on-surface-variant);">
        {{ where }} · {{ rangeLabel(event.starts_at, event.ends_at, event.timezone) }} ·
        {{ event.act_count }} {{ event.act_count === 1 ? 'ACT' : 'ACTS' }} · {{ event.stage_count }} {{ event.stage_count === 1 ? 'STAGE' : 'STAGES' }}
        <span v-if="event.untimed_count" style="color:var(--color-secondary);"> · {{ event.untimed_count }} UNTIMED</span>
      </div>
    </div>
  </NuxtLink>
</template>
