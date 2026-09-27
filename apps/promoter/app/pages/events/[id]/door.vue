<script setup lang="ts">
import { KeyRound, MonitorSmartphone, ScanLine, ShieldBan } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useDoorStore } from '~/stores/door'
import { useEventStore } from '~/stores/event'
import { usePrivacyStore } from '~/stores/privacy'
import { useSealedStore } from '~/stores/sealed'
import { useSessionStore } from '~/stores/session'
import type { ApiError } from '~/types/event'
import type { DoorDevice, PinResult, PinWindow } from '~/types/door'
import { keyFingerprint } from '~/utils/sealed/keys'
import { sealedErrorText } from '~/utils/sealedText'
import { dayLabel, instantToZoned, timeLabel, zonedToInstant } from '~/utils/datetime'
import { defaultPinValidUntil, MAX_PIN_WINDOW_HOURS } from '~/utils/doorState'
import { shortDate } from '~/utils/privacy'

/**
 * Event DOOR tab (P2.3): prepare this browser as a door device, generate
 * the staff and manager PINs (each shown once; both stay visible until
 * hidden), see a PIN lockout, list and revoke devices, and open the door.
 * P2.6: registering makes the device's keypair in this browser; device
 * rows show their sealed status and PROVISION (with unlocked keys) gives a
 * device the ban list — after the manager ticks that the key fingerprint
 * matches what the device shows (its door menu or login screen).
 */
const { current } = storeToRefs(useEventStore())
const store = useDoorStore()
const { device, devices, pinStatus, deviceFp } = storeToRefs(store)
const uid = useId()

const id = computed(() => current.value?.id ?? '')
const loadError = ref(false)
function load() {
  if (!id.value) return Promise.resolve(null)
  loadError.value = false
  return Promise.all([store.fetchDevices(), store.fetchPinStatus(id.value)]).then(() => true).catch(() => {
    loadError.value = true
    return false
  })
}
const { refresh } = await useAsyncData(() => `door-${id.value}`, load, { watch: [id] })

// Retention (P2.5): an erased event has no names left to check against, so no new PINs.
const privacy = usePrivacyStore()
const { events: privacyByEvent } = storeToRefs(privacy)
await useAsyncData(() => `privacy-${id.value}`, () => (id.value ? privacy.fetchEvent(id.value).then(() => true) : Promise.resolve(null)), { watch: [id] })
const purgedAt = computed(() => privacyByEvent.value[id.value]?.purged_at ?? null)

onMounted(async () => {
  store.loadDevice()
  await store.loadDeviceFp()
})

// ---------------------------------------------------------------- ban list on door devices (P2.6)
const sealed = useSealedStore()
const { state: sealedState, working: sealedWorking } = storeToRefs(sealed)
const { me } = storeToRefs(useSessionStore())
const thisHasKey = ref<boolean | null>(null)
onMounted(async () => {
  if (!sealed.loaded) await sealed.fetchStatus()
  thisHasKey.value = device.value ? await store.hasDeviceKey() : null
})
watch(() => device.value?.id, async (d) => {
  thisHasKey.value = d ? await store.hasDeviceKey() : null
})

type SealedTag = 'none' | 'key' | 'provisioned' | null
/** The device's sealed status, when the API says (older APIs leave the fields out). */
function sealedTag(d: DoorDevice): SealedTag {
  if (d.public_key === undefined && d.has_wrap === undefined) return null
  if (d.has_wrap) return 'provisioned'
  return d.public_key ? 'key' : 'none'
}
const SEALED_TAG: Record<Exclude<SealedTag, null>, string> = { none: 'NEEDS RE-REGISTERING', key: 'READY TO PROVISION', provisioned: 'GETS THE BAN LIST' }
const provisionError = ref<ApiError | null>(null)
const provisionNotice = ref('')

/** The fingerprints the manager compared, per device and exact public key (a new key needs a new tick). */
const confirmed = ref<Record<string, boolean>>({})
const confirmKey = (d: DoorDevice) => `${d.id}|${d.public_key ?? ''}`
function setConfirmed(d: DoorDevice, on: boolean) {
  confirmed.value = { ...confirmed.value, [confirmKey(d)]: on }
}
/** For the device registered in this browser: does the server's key match the one this browser holds? */
function localMatch(d: DoorDevice): 'match' | 'mismatch' | null {
  if (device.value?.id !== d.id || !deviceFp.value || !d.public_key) return null
  return keyFingerprint(d.public_key) === deviceFp.value ? 'match' : 'mismatch'
}
const canProvision = (d: DoorDevice) => !!confirmed.value[confirmKey(d)] && localMatch(d) !== 'mismatch'

async function provision(d: DoorDevice) {
  if (!d.public_key || !canProvision(d)) return
  busy.value = `provision-${d.id}`
  provisionError.value = null
  provisionNotice.value = ''
  try {
    await sealed.provisionDevice(d.id, d.public_key)
    await store.fetchDevices()
    provisionNotice.value = `${d.label} now gets the ban list at its next door login.`
  } catch (e) {
    provisionError.value = e as ApiError
  } finally {
    busy.value = null
  }
}

const tz = computed(() => current.value?.timezone ?? 'UTC')
const eventRef = computed(() => (current.value ? { id: current.value.id, title: current.value.title, starts_at: current.value.starts_at } : null))
const deviceState = computed(() => (!device.value ? 'none' : device.value.event.id === id.value ? 'this' : 'other'))

// ---------------------------------------------------------------- this browser
const label = ref('')
const labelTouched = ref(false)
const suggestedLabel = computed(() => `Door ${devices.value.filter(d => !d.revoked_at).length + 1}`)
watch(suggestedLabel, (l) => {
  if (!labelTouched.value) label.value = l
}, { immediate: true })
const busy = ref<string | null>(null)
const notice = ref('')
const error = ref('')

async function register() {
  if (!eventRef.value || !label.value.trim()) return
  busy.value = 'register'
  error.value = ''
  try {
    const d = await store.registerDevice(label.value, eventRef.value, me.value?.org_id)
    notice.value = `This browser is now the door device ${d.label}.`
  } catch {
    error.value = 'Could not register this browser. Check the label (1–80 characters) and try again.'
  } finally {
    busy.value = null
  }
}

function switchEvent() {
  if (!eventRef.value) return
  store.assignEvent(eventRef.value)
  notice.value = `This browser now opens the door for ${eventRef.value.title}.`
}

async function forget() {
  if (!window.confirm('Forget the door device on this browser? It will need to be registered again.')) return
  await store.forgetDevice()
  notice.value = 'This browser is no longer a door device.'
}

// ---------------------------------------------------------------- PINs
const until = reactive({ date: '', time: '' })
function resetUntil() {
  if (!current.value) return
  Object.assign(until, instantToZoned(defaultPinValidUntil(current.value.ends_at, Date.now()).toISOString(), tz.value))
}
onMounted(resetUntil)
watch(id, resetUntil)

// Both PINs stay on screen once generated (the manager one does not hide the staff one).
const shown = reactive<{ staff: PinResult | null, manager: PinResult | null }>({ staff: null, manager: null })
const pinError = ref('')

async function generate(manager: boolean) {
  if (!current.value || !until.date || !until.time) return
  const kind = manager ? 'manager' : 'staff'
  const existing = manager ? pinStatus.value?.manager : pinStatus.value?.staff
  if (existing && !window.confirm(`Replace the ${kind} PIN? The current one stops working.`)) return
  busy.value = kind
  pinError.value = ''
  shown[kind] = null
  try {
    const valid = zonedToInstant(until.date, until.time, tz.value).toISOString()
    shown[kind] = await store.setPin(current.value.id, { manager, valid_until: valid })
  } catch (e) {
    const err = e as ApiError
    pinError.value = err.error === 'invalid_input'
      ? `Pick an end time in the next ${MAX_PIN_WINDOW_HOURS} hours.`
      : err.error === 'event_purged'
        ? 'Guest names for this event were erased, so there is no list for the door. PINs can no longer be generated.'
        : 'Could not generate the PIN. Try again.'
    if (err.error === 'event_purged') await privacy.fetchEvent(current.value.id)
  } finally {
    busy.value = null
  }
}

const at = (iso: string) => `${dayLabel(iso, tz.value).toUpperCase()} ${timeLabel(iso, tz.value)}`
const statusLine = (s: PinWindow | null | undefined) =>
  (s ? `VALID UNTIL ${at(s.valid_until)}` : 'NOT SET')
const lockLine = (s: PinWindow | null | undefined) =>
  (s?.locked_until && Date.parse(s.locked_until) > Date.now() ? `LOCKED UNTIL ${timeLabel(s.locked_until, tz.value)} · A NEW PIN UNLOCKS IT` : '')

// ---------------------------------------------------------------- devices
async function revoke(d: { id: string, label: string }) {
  if (!window.confirm(`Revoke ${d.label}? It is logged out of the door and cannot log in again.`)) return
  busy.value = `revoke-${d.id}`
  try {
    await store.revokeDevice(d.id)
    notice.value = `${d.label} revoked.`
  } catch {
    error.value = 'Could not revoke the device.'
  } finally {
    busy.value = null
  }
}
const when = (iso: string | null) => (iso ? `${dayLabel(iso, tz.value)} ${timeLabel(iso, tz.value)}` : '—')
</script>

<template>
  <div v-if="current" class="door-tab space-y-3">
    <p v-if="notice" role="status" class="glass" style="padding:10px 14px;border-left:3px solid var(--color-primary);font-size:13px;margin:0;">{{ notice }}</p>
    <p v-if="error" role="alert" class="glass" style="padding:10px 14px;border-left:3px solid var(--color-error);font-size:13px;margin:0;">{{ error }}</p>
    <p v-if="loadError" role="alert" class="glass" style="padding:10px 14px;border-left:3px solid var(--color-error);font-size:13px;margin:0;display:flex;flex-wrap:wrap;align-items:center;gap:8px;">
      COULD NOT LOAD DOOR DEVICES AND PINS.
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="refresh()">RETRY</button>
    </p>

    <div class="grid">
      <section class="hud-card glass panel" aria-labelledby="this-h">
        <h2 id="this-h" class="section-lbl" style="margin:0;"><MonitorSmartphone class="ic" aria-hidden="true" /> THIS BROWSER</h2>
        <p v-if="purgedAt" class="txt" data-testid="door-device-purged">This event's guest list was erased, so it can't be used at the door.</p>
        <ClientOnly v-else>
          <template v-if="deviceState === 'none'">
            <p class="txt">Use this phone or tablet at the door. It downloads the guest list for the night, keeps it encrypted, and works without signal.</p>
            <p class="warn" data-testid="door-admin-warning">
              Logging in at the door signs this browser out of the admin. Use a separate phone, or a separate browser profile.
            </p>
            <form class="row" @submit.prevent="register">
              <div style="display:grid;gap:4px;min-width:0;flex:1;">
                <label :for="`${uid}-label`" class="section-lbl">DEVICE NAME</label>
                <input :id="`${uid}-label`" v-model="label" class="hud-input" maxlength="80" required @input="labelTouched = true">
              </div>
              <button type="submit" class="btn-hud btn-hud-cta" style="min-height:44px;align-self:end;" :disabled="!label.trim() || busy === 'register'">
                {{ busy === 'register' ? 'REGISTERING…' : 'USE THIS BROWSER AS A DOOR DEVICE' }}
              </button>
            </form>
          </template>
          <template v-else-if="deviceState === 'this'">
            <p class="txt" data-testid="door-this-device">This browser is the door device <strong>{{ device!.label }}</strong> for this event.</p>
            <p v-if="deviceFp" class="hint">KEY FINGERPRINT <span class="mono" data-testid="door-this-fp">{{ deviceFp }}</span></p>
            <div class="row">
              <!-- A full page load: the door is its own document (camera policy, service worker scope). -->
              <a href="/door" class="btn-hud btn-hud-cta" style="min-height:44px;"><ScanLine class="ic" aria-hidden="true" /> OPEN THE DOOR</a>
              <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="forget">FORGET ON THIS BROWSER</button>
            </div>
          </template>
          <template v-else>
            <p class="txt">This browser is the door device <strong>{{ device!.label }}</strong> for <strong>{{ device!.event.title }}</strong>.</p>
            <div class="row">
              <button type="button" class="btn-hud btn-hud-cta" style="min-height:44px;" @click="switchEvent">USE IT FOR THIS EVENT</button>
              <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" @click="forget">FORGET ON THIS BROWSER</button>
            </div>
          </template>
          <p v-if="deviceState !== 'none'" class="hint">
            The door uses its own session: logging in at /door signs this browser out of the admin. Prefer a separate phone, or a separate browser profile.
          </p>
        </ClientOnly>
      </section>

      <section class="hud-card glass panel" aria-labelledby="pins-h">
        <h2 id="pins-h" class="section-lbl" style="margin:0;"><KeyRound class="ic" aria-hidden="true" /> DOOR PINS</h2>
        <p class="txt">
          Staff log in at the door with the <strong>staff PIN</strong>. The <strong>manager PIN</strong> approves adding someone at the door.
          Each PIN is shown once; generating a new one replaces the old.
        </p>
        <p v-if="purgedAt" class="warn" data-testid="door-purged">
          Guest names and contacts for this event were erased on {{ shortDate(purgedAt, tz) }}. The door has no list to check against, so new PINs can't be generated.
        </p>
        <dl class="status" aria-label="PIN status">
          <div>
            <dt class="section-lbl">STAFF PIN</dt>
            <dd data-testid="pin-status-staff">{{ statusLine(pinStatus?.staff) }}</dd>
            <dd v-if="lockLine(pinStatus?.staff)" class="locked" data-testid="pin-status-staff-locked">{{ lockLine(pinStatus?.staff) }}</dd>
          </div>
          <div>
            <dt class="section-lbl">MANAGER PIN</dt>
            <dd data-testid="pin-status-manager">{{ statusLine(pinStatus?.manager) }}</dd>
            <dd v-if="lockLine(pinStatus?.manager)" class="locked">{{ lockLine(pinStatus?.manager) }}</dd>
          </div>
        </dl>
        <fieldset class="row" style="border:0;padding:0;margin:0;" :disabled="!!purgedAt">
          <legend class="section-lbl" style="margin-bottom:4px;">VALID UNTIL ({{ tz.toUpperCase() }}, AT MOST {{ MAX_PIN_WINDOW_HOURS }} H AHEAD)</legend>
          <label :for="`${uid}-d`" class="sr-only">Date</label>
          <input :id="`${uid}-d`" v-model="until.date" class="hud-input" type="date" style="flex:1;min-width:140px;">
          <label :for="`${uid}-t`" class="sr-only">Time</label>
          <input :id="`${uid}-t`" v-model="until.time" class="hud-input" type="time" style="width:120px;">
        </fieldset>
        <div class="row">
          <button type="button" class="btn-hud btn-hud-cta" style="min-height:44px;" :disabled="!!busy || !!purgedAt" @click="generate(false)">
            {{ busy === 'staff' ? 'GENERATING…' : pinStatus?.staff ? 'NEW STAFF PIN' : 'GENERATE STAFF PIN' }}
          </button>
          <button type="button" class="btn-hud btn-hud-ghost" style="min-height:44px;" :disabled="!!busy || !!purgedAt" @click="generate(true)">
            {{ busy === 'manager' ? 'GENERATING…' : pinStatus?.manager ? 'NEW MANAGER PIN' : 'GENERATE MANAGER PIN' }}
          </button>
        </div>
        <p v-if="pinError" role="alert" style="margin:0;font-size:13px;color:var(--color-error);">{{ pinError }}</p>
        <template v-for="kind in (['staff', 'manager'] as const)" :key="kind">
          <div v-if="shown[kind]" class="shown" role="status" aria-live="polite">
            <span class="section-lbl">{{ kind === 'manager' ? 'MANAGER PIN' : 'STAFF PIN' }} · SHOWN ONCE</span>
            <div class="row" style="align-items:center;">
              <span class="pin" :data-testid="kind === 'manager' ? 'door-manager-pin' : 'door-staff-pin'">{{ shown[kind]!.pin }}</span>
              <ExportCopyButton :text="shown[kind]!.pin" :label="`Copy the ${kind} PIN`" />
              <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" :aria-label="`Hide the ${kind} PIN`" @click="shown[kind] = null">HIDE</button>
            </div>
            <span class="hint">Valid until {{ when(shown[kind]!.valid_until) }}. Tell it to the door staff in person; do not post it in a group chat.</span>
          </div>
        </template>
      </section>
    </div>

    <ClientOnly>
      <section class="hud-card glass panel" aria-labelledby="ban-h" data-testid="door-ban-panel">
        <h2 id="ban-h" class="section-lbl" style="margin:0;"><ShieldBan class="ic" aria-hidden="true" /> BAN LIST AT THE DOOR</h2>
        <p class="txt">
          A provisioned door device downloads the ban list encrypted to its own key and checks names offline. A possible match asks for a manager;
          the reason shows only with the manager PIN.
        </p>
        <p class="brief" data-testid="door-staff-briefing"><strong>Brief the door staff:</strong> MANAGER CHECK on the door means a possible ban list match.</p>
        <p v-if="pinStatus && !pinStatus.manager" class="warn" data-testid="door-no-manager-pin">
          No manager PIN yet: the door can flag a MANAGER CHECK but can't show the reason. Generate a manager PIN above and give it to the manager on duty.
        </p>
        <p v-if="device && thisHasKey === false" class="warn" data-testid="door-no-device-key">
          This browser's door device has no key (it was registered before the ban list existed). Forget it on this browser and register it again to use the ban list here.
        </p>
        <p v-if="sealedState === 'loading'" role="status" class="hint">LOADING…</p>
        <p v-else-if="sealedState === 'local_required'" class="txt">Sealed data needs a local KlubHub account for now, so you can't provision devices from this account.</p>
        <template v-else-if="sealedState === 'not_setup' || sealedState === 'no_key'">
          <p class="txt">Sealed data is not set up for you yet.</p>
          <NuxtLink to="/settings#sealed" class="btn-hud btn-hud-ghost" style="min-height:44px;justify-self:start;">ENCRYPTION SETTINGS →</NuxtLink>
        </template>
        <p v-else-if="sealedState === 'no_access'" class="txt">You don't have access to the ban list key, so you can't provision devices. Someone with access (an owner) can.</p>
        <SettingsSealedUnlock v-else-if="sealedState === 'locked'" title="UNLOCK TO PROVISION" why="Unlock with your sealed passphrase, then PROVISION the devices marked READY TO PROVISION." />
        <p v-else-if="sealedState === 'unlocked'" class="txt" data-testid="door-ban-unlocked">
          Unlocked. For each device marked READY TO PROVISION, compare its key fingerprint with the one in that device's door menu, tick the box,
          then PROVISION. Devices marked NEEDS RE-REGISTERING must be registered again from their browser.
        </p>
        <p v-else-if="sealedState === 'error'" role="alert" class="txt">
          Couldn't load the encryption status.
          <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="sealed.fetchStatus()">RETRY</button>
        </p>
      </section>
    </ClientOnly>

    <section class="hud-card glass panel" aria-labelledby="dev-h">
      <h2 id="dev-h" class="section-lbl" style="margin:0;">DOOR DEVICES · {{ devices.filter(d => !d.revoked_at).length }} ACTIVE</h2>
      <p v-if="!devices.length" class="txt">No door devices yet.</p>
      <ul v-else class="devs" aria-label="Door devices">
        <li v-for="d in devices" :key="d.id" :class="{ revoked: !!d.revoked_at }">
          <div style="min-width:0;">
            <strong>{{ d.label }}</strong>
            <ClientOnly><span v-if="device?.id === d.id" class="data-frag" style="margin-left:6px;">THIS BROWSER</span></ClientOnly>
            <span v-if="d.revoked_at" class="data-frag" style="margin-left:6px;color:var(--color-error);">REVOKED</span>
            <span v-else-if="sealedTag(d)" class="data-frag sealed-tag" :class="`t-${sealedTag(d)}`" :data-testid="`door-sealed-${d.label}`">
              {{ SEALED_TAG[sealedTag(d)!] }}
            </span>
            <div class="hint">Added {{ when(d.created_at) }} · last login {{ when(d.last_seen_at) }}</div>
            <div v-if="!d.revoked_at && d.public_key" class="hint">
              KEY FINGERPRINT <span class="mono fpv" :data-testid="`door-fp-${d.label}`">{{ keyFingerprint(d.public_key) }}</span>
              <ClientOnly>
                <span v-if="localMatch(d) === 'match'" class="fp-ok"> · SAME AS THIS BROWSER'S KEY</span>
                <span v-else-if="localMatch(d) === 'mismatch'" class="fp-bad" role="alert"> · DOES NOT MATCH THIS BROWSER'S KEY — DON'T PROVISION</span>
              </ClientOnly>
            </div>
            <label v-if="!d.revoked_at && sealedTag(d) === 'key' && sealedState === 'unlocked'" class="ack">
              <input
                type="checkbox" :checked="!!confirmed[confirmKey(d)]" :disabled="!!busy" :data-testid="`door-confirm-${d.label}`"
                @change="setConfirmed(d, ($event.target as HTMLInputElement).checked)"
              >
              Fingerprint matches what {{ d.label }} shows on its screen
            </label>
          </div>
          <span class="dev-acts">
            <button
              v-if="!d.revoked_at && sealedTag(d) === 'key' && sealedState === 'unlocked'" type="button" class="btn-hud btn-hud-cta btn-hud-sm"
              style="min-height:44px;" :disabled="!!busy || sealedWorking !== null || !canProvision(d)" :aria-label="`Provision ${d.label} with the ban list`"
              :aria-busy="busy === `provision-${d.id}`" @click="provision(d)"
            >
              {{ busy === `provision-${d.id}` ? 'PROVISIONING…' : 'PROVISION' }}
            </button>
            <button
            v-if="!d.revoked_at" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;color:var(--color-error);"
            :disabled="busy === `revoke-${d.id}`" :aria-label="`Revoke ${d.label}`" @click="revoke(d)"
          >
            REVOKE
          </button>
          </span>
        </li>
      </ul>
      <p v-if="provisionNotice" role="status" class="prov-ok" data-testid="door-provision-notice">{{ provisionNotice }}</p>
      <p v-if="provisionError" role="alert" style="margin:0;font-size:13px;color:var(--color-error);" data-testid="door-provision-error">{{ sealedErrorText(provisionError) }}</p>
    </section>

  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}
@media (min-width: 1000px) {
  .grid { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
}
.panel {
  padding: 14px;
  display: grid;
  gap: 10px;
  min-width: 0;
}
.ic {
  width: 14px;
  height: 14px;
  display: inline;
  vertical-align: -2px;
}
.txt {
  margin: 0;
  font-size: 13px;
  color: var(--color-on-surface-variant);
}
.hint {
  margin: 0;
  font-size: 12px;
  color: var(--color-tertiary);
}
.row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.status {
  margin: 0;
  display: grid;
  gap: 6px;
}
.status dd {
  margin: 2px 0 0;
  font-family: var(--font-terminal);
  font-size: 12px;
  letter-spacing: .04em;
}
.status dd.locked {
  color: var(--color-status-archived);
  font-weight: 600;
}
.warn {
  margin: 0;
  padding: 8px 10px;
  font-size: 13px;
  color: var(--color-on-surface);
  border-left: 3px solid var(--color-status-archived);
  background: var(--color-surface-container);
}
.shown {
  display: grid;
  gap: 6px;
  padding: 10px 12px;
  border-left: 3px solid var(--color-primary);
  background: var(--color-surface-container);
}
.pin {
  font-family: var(--font-command);
  font-size: 32px;
  font-weight: 700;
  letter-spacing: .2em;
  color: var(--color-primary);
}
.devs {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 6px;
}
.devs li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 10px;
  background: var(--color-surface-container);
  font-size: 13px;
}
.devs li.revoked {
  opacity: .6;
}
.dev-acts {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  justify-content: flex-end;
}
.sealed-tag {
  margin-left: 6px;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.fpv {
  font-weight: 700;
  letter-spacing: .06em;
  color: var(--color-on-surface);
}
.fp-ok {
  color: var(--color-primary);
}
.fp-bad {
  color: var(--color-error);
  font-weight: 600;
}
.ack {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 44px;
  font-size: 12px;
  cursor: pointer;
}
.ack input {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}
.brief {
  margin: 0;
  font-size: 13px;
  color: var(--color-on-surface);
}
.prov-ok {
  margin: 0;
  padding: 8px 10px;
  font-size: 13px;
  border-left: 3px solid var(--color-primary);
  background: var(--color-surface-container);
}
.sealed-tag.t-provisioned {
  color: var(--color-primary);
}
.sealed-tag.t-none {
  color: var(--color-on-surface-variant);
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
