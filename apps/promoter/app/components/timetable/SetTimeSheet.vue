<script setup lang="ts">
import type { EventDetail, LineupEntry, Stage } from '~/types/event'
import { instantToZoned, timeLabel, zonedToInstant } from '~/utils/datetime'

/** Set-time editor with 48px steppers (UX §4.5); applies to the local draft. */
const props = defineProps<{ event: EventDetail, stages: Stage[], entry: LineupEntry | null, lineup: LineupEntry[] }>()
const emit = defineEmits<{ close: [], apply: [LineupEntry[]] }>()

const MIN = 60_000
const open = computed({ get: () => !!props.entry, set: v => { if (!v) emit('close') } })
const start = ref(0)
const end = ref(0)
const stageId = ref<string | null>(null)
const ripple = ref(false)

watch(() => props.entry, (e) => {
  if (!e) return
  const def = Date.parse(props.event.starts_at)
  start.value = e.set_start ? Date.parse(e.set_start) : def
  end.value = e.set_end ? Date.parse(e.set_end) : def + 60 * MIN
  stageId.value = e.stage_id ?? props.stages[0]?.id ?? null
  ripple.value = false
}, { immediate: true })

const tz = computed(() => props.event.timezone)
const fmt = (ms: number) => timeLabel(new Date(ms).toISOString(), tz.value)
function setTime(which: 'start' | 'end', hhmm: string) {
  const ref_ = which === 'start' ? start : end
  const { date } = instantToZoned(new Date(ref_.value).toISOString(), tz.value)
  let t = zonedToInstant(date, hhmm, tz.value).getTime()
  if (which === 'end' && t <= start.value) t += 24 * 60 * MIN // overnight
  ref_.value = t
}
const stage = computed(() => props.stages.find(s => s.id === stageId.value) ?? null)
const curfewNote = computed(() => {
  if (!stage.value?.curfew_at) return null
  const over = Math.round((end.value - Date.parse(stage.value.curfew_at)) / MIN)
  return over > 0 ? `Ends ${over} min past the ${stage.value.name} curfew.` : null
})

function apply() {
  const e = props.entry!
  const delta = start.value - (e.set_start ? Date.parse(e.set_start) : start.value)
  const next = props.lineup.map((l) => {
    if (l.id === e.id || (e.b2b_group != null && l.b2b_group === e.b2b_group && l.stage_id === e.stage_id)) {
      return { ...l, stage_id: stageId.value, set_start: new Date(start.value).toISOString(), set_end: new Date(end.value).toISOString() }
    }
    if (ripple.value && delta && l.stage_id === e.stage_id && l.set_start && e.set_start && Date.parse(l.set_start) > Date.parse(e.set_start)) {
      return { ...l, set_start: new Date(Date.parse(l.set_start) + delta).toISOString(), set_end: new Date(Date.parse(l.set_end!) + delta).toISOString() }
    }
    return l
  })
  emit('apply', next)
}
function unschedule() {
  emit('apply', props.lineup.map(l => (l.id === props.entry!.id ? { ...l, set_start: null, set_end: null } : l)))
}
</script>

<template>
  <Sheet v-model:open="open" side="bottom">
    <form v-if="entry" style="padding:16px;display:grid;gap:12px;max-width:560px;margin:0 auto;" @submit.prevent="apply">
      <div class="section-lbl">{{ entry.display_name }} · {{ stage?.name ?? 'NO STAGE' }}</div>
      <div v-for="which in (['start', 'end'] as const)" :key="which" style="display:flex;align-items:center;gap:6px;">
        <span class="section-lbl" style="width:48px;">{{ which.toUpperCase() }}</span>
        <button v-for="d in [-15, -5]" :key="d" type="button" class="btn-hud btn-hud-ghost" style="min-width:48px;min-height:48px;" :aria-label="`${which} ${d} minutes`" @click="which === 'start' ? (start += d * MIN) : (end += d * MIN)">{{ d }}</button>
        <input
          :aria-label="`${which} time`" class="hud-input" type="time" style="width:110px;text-align:center;"
          :value="fmt(which === 'start' ? start : end)" @change="setTime(which, ($event.target as HTMLInputElement).value)"
        >
        <button v-for="d in [5, 15]" :key="d" type="button" class="btn-hud btn-hud-ghost" style="min-width:48px;min-height:48px;" :aria-label="`${which} +${d} minutes`" @click="which === 'start' ? (start += d * MIN) : (end += d * MIN)">+{{ d }}</button>
      </div>
      <label style="display:flex;align-items:center;gap:8px;min-height:44px;font-size:13px;">
        <input v-model="ripple" type="checkbox"> Shift all later sets on this stage too
      </label>
      <p v-if="curfewNote" role="status" style="margin:0;color:var(--color-error);font-size:13px;">⚠ {{ curfewNote }}</p>
      <label><span class="section-lbl">STAGE</span>
        <select v-model="stageId" class="hud-input"><option v-for="s in stages" :key="s.id" :value="s.id">{{ s.name }}</option></select>
      </label>
      <div style="display:flex;gap:8px;justify-content:space-between;">
        <button v-if="entry.set_start" type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="unschedule">UNSCHEDULE</button>
        <span />
        <div style="display:flex;gap:8px;">
          <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="emit('close')">CANCEL</button>
          <button type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="!stageId || end <= start">APPLY</button>
        </div>
      </div>
    </form>
  </Sheet>
</template>
