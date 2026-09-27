<script setup lang="ts">
import { KeyRound, MonitorSmartphone, ScanLine } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useDoorStore } from '~/stores/door'
import { useEventStore } from '~/stores/event'
import { usePrivacyStore } from '~/stores/privacy'
import type { ApiError } from '~/types/event'
import type { PinResult, PinWindow } from '~/types/door'
import { dayLabel, instantToZoned, timeLabel, zonedToInstant } from '~/utils/datetime'
import { defaultPinValidUntil, MAX_PIN_WINDOW_HOURS } from '~/utils/doorState'
import { shortDate } from '~/utils/privacy'

/**
 * Event DOOR tab (P2.3): prepare this browser as a door device, generate
 * the staff and manager PINs (each shown once; both stay visible until
 * hidden), see a PIN lockout, list and revoke devices, and open the door.
 */
const { current } = storeToRefs(useEventStore())
const store = useDoorStore()
const { device, devices, pinStatus } = storeToRefs(store)
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

onMounted(() => store.loadDevice())

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
    const d = await store.registerDevice(label.value, eventRef.value)
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
        <ClientOnly>
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
        <fieldset class="row" style="border:0;padding:0;margin:0;">
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

    <section class="hud-card glass panel" aria-labelledby="dev-h">
      <h2 id="dev-h" class="section-lbl" style="margin:0;">DOOR DEVICES · {{ devices.filter(d => !d.revoked_at).length }} ACTIVE</h2>
      <p v-if="!devices.length" class="txt">No door devices yet.</p>
      <ul v-else class="devs" aria-label="Door devices">
        <li v-for="d in devices" :key="d.id" :class="{ revoked: !!d.revoked_at }">
          <div style="min-width:0;">
            <strong>{{ d.label }}</strong>
            <ClientOnly><span v-if="device?.id === d.id" class="data-frag" style="margin-left:6px;">THIS BROWSER</span></ClientOnly>
            <span v-if="d.revoked_at" class="data-frag" style="margin-left:6px;color:var(--color-error);">REVOKED</span>
            <div class="hint">Added {{ when(d.created_at) }} · last login {{ when(d.last_seen_at) }}</div>
          </div>
          <button
            v-if="!d.revoked_at" type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;color:var(--color-error);"
            :disabled="busy === `revoke-${d.id}`" :aria-label="`Revoke ${d.label}`" @click="revoke(d)"
          >
            REVOKE
          </button>
        </li>
      </ul>
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
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
