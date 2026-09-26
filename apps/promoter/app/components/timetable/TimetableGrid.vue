<script setup lang="ts">
import type { EventDetail, Issue, LineupEntry, Stage } from '~/types/event'
import { timeLabel } from '~/utils/datetime'

/**
 * Per-stage timetable (UX §4.4). Pointer: drag a set to move it (across
 * stages too), drag its bottom edge to resize. Keyboard (UX §7): focus a
 * set, Space picks it up, ↑/↓ move by the snap (Shift = 5 min), Alt+↑/↓
 * resize, ←/→ change stage, Space drops, Escape cancels. Every change is
 * announced in a polite live region.
 */
const props = defineProps<{
  event: EventDetail
  stages: Stage[]
  lineup: LineupEntry[]
  issues: Issue[]
  readonly?: boolean
  snap: number
}>()
const emit = defineEmits<{ update: [LineupEntry[]], edit: [id: string] }>()

const PX_PER_MIN = 1.1
const MIN = 60_000
const t0 = computed(() => Date.parse(props.event.doors_at ?? props.event.starts_at))
const t1 = computed(() => Date.parse(props.event.ends_at))
const height = computed(() => ((t1.value - t0.value) / MIN) * PX_PER_MIN)
const y = (iso: string) => ((Date.parse(iso) - t0.value) / MIN) * PX_PER_MIN

const hours = computed(() => {
  const out: { top: number, label: string, midnight: boolean }[] = []
  const first = new Date(t0.value)
  first.setMinutes(0, 0, 0)
  for (let t = first.getTime(); t <= t1.value; t += 60 * MIN) {
    if (t < t0.value) continue
    const label = timeLabel(new Date(t).toISOString(), props.event.timezone)
    out.push({ top: ((t - t0.value) / MIN) * PX_PER_MIN, label, midnight: label === '00:00' })
  }
  return out
})

const conflictIds = computed(() => {
  const s = new Set<string>()
  for (const i of props.issues) if (i.severity === 'error') i.entry_ids?.forEach(id => s.add(id))
  return s
})
const stageIssues = (id: string, code: string) => props.issues.filter(i => i.stage_id === id && i.code === code)
const timed = (stageId: string) => props.lineup.filter(l => l.stage_id === stageId && l.set_start && l.set_end)

// ---- pointer drag --------------------------------------------------------
const columns = ref<HTMLElement[]>([])
const drag = ref<null | { id: string, mode: 'move' | 'resize', y0: number, start: number, end: number, stage: string | null }>(null)
const preview = ref<null | { id: string, start: number, end: number, stage: string | null }>(null)
const snapMin = (m: number, step: number) => Math.round(m / step) * step

function onPointerDown(e: PointerEvent, l: LineupEntry, mode: 'move' | 'resize') {
  if (props.readonly || !l.set_start || !l.set_end || e.button !== 0) return
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  drag.value = { id: l.id, mode, y0: e.clientY, start: Date.parse(l.set_start), end: Date.parse(l.set_end), stage: l.stage_id }
  preview.value = { id: l.id, start: drag.value.start, end: drag.value.end, stage: l.stage_id }
}
function onPointerMove(e: PointerEvent) {
  const d = drag.value
  if (!d) return
  const step = e.shiftKey ? 5 : props.snap
  const dm = snapMin((e.clientY - d.y0) / PX_PER_MIN, step) * MIN
  let stage = d.stage
  if (d.mode === 'move') {
    const col = columns.value.findIndex(c => { const r = c.getBoundingClientRect(); return e.clientX >= r.left && e.clientX <= r.right })
    if (col >= 0) stage = props.stages[col]!.id
  }
  preview.value = d.mode === 'move'
    ? { id: d.id, start: d.start + dm, end: d.end + dm, stage }
    : { id: d.id, start: d.start, end: Math.max(d.start + step * MIN, d.end + dm), stage }
}
function onPointerUp() {
  if (drag.value && preview.value) commit(preview.value.id, preview.value.start, preview.value.end, preview.value.stage)
  drag.value = null
  preview.value = null
}

function commit(id: string, start: number, end: number, stage: string | null) {
  const l = props.lineup.find(x => x.id === id)
  if (!l) return
  const moved = { set_start: new Date(start).toISOString(), set_end: new Date(end).toISOString(), stage_id: stage }
  // B2B partners move together.
  emit('update', props.lineup.map(x => (x.id === id || (l.b2b_group != null && x.b2b_group === l.b2b_group && x.stage_id === l.stage_id)) ? { ...x, ...moved } : x))
  const name = props.stages.find(s => s.id === stage)?.name ?? 'no stage'
  announce(`${l.display_name}, ${name}, ${timeLabel(moved.set_start, props.event.timezone)} to ${timeLabel(moved.set_end, props.event.timezone)}.`)
}

// ---- keyboard --------------------------------------------------------------
const grabbed = ref<null | { id: string, start: number, end: number, stage: string | null }>(null)
const live = ref('')
function announce(msg: string) {
  live.value = ''
  nextTick(() => { live.value = msg })
}
function onKey(e: KeyboardEvent, l: LineupEntry) {
  if (props.readonly || !l.set_start || !l.set_end) return
  if (e.key === 'Enter') {
    emit('edit', l.id)
    return
  }
  if (e.key === ' ') {
    e.preventDefault()
    if (grabbed.value) {
      grabbed.value = null
      announce(`${l.display_name} dropped.`)
    } else {
      grabbed.value = { id: l.id, start: Date.parse(l.set_start), end: Date.parse(l.set_end), stage: l.stage_id }
      announce(`${l.display_name} picked up. Arrow keys move, Space drops, Escape cancels.`)
    }
    return
  }
  const g = grabbed.value
  if (!g || g.id !== l.id) return
  if (e.key === 'Escape') {
    commit(l.id, g.start, g.end, g.stage)
    grabbed.value = null
    announce('Move cancelled.')
    return
  }
  const step = (e.shiftKey ? 5 : props.snap) * MIN
  const s = Date.parse(l.set_start)
  const en = Date.parse(l.set_end)
  const idx = props.stages.findIndex(st => st.id === l.stage_id)
  if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
    e.preventDefault()
    const d = e.key === 'ArrowUp' ? -step : step
    if (e.altKey) commit(l.id, s, Math.max(s + step, en + d), l.stage_id)
    else commit(l.id, s + d, en + d, l.stage_id)
  } else if ((e.key === 'ArrowLeft' && idx > 0) || (e.key === 'ArrowRight' && idx < props.stages.length - 1)) {
    e.preventDefault()
    commit(l.id, s, en, props.stages[idx + (e.key === 'ArrowLeft' ? -1 : 1)]!.id)
  }
}

function blockStyle(l: LineupEntry) {
  const p = preview.value?.id === l.id ? preview.value : null
  const start = p ? new Date(p.start).toISOString() : l.set_start!
  const end = p ? new Date(p.end).toISOString() : l.set_end!
  return { top: `${y(start)}px`, height: `${Math.max(44, y(end) - y(start))}px`, opacity: p && p.stage !== l.stage_id ? 0.4 : 1 }
}
</script>

<template>
  <div>
    <div class="sr-only" aria-live="polite">{{ live }}</div>
    <div style="display:grid;gap:0;overflow-x:auto;" :style="{ gridTemplateColumns: `56px repeat(${stages.length}, minmax(180px, 1fr))` }">
      <div />
      <div v-for="s in stages" :key="s.id" class="section-lbl" style="padding:6px 8px;">
        {{ s.name }} · CO {{ s.changeover_minutes }}′<template v-if="s.curfew_at"> · CURFEW {{ timeLabel(s.curfew_at, event.timezone) }}</template>
      </div>

      <div style="position:relative;" :style="{ height: `${height}px` }" aria-hidden="true">
        <div v-for="h in hours" :key="h.top" style="position:absolute;right:6px;font-family:var(--font-terminal);font-size:9px;color:var(--color-tertiary);" :style="{ top: `${h.top - 6}px` }">
          {{ h.label }}<span v-if="h.midnight" style="display:block;color:var(--color-secondary);">+1</span>
        </div>
      </div>

      <section
        v-for="s in stages" :key="s.id" ref="columns"
        :aria-label="`${s.name}, changeover ${s.changeover_minutes} minutes${s.curfew_at ? `, curfew ${timeLabel(s.curfew_at, event.timezone)}` : ''}`"
        style="position:relative;border-left:1px dashed color-mix(in srgb, var(--color-primary) 15%, transparent);"
        :style="{ height: `${height}px` }"
        @pointermove="onPointerMove" @pointerup="onPointerUp" @pointercancel="onPointerUp"
      >
        <div v-for="h in hours" :key="h.top" aria-hidden="true" style="position:absolute;left:0;right:0;border-top:1px solid color-mix(in srgb, var(--color-on-surface) 6%, transparent);" :style="{ top: `${h.top}px` }" />
        <div
          v-for="(d, i) in stageIssues(s.id, 'dead_air')" :key="`dead-${i}`" aria-hidden="true"
          style="position:absolute;left:4px;right:4px;background:repeating-linear-gradient(45deg, transparent 0 6px, color-mix(in srgb, var(--color-secondary) 14%, transparent) 6px 8px);font-size:9px;padding:2px 6px;color:var(--color-secondary);"
          :style="{ top: `${y(d.from!)}px`, height: `${y(d.to!) - y(d.from!)}px` }"
        >
          DEAD AIR {{ d.minutes }}′
        </div>
        <div
          v-for="(o, i) in stageIssues(s.id, 'overlap')" :key="`ov-${i}`" aria-hidden="true"
          style="position:absolute;left:0;right:0;z-index:3;pointer-events:none;background:repeating-linear-gradient(-45deg, color-mix(in srgb, var(--color-error) 35%, transparent) 0 4px, transparent 4px 8px);"
          :style="{ top: `${y(o.from!)}px`, height: `${Math.max(4, y(o.to!) - y(o.from!))}px` }"
        />
        <div
          v-if="s.curfew_at" aria-hidden="true"
          style="position:absolute;left:0;right:0;z-index:4;border-top:2px dashed var(--color-error);font-family:var(--font-terminal);font-size:8px;color:var(--color-error);padding-left:4px;"
          :style="{ top: `${y(s.curfew_at)}px` }"
        >
          CURFEW {{ timeLabel(s.curfew_at, event.timezone) }}
        </div>
        <ol style="list-style:none;margin:0;padding:0;">
          <li v-for="l in timed(s.id)" :key="l.id">
            <button
              type="button"
              class="glass"
              :aria-label="`${l.display_name}, ${timeLabel(l.set_start!, event.timezone)} to ${timeLabel(l.set_end!, event.timezone)}, ${s.name}${conflictIds.has(l.id) ? ', conflict' : ''}${l.b2b_group != null ? ', back to back' : ''}`"
              :aria-pressed="grabbed?.id === l.id"
              style="position:absolute;left:6px;right:6px;z-index:2;text-align:left;padding:6px 8px;cursor:grab;touch-action:none;border-left:3px solid var(--color-primary);overflow:hidden;"
              :style="[blockStyle(l), conflictIds.has(l.id) ? 'border-left-color:var(--color-error);outline:1px solid var(--color-error);' : '', grabbed?.id === l.id ? 'outline:2px solid var(--color-primary);' : '']"
              @pointerdown="onPointerDown($event, l, 'move')" @keydown="onKey($event, l)" @dblclick="emit('edit', l.id)"
            >
              <span style="display:block;font-family:var(--font-command);font-size:12px;font-weight:700;text-transform:uppercase;">
                {{ l.display_name }}<span v-if="conflictIds.has(l.id)" style="color:var(--color-error);"> ⚠</span>
              </span>
              <span style="font-size:10px;color:var(--color-on-surface-variant);">
                {{ timeLabel(l.set_start!, event.timezone) }}–{{ timeLabel(l.set_end!, event.timezone) }}<template v-if="l.b2b_group != null"> · B2B</template>
              </span>
              <span
                v-if="!readonly" aria-hidden="true"
                style="position:absolute;left:0;right:0;bottom:0;height:10px;cursor:ns-resize;"
                @pointerdown.stop="onPointerDown($event, l, 'resize')"
              />
            </button>
          </li>
        </ol>
      </section>
    </div>
  </div>
</template>
