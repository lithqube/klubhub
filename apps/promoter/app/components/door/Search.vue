<script setup lang="ts">
import { QrCode, Search } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useDoorStore } from '~/stores/door'
import type { DoorSubject } from '~/types/door'
import { searchDoor } from '~/utils/doorSearch'
import { headsBySubject, subjectKey } from '~/utils/doorState'

// Same words as the guest table's ticket badge.
const TICKET_FLAG: Record<string, string> = { refunded: 'REFUNDED', cancelled: 'CANCELLED', pending: 'UNPAID' }

/**
 * Name search over the offline bundle, from the first letter: accents and
 * one typo per word forgiven; a ticket's order number or QR code matches
 * exactly. Results show heads in and the allowance ("ALL IN" once used up).
 * Enter opens the top hit when it is the one best match; otherwise the
 * page announces how many match. It sits at the top of the door so its
 * results stay above the phone keyboard.
 */
const q = defineModel<string>({ default: '' })
defineProps<{ note?: string }>()
const emit = defineEmits<{ pick: [subject: DoorSubject], scan: [], announce: [text: string] }>()
const store = useDoorStore()
const { index, checkins, bundle } = storeToRefs(store)
const input = ref<HTMLInputElement | null>(null)

const heads = computed(() => headsBySubject(checkins.value))
const listName = (id: string) => bundle.value?.lists.find(l => l.id === id)?.name ?? ''
const hits = computed(() => searchDoor(index.value, q.value, 30).map((h) => {
  const s = { kind: h.entry.kind, id: h.entry.id }
  const b = bundle.value!
  const g = s.kind === 'guest' ? b.guests.find(x => x.id === s.id) : undefined
  const t = s.kind === 'ticket' ? b.tickets.find(x => x.id === s.id) : undefined
  const allow = g ? 1 + g.plus_n : 1
  const inside = Math.max(0, heads.value.get(subjectKey(s)) ?? 0)
  return {
    s, key: subjectKey(s), name: h.entry.name, inside, allow, score: h.score,
    sub: g ? listName(g.list_id) : t ? `${t.ticket_type || 'Ticket'} · ${t.order_ref}` : '',
    flag: t ? TICKET_FLAG[t.status] ?? '' : g && g.status !== 'going' ? g.status.toUpperCase() : '',
  }
}))

function submit() {
  const [top, next] = hits.value
  if (!top) {
    if (q.value.trim()) emit('announce', 'No matches')
    return
  }
  if (!next || top.score > next.score) {
    emit('pick', top.s)
    return
  }
  emit('announce', `${hits.value.length} matches, pick one`)
}

defineExpose({ focus: () => input.value?.focus() })
onMounted(() => input.value?.focus())
</script>

<template>
  <section class="search" aria-label="Find a guest or ticket">
    <form class="q-bar" role="search" @submit.prevent="submit">
      <label for="door-q" class="sr-only">Search guests and tickets</label>
      <div class="field">
        <Search class="icon" aria-hidden="true" />
        <input
          id="door-q" ref="input" v-model="q" class="hud-input" type="search" inputmode="search" autocomplete="off" autocapitalize="off"
          spellcheck="false" placeholder="NAME, ORDER NO. OR CODE" aria-describedby="door-q-hint"
        >
      </div>
      <button type="button" class="btn-hud btn-hud-cta scan" aria-label="Scan a QR code" @click="emit('scan')">
        <QrCode style="width:22px;height:22px;" aria-hidden="true" /> SCAN
      </button>
    </form>
    <p id="door-q-hint" class="sr-only">Results update as you type. Enter opens the best match.</p>
    <p v-if="note && !q.trim()" role="status" class="note">{{ note }}</p>

    <p v-if="q.trim() && !hits.length" role="status" class="empty">No one matches “{{ q.trim() }}”. Check the spelling, or add them with a manager PIN.</p>
    <ul v-else-if="hits.length" class="results" aria-label="Search results">
      <li v-for="h in hits" :key="h.key">
        <button type="button" class="hit" @click="emit('pick', h.s)">
          <span class="nm">{{ h.name }}</span>
          <span class="sub">{{ h.sub }}<template v-if="h.flag"> · <b class="flag">{{ h.flag }}</b></template></span>
          <span v-if="h.inside >= h.allow" class="data-frag cnt full">ALL IN</span>
          <span v-else class="data-frag cnt">{{ h.inside }}/{{ h.allow }} IN</span>
        </button>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.search {
  display: grid;
  gap: 8px;
  min-width: 0;
}
.q-bar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
}
.field {
  position: relative;
}
.icon {
  position: absolute;
  left: 12px;
  top: 50%;
  width: 18px;
  height: 18px;
  transform: translateY(-50%);
  color: var(--color-tertiary);
  pointer-events: none;
}
.field .hud-input {
  scroll-margin-top: calc(var(--door-head-h, 80px) + 8px);
  height: 56px;
  padding-left: 38px;
  font-size: 18px;
}
.scan {
  min-height: 56px;
  height: 56px;
  font-size: 11px;
}
.empty {
  margin: 0;
  padding: 12px;
  font-size: 14px;
  color: var(--color-on-surface-variant);
}
.results {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 4px;
}
.hit {
  width: 100%;
  min-height: 56px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-areas: "nm cnt" "sub cnt";
  align-items: center;
  column-gap: 8px;
  padding: 8px 12px;
  text-align: left;
  background: var(--color-surface-container);
  border: 0;
  border-left: 2px solid transparent;
  color: var(--color-on-surface);
  cursor: pointer;
}
.hit:hover,
.hit:focus-visible {
  border-left-color: var(--color-primary);
  background: var(--color-surface-container-high);
}
.nm {
  grid-area: nm;
  font-size: 17px;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.sub {
  grid-area: sub;
  font-size: 12px;
  color: var(--color-on-surface-variant);
  overflow-wrap: anywhere;
}
.flag {
  color: var(--color-status-archived);
  font-weight: 600;
}
.cnt {
  grid-area: cnt;
  font-size: 11px;
}
.cnt.full {
  color: var(--color-status-archived);
  border-color: var(--color-status-archived);
}
.note {
  margin: 0;
  padding: 10px 12px;
  font-size: 14px;
  color: var(--color-status-archived);
  border-left: 3px solid var(--color-status-archived);
  background: var(--color-surface-container);
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
