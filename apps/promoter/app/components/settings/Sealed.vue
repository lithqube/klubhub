<script setup lang="ts">
import { Lock, LockOpen } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useSealedStore } from '~/stores/sealed'
import { formatFingerprint } from '~/utils/sealed/keys'

/**
 * ENCRYPTION & BAN LIST (P2.6), driven by the sealed state:
 * no key → SET UP YOUR KEY; owners with nothing set up → the setup wizard
 * (recovery kit); set up without access → ask an owner (owners: RECOVER
 * WITH KIT); locked → UNLOCK (owners: RECOVER WITH KIT); unlocked → status,
 * LOCK, and for owners who can open it plus ROTATE NOW when pending.
 * Loaded in the browser only: nothing sealed is part of the SSR payload.
 */
const store = useSealedStore()
const { state, org, isOwner, unlockedVersion, error, loading } = storeToRefs(store)
const uid = useId()

const recovering = ref(false)
const notice = ref('')
const noticeEl = ref<HTMLElement | null>(null)

onMounted(() => {
  if (!store.loaded) void store.fetchStatus()
})

function say(text: string) {
  notice.value = text
  recovering.value = false
  nextTick(() => noticeEl.value?.focus())
}

const RECOVERED = 'Recovered. Your new passphrase unlocks from now on; the kit still works.'
/**
 * One notice per step, from the state change itself (the form that did it
 * is gone by then). Locking by itself (idle, sign-out) clears an
 * "Unlocked" notice: the unlock form says why instead.
 */
watch(state, (s, was) => {
  if (s === was || s === 'loading' || s === 'error' || was === 'loading' || was === 'error') return
  if (was === 'unlocked') {
    if (store.lockedBy !== 'manual') notice.value = ''
    return
  }
  if (s === 'unlocked') {
    say(recovering.value ? RECOVERED : was === 'not_setup' ? 'Sealed data is set up and unlocked. Keep the recovery kit offline.' : 'Unlocked. Locks again after 30 minutes without activity.')
    return
  }
  if (was === 'no_key') say(s === 'not_setup' && isOwner.value ? 'Your key is ready. Next: set up sealed data for the collective.' : 'Your key is ready.')
})

const orgLine = computed(() => {
  const o = org.value
  if (!o || o.status === 'not_setup') return 'NOT SET UP'
  return `${o.status === 'rotation_pending' ? 'ROTATION PENDING' : 'READY'} · KEY VERSION ${o.version}`
})
const keyLine = computed(() => ({
  loading: '…', error: '…', no_key: 'NO KEY YET', not_setup: 'KEY READY', no_access: 'NO ACCESS YET', locked: 'LOCKED', unlocked: 'UNLOCKED',
}[state.value]))

function lock() {
  store.lock('manual')
  say('Locked. Your keys are gone from this browser until you unlock again.')
}
</script>

<template>
  <div class="space-y-3" data-testid="sealed-section">
    <p class="status" data-testid="sealed-status">
      <span>SEALED DATA · <strong>{{ orgLine }}</strong></span>
      <span>YOUR KEY · <strong>{{ keyLine }}</strong></span>
    </p>
    <p v-if="notice" ref="noticeEl" tabindex="-1" role="status" class="ok" data-testid="sealed-notice">{{ notice }}</p>

    <p v-if="state === 'loading'" role="status" class="data-frag" style="font-size:11px;">LOADING ENCRYPTION STATUS…</p>
    <p v-else-if="state === 'error'" role="alert" class="glass accent-bar-failed row" style="padding:10px 14px;margin:0;font-size:13px;">
      COULDN'T LOAD THE ENCRYPTION STATUS.
      <button type="button" class="btn-hud btn-hud-ghost act" :disabled="loading" @click="store.fetchStatus()">RETRY</button>
      <span v-if="error?.error === 'local_identity_required'" class="hint">Sealed data needs a KlubHub account (single sign-on accounts are not supported yet).</span>
    </p>

    <template v-else-if="state === 'no_key'">
      <SettingsSealedRecover v-if="recovering" @cancel="recovering = false" />
      <template v-else>
        <SettingsSealedPassphrase />
        <p v-if="isOwner && org && org.status !== 'not_setup'" class="hint">
          Lost your passphrase and have the recovery kit?
          <button type="button" class="linkbtn" @click="recovering = true">RECOVER WITH KIT</button>
        </p>
      </template>
    </template>

    <template v-else-if="state === 'not_setup'">
      <SettingsSealedSetup v-if="isOwner" />
      <p v-else class="glass panel txt" data-testid="sealed-wait-setup">
        Your key is ready. An owner has to set up sealed data for the collective (here, in Settings) before the ban list can be used.
      </p>
    </template>

    <template v-else-if="state === 'no_access'">
      <SettingsSealedRecover v-if="recovering" @cancel="recovering = false" />
      <section v-else class="glass panel" :aria-labelledby="`${uid}-na`" data-testid="sealed-no-access">
        <h3 :id="`${uid}-na`" class="lbl">WAITING FOR ACCESS</h3>
        <p class="txt">
          Your key is ready, but nobody has given it access to the collective key yet.
          <template v-if="isOwner">Ask another owner to grant it here, or recover with the kit.</template>
          <template v-else>Ask an owner of your collective to open Settings → Encryption &amp; ban list, unlock, and grant you access.</template>
        </p>
        <div v-if="isOwner" class="row start">
          <button type="button" class="btn-hud btn-hud-ghost act" @click="recovering = true">RECOVER WITH KIT</button>
        </div>
      </section>
    </template>

    <template v-else-if="state === 'locked'">
      <SettingsSealedRecover v-if="recovering" @cancel="recovering = false" />
      <SettingsSealedUnlock v-else :why="org?.status === 'rotation_pending' && isOwner ? 'A key rotation is pending: unlock to rotate.' : 'Unlock to see the ban list and manage access. Keys stay in this tab\'s memory only.'">
        <p v-if="isOwner" class="hint">
          Forgot your passphrase?
          <button type="button" class="linkbtn" data-testid="sealed-show-recover" @click="recovering = true">RECOVER WITH KIT</button>
        </p>
      </SettingsSealedUnlock>
    </template>

    <template v-else-if="state === 'unlocked'">
      <section class="glass panel" :aria-labelledby="`${uid}-u`" data-testid="sealed-unlocked">
        <h3 :id="`${uid}-u`" class="lbl"><LockOpen class="ic" aria-hidden="true" /> UNLOCKED · KEY VERSION {{ unlockedVersion }}</h3>
        <p class="txt">
          The keys are in this tab's memory only. They lock after 30 minutes without activity, when you sign out, or with LOCK.
          <template v-if="org?.recovery_fingerprint"> Recovery kit fingerprint: <span class="mono">{{ formatFingerprint(org.recovery_fingerprint) }}</span>.</template>
        </p>
        <div class="row start">
          <NuxtLink to="/ban-list" class="btn-hud btn-hud-cta act">OPEN THE BAN LIST →</NuxtLink>
          <button type="button" class="btn-hud btn-hud-ghost act" data-testid="sealed-lock" @click="lock"><Lock class="ic" aria-hidden="true" /> LOCK</button>
        </div>
      </section>
      <SettingsSealedAccess v-if="isOwner" />
    </template>

    <p class="hint">
      Sealed data is encrypted in your browser: the server stores ciphertext only and cannot read it. Door devices get their own access and match
      guests against the ban list offline.
    </p>
  </div>
</template>

<style scoped>
.status {
  margin: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .06em;
  color: var(--color-on-surface-variant);
}
.status strong { color: var(--color-on-surface); font-weight: 600; }
.panel { padding: 14px; display: grid; gap: 10px; min-width: 0; }
.lbl { margin: 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .07em; color: var(--color-tertiary); display: flex; align-items: center; gap: 6px; }
.txt { margin: 0; font-size: 13px; color: var(--color-on-surface-variant); }
.hint { margin: 0; font-size: 12px; color: var(--color-on-surface-variant); }
.ok { margin: 0; font-size: 13px; color: var(--color-on-surface); border-left: 3px solid var(--color-primary); padding: 6px 10px; background: var(--color-surface-container); }
.ok:focus { outline: none; }
.ok:focus-visible { outline: 1px solid var(--color-primary); }
.row { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; justify-content: flex-end; }
.row.start { justify-content: flex-start; }
.act { min-height: 44px; font-size: 11px; }
.ic { width: 14px; height: 14px; }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.linkbtn {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  padding: 0 4px;
  background: none;
  border: 0;
  cursor: pointer;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .06em;
  color: var(--color-primary);
}
</style>
