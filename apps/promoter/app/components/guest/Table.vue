<script setup lang="ts">
import { Search, Undo2, X } from 'lucide-vue-next'
import { useGuestStore } from '~/stores/guest'
import type { ApiError } from '~/types/event'
import type { Guest, GuestList, GuestStatus, Ticket } from '~/types/guest'
import { IMPORT_PRESETS } from '~/utils/attendeeImport'
import { timeLabel } from '~/utils/datetime'
import { arrivalTag, countByStatus, guestErrorText, isCheckedIn, searchGuests, searchTickets, STATUS_LABEL, STATUSES } from '~/utils/guests'

/**
 * The guest table (P2.1): status tabs with counts, search (in the browser,
 * over the decrypted page), list filter, inline status change, quick
 * approve / decline for pending names, edit and remove.
 *
 * Imported ticket holders (P2.2) sit in the same table: ALL shows guests
 * and tickets together, TICKETS only tickets. They are on no list and have
 * no guest status, so the status tabs and the list filter leave them out;
 * they change only through a re-import of the platform's export.
 *
 * Check-in state (P2.3): each guest shows "IN 2/3" (heads inside / heads
 * allowed) next to its status, with the first-in time in the event's
 * timezone, or "OUT 0/3" once everyone left again; a ticket shows "IN".
 * CHECKED IN lists guests with heads inside (any status) and scanned
 * tickets. The state is a snapshot from the last load.
 *
 * A status change says what it did with an UNDO (back to the previous
 * status); DECLINED from the dropdown asks first. Errors show on the row.
 * A list filter shows as a removable "LIST: name ✕" chip.
 *
 * Erased rows (P2.5 retention): "Erased guest" / "Erased ticket holder",
 * still with list, +N, status and check-in state, and no row actions (one
 * hint above the table says so). Search is off: there are no names left.
 */
const props = withDefaults(defineProps<{ guests: Guest[], lists: GuestList[], tickets?: Ticket[], timezone?: string, purged?: boolean }>(), { tickets: () => [], timezone: 'UTC', purged: false })
const listFilter = defineModel<string>('list', { default: '' })
const store = useGuestStore()

type Tab = 'all' | GuestStatus | 'checked_in' | 'tickets'
type Row = { kind: 'guest', key: string, g: Guest } | { kind: 'ticket', key: string, t: Ticket }
const sourceLabel = (s: string) => IMPORT_PRESETS.find(p => p.id === s)?.label ?? s.toUpperCase()
const tab = ref<Tab>('all')
const q = ref('')
const notice = ref('')
const rowError = ref<Record<string, string>>({})
const changed = ref<{ id: string, name: string, from: GuestStatus, to: GuestStatus } | null>(null)
const heading = ref<HTMLElement | null>(null)
const section = ref<HTMLElement | null>(null)
const filteredList = computed(() => props.lists.find(l => l.id === listFilter.value) ?? null)

/** Bring the table into view and focus its heading (after picking a list's GUESTS on a phone). */
function focusHeading() {
  section.value?.scrollIntoView({ block: 'start', behavior: 'smooth' })
  heading.value?.focus({ preventScroll: true })
}
defineExpose({ focusHeading })
const editing = ref<string | null>(null)
const draft = reactive({ name: '', plus_n: 0, note: '', email: '', phone: '' })
const editError = ref<ApiError | null>(null)

const firstIn = (iso: string | null) => (iso ? timeLabel(iso, props.timezone) : '')
const tagOf = (g: Guest) => arrivalTag(g, firstIn(g.first_in_at))
const listName = (id: string) => props.lists.find(l => l.id === id)?.name ?? '—'
const allocLabel = (g: Guest) => props.lists.flatMap(l => l.allocations).find(a => a.id === g.allocation_id)?.label ?? ''
const collects = (g: Guest) => props.lists.find(l => l.id === g.list_id)?.collect_contact ?? false

const inList = computed(() => (listFilter.value ? props.guests.filter(g => g.list_id === listFilter.value) : props.guests))
const ticketsInView = computed(() => (listFilter.value ? [] : props.tickets))
const counts = computed(() => countByStatus(inList.value))
const ticketsIn = computed(() => ticketsInView.value.filter(t => t.checked_in))
function guestsInTab(t: Tab): Guest[] {
  if (t === 'tickets') return []
  if (t === 'all') return inList.value
  if (t === 'checked_in') return inList.value.filter(isCheckedIn)
  return inList.value.filter(g => g.status === t)
}
function ticketsInTab(t: Tab): Ticket[] {
  if (t === 'all' || t === 'tickets') return ticketsInView.value
  return t === 'checked_in' ? ticketsIn.value : []
}
const shown = computed<Row[]>(() => {
  const guestRows = searchGuests(guestsInTab(tab.value), q.value)
  const ticketRows = searchTickets(ticketsInTab(tab.value), q.value)
  return [
    ...guestRows.map(g => ({ kind: 'guest' as const, key: `g-${g.id}`, g })),
    ...ticketRows.map(t => ({ kind: 'ticket' as const, key: `t-${t.id}`, t })),
  ]
})
const emptyText = computed(() => {
  if (props.purged && !props.guests.length && !props.tickets.length) return 'Nobody was on this event\'s lists when its guest data was erased.'
  if (props.purged) return 'Nobody in this view.'
  if (q.value) return `No guest matches “${q.value}”.`
  return props.guests.length || props.tickets.length ? 'Nobody in this view.' : 'No guests yet. Add names, paste a list or import attendees.'
})
// Search is off on an erased event; a query typed before the erase must not hide rows.
watch(() => props.purged, (p) => {
  if (p) q.value = ''
}, { immediate: true })
const TABS = computed(() => [
  { id: 'all' as Tab, label: 'ALL', n: counts.value.all + ticketsInView.value.length },
  ...STATUSES.map(s => ({ id: s as Tab, label: STATUS_LABEL[s], n: counts.value[s] })),
  { id: 'checked_in' as Tab, label: 'CHECKED IN', n: counts.value.checked_in + ticketsIn.value.length },
  ...(props.tickets.length ? [{ id: 'tickets' as Tab, label: 'TICKETS', n: ticketsInView.value.length }] : []),
])

async function setStatus(g: Guest, status: GuestStatus, opts: { undoable?: boolean } = {}): Promise<boolean> {
  if (status === g.status) return true
  const { [g.id]: _drop, ...rest } = rowError.value
  rowError.value = rest
  const from = g.status
  try {
    await store.updateGuest(g.id, { name: g.name, plus_n: g.plus_n, note: g.note, email: g.email, phone: g.phone, status })
    changed.value = opts.undoable === false ? null : { id: g.id, name: g.name, from, to: status }
    return true
  } catch (e) {
    rowError.value = { ...rowError.value, [g.id]: guestErrorText(e as ApiError) }
    return false
  }
}

/** The status dropdown: DECLINED asks first; a refusal puts the old value back. */
async function pickStatus(g: Guest, el: HTMLSelectElement) {
  const status = el.value as GuestStatus
  if (status === 'declined' && !window.confirm(`Set ${g.name} to DECLINED? They will not be let in from this list.`)) {
    el.value = g.status
    return
  }
  if (!(await setStatus(g, status))) el.value = g.status
}

async function undoStatus() {
  const c = changed.value
  const g = c && props.guests.find(x => x.id === c.id)
  if (!c || !g) return
  if (await setStatus(g, c.from, { undoable: false })) changed.value = null
}

function edit(g: Guest) {
  editing.value = g.id
  editError.value = null
  Object.assign(draft, { name: g.name, plus_n: g.plus_n, note: g.note, email: g.email, phone: g.phone })
}

async function saveEdit(g: Guest) {
  editError.value = null
  try {
    await store.updateGuest(g.id, { ...draft, plus_n: Number(draft.plus_n) || 0, status: g.status })
    editing.value = null
  } catch (e) {
    editError.value = e as ApiError
  }
}

async function remove(g: Guest) {
  if (!window.confirm(`Remove ${g.name} from the list? Their details are deleted.`)) return
  try {
    await store.deleteGuest(g.id)
  } catch (e) {
    notice.value = guestErrorText(e as ApiError)
  }
}
</script>

<template>
  <section ref="section" aria-labelledby="guest-table-h" style="min-width:0;scroll-margin-top:72px;">
    <h2 id="guest-table-h" ref="heading" class="table-h" tabindex="-1">GUEST TABLE</h2>
    <nav class="tabs-bar" aria-label="Guest status" style="overflow-x:auto;">
      <button
        v-for="t in TABS" :key="t.id" type="button" class="tab-item" :class="{ active: tab === t.id }" :aria-pressed="tab === t.id"
        style="min-height:44px;background:none;border-top:0;border-left:0;border-right:0;cursor:pointer;white-space:nowrap;" @click="tab = t.id"
      >
        {{ t.label }} <span class="data-frag" style="font-size:11px;margin-left:2px;">{{ t.n }}</span>
      </button>
    </nav>

    <div style="display:flex;flex-wrap:wrap;gap:8px;margin:10px 0;">
      <label style="position:relative;flex:1;min-width:180px;">
        <span class="sr-only">Search guests</span>
        <Search style="position:absolute;left:10px;top:13px;width:14px;height:14px;color:var(--color-tertiary);" aria-hidden="true" />
        <input
          v-model="q" class="hud-input" type="search" :placeholder="purged ? 'Names were erased' : 'Search names, emails, notes'" style="padding-left:30px;"
          autocomplete="off" :disabled="purged"
        >
      </label>
      <label style="min-width:160px;">
        <span class="sr-only">Filter by list</span>
        <select v-model="listFilter" class="hud-input">
          <option value="">All lists</option>
          <option v-for="l in lists" :key="l.id" :value="l.id">{{ l.name }}</option>
        </select>
      </label>
    </div>

    <div v-if="filteredList" class="chip-row">
      <button type="button" class="chip" :aria-label="`Remove the list filter ${filteredList.name}`" @click="listFilter = ''">
        LIST: {{ filteredList.name }} <X style="width:14px;height:14px;" aria-hidden="true" />
      </button>
    </div>

    <p v-if="notice" role="alert" style="margin:0 0 8px;font-size:13px;color:var(--color-error);">{{ notice }}</p>
    <p v-if="changed" role="status" class="changed" data-testid="status-changed">
      <span>Status of {{ changed.name }} → {{ STATUS_LABEL[changed.to] }}</span>
      <button type="button" class="btn-hud btn-hud-ghost act" @click="undoStatus">
        <Undo2 style="width:14px;height:14px;" aria-hidden="true" /> UNDO
      </button>
      <button type="button" class="btn-hud btn-hud-ghost act" aria-label="Dismiss" @click="changed = null">
        <X style="width:14px;height:14px;" aria-hidden="true" />
      </button>
    </p>

    <p v-if="purged && shown.length" class="erased-hint" data-testid="erased-hint">
      Erased rows keep their list, +N, status and check-ins. They can't be changed.
    </p>
    <p v-if="!shown.length" class="glass" style="padding:14px;font-size:13px;color:var(--color-on-surface-variant);" data-testid="guest-table-empty">
      {{ emptyText }}
    </p>
    <table v-else class="guest-table">
      <thead>
        <tr>
          <th scope="col">GUEST</th>
          <th scope="col">LIST / TICKET</th>
          <th scope="col">STATUS</th>
          <th scope="col"><span class="sr-only">Actions</span></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in shown" :key="row.key" :class="{ 'accent-bar-pending': row.kind === 'guest' && row.g.status === 'pending' && !row.g.purged }">
          <template v-if="row.kind === 'ticket'">
            <td>
              <span v-if="row.t.purged" class="erased" data-testid="erased-name">Erased ticket holder</span>
              <span v-else style="font-size:14px;">{{ row.t.name }}</span>
              <span v-if="row.t.email" style="display:block;font-size:11px;color:var(--color-on-surface-variant);overflow:hidden;text-overflow:ellipsis;">{{ row.t.email }}</span>
            </td>
            <td style="font-size:12px;">
              {{ row.t.ticket_type }}
              <span style="display:block;font-size:11px;color:var(--color-on-surface-variant);">{{ sourceLabel(row.t.source) }} · order {{ row.t.order_ref }}</span>
            </td>
            <td>
              <span class="state-cell">
                <GuestTicketBadge :status="row.t.status" />
                <span
                  v-if="row.t.checked_in" class="in-tag in-tag-in" data-testid="checkin-tag"
                  :aria-label="`Checked in${row.t.first_in_at ? ` at ${firstIn(row.t.first_in_at)}` : ''}`"
                ><span aria-hidden="true">IN<template v-if="row.t.first_in_at"> · {{ firstIn(row.t.first_in_at) }}</template></span></span>
              </span>
            </td>
            <td class="actions-cell" />
          </template>
          <template v-else-if="editing === row.g.id">
            <td colspan="4">
              <form style="display:grid;gap:8px;" @submit.prevent="saveEdit(row.g)">
                <div class="edit-grid">
                  <label><span class="section-lbl">NAME</span><input v-model="draft.name" class="hud-input" maxlength="120"></label>
                  <label><span class="section-lbl">+N</span><input v-model.number="draft.plus_n" class="hud-input" type="number" min="0" max="10"></label>
                  <template v-if="collects(row.g)">
                    <label><span class="section-lbl">EMAIL</span><input v-model="draft.email" class="hud-input" type="email"></label>
                    <label><span class="section-lbl">PHONE</span><input v-model="draft.phone" class="hud-input" type="tel"></label>
                  </template>
                  <label style="grid-column:1/-1;"><span class="section-lbl">NOTE</span><input v-model="draft.note" class="hud-input" maxlength="500"></label>
                </div>
                <p v-if="editError" role="alert" style="margin:0;font-size:13px;color:var(--color-error);">{{ guestErrorText(editError) }}</p>
                <div style="display:flex;gap:6px;">
                  <button type="submit" class="btn-hud btn-hud-cta btn-hud-sm" style="min-height:44px;">SAVE</button>
                  <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="editing = null">CANCEL</button>
                </div>
              </form>
            </td>
          </template>
          <template v-else>
            <td>
              <span v-if="row.g.purged" class="erased" data-testid="erased-name">Erased guest</span>
              <span v-else style="font-size:14px;">{{ row.g.name }}</span>
              <span v-if="row.g.plus_n" class="data-frag" style="font-size:11px;margin-left:6px;">+{{ row.g.plus_n }}</span>
              <span v-if="row.g.email || row.g.note" style="display:block;font-size:11px;color:var(--color-on-surface-variant);overflow:hidden;text-overflow:ellipsis;">
                {{ [row.g.email, row.g.note].filter(Boolean).join(' · ') }}
              </span>
            </td>
            <td style="font-size:12px;">
              {{ listName(row.g.list_id) }}
              <span v-if="allocLabel(row.g)" style="display:block;font-size:11px;color:var(--color-on-surface-variant);">via {{ allocLabel(row.g) }}</span>
            </td>
            <td>
              <span class="state-cell">
                <GuestStatusBadge :status="row.g.status" />
                <span
                  v-if="tagOf(row.g)" class="in-tag" :class="`in-tag-${tagOf(row.g)!.tone}`" data-testid="checkin-tag"
                  :aria-label="tagOf(row.g)!.label"
                ><span aria-hidden="true">{{ tagOf(row.g)!.text }}<template v-if="row.g.first_in_at"> · {{ firstIn(row.g.first_in_at) }}</template></span></span>
              </span>
            </td>
            <td v-if="row.g.purged" class="actions-cell" />
            <td v-else class="actions-cell">
              <div class="actions">
              <template v-if="row.g.status === 'pending'">
                <button type="button" class="btn-hud btn-hud-cta act" :aria-label="`Approve ${row.g.name}`" @click="setStatus(row.g, 'going')">APPROVE</button>
                <button type="button" class="btn-hud btn-hud-ghost act" :aria-label="`Decline ${row.g.name}`" @click="setStatus(row.g, 'declined')">DECLINE</button>
              </template>
              <label>
                <span class="sr-only">Status of {{ row.g.name }}</span>
                <select class="hud-input" style="height:44px;font-size:12px;" :value="row.g.status" @change="pickStatus(row.g, $event.target as HTMLSelectElement)">
                  <option v-for="s in STATUSES" :key="s" :value="s">{{ STATUS_LABEL[s] }}</option>
                </select>
              </label>
              <button type="button" class="btn-hud btn-hud-ghost act" :aria-label="`Edit ${row.g.name}`" @click="edit(row.g)">EDIT</button>
              <button type="button" class="btn-hud btn-hud-ghost act" :aria-label="`Remove ${row.g.name}`" style="color:var(--color-error);" @click="remove(row.g)">✕</button>
              </div>
              <p v-if="rowError[row.g.id]" role="alert" class="row-err">{{ rowError[row.g.id] }}</p>
            </td>
          </template>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.guest-table {
  width: 100%;
  border-collapse: collapse;
}
.guest-table th {
  text-align: left;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .07em;
  color: var(--color-tertiary);
  padding: 6px 8px;
  border-bottom: 1px dashed var(--color-outline-variant);
}
.guest-table td {
  padding: 8px;
  vertical-align: middle;
  border-bottom: 1px dashed var(--color-outline-variant);
  min-width: 0;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  align-items: center;
  gap: 6px 8px;
}
.actions select.hud-input {
  min-width: 104px;
}
/* Real 44 px targets (no pseudo hit area), so neighbours never overlap. */
.act {
  min-height: 44px;
  height: 44px;
  min-width: 44px;
  padding: 0 12px;
  font-size: 11px;
}
.state-cell {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 6px;
}
/* Check-in indicator: word + numbers, never colour alone. */
.in-tag {
  display: inline-flex;
  align-items: center;
  padding: 1px 6px;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  white-space: nowrap;
  border: 1px dashed currentColor;
}
.in-tag-in { color: var(--color-primary); }
.in-tag-over { color: var(--color-error); }
.in-tag-left { color: var(--color-on-surface-variant); }
.erased {
  font-size: 14px;
  font-style: italic;
  color: var(--color-on-surface-variant);
}
.erased-hint {
  margin: 0 0 8px;
  font-size: 13px;
  color: var(--color-on-surface-variant);
}
.row-err {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--color-error);
  text-align: right;
}
.table-h {
  margin: 0 0 6px;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .07em;
  color: var(--color-tertiary);
}
.table-h:focus {
  outline: none;
}
.table-h:focus-visible {
  outline: 1px solid var(--color-primary);
}
.chip-row {
  margin: 0 0 8px;
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 44px;
  padding: 0 12px;
  font-family: var(--font-terminal);
  font-size: 12px;
  letter-spacing: .05em;
  color: var(--color-primary);
  background: var(--color-surface-container);
  border: 1px dashed var(--color-primary-dim);
  cursor: pointer;
}
.changed {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin: 0 0 8px;
  padding: 0 0 0 12px;
  font-size: 13px;
  border-left: 3px solid var(--color-primary);
  background: var(--color-surface-container);
}
.edit-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 8px;
}
.edit-grid label { display: grid; gap: 4px; }
@media (min-width: 640px) {
  .edit-grid { grid-template-columns: minmax(0, 3fr) minmax(0, 1fr); }
}
/* Phones: each guest becomes a card; no sideways scrolling. */
@media (max-width: 639px) {
  .guest-table thead { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
  .guest-table tr { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 2px 8px; padding: 8px 0; border-bottom: 1px solid var(--color-outline-variant); }
  .guest-table td { display: block; border: 0; padding: 2px 8px; }
  .guest-table td[colspan] { grid-column: 1 / -1; }
  .guest-table td.actions-cell { grid-column: 1 / -1; }
  .actions { justify-content: flex-start; }
  .row-err { text-align: left; }
}
</style>
