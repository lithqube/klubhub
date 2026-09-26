<script setup lang="ts">
import type { EventDetail, StageInput } from '~/types/event'
import { instantToZoned, timeLabel, zonedToInstant } from '~/utils/datetime'

/** Stage list editor (name, changeover, curfew); saves immediately. */
const props = defineProps<{ event: EventDetail, busy?: boolean }>()
const emit = defineEmits<{ save: [StageInput[]] }>()

const rows = ref<{ id?: string, name: string, changeover: number, curfew: string }[]>([])
watch(() => props.event.stages, (s) => {
  rows.value = s.map(x => ({ id: x.id, name: x.name, changeover: x.changeover_minutes, curfew: x.curfew_at ? timeLabel(x.curfew_at, props.event.timezone) : '' }))
}, { immediate: true })

function curfewInstant(hhmm: string): string | null {
  if (!hhmm) return null
  const start = instantToZoned(props.event.starts_at, props.event.timezone)
  let t = zonedToInstant(start.date, hhmm, props.event.timezone).getTime()
  if (t <= Date.parse(props.event.starts_at)) t += 86_400_000 // curfew after midnight
  return new Date(t).toISOString()
}
function save() {
  emit('save', rows.value.filter(r => r.name.trim()).map(r => ({ id: r.id, name: r.name.trim(), changeover_minutes: r.changeover || 0, curfew_at: curfewInstant(r.curfew) })))
}
</script>

<template>
  <details class="glass" style="padding:10px 14px;">
    <summary class="section-lbl" style="cursor:pointer;min-height:32px;display:flex;align-items:center;">STAGES · {{ event.stages.length }}</summary>
    <form style="display:grid;gap:6px;margin-top:8px;" @submit.prevent="save">
      <div v-for="(r, i) in rows" :key="r.id ?? i" style="display:grid;grid-template-columns:1fr 110px 120px 44px;gap:6px;align-items:end;">
        <label><span class="section-lbl">NAME</span><input v-model="r.name" class="hud-input" maxlength="80"></label>
        <label><span class="section-lbl">CHANGEOVER ′</span><input v-model.number="r.changeover" class="hud-input" type="number" min="0" max="120"></label>
        <label><span class="section-lbl">CURFEW</span><input v-model="r.curfew" class="hud-input" type="time"></label>
        <button type="button" class="btn-hud btn-hud-ghost" style="min-height:40px;" :aria-label="`Remove stage ${r.name}`" @click="rows.splice(i, 1)">✕</button>
      </div>
      <div style="display:flex;gap:8px;">
        <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="rows.push({ name: `Stage ${rows.length + 1}`, changeover: 10, curfew: '' })">+ STAGE</button>
        <button type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="busy">SAVE STAGES</button>
      </div>
      <p style="margin:0;font-size:12px;color:var(--color-on-surface-variant);">Removing a stage leaves its acts unassigned.</p>
    </form>
  </details>
</template>
