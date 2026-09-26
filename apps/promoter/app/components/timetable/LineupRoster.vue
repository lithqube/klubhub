<script setup lang="ts">
import { ArrowDown, ArrowUp, X, Users } from 'lucide-vue-next'
import type { LineupEntry } from '~/types/event'

/** Billing-order roster with paste-to-add (UX §4.4). */
const props = defineProps<{ lineup: LineupEntry[], readonly?: boolean, untimedIds: Set<string> }>()
const emit = defineEmits<{ update: [LineupEntry[]], schedule: [id: string] }>()

const paste = ref('')
function add() {
  const names = paste.value.split(/[\n,]+/).map(s => s.replace(/^\s*[-•*\d.)]+\s*/, '').trim()).filter(Boolean)
  if (!names.length) return
  emit('update', [...props.lineup, ...names.map(n => ({
    id: `tmp-${Math.random().toString(36).slice(2, 10)}`, stage_id: null, display_name: n.slice(0, 120), profile_url: null,
    billing_order: 0, b2b_group: null, set_start: null, set_end: null,
  }))])
  paste.value = ''
}
function move(i: number, d: number) {
  const next = [...props.lineup]
  const [x] = next.splice(i, 1)
  next.splice(i + d, 0, x!)
  emit('update', next)
}
function remove(i: number) {
  emit('update', props.lineup.filter((_, j) => j !== i))
}
/** B2B with the act above: same stage, same times, shared group number. */
function toggleB2B(i: number) {
  const next = props.lineup.map(l => ({ ...l }))
  const cur = next[i]!
  const prev = next[i - 1]
  if (cur.b2b_group != null) {
    cur.b2b_group = null
  } else if (prev) {
    const group = prev.b2b_group ?? Math.max(0, ...next.map(l => l.b2b_group ?? 0)) + 1
    prev.b2b_group = group
    Object.assign(cur, { b2b_group: group, stage_id: prev.stage_id, set_start: prev.set_start, set_end: prev.set_end })
  }
  emit('update', next)
}
</script>

<template>
  <section class="glass" style="padding:14px;" aria-labelledby="roster-title">
    <h2 id="roster-title" class="section-lbl">LINEUP · BILLING ORDER</h2>
    <form v-if="!readonly" style="margin:10px 0;" @submit.prevent="add">
      <label for="paste-acts" class="sr-only">Add acts — paste a list</label>
      <textarea
        id="paste-acts" v-model="paste" class="hud-textarea" rows="2"
        placeholder="Add acts — paste a list, one per line" @keydown.meta.enter.prevent="add" @keydown.ctrl.enter.prevent="add"
      />
      <button type="submit" class="btn-hud btn-hud-ghost" style="min-height:44px;margin-top:6px;" :disabled="!paste.trim()">ADD</button>
    </form>
    <p v-if="!lineup.length" style="font-size:13px;color:var(--color-on-surface-variant);">No acts yet. Paste your lineup above.</p>
    <ol style="list-style:none;margin:0;padding:0;display:grid;gap:4px;">
      <li
        v-for="(l, i) in lineup" :key="l.id"
        style="display:flex;align-items:center;gap:6px;min-height:44px;padding:4px 6px;border-left:2px solid transparent;"
        :style="l.b2b_group != null ? 'border-left-color:var(--color-secondary);' : ''"
      >
        <span style="width:18px;font-family:var(--font-command);font-size:11px;color:var(--color-tertiary);">{{ i + 1 }}</span>
        <span style="flex:1;min-width:0;font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">
          {{ l.display_name }}
          <span v-if="l.b2b_group != null" class="data-frag" style="font-size:7px;">B2B</span>
        </span>
        <button v-if="untimedIds.has(l.id) && !readonly" type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" @click="emit('schedule', l.id)">SCHEDULE</button>
        <template v-if="!readonly">
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`B2B ${l.display_name} with the act above`" :aria-pressed="l.b2b_group != null" :disabled="i === 0 && l.b2b_group == null" @click="toggleB2B(i)"><Users style="width:12px;height:12px;" /></button>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`Move ${l.display_name} up`" :disabled="i === 0" @click="move(i, -1)"><ArrowUp style="width:12px;height:12px;" /></button>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`Move ${l.display_name} down`" :disabled="i === lineup.length - 1" @click="move(i, 1)"><ArrowDown style="width:12px;height:12px;" /></button>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`Remove ${l.display_name}`" @click="remove(i)"><X style="width:12px;height:12px;" /></button>
        </template>
      </li>
    </ol>
  </section>
</template>
