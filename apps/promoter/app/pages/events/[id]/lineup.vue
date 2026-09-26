<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useOnline } from '@vueuse/core'
import { useEventStore } from '~/stores/event'
import type { ApiError, LineupEntry, StageInput } from '~/types/event'
import { hasErrors } from '~/utils/timetable'

const store = useEventStore()
const { current } = storeToRefs(store)
const { lineup, issues, dirtyCount, update, discard, toInput } = useTimetableDraft(current)
const online = useOnline()

const snap = ref(15)
const view = ref<'grid' | 'list'>('grid')
const editing = ref<string | null>(null)
const busy = ref(false)
const message = ref<{ kind: 'ok' | 'error', text: string } | null>(null)
const isPast = computed(() => !!current.value && Date.parse(current.value.ends_at) <= Date.now())
const untimedIds = computed(() => new Set(lineup.value.filter(l => !l.set_start).map(l => l.id)))
const editingEntry = computed<LineupEntry | null>(() => lineup.value.find(l => l.id === editing.value) ?? null)

async function save() {
  if (!current.value || !online.value) return
  busy.value = true
  message.value = null
  try {
    const d = await store.replaceLineup(current.value.id, current.value.version, toInput())
    discard()
    message.value = hasErrors(d.issues)
      ? { kind: 'error', text: 'SAVED WITH CONFLICTS · publishing and export stay blocked until they are fixed.' }
      : { kind: 'ok', text: 'TIMETABLE SAVED' }
  } catch (e) {
    const err = e as ApiError
    message.value = { kind: 'error', text: err.error === 'version_conflict' ? 'SOMEONE ELSE SAVED THIS EVENT. Your changes are still here; reload their version to compare.' : `NOT SAVED · ${err.problem ?? err.error}. Your changes are still here.` }
  } finally {
    busy.value = false
  }
}
async function saveStages(stages: StageInput[]) {
  if (!current.value) return
  busy.value = true
  try {
    await store.replaceStages(current.value.id, current.value.version, stages)
  } catch (e) {
    message.value = { kind: 'error', text: `STAGES NOT SAVED · ${(e as ApiError).problem ?? (e as ApiError).error}` }
  } finally {
    busy.value = false
  }
}
function focusEntry(id: string) {
  view.value = 'grid'
  nextTick(() => (document.querySelector(`[aria-label^="${CSS.escape(lineup.value.find(l => l.id === id)?.display_name ?? '')}"]`) as HTMLElement | null)?.focus())
}
function onKey(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 's') {
    e.preventDefault()
    if (dirtyCount.value) save()
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div v-if="current" class="space-y-3" style="padding-bottom:72px;">
    <p v-if="isPast" class="glass" style="padding:10px 14px;font-size:13px;">READ ONLY · this night is over.</p>
    <TimetableConflictPanel :event="current" :issues="issues" :stages="current.stages" :lineup="lineup" @focus="focusEntry" />
    <TimetableStagesEditor v-if="!isPast" :event="current" :busy="busy" @save="saveStages" />
    <div v-if="!current.stages.length" class="glass" style="padding:14px;font-size:13px;">
      Add a stage to start scheduling<template v-if="current.venue">, or set a venue with rooms on DETAILS</template>.
    </div>

    <div style="display:grid;gap:12px;" class="lg:grid-cols-[280px_1fr]">
      <TimetableLineupRoster class="order-2 lg:order-1" style="min-width:0;" :lineup="lineup" :readonly="isPast" :untimed-ids="untimedIds" @update="update" @schedule="id => (editing = id)" />
      <div class="space-y-2 order-1 lg:order-2" style="min-width:0;">
        <div style="display:flex;flex-wrap:wrap;gap:8px;align-items:center;">
          <div class="theme-seg hidden lg:flex" role="radiogroup" aria-label="Timetable view">
            <button v-for="v in (['grid', 'list'] as const)" :key="v" type="button" role="radio" class="theme-opt" :class="{ active: view === v }" :aria-checked="view === v" style="min-height:40px;" @click="view = v">{{ v.toUpperCase() }}</button>
          </div>
          <label class="hidden lg:flex" style="align-items:center;gap:6px;font-size:12px;">SNAP
            <select v-model.number="snap" class="hud-input" style="width:80px;height:36px;"><option :value="5">5′</option><option :value="15">15′</option><option :value="30">30′</option></select>
          </label>
        </div>
        <TimetableGrid
          v-if="current.stages.length" class="hidden lg:block" :class="{ 'lg:hidden': view === 'list' }"
          :event="current" :stages="current.stages" :lineup="lineup" :issues="issues" :readonly="isPast" :snap="snap"
          @update="update" @edit="id => (editing = id)"
        />
        <TimetableList
          :class="view === 'grid' ? 'lg:hidden' : ''"
          :event="current" :stages="current.stages" :lineup="lineup" :issues="issues" :readonly="isPast"
          @edit="id => (editing = id)"
        />
      </div>
    </div>

    <TimetableSetTimeSheet :event="current" :stages="current.stages" :entry="editingEntry" :lineup="lineup" @close="editing = null" @apply="l => { update(l); editing = null }" />

    <div
      v-if="dirtyCount || message" role="region" aria-label="Save timetable"
      class="glass" style="position:sticky;bottom:72px;z-index:5;display:flex;flex-wrap:wrap;gap:8px;align-items:center;justify-content:space-between;padding:10px 14px;"
    >
      <span :role="message?.kind === 'error' ? 'alert' : 'status'" style="font-family:var(--font-terminal);font-size:10px;letter-spacing:.06em;" :style="message?.kind === 'error' ? 'color:var(--color-error);' : ''">
        <template v-if="message">{{ message.text }}</template>
        <template v-else-if="!online">OFFLINE · {{ dirtyCount }} {{ dirtyCount === 1 ? 'CHANGE' : 'CHANGES' }} WAITING · SAVED ON THIS DEVICE</template>
        <template v-else>UNSAVED · {{ dirtyCount }} {{ dirtyCount === 1 ? 'CHANGE' : 'CHANGES' }} · SAVED ON THIS DEVICE</template>
      </span>
      <span v-if="dirtyCount" style="display:flex;gap:8px;">
        <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="discard(); message = null">DISCARD</button>
        <button type="button" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="busy || !online" @click="save">{{ busy ? 'SAVING…' : 'SAVE TIMETABLE' }}</button>
      </span>
    </div>
  </div>
</template>
