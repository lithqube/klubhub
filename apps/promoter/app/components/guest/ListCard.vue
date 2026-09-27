<script setup lang="ts">
import { Lock, UserRound } from 'lucide-vue-next'
import { useGuestStore } from '~/stores/guest'
import type { ApiError } from '~/types/event'
import type { Allocation, AllocationInput, GuestList, ListInput } from '~/types/guest'
import { dayLabel, timeLabel } from '~/utils/datetime'
import { allocationState, guestErrorText, LIST_TYPES } from '~/utils/guests'

/**
 * One list with its entry terms and allocations (quota bars, revoke).
 * purged: the event's guest data was erased, so allocations take no
 * submitter contact. retentionDays feeds the collect-contact hint.
 */
const props = defineProps<{ list: GuestList, event: { starts_at: string, timezone: string }, purged?: boolean, retentionDays?: number | null }>()
const emit = defineEmits<{ filter: [listId: string] }>()
const store = useGuestStore()

const editing = ref(false)
const allocEdit = ref<Allocation | 'new' | null>(null)
const error = ref<ApiError | null>(null)
const saving = ref(false)
const notice = ref('')

const typeLabel = computed(() => LIST_TYPES.find(t => t.id === props.list.type)?.label ?? props.list.type)
const terms = computed(() => {
  const t = props.list.entry_terms
  const parts = [t.price_mode === 'reduced' ? `REDUCED · ${t.reduced_price_text}` : 'FREE']
  if (t.cutoff_at) parts.push(`CUTOFF ${timeLabel(t.cutoff_at, props.event.timezone)}`)
  return parts.join(' · ')
})

async function run(fn: () => Promise<unknown>, done: () => void) {
  saving.value = true
  error.value = null
  try {
    await fn()
    done()
  } catch (e) {
    error.value = e as ApiError
  } finally {
    saving.value = false
  }
}

const saveList = (input: ListInput) => run(() => store.updateList(props.list.id, input), () => { editing.value = false })

async function remove() {
  if (!window.confirm(`Delete the list ${props.list.name}?`)) return
  try {
    await store.deleteList(props.list.id)
  } catch (e) {
    const err = e as ApiError
    if (err.error !== 'list_not_empty') {
      notice.value = guestErrorText(err)
      return
    }
    if (window.confirm(`${props.list.name} has ${Number(err.detail?.guests)} guests. Delete the list and its guests?`)) {
      await store.deleteList(props.list.id, true).catch((x: ApiError) => { notice.value = guestErrorText(x) })
    }
  }
}

function saveAlloc(input: AllocationInput) {
  const cur = allocEdit.value
  return run(
    () => (cur && cur !== 'new' ? store.updateAllocation(props.list.id, cur.id, input) : store.createAllocation(props.list.id, input)),
    () => { allocEdit.value = null },
  )
}

async function revoke(a: Allocation) {
  if (!window.confirm(`Revoke ${a.label}'s allocation? Guests already on it stay; no new names can be added.`)) return
  await store.revokeAllocation(props.list.id, a.id).catch((x: ApiError) => { notice.value = guestErrorText(x) })
}

const stateLabel = (a: Allocation) => ({ open: '', closed: 'CLOSED', revoked: 'REVOKED' }[allocationState(a)])
</script>

<template>
  <section class="hud-card" style="padding:12px 14px;" :aria-label="`List ${list.name}`">
    <GuestListForm
      v-if="editing" :initial="list" :event="event" :error="error" :saving="saving" :retention-days="retentionDays"
      @save="saveList" @cancel="editing = false; error = null"
    />
    <template v-else>
      <div style="display:flex;flex-wrap:wrap;align-items:center;gap:6px;justify-content:space-between;">
        <div style="min-width:0;display:flex;flex-wrap:wrap;align-items:center;gap:6px;">
          <h3 style="margin:0;font-size:14px;font-weight:600;">{{ list.name }}</h3>
          <span class="badge-hud badge-published">{{ typeLabel }}</span>
          <span v-if="list.collect_contact" class="badge-hud badge-draft" title="Emails and phones stored encrypted"><Lock style="width:11px;height:11px;" aria-hidden="true" /> CONTACTS</span>
          <span v-else class="badge-hud badge-published">NAME-ONLY</span>
        </div>
        <div style="display:flex;gap:8px;">
          <button type="button" class="btn-hud btn-hud-ghost act" :aria-label="`Show guests of ${list.name}`" @click="emit('filter', list.id)">GUESTS</button>
          <button type="button" class="btn-hud btn-hud-ghost act" :aria-label="`Edit ${list.name}`" @click="editing = true">EDIT</button>
          <button type="button" class="btn-hud btn-hud-ghost act" :aria-label="`Delete ${list.name}`" style="color:var(--color-error);" @click="remove">DELETE</button>
        </div>
      </div>
      <p class="data-frag" style="font-size:11px;margin:6px 0 0;">
        {{ terms }}<template v-if="list.entry_terms.perks.length"> · {{ list.entry_terms.perks.join(', ').toUpperCase() }}</template>
      </p>
      <p style="margin:4px 0 0;font-size:12px;color:var(--color-on-surface-variant);">
        {{ list.guests }} {{ list.guests === 1 ? 'guest' : 'guests' }} · {{ list.heads }} heads<template v-if="list.pending"> · <strong style="color:var(--color-secondary);">{{ list.pending }} to approve</strong></template>
      </p>
    </template>

    <div style="margin-top:10px;display:grid;gap:8px;">
      <div
        v-for="a in list.allocations" :key="a.id"
        :class="allocationState(a) === 'open' ? 'accent-bar-ready' : 'accent-bar-published'" style="padding:6px 8px;"
      >
        <GuestAllocationForm
          v-if="allocEdit !== 'new' && allocEdit?.id === a.id" :initial="a" :timezone="event.timezone" :error="error" :saving="saving" :purged="purged"
          @save="saveAlloc" @cancel="allocEdit = null; error = null"
        />
        <template v-else>
          <div style="display:flex;flex-wrap:wrap;align-items:center;gap:6px;justify-content:space-between;">
            <span style="display:inline-flex;align-items:center;gap:6px;font-size:13px;min-width:0;">
              <UserRound style="width:12px;height:12px;flex-shrink:0;" aria-hidden="true" />
              <span style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ a.label }}</span>
              <span v-if="stateLabel(a)" class="badge-hud" :class="allocationState(a) === 'revoked' ? 'badge-failed' : 'badge-archived'">{{ stateLabel(a) }}</span>
              <span v-if="a.requires_approval" class="badge-hud badge-draft">APPROVAL</span>
            </span>
            <span v-if="!a.revoked_at" style="display:flex;gap:8px;">
              <button type="button" class="btn-hud btn-hud-ghost act" :aria-label="`Edit allocation ${a.label}`" @click="allocEdit = a">EDIT</button>
              <button type="button" class="btn-hud btn-hud-ghost act" :aria-label="`Revoke allocation ${a.label}`" @click="revoke(a)">REVOKE</button>
            </span>
          </div>
          <GuestQuotaBar :used="a.used" :quota="a.quota" :label="`${a.label} allocation`" style="margin-top:4px;" />
          <p class="data-frag" style="font-size:11px;margin:4px 0 0;">
            +{{ a.plus_n_max }} EACH<template v-if="a.deadline"> · DEADLINE {{ dayLabel(a.deadline, event.timezone) }} {{ timeLabel(a.deadline, event.timezone) }}</template>
            <template v-if="a.pending"> · {{ a.pending }} PENDING</template>
          </p>
        </template>
      </div>
      <GuestAllocationForm
        v-if="allocEdit === 'new'" :timezone="event.timezone" :error="error" :saving="saving" :purged="purged"
        @save="saveAlloc" @cancel="allocEdit = null; error = null"
      />
      <button v-else-if="!editing" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;justify-self:start;" @click="allocEdit = 'new'">
        + ALLOCATION
      </button>
    </div>
    <p v-if="notice" role="alert" style="margin:8px 0 0;font-size:13px;color:var(--color-error);">{{ notice }}</p>
  </section>
</template>

<style scoped>
/* Real 44 px targets with 8 px between them (no overlapping pseudo hit areas). */
.act {
  min-height: 44px;
  height: 44px;
  min-width: 44px;
  padding: 0 12px;
  font-size: 11px;
}
</style>
