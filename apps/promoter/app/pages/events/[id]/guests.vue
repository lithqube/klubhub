<script setup lang="ts">
import { Download, FileUp, ListPlus, MailCheck, UserPlus } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useEventStore } from '~/stores/event'
import { useGuestStore } from '~/stores/guest'
import { usePrivacyStore } from '~/stores/privacy'
import type { ApiError } from '~/types/event'
import type { ListInput } from '~/types/guest'
import { guestErrorText } from '~/utils/guests'
import { shortDate } from '~/utils/privacy'

const { current } = storeToRefs(useEventStore())
const store = useGuestStore()
const { lists, guests, tickets, counts, loading, error } = storeToRefs(store)

const id = computed(() => current.value?.id ?? '')
await useAsyncData(() => `guests-${id.value}`, () => (id.value ? store.load(id.value).then(() => true) : Promise.resolve(null)), { watch: [id] })

// Retention (P2.5): once erased, rows stay as "Erased guest" and every
// personal-data action is off with the reason. Erased rows alone also tell,
// should the privacy state fail to load.
const privacy = usePrivacyStore()
const { events: privacyByEvent } = storeToRefs(privacy)
await useAsyncData(() => `privacy-${id.value}`, () => (id.value ? privacy.fetchEvent(id.value).then(() => true) : Promise.resolve(null)), { watch: [id] })
const purgedAt = computed(() => privacyByEvent.value[id.value]?.purged_at ?? null)
const purged = computed(() => !!purgedAt.value || guests.value.some(g => g.purged) || tickets.value.some(t => t.purged))
const purgedReason = computed(() => {
  const when = purgedAt.value && current.value ? ` on ${shortDate(purgedAt.value, current.value.timezone)}` : ''
  return `Guest names and contacts were erased${when}, so adding guests, importing attendees, status by email and the CSV export are off.`
})

const panel = ref<'add' | 'bulk' | 'list' | 'import' | null>(null)
const listFilter = ref('')
const table = ref<{ focusHeading: () => void } | null>(null)

/** A list's GUESTS: filter the table; where the table sits below the lists (phones, tablets), go to it. */
function showList(listId: string) {
  listFilter.value = listId
  if (window.matchMedia('(max-width: 1199px)').matches) nextTick(() => table.value?.focusHeading())
}
const notice = ref('')
const listError = ref<ApiError | null>(null)
const saving = ref(false)

const eventWindow = computed(() => (current.value ? { starts_at: current.value.starts_at, timezone: current.value.timezone } : null))
const pendingTotal = computed(() => lists.value.reduce((n, l) => n + l.pending, 0))

function done(msg: string) {
  notice.value = msg
  panel.value = null
}

/** Erased from the banner, or refused with event_purged meanwhile: reload into the erased state. */
async function onPurged() {
  panel.value = null
  if (!current.value) return
  await Promise.all([privacy.fetchEvent(current.value.id), store.load(current.value.id)])
}
watch(purged, (p) => {
  if (p) panel.value = null
})

async function createList(input: ListInput) {
  saving.value = true
  listError.value = null
  try {
    const l = await store.createList(input)
    done(`List ${l.name} created.`)
  } catch (e) {
    listError.value = e as ApiError
  } finally {
    saving.value = false
  }
}

const exporting = ref(false)
async function exportCsv() {
  if (!current.value) return
  exporting.value = true
  notice.value = ''
  try {
    const csv = await store.exportCsv(listFilter.value ? { list_id: listFilter.value } : {})
    const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${current.value.slug}-guests.csv`
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (e) {
    notice.value = guestErrorText(e as ApiError)
    if ((e as ApiError).error === 'event_purged') await onPurged()
  } finally {
    exporting.value = false
  }
}

function openPanel(p: 'add' | 'bulk' | 'list' | 'import') {
  notice.value = ''
  listError.value = null
  panel.value = panel.value === p ? null : p
}
</script>

<template>
  <div v-if="current" class="space-y-3">
    <EventPrivacyBanner :event="current" @purged="onPurged" />
    <div style="display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px;">
      <p class="data-frag" style="font-size:11px;margin:0;" aria-label="Guest summary">
        GOING {{ counts.going }} · {{ counts.going_heads }} HEADS<template v-if="current.capacity"> OF {{ current.capacity }} CAP.</template>
        · TICKETS {{ counts.tickets }} · PENDING {{ counts.pending }} · LISTS {{ lists.length }}
      </p>
      <div style="display:flex;flex-wrap:wrap;gap:6px;">
        <button type="button" class="btn-hud btn-hud-cta" style="min-height:44px;" :aria-expanded="panel === 'add'" :disabled="!lists.length || purged" @click="openPanel('add')">
          <UserPlus style="width:14px;height:14px;" aria-hidden="true" /> ADD GUESTS
        </button>
        <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" :aria-expanded="panel === 'bulk'" :disabled="!guests.length || purged" @click="openPanel('bulk')">
          <MailCheck style="width:14px;height:14px;" aria-hidden="true" /> STATUS BY EMAIL
        </button>
        <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" :aria-expanded="panel === 'import'" :disabled="purged" @click="openPanel('import')">
          <FileUp style="width:14px;height:14px;" aria-hidden="true" /> IMPORT ATTENDEES
        </button>
        <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" :disabled="!guests.length || exporting || purged" data-testid="guest-export" @click="exportCsv">
          <Download style="width:14px;height:14px;" aria-hidden="true" /> {{ exporting ? 'EXPORTING…' : 'EXPORT CSV' }}
        </button>
      </div>
    </div>

    <p v-if="purged" class="purged-reason" data-testid="purged-reason">{{ purgedReason }}</p>
    <p v-if="notice" role="status" class="glass" style="padding:10px 14px;border-left:3px solid var(--color-primary);font-size:13px;margin:0;">{{ notice }}</p>
    <p v-if="error" role="alert" class="glass" style="padding:10px 14px;border-left:3px solid var(--color-error);font-size:13px;margin:0;">
      COULD NOT LOAD THE GUEST LIST.
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="store.load(current.id)">RETRY</button>
    </p>
    <p v-else-if="loading && !lists.length" role="status" class="data-frag" style="font-size:11px;">LOADING GUESTS…</p>

    <GuestAddPanel v-if="panel === 'add'" :lists="lists" :default-list-id="listFilter || null" @done="done" @cancel="panel = null" @purged="onPurged" />
    <GuestBulkStatus v-if="panel === 'bulk'" @done="done" @cancel="panel = null" />
    <GuestImportPanel v-if="panel === 'import'" @done="done" @cancel="panel = null" @purged="onPurged" />

    <div class="guests-grid">
      <section class="space-y-3" style="min-width:0;" aria-labelledby="lists-h">
        <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;">
          <h2 id="lists-h" class="section-lbl" style="margin:0;">LISTS<template v-if="pendingTotal"> · {{ pendingTotal }} TO APPROVE</template></h2>
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" :aria-expanded="panel === 'list'" @click="openPanel('list')">
            <ListPlus style="width:14px;height:14px;" aria-hidden="true" /> NEW LIST
          </button>
        </div>
        <GuestListForm
          v-if="panel === 'list'" :event="eventWindow" :error="listError" :saving="saving" submit-label="CREATE LIST"
          @save="createList" @cancel="panel = null"
        />
        <p v-if="!lists.length && panel !== 'list'" class="glass" style="padding:14px;font-size:13px;">
          No lists yet. Create one, or set up <NuxtLink to="/guests" style="color:var(--color-primary);">standing lists</NuxtLink> so every new event starts with them.
        </p>
        <GuestListCard v-for="l in lists" :key="l.id" :list="l" :event="eventWindow!" @filter="showList" />
      </section>

      <GuestTable ref="table" v-model:list="listFilter" :guests="guests" :lists="lists" :tickets="tickets" :timezone="current.timezone" :purged="purged" />
    </div>
  </div>
</template>

<style scoped>
.purged-reason {
  margin: 0;
  font-size: 13px;
  color: var(--color-on-surface-variant);
}
.guests-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
@media (min-width: 1200px) {
  .guests-grid { grid-template-columns: minmax(0, 4fr) minmax(0, 7fr); }
}
</style>
