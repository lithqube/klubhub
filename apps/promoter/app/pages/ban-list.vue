<script setup lang="ts">
import { Lock, Plus, Search, ShieldBan } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useBanListStore } from '~/stores/banList'
import { useSealedStore } from '~/stores/sealed'
import type { ApiError } from '~/types/event'
import type { BanEntry, BanPlain } from '~/types/sealed'
import { shortDate } from '~/utils/privacy'
import { searchBan } from '~/utils/sealed/ban'
import { banErrorText } from '~/utils/sealedText'

/**
 * The ban list (P2.6): sealed data, decrypted in this browser only while
 * unlocked. Add, edit and remove entries (name and reason required, an
 * expiry up to 3 years); search by name or email. There is deliberately no
 * export. Door devices get the list encrypted and match guests offline.
 */
useHead({ title: 'Ban list' })

const sealed = useSealedStore()
const { state, isOwner } = storeToRefs(sealed)
const ban = useBanListStore()
const { sorted, loading, loaded, error, unreadable } = storeToRefs(ban)
const uid = useId()
const tz = import.meta.client ? Intl.DateTimeFormat().resolvedOptions().timeZone : 'UTC'

onMounted(async () => {
  if (!sealed.loaded) await sealed.fetchStatus()
  if (sealed.unlocked && !ban.loaded) await ban.load()
})
watch(state, (s) => {
  if (s === 'unlocked' && !ban.loaded && !ban.loading) void ban.load()
})

const q = ref('')
const shown = computed(() => searchBan(sorted.value, q.value))
const editing = ref<BanEntry | 'new' | null>(null)
const busy = ref(false)
const formError = ref<ApiError | null>(null)
const stale = ref<{ retry: () => Promise<void> } | null>(null)
const notice = ref('')
const noticeEl = ref<HTMLElement | null>(null)
const addBtn = ref<HTMLButtonElement | null>(null)

function say(text: string) {
  notice.value = text
  nextTick(() => noticeEl.value?.focus())
}

function open(e: BanEntry | 'new') {
  formError.value = null
  stale.value = null
  notice.value = ''
  editing.value = e
}

function close() {
  editing.value = null
  formError.value = null
  stale.value = null
  nextTick(() => addBtn.value?.focus())
}

async function save(plain: BanPlain, expiresAt: string) {
  busy.value = true
  formError.value = null
  const cur = editing.value
  try {
    const e = cur === 'new' || !cur ? await ban.add(plain, expiresAt) : await ban.update(cur.id, plain, expiresAt)
    editing.value = null
    stale.value = null
    say(`${e.plain?.name ?? 'Entry'} ${cur === 'new' ? 'added' : 'saved'}. Door devices get it at their next login.`)
  } catch (err) {
    formError.value = err as ApiError
    if (formError.value.error === 'key_version_stale' || formError.value.error === 'version_conflict') {
      stale.value = { retry: () => save(plain, expiresAt) }
    }
  } finally {
    busy.value = false
  }
}

async function retryStale() {
  const again = stale.value
  if (!again) return
  busy.value = true
  const ok = await ban.refreshStale()
  busy.value = false
  if (!ok) {
    stale.value = null
    formError.value = null
    editing.value = null
    say('The key changed and this browser had to lock. Unlock again, then add the entry once more.')
    return
  }
  await again.retry()
}

async function remove(e: BanEntry) {
  const name = e.plain?.name ?? 'this unreadable entry'
  if (!window.confirm(`Remove ${name} from the ban list? Door devices stop matching it after their next login.`)) return
  try {
    await ban.remove(e.id)
    say(`${name} removed.`)
  } catch (err) {
    say(banErrorText(err as ApiError))
  }
}

const date = (iso: string) => (iso ? shortDate(iso, tz) : '—')
const soon = (iso: string) => Date.parse(iso) - Date.now() < 14 * 86_400_000
</script>

<template>
  <div>
    <div class="page-header">
      <div>
        <h1 class="page-title">BAN LIST</h1>
        <p class="page-sub">Sealed: decrypted in this browser only. No export.</p>
      </div>
      <NuxtLink to="/settings#sealed" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;">ENCRYPTION SETTINGS</NuxtLink>
    </div>
    <div class="page-body space-y-3">
      <ClientOnly>
        <p v-if="state === 'loading'" role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
        <p v-else-if="state === 'error'" role="alert" class="glass box accent-bar-failed">
          COULDN'T LOAD THE ENCRYPTION STATUS.
          <button type="button" class="btn-hud btn-hud-ghost act" @click="sealed.fetchStatus()">RETRY</button>
        </p>

        <section v-else-if="state !== 'locked' && state !== 'unlocked'" class="glass panel" aria-labelledby="ban-setup-h" data-testid="ban-not-ready">
          <h2 id="ban-setup-h" class="lbl"><ShieldBan class="ic" aria-hidden="true" /> THE BAN LIST IS SEALED</h2>
          <p class="txt">
            <template v-if="state === 'no_key'">Create your personal key first (a sealed passphrase), in Settings.</template>
            <template v-else-if="state === 'not_setup'">{{ isOwner ? 'Set up sealed data for the collective first, in Settings. It takes two minutes and gives you a recovery kit.' : 'An owner has to set up sealed data for the collective first.' }}</template>
            <template v-else>You don't have access to the collective key yet. {{ isOwner ? 'Ask another owner to grant it, or recover with the kit in Settings.' : 'Ask an owner to grant you access in Settings.' }}</template>
          </p>
          <NuxtLink to="/settings#sealed" class="btn-hud btn-hud-cta act" style="justify-self:start;">GO TO ENCRYPTION SETTINGS</NuxtLink>
        </section>

        <SettingsSealedUnlock
          v-else-if="state === 'locked'" title="UNLOCK THE BAN LIST"
          why="The ban list is encrypted. Your sealed passphrase opens it in this browser; it locks again after 30 minutes without activity."
        />

        <template v-else>
          <p v-if="notice" ref="noticeEl" tabindex="-1" role="status" class="ok" data-testid="ban-notice">{{ notice }}</p>

          <div class="toolbar">
            <div class="search">
              <Search class="ic sic" aria-hidden="true" />
              <label :for="`${uid}-q`" class="sr-only">Search the ban list</label>
              <input :id="`${uid}-q`" v-model="q" class="hud-input" type="search" placeholder="Search name or email" autocomplete="off" data-testid="ban-search">
            </div>
            <button ref="addBtn" type="button" class="btn-hud btn-hud-cta act" :aria-expanded="editing === 'new'" data-testid="ban-add" @click="open('new')">
              <Plus class="ic" aria-hidden="true" /> ADD ENTRY
            </button>
            <button type="button" class="btn-hud btn-hud-ghost act" data-testid="ban-lock" @click="sealed.lock('manual')"><Lock class="ic" aria-hidden="true" /> LOCK</button>
          </div>

          <div v-if="stale" role="alert" class="glass box warn" data-testid="ban-stale">
            <span>{{ banErrorText(formError) }}</span>
            <button type="button" class="btn-hud btn-hud-cta act" :disabled="busy" data-testid="ban-stale-retry" @click="retryStale">RELOAD AND RETRY</button>
          </div>

          <BanEntryForm
            v-if="editing === 'new'" :busy="busy" :server-error="formError" @save="save" @cancel="close"
          />
          <p v-if="editing === 'new' && formError && !stale" role="alert" class="err" data-testid="ban-error">{{ banErrorText(formError) }}</p>

          <p v-if="error" role="alert" class="glass box accent-bar-failed">
            COULDN'T LOAD THE BAN LIST.
            <button type="button" class="btn-hud btn-hud-ghost act" :disabled="loading" @click="ban.load()">RETRY</button>
          </p>
          <p v-else-if="loading && !loaded" role="status" class="data-frag" style="font-size:11px;">DECRYPTING…</p>
          <KhEmptyState
            v-else-if="loaded && !sorted.length && editing !== 'new'" title="NO ONE ON THE BAN LIST"
            hint="Add people who must not get in. The door flags a possible match and asks a manager."
          />
          <template v-else-if="loaded && sorted.length">
            <p v-if="unreadable" class="hint warnline" data-testid="ban-unreadable">
              {{ unreadable }} {{ unreadable === 1 ? 'entry does' : 'entries do' }} not open with the current key. Remove {{ unreadable === 1 ? 'it' : 'them' }} before the next key rotation.
            </p>
            <p class="hint" role="status">{{ q.trim() ? `${shown.length} of ${sorted.length} entries` : `${sorted.length} ${sorted.length === 1 ? 'entry' : 'entries'}` }}</p>
            <p v-if="q.trim() && !shown.length" class="glass box">No entries match “{{ q.trim() }}”.</p>
            <table v-else class="bans" :aria-label="`Ban list, ${shown.length} entries`" data-testid="ban-table">
              <thead>
                <tr><th scope="col">NAME</th><th scope="col">REASON</th><th scope="col">EXPIRES</th><th scope="col">ADDED</th><th scope="col"><span class="sr-only">Actions</span></th></tr>
              </thead>
              <tbody>
                <template v-for="e in shown" :key="e.id">
                  <tr v-if="editing !== 'new' && editing?.id === e.id">
                    <td colspan="5" class="edit-cell">
                      <BanEntryForm :entry="e" :busy="busy" :server-error="formError" @save="save" @cancel="close" />
                      <p v-if="formError && !stale" role="alert" class="err">{{ banErrorText(formError) }}</p>
                    </td>
                  </tr>
                  <tr v-else :data-testid="`ban-row-${e.id}`">
                    <th scope="row" data-label="NAME" class="name">
                      <template v-if="e.plain">{{ e.plain.name }}<span v-if="e.plain.email" class="sub">{{ e.plain.email }}</span></template>
                      <span v-else class="unread">UNREADABLE ENTRY</span>
                    </th>
                    <td data-label="REASON" class="reason">
                      <template v-if="e.plain">{{ e.plain.reason }}<span v-if="e.plain.note" class="sub">{{ e.plain.note }}</span></template>
                      <span v-else class="sub">Sealed under another key.</span>
                    </td>
                    <td data-label="EXPIRES" :class="{ soon: soon(e.expires_at) }">{{ date(e.expires_at) }}</td>
                    <td data-label="ADDED">{{ date(e.created_at) }}</td>
                    <td class="acts">
                      <button v-if="e.plain" type="button" class="btn-hud btn-hud-ghost act" :aria-label="`Edit ${e.plain.name}`" @click="open(e)">EDIT</button>
                      <button type="button" class="btn-hud btn-hud-ghost act danger" :aria-label="`Remove ${e.plain?.name ?? 'unreadable entry'}`" @click="remove(e)">REMOVE</button>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </template>
        </template>
        <template #fallback>
          <p role="status" class="data-frag" style="font-size:11px;">LOADING…</p>
        </template>
      </ClientOnly>
    </div>
  </div>
</template>

<style scoped>
.panel { padding: 14px; display: grid; gap: 10px; min-width: 0; }
.box { padding: 10px 14px; margin: 0; font-size: 13px; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.box.warn { border-left: 3px solid var(--color-status-archived); }
.lbl { margin: 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .07em; color: var(--color-tertiary); display: flex; align-items: center; gap: 6px; }
.txt { margin: 0; font-size: 13px; color: var(--color-on-surface-variant); }
.hint { margin: 0; font-size: 12px; color: var(--color-on-surface-variant); }
.warnline { color: var(--color-status-archived); }
.err { margin: 0; font-size: 12px; color: var(--color-error); }
.ok { margin: 0; font-size: 13px; color: var(--color-on-surface); border-left: 3px solid var(--color-primary); padding: 6px 10px; background: var(--color-surface-container); }
.ok:focus { outline: none; }
.ok:focus-visible { outline: 1px solid var(--color-primary); }
.act { min-height: 44px; font-size: 11px; }
.ic { width: 14px; height: 14px; }
.toolbar { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
.search { position: relative; flex: 1 1 220px; min-width: 0; }
.search .hud-input { height: 44px; padding-left: 34px; }
.sic { position: absolute; left: 11px; top: 15px; color: var(--color-on-surface-variant); }
.bans { width: 100%; border-collapse: separate; border-spacing: 0 4px; font-size: 13px; }
.bans thead th {
  text-align: left;
  padding: 4px 10px;
  font-family: var(--font-terminal);
  font-size: 11px;
  font-weight: 400;
  letter-spacing: .07em;
  color: var(--color-tertiary);
}
.bans tbody th, .bans tbody td { padding: 8px 10px; background: var(--color-surface-container); vertical-align: top; text-align: left; font-weight: 400; overflow-wrap: anywhere; }
.bans .name { font-weight: 600; min-width: 140px; }
.bans .reason { min-width: 180px; }
.sub { display: block; font-size: 12px; font-weight: 400; color: var(--color-on-surface-variant); }
.unread { font-family: var(--font-terminal); font-size: 11px; color: var(--color-status-archived); }
.soon { color: var(--color-status-archived); }
.acts { white-space: nowrap; text-align: right; }
.acts .act + .act { margin-left: 4px; }
.danger { color: var(--color-error); }
.edit-cell { padding: 0 !important; background: none !important; }
@media (max-width: 719px) {
  .bans thead { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
  .bans, .bans tbody, .bans tr, .bans tbody th, .bans tbody td { display: block; width: 100%; }
  .bans { border-spacing: 0; }
  .bans tbody tr { margin-bottom: 6px; background: var(--color-surface-container); }
  .bans tbody th, .bans tbody td { padding: 4px 12px; min-width: 0; }
  .bans tbody th { padding-top: 10px; font-size: 15px; }
  .bans tbody td[data-label]::before {
    content: attr(data-label);
    display: block;
    font-family: var(--font-terminal);
    font-size: 10px;
    letter-spacing: .07em;
    color: var(--color-tertiary);
  }
  .acts { text-align: left; padding-bottom: 10px !important; }
}
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
</style>
