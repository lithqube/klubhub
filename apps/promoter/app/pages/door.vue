<script setup lang="ts">
import { onClickOutside, useNow, useOnline, useResizeObserver } from '@vueuse/core'
import { ArrowLeftFromLine, LogOut, MoreVertical, Ticket, Undo2, UserPlus } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useDoorStore } from '~/stores/door'
import type { ApiError } from '~/types/event'
import type { DoorSubject, DoorToast } from '~/types/door'
import { dayLabel, timeLabel } from '~/utils/datetime'
import { banMatches } from '~/utils/doorBan'
import { admitReasons, rejectionText, sessionEndingSoon, subjectView } from '~/utils/doorState'
import { loadJsQR, nativeQrDetector } from '~/utils/qrDecode'

/**
 * The door (P2.3): a bare, dark, offline-first page for staff on a PIN.
 * Device not prepared → how to prepare it; prepared → event + PIN pad →
 * login → bundle download → search / scan / check-in. Everything works
 * without signal after the login; the sync badge says what is waiting.
 *
 * Phone layout (375 px, keyboard up): a sticky header with the event,
 * "INSIDE 212/300", the sync badge and a menu (LOG OUT, behind a confirm);
 * the search field right under it so results stay above the keyboard; then
 * the walk-up / out counters, ADD GUEST, and the RECENT actions with UNDO.
 * The undo toast sits under the header and disappears when a card opens.
 */
definePageMeta({ public: true, layout: false })
useHead({
  title: 'Door',
  htmlAttrs: { 'data-theme': 'dark' },
  link: [{ rel: 'manifest', href: '/door.webmanifest' }],
  meta: [
    { name: 'theme-color', content: '#0E0E0F' },
    { name: 'mobile-web-app-capable', content: 'yes' },
    { name: 'apple-mobile-web-app-capable', content: 'yes' },
    { name: 'apple-mobile-web-app-status-bar-style', content: 'black' },
  ],
})

const store = useDoorStore()
const { phase, device, bundle, rejections, sessionEnded, queued, occ, checkins, wipedBecause, recent, offline, syncError, ban, banStatus } = storeToRefs(store)

// ---------------------------------------------------------------- forced dark theme
const { theme } = useTheme()
const setDark = () => document.documentElement.setAttribute('data-theme', 'dark')
let stopThemeWatch: (() => void) | undefined
onMounted(() => {
  setDark()
  // The app shell applies the saved theme after this page mounts: re-assert dark.
  stopThemeWatch = watch(theme, () => nextTick(setDark))
})
onBeforeUnmount(() => {
  stopThemeWatch?.()
  document.documentElement.setAttribute('data-theme', theme.value)
})

// ---------------------------------------------------------------- boot + service worker
function warmCache() {
  if (import.meta.dev || !('serviceWorker' in navigator)) return
  const urls = performance.getEntriesByType('resource').map(e => e.name)
    .filter(u => new URL(u, location.href).pathname.startsWith('/_nuxt/'))
  void navigator.serviceWorker.ready.then(r => r.active?.postMessage({ type: 'door-cache', urls: ['/door', ...urls] }))
}

/**
 * The door must be its own document: the camera is only allowed on /door
 * (Permissions-Policy), and the service worker only controls documents
 * loaded under /door. Arriving by an in-app link loads it properly.
 */
function loadedElsewhere(): boolean {
  const nav = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined
  return !!nav && new URL(nav.name).pathname.replace(/\/$/, '') !== '/door'
}

onMounted(async () => {
  if (loadedElsewhere()) {
    location.replace('/door')
    return
  }
  await store.init()
  if (!import.meta.dev && 'serviceWorker' in navigator) {
    navigator.serviceWorker.register('/door-sw.js', { scope: '/door' }).then(() => warmCache()).catch(() => undefined)
  }
})

// Browsers without BarcodeDetector (iOS Safari) scan with jsQR: fetch it now so it is cached for offline.
watch(phase, async (p) => {
  if (p !== 'ready' || !import.meta.client) return
  if (!(await nativeQrDetector())) {
    await loadJsQR().catch(() => undefined)
    warmCache()
  }
}, { immediate: true })

// ---------------------------------------------------------------- announcements
const announce = ref('')
/** Clear, then set on the next tick, so a repeated text is read again. */
function say(text: string) {
  announce.value = ''
  nextTick(() => (announce.value = text))
}

// ---------------------------------------------------------------- login
const isOnline = useOnline()
const loginBusy = ref(false)
const loginError = ref('')
const relogin = ref(false)
const lockedUntil = ref<number | null>(null)
const LOGIN_ERRORS: Record<string, string> = {
  invalid_credentials: 'Wrong PIN. After 5 wrong tries the PIN locks for 15 minutes.',
  pin_expired: 'This door PIN has expired. Ask a manager for a new one (the event’s DOOR tab).',
  network_error: 'No connection. The first login needs the network; after that the door works offline.',
  not_prepared: 'This browser is not a door device.',
  event_purged: 'Guest names for this event were erased after the night, so there is no list to open.',
}
async function login(pin: string) {
  loginBusy.value = true
  loginError.value = ''
  try {
    await store.login(pin)
    relogin.value = false
    lockedUntil.value = null
    await nextTick()
    searchRef.value?.focus()
  } catch (e) {
    const err = e as ApiError
    if (err.error === 'pin_locked') {
      const until = Date.parse(String(err.detail?.retry_after ?? ''))
      lockedUntil.value = Number.isFinite(until) ? until : Date.now() + 15 * 60_000
    } else {
      loginError.value = LOGIN_ERRORS[err.error] ?? 'Could not open the door. Try again.'
    }
  } finally {
    loginBusy.value = false
  }
}

// ---------------------------------------------------------------- door UI
const now = useNow({ interval: 30_000 })
const q = ref('')
const selected = ref<DoorSubject | null>(null)
const scanning = ref(false)
const adding = ref(false)
const menuOpen = ref(false)
const searchNote = ref('')
const searchRef = ref<{ focus: () => void } | null>(null)
const addBtn = ref<HTMLButtonElement | null>(null)
const menuWrap = ref<HTMLElement | null>(null)
const menuBtn = ref<HTMLButtonElement | null>(null)
const headRef = ref<HTMLElement | null>(null)
const headH = ref(80)
const view = computed(() => (bundle.value && selected.value ? subjectView(bundle.value, checkins.value, selected.value, now.value.getTime()) : null))
const tz = computed(() => bundle.value?.event.timezone ?? 'UTC')
/** Possible ban list matches for the open card (P2.6; empty when the bundle has no sealed block). */
const banHits = computed(() => (view.value ? banMatches(ban.value, view.value.name) : []))
const BAN_STATUS: Record<string, string> = {
  ready: 'BAN LIST CHECKED ON THIS DEVICE',
  no_key: 'BAN LIST NOT AVAILABLE: THIS DEVICE HAS NO KEY',
  unreadable: 'BAN LIST NOT AVAILABLE: IT DOES NOT OPEN ON THIS DEVICE',
}

// The sticky header's height, so the search field scrolls to just under it.
useResizeObserver(headRef, (entries) => {
  const h = entries[0]?.target.getBoundingClientRect().height
  if (h) headH.value = Math.ceil(h)
})
onClickOutside(menuWrap, () => (menuOpen.value = false))
function closeMenu() {
  menuOpen.value = false
  menuBtn.value?.focus()
}

const MODE_KEY = 'klubhub-door-mode'
const express = ref(false)
onMounted(() => {
  try {
    express.value = localStorage.getItem(MODE_KEY) === 'express'
  } catch {
    // Storage blocked: Standard mode.
  }
})
watch(express, (x) => {
  try {
    localStorage.setItem(MODE_KEY, x ? 'express' : 'standard')
  } catch {
    // Not remembered; fine.
  }
})

const toast = ref<DoorToast | null>(null)
let toastSeq = 0
function notify(text: string, nonce: string | null, tone: 'ok' | 'warn' = 'ok') {
  toast.value = { id: ++toastSeq, text, nonce, tone }
  say(text)
}
// A card replaces the toast: its UNDO must never sit under the next ADMIT.
watch(selected, (s) => {
  if (s) toast.value = null
})
watch(q, (v) => {
  if (v.trim()) searchNote.value = ''
})

// Loud dark room: a short full-screen flash (a static border with reduced motion) and a buzz.
const flash = ref<{ id: number, tone: 'ok' | 'warn' } | null>(null)
let flashSeq = 0
let flashTimer: ReturnType<typeof setTimeout> | undefined
function feedback(tone: 'ok' | 'warn') {
  flash.value = { id: ++flashSeq, tone }
  clearTimeout(flashTimer)
  flashTimer = setTimeout(() => (flash.value = null), 400)
  try {
    navigator.vibrate?.(tone === 'ok' ? 40 : [80, 60, 80])
  } catch {
    // No vibration motor or not allowed: the flash is enough.
  }
}
onBeforeUnmount(() => clearTimeout(flashTimer))

function focusSearch() {
  nextTick(() => searchRef.value?.focus())
}

/** Back to the search. Focus returns to the field only for keyboard users (on a phone it would pop the keyboard). */
function backToSearch(focus = true) {
  selected.value = null
  if (focus) focusSearch()
}

async function admit(count: number, keyboard: boolean) {
  const v = view.value
  if (!v) return
  const nonce = await store.checkIn(v.subject, count)
  feedback('ok')
  q.value = ''
  backToSearch(keyboard)
  notify(`${v.name} · ${count} IN`, nonce)
}

async function undo(nonce: string) {
  const name = recent.value.find(r => r.nonce === nonce)?.name
  await store.undo(nonce)
  notify(name ? `UNDONE · ${name}` : 'UNDONE', null)
}

async function walkup() {
  notify('WALK-UP +1', await store.counter('walkup'))
}
async function out() {
  notify('OUT −1', await store.counter('out'))
}

const scanResult = ref<{ id: number, text: string, tone: 'ok' | 'warn' } | null>(null)
let scanSeq = 0
async function onCode(code: string) {
  const b = bundle.value
  if (!b) return
  const t = b.tickets.find(x => x.secret === code) ?? b.tickets.find(x => x.order_ref.toLowerCase() === code.toLowerCase())
  if (!t) {
    feedback('warn')
    if (express.value) {
      scanResult.value = { id: ++scanSeq, text: 'CODE NOT FOUND · CLOSE ✕ AND SEARCH THE NAME', tone: 'warn' }
      say('Code not found')
      return
    }
    scanning.value = false
    searchNote.value = 'Code not found — search the name.'
    say(searchNote.value)
    focusSearch()
    return
  }
  const s: DoorSubject = { kind: 'ticket', id: t.id }
  const v = subjectView(b, checkins.value, s, now.value.getTime())
  const clean = !!v && !v.blocked && v.remaining > 0 && !admitReasons(v).length && !banMatches(ban.value, v.name).length
  if (express.value && v && clean) {
    const nonce = await store.checkIn(s, 1)
    feedback('ok')
    scanResult.value = { id: ++scanSeq, text: `${v.name} · 1 IN`, tone: 'ok' }
    notify(`${v.name} · 1 IN`, nonce)
    return
  }
  // A card must open: the scanner closes.
  feedback(clean ? 'ok' : 'warn')
  scanning.value = false
  selected.value = s
}

function added(r: { nonce: string, name: string, count: number }) {
  adding.value = false
  feedback('ok')
  notify(`${r.name} ADDED · ${r.count} IN`, r.nonce)
}
function closeAdd() {
  adding.value = false
  nextTick(() => addBtn.value?.focus())
}

const OFFLINE_LOGOUT = 'You are offline. If you log out, this phone cannot open the door again until it has signal.'
const ONLINE_LOGOUT = 'Log out? You need the door PIN and a connection to log in again.'
async function logout() {
  let msg = offline.value || !isOnline.value ? OFFLINE_LOGOUT : ONLINE_LOGOUT
  if (queued.value) msg += ` ${queued.value} door actions have not synced yet and will be lost.`
  if (!window.confirm(msg)) return
  menuOpen.value = false
  await store.logout()
}

// Only transitions into and out of trouble are announced (not every sync).
const syncKind = computed(() => (sessionEnded.value ? 'signedout' : offline.value ? 'offline' : syncError.value ? 'error' : 'ok'))
const SYNC_SAY: Record<string, string> = {
  offline: 'Offline. Door actions are kept on this phone.',
  signedout: 'Signed out. Log in again to sync.',
  error: 'Sync error.',
}
watch(syncKind, (k, was) => {
  if (phase.value !== 'ready') return
  say(k === 'ok' ? (was === 'offline' ? 'Back online.' : 'Sync working again.') : SYNC_SAY[k]!)
})

const endingSoon = computed(() => !!bundle.value && !sessionEnded.value && sessionEndingSoon(bundle.value, now.value.getTime()))

const eventLine = computed(() => {
  const e = bundle.value?.event
  return e ? dayLabel(e.starts_at, e.timezone) : device.value ? dayLabel(device.value.event.starts_at, Intl.DateTimeFormat().resolvedOptions().timeZone) : ''
})
const WIPED: Record<string, string> = {
  logout: 'Logged out. Nothing from the guest list is left on this device.',
  session: 'The door session ended, so this device forgot the guest list. Enter a new PIN to continue.',
  event: 'The night is over, so this device forgot the guest list.',
}
const REJECTION_HELP = 'These were not saved. Check with a manager before letting these guests in again.'
/** Every rejection is an erased event (P2.5): nothing the door can fix. */
const rejectionHelp = computed(() => (rejections.value.length && rejections.value.every(r => r.error === 'event_purged')
  ? 'The event\'s guest data was erased, so these door adds weren\'t saved. Nothing to do at the door.'
  : REJECTION_HELP))
</script>

<template>
  <div class="door hud-bg" :style="{ '--door-head-h': `${headH}px` }">
    <p class="sr-only" aria-live="assertive" role="status" data-testid="door-announce">{{ announce }}</p>
    <div v-if="flash" :key="flash.id" class="flash" :class="flash.tone" aria-hidden="true" data-testid="door-flash" />

    <main v-if="phase === 'boot'" class="center">
      <p class="section-lbl" role="status">LOADING THE DOOR…</p>
    </main>

    <main v-else-if="phase === 'unprepared'" class="center narrow">
      <h1 class="title">DOOR</h1>
      <section class="glass panel">
        <h2 class="section-lbl" style="margin:0 0 8px;">THIS BROWSER IS NOT A DOOR DEVICE YET</h2>
        <p style="margin:0 0 10px;font-size:15px;">
          Prepare it once from the event, signed in as staff: <strong>EVENTS → the night → DOOR → Use this browser as a door device</strong>.
          Then generate a door PIN there and open the door here.
        </p>
        <NuxtLink to="/events" class="btn-hud btn-hud-cta big">OPEN EVENTS</NuxtLink>
      </section>
    </main>

    <main v-else-if="phase === 'locked' || phase === 'loading'" class="center narrow">
      <h1 class="title">{{ device?.event.title }}</h1>
      <p class="sub">{{ eventLine }} · {{ device?.label }}</p>
      <p v-if="wipedBecause" role="status" class="glass notice">{{ WIPED[wipedBecause] }}</p>
      <p v-if="!isOnline" role="status" class="offline-hint" data-testid="door-offline-hint">OFFLINE · CONNECT TO LOG IN</p>
      <p v-if="phase === 'loading'" role="status" class="section-lbl" style="text-align:center;">DOWNLOADING THE GUEST LIST…</p>
      <template v-else>
        <DoorLockNotice v-if="lockedUntil" :until="lockedUntil" @over="lockedUntil = null" />
        <DoorPinPad :busy="loginBusy" :disabled="!!lockedUntil" :error="loginError" @submit="login" />
      </template>
      <p class="foot">Wrong event? Switch it in that event’s DOOR tab.</p>
    </main>

    <template v-else-if="phase === 'ready' && bundle">
      <header ref="headRef" class="top">
        <div class="head-row">
          <div class="id">
            <h1 class="title small">{{ bundle.event.title }}</h1>
            <DoorOccupancy :occ="occ" />
          </div>
          <DoorSyncBadge />
          <div ref="menuWrap" class="menu-wrap" @keydown.esc="closeMenu">
            <button
              ref="menuBtn" type="button" class="btn-hud btn-hud-ghost menu-btn" aria-label="Door menu" :aria-expanded="menuOpen"
              aria-controls="door-menu" @click="menuOpen = !menuOpen"
            >
              <MoreVertical style="width:22px;height:22px;" aria-hidden="true" />
            </button>
            <div v-if="menuOpen" id="door-menu" class="menu glass">
              <p class="menu-meta">{{ device?.label }}<br>{{ eventLine }}</p>
              <p v-if="BAN_STATUS[banStatus]" class="menu-meta" data-testid="door-ban-status">{{ BAN_STATUS[banStatus] }}</p>
              <button type="button" class="btn-hud btn-hud-ghost big" style="color:var(--color-error);width:100%;" @click="logout">
                <LogOut style="width:18px;height:18px;" aria-hidden="true" /> LOG OUT
              </button>
            </div>
          </div>
        </div>
        <p v-if="endingSoon" role="status" class="ending" data-testid="door-ending">
          DOOR PIN ENDS {{ timeLabel(bundle.session_expires_at, tz) }} · ASK A MANAGER FOR A NEW ONE
        </p>
        <DoorUndoToast :toast="toast" @undo="undo" @expire="toast = null" />
      </header>

      <main class="body">
        <section v-if="sessionEnded" class="glass panel accent-bar-failed" aria-labelledby="ended-h">
          <h2 id="ended-h" class="section-lbl" style="margin:0 0 6px;">DOOR SESSION ENDED</h2>
          <p style="margin:0 0 10px;font-size:14px;">
            The PIN window closed or this device was revoked. {{ queued }} door actions are kept and sync after you log in again with a new PIN.
          </p>
          <template v-if="relogin">
            <p v-if="!isOnline" role="status" class="offline-hint">OFFLINE · CONNECT TO LOG IN</p>
            <DoorLockNotice v-if="lockedUntil" :until="lockedUntil" @over="lockedUntil = null" />
            <DoorPinPad :busy="loginBusy" :disabled="!!lockedUntil" :error="loginError" label="NEW DOOR PIN" @submit="login" />
          </template>
          <button v-else type="button" class="btn-hud btn-hud-cta big" @click="relogin = true">LOG IN AGAIN</button>
        </section>

        <DoorCard
          v-if="view" :view="view" :timezone="bundle.event.timezone" :self-device="bundle.device_id"
          :ban="banHits" :manager-pin="bundle.manager_pin" @admit="admit" @back="backToSearch()"
        />
        <DoorSearch v-show="!view" ref="searchRef" v-model="q" :note="searchNote" @pick="selected = $event" @scan="scanning = true" @announce="say" />

        <template v-if="!view">
          <div class="counters" role="group" aria-label="Counters">
            <button type="button" class="btn-hud btn-hud-ghost big" @click="walkup">
              <Ticket style="width:18px;height:18px;" aria-hidden="true" /> WALK-UP +1
            </button>
            <button type="button" class="btn-hud btn-hud-ghost big" @click="out">
              <ArrowLeftFromLine style="width:18px;height:18px;" aria-hidden="true" /> OUT −1
            </button>
          </div>
          <button ref="addBtn" type="button" class="btn-hud btn-hud-ghost big" style="width:100%;" @click="adding = true">
            <UserPlus style="width:18px;height:18px;" aria-hidden="true" /> ADD GUEST
          </button>
        </template>

        <section v-if="rejections.length" class="glass panel accent-bar-archived" aria-labelledby="rej-h">
          <h2 id="rej-h" class="section-lbl" style="margin:0 0 6px;">NOT ACCEPTED BY THE SERVER</h2>
          <p style="margin:0 0 6px;font-size:13px;color:var(--color-on-surface-variant);" data-testid="door-rejection-help">{{ rejectionHelp }}</p>
          <ul style="margin:0 0 8px;padding-left:18px;font-size:14px;">
            <li v-for="r in rejections" :key="`${r.what}|${r.at}`">{{ timeLabel(r.at, tz) }} · {{ r.what }}: {{ rejectionText(r.error) }}</li>
          </ul>
          <button type="button" class="btn-hud btn-hud-ghost big" @click="store.dismissRejections()">DISMISS</button>
        </section>

        <section v-if="!view && !q.trim() && recent.length" aria-labelledby="recent-h">
          <h2 id="recent-h" class="section-lbl" style="margin:4px 0 6px;">RECENT</h2>
          <ul class="recent" aria-labelledby="recent-h">
            <li v-for="r in recent" :key="r.nonce">
              <span class="r-main">
                <span class="r-name">{{ r.name }}</span>
                <span class="r-meta">{{ r.what }} · {{ timeLabel(r.at, tz) }} · {{ r.synced ? 'SYNCED' : 'QUEUED' }}</span>
              </span>
              <button type="button" class="btn-hud btn-hud-ghost r-undo" :aria-label="`Undo ${r.name} ${r.what} at ${timeLabel(r.at, tz)}`" @click="undo(r.nonce)">
                <Undo2 style="width:18px;height:18px;" aria-hidden="true" /> UNDO
              </button>
            </li>
          </ul>
        </section>
      </main>

      <DoorScanner v-if="scanning" v-model:express="express" :result="scanResult" @code="onCode" @close="scanning = false" />
      <DoorAddSheet v-if="adding" @added="added" @close="closeAdd" />
    </template>
  </div>
</template>

<style scoped>
.door {
  min-height: 100dvh;
  color: var(--color-on-surface);
  font-family: var(--font-data);
  overflow-x: hidden;
}
.center {
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 14px;
  padding: 24px 16px;
}
.narrow {
  max-width: 420px;
  margin: 0 auto;
}
.title {
  margin: 0;
  font-family: var(--font-command);
  font-size: 26px;
  font-weight: 700;
  text-align: center;
  overflow-wrap: anywhere;
}
.title.small {
  font-size: 17px;
  text-align: left;
}
.sub {
  margin: 0;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  text-transform: uppercase;
  color: var(--color-on-surface-variant);
  text-align: center;
}
.top .sub {
  text-align: left;
}
.panel {
  padding: 14px;
}
.notice {
  margin: 0;
  padding: 10px 12px;
  font-size: 14px;
}
.foot {
  margin: 0;
  font-size: 12px;
  text-align: center;
  color: var(--color-tertiary);
}
.top {
  position: sticky;
  top: 0;
  z-index: 10;
  display: grid;
  gap: 6px;
  padding: calc(8px + env(safe-area-inset-top)) 16px 8px;
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-outline-variant);
}
.head-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.id {
  flex: 1;
  min-width: 0;
  display: grid;
  gap: 2px;
}
.id .title.small {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.menu-wrap {
  position: relative;
  flex-shrink: 0;
}
.menu-btn {
  min-width: 56px;
  min-height: 56px;
  height: 56px;
  padding: 0;
}
.menu {
  position: absolute;
  right: 0;
  top: calc(100% + 6px);
  z-index: 20;
  width: 240px;
  display: grid;
  gap: 8px;
  padding: 10px;
  background: var(--color-surface-container-high);
}
.menu-meta {
  margin: 0;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  text-transform: uppercase;
  color: var(--color-on-surface-variant);
  overflow-wrap: anywhere;
}
.ending {
  margin: 0;
  padding: 6px 10px;
  font-family: var(--font-terminal);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: .05em;
  color: var(--color-status-archived);
  border-left: 3px solid var(--color-status-archived);
  background: var(--color-surface-container);
}
.offline-hint {
  margin: 0;
  text-align: center;
  font-family: var(--font-terminal);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: .06em;
  color: var(--color-status-archived);
}
.recent {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 4px;
}
.recent li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-left: 12px;
  background: var(--color-surface-container);
}
.r-main {
  display: grid;
  min-width: 0;
}
.r-name {
  font-size: 15px;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.r-meta {
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .05em;
  color: var(--color-on-surface-variant);
}
.r-undo {
  min-height: 56px;
  height: 56px;
  font-size: 11px;
  flex-shrink: 0;
}
.flash {
  position: fixed;
  inset: 0;
  z-index: 60;
  pointer-events: none;
  background: color-mix(in srgb, var(--color-primary) 30%, transparent);
  animation: door-flash .4s ease-out forwards;
}
.flash.warn {
  background: color-mix(in srgb, var(--color-status-archived) 30%, transparent);
}
@keyframes door-flash {
  from { opacity: 1; }
  to { opacity: 0; }
}
@media (prefers-reduced-motion: reduce) {
  .flash {
    animation: none;
    background: none;
    box-shadow: inset 0 0 0 8px var(--color-primary);
  }
  .flash.warn {
    box-shadow: inset 0 0 0 8px var(--color-status-archived);
  }
}
.body {
  display: grid;
  gap: 12px;
  padding: 12px 16px calc(32px + env(safe-area-inset-bottom));
  max-width: 720px;
  margin: 0 auto;
  min-width: 0;
}
.counters {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}
.big {
  min-height: 56px;
  height: 56px;
  font-size: 11px;
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
