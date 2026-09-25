<script setup lang="ts">
import type { EventDetail, Issue, LineupEntry, Stage } from '~/types/event'
import { timeLabel } from '~/utils/datetime'

/** Conflict summary with jump links (UX §4.4, role=status). */
const props = defineProps<{ event: EventDetail, issues: Issue[], stages: Stage[], lineup: LineupEntry[] }>()
const emit = defineEmits<{ focus: [id: string] }>()

const name = (id?: string) => props.lineup.find(l => l.id === id)?.display_name ?? 'An act'
const stageName = (id?: string) => props.stages.find(s => s.id === id)?.name.toUpperCase() ?? ''
const t = (iso?: string) => (iso ? timeLabel(iso, props.event.timezone) : '')
const errors = computed(() => props.issues.filter(i => i.severity === 'error'))
const warnings = computed(() => props.issues.filter(i => i.severity === 'warning' && i.code !== 'untimed'))

function text(i: Issue): string {
  const [a, b] = i.entry_ids ?? []
  switch (i.code) {
    case 'overlap': return `OVERLAP · ${stageName(i.stage_id)} · ${name(a)} and ${name(b)} overlap by ${i.minutes} min. Move one set or make them B2B.`
    case 'past_curfew': return `PAST CURFEW ${t(i.from)} · ${name(a)} ends ${i.minutes} min after the ${stageName(i.stage_id)} curfew.`
    case 'outside_event': return `OUTSIDE THE NIGHT · ${name(a)} plays before doors or after the end.`
    case 'no_stage': return `NO STAGE · ${name(a)} has times but no stage.`
    case 'unknown_stage': return `STAGE REMOVED · ${name(a)} is on a stage that no longer exists.`
    case 'b2b_mismatch': return 'B2B PARTNERS DIFFER · back-to-back acts need the same stage and times.'
    case 'short_changeover': return `CHANGEOVER SHORT · ${stageName(i.stage_id)} · ${i.minutes} min between ${name(a)} and ${name(b)}.`
    case 'dead_air': return `DEAD AIR · ${stageName(i.stage_id)} · ${t(i.from)}–${t(i.to)} (${i.minutes} min) nobody is playing.`
    default: return i.code
  }
}
</script>

<template>
  <div role="status" aria-live="polite">
    <div v-if="errors.length" class="glass" style="padding:10px 14px;border-left:3px solid var(--color-error);">
      <div class="section-lbl" style="color:var(--color-error);">⚠ {{ errors.length }} {{ errors.length === 1 ? 'CONFLICT' : 'CONFLICTS' }} · PUBLISHING AND EXPORT ARE BLOCKED UNTIL FIXED</div>
      <ul style="margin:6px 0 0;padding-left:18px;font-size:13px;display:grid;gap:4px;">
        <li v-for="(i, k) in errors" :key="k">
          {{ text(i) }}
          <button v-if="i.entry_ids?.[0]" type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" @click="emit('focus', i.entry_ids[0])">SHOW</button>
        </li>
      </ul>
    </div>
    <details v-if="warnings.length" class="glass" style="padding:8px 14px;margin-top:6px;">
      <summary class="section-lbl" style="cursor:pointer;min-height:32px;display:flex;align-items:center;">{{ warnings.length }} {{ warnings.length === 1 ? 'NOTE' : 'NOTES' }} (DON'T BLOCK PUBLISHING)</summary>
      <ul style="margin:6px 0 0;padding-left:18px;font-size:13px;"><li v-for="(i, k) in warnings" :key="k">{{ text(i) }}</li></ul>
    </details>
  </div>
</template>
