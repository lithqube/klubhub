<script setup lang="ts">
import type { EventDetail, Issue, LineupEntry, Stage } from '~/types/event'
import { timeLabel } from '~/utils/datetime'

/** Mobile / backstage timetable and the screen-reader route (UX §4.5). */
const props = defineProps<{ event: EventDetail, stages: Stage[], lineup: LineupEntry[], issues: Issue[], readonly?: boolean }>()
const emit = defineEmits<{ edit: [id: string] }>()

const stageId = ref(props.stages[0]?.id ?? null)
watch(() => props.stages, s => { if (!s.some(x => x.id === stageId.value)) stageId.value = s[0]?.id ?? null })
const stage = computed(() => props.stages.find(s => s.id === stageId.value) ?? null)
const sets = computed(() => props.lineup
  .filter(l => l.stage_id === stageId.value && l.set_start)
  .sort((a, b) => Date.parse(a.set_start!) - Date.parse(b.set_start!)))
const untimed = computed(() => props.lineup.filter(l => !l.set_start))
const bad = computed(() => new Set(props.issues.filter(i => i.severity === 'error').flatMap(i => i.entry_ids ?? [])))
const now = Date.now()
const onNow = (l: LineupEntry) => Date.parse(l.set_start!) <= now && now < Date.parse(l.set_end!)
</script>

<template>
  <section class="glass" style="padding:12px;" aria-label="Timetable list">
    <div role="tablist" aria-label="Stages" style="display:flex;gap:6px;overflow-x:auto;margin-bottom:10px;">
      <button
        v-for="s in stages" :key="s.id" type="button" role="tab" class="theme-opt" :class="{ active: s.id === stageId }"
        :aria-selected="s.id === stageId" style="min-height:44px;padding:0 14px;" @click="stageId = s.id"
      >
        {{ s.name }}
      </button>
    </div>
    <p v-if="stage" class="section-lbl" style="margin:0 0 8px;">
      TIMES IN {{ event.timezone.split('/').pop()?.toUpperCase() }}<template v-if="stage.curfew_at"> · CURFEW {{ timeLabel(stage.curfew_at, event.timezone) }}</template>
    </p>
    <ol style="list-style:none;margin:0;padding:0;">
      <li v-for="l in sets" :key="l.id">
        <button
          type="button" :disabled="readonly"
          style="display:flex;width:100%;align-items:center;gap:10px;min-height:56px;padding:0 10px;background:none;border:0;border-left:3px solid var(--color-primary);color:inherit;text-align:left;"
          :style="bad.has(l.id) ? 'border-left-color:var(--color-error);' : ''"
          @click="emit('edit', l.id)"
        >
          <span style="font-family:var(--font-terminal);font-size:12px;width:92px;">{{ timeLabel(l.set_start!, event.timezone) }}–{{ timeLabel(l.set_end!, event.timezone) }}</span>
          <span style="flex:1;font-size:14px;">{{ l.display_name }}<span v-if="bad.has(l.id)" style="color:var(--color-error);"> ⚠ conflict</span></span>
          <span v-if="onNow(l)" class="data-frag" style="font-size:8px;"><span class="pulse-dot" aria-hidden="true" /> ON NOW</span>
          <span aria-hidden="true">›</span>
        </button>
      </li>
    </ol>
    <p v-if="!sets.length" style="font-size:13px;color:var(--color-on-surface-variant);">Nothing scheduled on this stage yet.</p>
    <div v-if="untimed.length" style="margin-top:10px;">
      <div class="section-lbl">UNTIMED · {{ untimed.length }}</div>
      <button
        v-for="l in untimed" :key="l.id" type="button" :disabled="readonly"
        style="display:flex;width:100%;justify-content:space-between;min-height:48px;padding:0 10px;background:none;border:0;color:inherit;"
        @click="emit('edit', l.id)"
      >
        <span>{{ l.display_name }}</span><span aria-hidden="true">SCHEDULE ›</span>
      </button>
    </div>
  </section>
</template>
