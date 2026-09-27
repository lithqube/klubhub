<script setup lang="ts">
import { useNow } from '@vueuse/core'
import { RefreshCw } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useSealedStore } from '~/stores/sealed'
import { useSessionStore } from '~/stores/session'
import type { ApiError } from '~/types/event'
import type { DeviceRecipient, MemberRecipient } from '~/types/sealed'
import { keyFingerprint } from '~/utils/sealed/keys'
import { sealedErrorText, securityBlock, securityRefusal } from '~/utils/sealedText'

/**
 * Owners, unlocked: who can open the sealed data. Members and door devices
 * without a wrap get GRANT ACCESS / PROVISION (the collective key sealed to
 * their public key in this browser). A pending rotation (a member removed,
 * a device revoked) shows ROTATE NOW, which re-encrypts the ban list.
 *
 * Key substitution: the server hands out the public keys, so every
 * recipient shows its key fingerprint, and GRANT ACCESS / PROVISION stays
 * disabled until the owner ticks "Fingerprint matches what <name> sees on
 * their screen" (the member's Settings, the door device's menu). The tick
 * belongs to that exact key: a different key needs a new tick.
 */
const store = useSealedStore()
const { org, recipients, recipientsError, working } = storeToRefs(store)
const { me } = storeToRefs(useSessionStore())
const uid = useId()
const now = useNow({ interval: 30_000 })
const next = '/settings#sealed'

const block = computed(() => securityBlock(me.value, now.value.getTime()))
const loadRefusal = computed(() => securityRefusal(recipientsError.value))
const busyId = ref<string | null>(null)
const error = ref<ApiError | null>(null)
const refusal = computed(() => securityRefusal(error.value))
const notice = ref('')
const pending = computed(() => org.value?.status === 'rotation_pending')

onMounted(() => {
  if (!block.value) void store.fetchRecipients()
})

const members = computed(() => (recipients.value?.members ?? []).filter(m => m.user_id !== store.myId))
const devices = computed(() => (recipients.value?.devices ?? []).filter(d => !d.revoked))
const grantable = computed(() => [
  ...members.value.filter(m => !m.has_wrap && m.public_key).map(m => ({ kind: 'member' as const, id: m.user_id, public_key: m.public_key!, name: m.name })),
  ...devices.value.filter(d => !d.has_wrap && d.public_key).map(d => ({ kind: 'device' as const, id: d.device_id, public_key: d.public_key!, name: d.label })),
])
/** Fingerprints the owner compared, per recipient and exact public key. */
const confirmed = ref<Record<string, boolean>>({})
const confirmKey = (id: string, publicKey: string | null) => `${id}|${publicKey ?? ''}`
const isConfirmed = (id: string, publicKey: string | null) => !!confirmed.value[confirmKey(id, publicKey)]
function setConfirmed(id: string, publicKey: string | null, on: boolean) {
  confirmed.value = { ...confirmed.value, [confirmKey(id, publicKey)]: on }
}
const ready = computed(() => grantable.value.filter(t => isConfirmed(t.id, t.public_key)))

async function give(targets: typeof grantable.value) {
  if (!targets.length) return
  error.value = null
  notice.value = ''
  busyId.value = targets.length > 1 ? 'all' : targets[0]!.id
  try {
    await store.grant(targets.map(({ kind, id, public_key }) => ({ kind, id, public_key })))
    notice.value = targets.length > 1 ? `Access given to ${targets.length} members and devices.` : `${targets[0]!.name} can now open the sealed data.`
  } catch (e) {
    error.value = e as ApiError
  } finally {
    busyId.value = null
  }
}

const memberTarget = (m: MemberRecipient) => ready.value.filter(t => t.kind === 'member' && t.id === m.user_id)
const deviceTarget = (d: DeviceRecipient) => ready.value.filter(t => t.kind === 'device' && t.id === d.device_id)

async function rotate() {
  if (!window.confirm('Rotate to a new key now? Everyone who still has access gets the new key and the ban list is re-encrypted. Door devices need a new login to download it.')) return
  error.value = null
  notice.value = ''
  busyId.value = 'rotate'
  try {
    await store.rotate()
    notice.value = `Rotated. The collective key is now version ${store.unlockedVersion}; removed members and revoked devices can't open anything new.`
  } catch (e) {
    error.value = e as ApiError
  } finally {
    busyId.value = null
  }
}
</script>

<template>
  <div class="space-y-3">
    <section v-if="pending" class="glass panel warnbox" role="region" :aria-labelledby="`${uid}-rot`" data-testid="sealed-rotation">
      <h3 :id="`${uid}-rot`" class="lbl warn">ROTATION PENDING</h3>
      <p class="txt">
        A member was removed or a door device revoked. They may still hold the current key. Rotate to a new key: everyone who still has access gets it,
        and the ban list is re-encrypted in this browser.
      </p>
      <SettingsSealedSecurity v-if="block" :kind="block" :next="next" action="Rotating the key" />
      <div class="row">
        <button type="button" class="btn-hud btn-hud-cta act" :disabled="!!block || working !== null" :aria-busy="working === 'rotate'" data-testid="sealed-rotate" @click="rotate">
          <RefreshCw class="ic" aria-hidden="true" /> {{ working === 'rotate' ? 'ROTATING…' : 'ROTATE NOW' }}
        </button>
      </div>
    </section>

    <section class="glass panel" :aria-labelledby="`${uid}-acc`" data-testid="sealed-access">
      <h3 :id="`${uid}-acc`" class="lbl">WHO CAN OPEN SEALED DATA</h3>
      <p v-if="notice" role="status" class="ok" data-testid="sealed-access-notice">{{ notice }}</p>
      <SettingsSealedSecurity v-if="refusal" :kind="refusal" :next="next" action="Giving access" />
      <p v-else-if="error" role="alert" class="err" data-testid="sealed-access-error">{{ sealedErrorText(error) }}</p>

      <p class="txt">
        Everyone listed with HAS ACCESS or GETS THE BAN LIST can decrypt the ban list. Before you give access, compare the key fingerprint with what
        the person sees in their Settings, or what the door device shows in its menu, on their own screen. If it differs, don't grant: someone may
        have swapped the key.
      </p>
      <!-- One 2FA / sign-in note at a time: the rotation box above already shows it when a rotation is pending. -->
      <SettingsSealedSecurity v-if="block && !recipients && !pending" :kind="block" :next="next" action="Seeing and giving access" />
      <SettingsSealedSecurity v-else-if="loadRefusal" :kind="loadRefusal" :next="next" action="Seeing and giving access" />
      <p v-else-if="recipientsError" role="alert" class="err">
        Couldn't load members and devices.
        <button type="button" class="btn-hud btn-hud-ghost act" @click="store.fetchRecipients()">RETRY</button>
      </p>
      <p v-else-if="!recipients" role="status" class="hint">LOADING MEMBERS AND DEVICES…</p>

      <template v-if="recipients">
        <div v-if="ready.length > 1" class="row start">
          <button type="button" class="btn-hud btn-hud-ghost act" :disabled="working !== null" data-testid="sealed-grant-all" @click="give(ready)">
            GRANT ALL CONFIRMED ({{ ready.length }})
          </button>
        </div>
        <h4 class="sub">MEMBERS</h4>
        <p v-if="!members.length" class="hint">No other members yet.</p>
        <ul v-else class="rows" aria-label="Members">
          <li v-for="m in members" :key="m.user_id" :data-testid="`sealed-member-${m.name}`">
            <span class="who">
              <strong>{{ m.name }}</strong><span class="meta">{{ m.role.toUpperCase() }} · {{ m.email }}</span>
              <span v-if="m.public_key" class="meta">KEY FINGERPRINT <span class="mono fpv" :data-testid="`sealed-fp-${m.name}`">{{ keyFingerprint(m.public_key) }}</span></span>
            </span>
            <span v-if="m.has_wrap" class="tag ok-tag">HAS ACCESS</span>
            <span v-else-if="!m.public_key" class="tag">NO KEY YET · THEY SET ONE UP IN SETTINGS</span>
            <span v-else class="grant">
              <label class="ack">
                <input
                  type="checkbox" :checked="isConfirmed(m.user_id, m.public_key)" :disabled="working !== null" :data-testid="`sealed-confirm-${m.name}`"
                  @change="setConfirmed(m.user_id, m.public_key, ($event.target as HTMLInputElement).checked)"
                >
                Fingerprint matches what {{ m.name }} sees on their screen
              </label>
              <button
                type="button" class="btn-hud btn-hud-ghost act" :disabled="working !== null || !isConfirmed(m.user_id, m.public_key)" :aria-busy="busyId === m.user_id"
                :aria-label="`Grant access to ${m.name}`" @click="give(memberTarget(m))"
              >
                {{ busyId === m.user_id ? 'GRANTING…' : 'GRANT ACCESS' }}
              </button>
            </span>
          </li>
        </ul>
        <h4 class="sub">DOOR DEVICES</h4>
        <p v-if="!devices.length" class="hint">No door devices. Register one from an event's DOOR tab.</p>
        <ul v-else class="rows" aria-label="Door devices">
          <li v-for="d in devices" :key="d.device_id" :data-testid="`sealed-device-${d.label}`">
            <span class="who">
              <strong>{{ d.label }}</strong>
              <span v-if="d.public_key" class="meta">KEY FINGERPRINT <span class="mono fpv" :data-testid="`sealed-fp-${d.label}`">{{ keyFingerprint(d.public_key) }}</span></span>
            </span>
            <span v-if="d.has_wrap" class="tag ok-tag">GETS THE BAN LIST</span>
            <span v-else-if="!d.public_key" class="tag">NEEDS RE-REGISTERING</span>
            <span v-else class="grant">
              <label class="ack">
                <input
                  type="checkbox" :checked="isConfirmed(d.device_id, d.public_key)" :disabled="working !== null" :data-testid="`sealed-confirm-${d.label}`"
                  @change="setConfirmed(d.device_id, d.public_key, ($event.target as HTMLInputElement).checked)"
                >
                Fingerprint matches what {{ d.label }} shows in its menu
              </label>
              <button
                type="button" class="btn-hud btn-hud-ghost act" :disabled="working !== null || !isConfirmed(d.device_id, d.public_key)" :aria-busy="busyId === d.device_id"
                :aria-label="`Provision ${d.label}`" @click="give(deviceTarget(d))"
              >
                {{ busyId === d.device_id ? 'PROVISIONING…' : 'PROVISION' }}
              </button>
            </span>
          </li>
        </ul>
      </template>
    </section>
  </div>
</template>

<style scoped>
.panel { padding: 14px; display: grid; gap: 10px; min-width: 0; }
.warnbox { border-left: 3px solid var(--color-status-archived); }
.lbl { margin: 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .07em; color: var(--color-tertiary); }
.lbl.warn { color: var(--color-status-archived); }
.sub { margin: 4px 0 0; font-family: var(--font-terminal); font-size: 11px; letter-spacing: .07em; color: var(--color-on-surface-variant); }
.txt { margin: 0; font-size: 13px; color: var(--color-on-surface-variant); }
.hint { margin: 0; font-size: 12px; color: var(--color-on-surface-variant); }
.ok { margin: 0; font-size: 13px; color: var(--color-on-surface); border-left: 3px solid var(--color-primary); padding-left: 8px; }
.err { margin: 0; font-size: 12px; color: var(--color-error); display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.row { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; }
.row.start { justify-content: flex-start; }
.act { min-height: 44px; font-size: 11px; }
.ic { width: 16px; height: 16px; }
.rows { list-style: none; margin: 0; padding: 0; display: grid; gap: 4px; }
.rows li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 6px 10px;
  padding: 6px 10px;
  min-height: 56px;
  background: var(--color-surface-container);
  font-size: 13px;
}
.who { display: grid; min-width: 0; overflow-wrap: anywhere; }
.meta { font-size: 12px; color: var(--color-on-surface-variant); }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }
.fpv { font-size: 13px; font-weight: 700; letter-spacing: .06em; color: var(--color-on-surface); }
.grant { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 6px 10px; min-width: 0; }
.ack { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; font-size: 12px; cursor: pointer; }
.ack input { width: 18px; height: 18px; flex-shrink: 0; }
.tag { font-family: var(--font-terminal); font-size: 11px; letter-spacing: .05em; color: var(--color-on-surface-variant); }
.ok-tag { color: var(--color-primary); }
</style>
