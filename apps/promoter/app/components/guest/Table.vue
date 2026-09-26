<script setup lang="ts">
import { Search } from 'lucide-vue-next'
import { useGuestStore } from '~/stores/guest'
import type { ApiError } from '~/types/event'
import type { Guest, GuestList, GuestStatus } from '~/types/guest'
import { countByStatus, guestErrorText, searchGuests, STATUS_LABEL, STATUSES } from '~/utils/guests'

/**
 * The guest table (P2.1): status tabs with counts, search (in the browser,
 * over the decrypted page), list filter, inline status change, quick
 * approve / decline for pending names, edit and remove.
 */
const props = defineProps<{ guests: Guest[], lists: GuestList[] }>()
const listFilter = defineModel<string>('list', { default: '' })
const store = useGuestStore()

type Tab = 'all' | GuestStatus
const tab = ref<Tab>('all')
const q = ref('')
const notice = ref('')
const editing = ref<string | null>(null)
const draft = reactive({ name: '', plus_n: 0, note: '', email: '', phone: '' })
const editError = ref<ApiError | null>(null)

const listName = (id: string) => props.lists.find(l => l.id === id)?.name ?? '—'
const allocLabel = (g: Guest) => props.lists.flatMap(l => l.allocations).find(a => a.id === g.allocation_id)?.label ?? ''
const collects = (g: Guest) => props.lists.find(l => l.id === g.list_id)?.collect_contact ?? false

const inList = computed(() => (listFilter.value ? props.guests.filter(g => g.list_id === listFilter.value) : props.guests))
const counts = computed(() => countByStatus(inList.value))
const shown = computed(() => searchGuests(tab.value === 'all' ? inList.value : inList.value.filter(g => g.status === tab.value), q.value))
const TABS = computed(() => [{ id: 'all' as Tab, label: 'ALL', n: counts.value.all }, ...STATUSES.map(s => ({ id: s as Tab, label: STATUS_LABEL[s], n: counts.value[s] }))])

async function setStatus(g: Guest, status: GuestStatus) {
  notice.value = ''
  try {
    await store.updateGuest(g.id, { name: g.name, plus_n: g.plus_n, note: g.note, email: g.email, phone: g.phone, status })
  } catch (e) {
    notice.value = guestErrorText(e as ApiError)
  }
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
  <section aria-labelledby="guest-table-h" style="min-width:0;">
    <h2 id="guest-table-h" class="sr-only">Guest table</h2>
    <nav class="tabs-bar" aria-label="Guest status" style="overflow-x:auto;">
      <button
        v-for="t in TABS" :key="t.id" type="button" class="tab-item" :class="{ active: tab === t.id }" :aria-pressed="tab === t.id"
        style="min-height:44px;background:none;border-top:0;border-left:0;border-right:0;cursor:pointer;white-space:nowrap;" @click="tab = t.id"
      >
        {{ t.label }} <span class="data-frag" style="font-size:8px;margin-left:2px;">{{ t.n }}</span>
      </button>
    </nav>

    <div style="display:flex;flex-wrap:wrap;gap:8px;margin:10px 0;">
      <label style="position:relative;flex:1;min-width:180px;">
        <span class="sr-only">Search guests</span>
        <Search style="position:absolute;left:10px;top:13px;width:14px;height:14px;color:var(--color-tertiary);" aria-hidden="true" />
        <input v-model="q" class="hud-input" type="search" placeholder="Search names, emails, notes" style="padding-left:30px;" autocomplete="off">
      </label>
      <label style="min-width:160px;">
        <span class="sr-only">Filter by list</span>
        <select v-model="listFilter" class="hud-input">
          <option value="">All lists</option>
          <option v-for="l in lists" :key="l.id" :value="l.id">{{ l.name }}</option>
        </select>
      </label>
    </div>

    <p v-if="notice" role="alert" style="margin:0 0 8px;font-size:13px;color:var(--color-error);">{{ notice }}</p>

    <p v-if="!shown.length" class="glass" style="padding:14px;font-size:13px;color:var(--color-on-surface-variant);">
      {{ q ? `No guest matches “${q}”.` : guests.length ? 'Nobody in this view.' : 'No guests yet. Add names or paste a list.' }}
    </p>
    <table v-else class="guest-table">
      <thead>
        <tr>
          <th scope="col">GUEST</th>
          <th scope="col">LIST</th>
          <th scope="col">STATUS</th>
          <th scope="col"><span class="sr-only">Actions</span></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="g in shown" :key="g.id" :class="{ 'accent-bar-pending': g.status === 'pending' }">
          <template v-if="editing === g.id">
            <td colspan="4">
              <form style="display:grid;gap:8px;" @submit.prevent="saveEdit(g)">
                <div class="edit-grid">
                  <label><span class="section-lbl">NAME</span><input v-model="draft.name" class="hud-input" maxlength="120"></label>
                  <label><span class="section-lbl">+N</span><input v-model.number="draft.plus_n" class="hud-input" type="number" min="0" max="10"></label>
                  <template v-if="collects(g)">
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
              <span style="font-size:14px;">{{ g.name }}</span>
              <span v-if="g.plus_n" class="data-frag" style="font-size:9px;margin-left:6px;">+{{ g.plus_n }}</span>
              <span v-if="g.email || g.note" style="display:block;font-size:11px;color:var(--color-on-surface-variant);overflow:hidden;text-overflow:ellipsis;">
                {{ [g.email, g.note].filter(Boolean).join(' · ') }}
              </span>
            </td>
            <td style="font-size:12px;">
              {{ listName(g.list_id) }}
              <span v-if="allocLabel(g)" style="display:block;font-size:11px;color:var(--color-on-surface-variant);">via {{ allocLabel(g) }}</span>
            </td>
            <td>
              <GuestStatusBadge :status="g.status" />
            </td>
            <td class="actions-cell">
              <div class="actions">
              <template v-if="g.status === 'pending'">
                <button type="button" class="btn-hud btn-hud-cta btn-hud-xs hit-44" :aria-label="`Approve ${g.name}`" @click="setStatus(g, 'going')">APPROVE</button>
                <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`Decline ${g.name}`" @click="setStatus(g, 'declined')">DECLINE</button>
              </template>
              <label>
                <span class="sr-only">Status of {{ g.name }}</span>
                <select class="hud-input" style="height:36px;min-width:112px;font-size:11px;" :value="g.status" @change="setStatus(g, ($event.target as HTMLSelectElement).value as GuestStatus)">
                  <option v-for="s in STATUSES" :key="s" :value="s">{{ STATUS_LABEL[s] }}</option>
                </select>
              </label>
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`Edit ${g.name}`" @click="edit(g)">EDIT</button>
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs hit-44" :aria-label="`Remove ${g.name}`" style="color:var(--color-error);" @click="remove(g)">✕</button>
              </div>
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
  font-size: 8px;
  letter-spacing: .07em;
  color: var(--color-tertiary);
  padding: 6px 8px;
  border-bottom: 1px solid var(--color-outline-variant);
}
.guest-table td {
  padding: 8px;
  vertical-align: middle;
  border-bottom: 1px solid var(--color-outline-variant);
  min-width: 0;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  align-items: center;
  gap: 4px;
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
}
</style>
