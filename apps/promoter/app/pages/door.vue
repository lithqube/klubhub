<script setup lang="ts">
import { useNow } from '@vueuse/core'
import { LogOut, UserPlus } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useDoorStore } from '~/stores/door'
import type { ApiError } from '~/types/event'
import type { DoorSubject, DoorToast } from '~/types/door'
import { dayLabel } from '~/utils/datetime'
import { rejectionText, subjectView } from '~/utils/doorState'
import { loadJsQR, nativeQrDetector } from '~/utils/qrDecode'

/**
 * The door (P2.3): a bare, dark, offline-first page for staff on a PIN.
 * Device not prepared → how to prepare it; prepared → event + PIN pad →
 * login → bundle download → search / scan / check-in. Everything works
 * without signal after the login; the sync badge says what is waiting.
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
const { phase, device, bundle, rejections, sessionEnded, queued, occ, checkins, wipedBecause } = storeToRefs(store)

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

// ---------------------------------------------------------------- login
const loginBusy = ref(false)
const loginError = ref('')
const relogin = ref(false)
const LOGIN_ERRORS: Record<string, string> = {
  invalid_credentials: 'Wrong PIN, or the PIN has expired. After 5 wrong tries the PIN locks for 15 minutes.',
  network_error: 'No connection. The first login needs the network; after that the door works offline.',
  not_prepared: 'This browser is not a door device.',
}
async function login(pin: string) {
  loginBusy.value = true
  loginError.value = ''
  try {
    await store.login(pin)
    relogin.value = false
    await nextTick()
    searchRef.value?.focus()
  } catch (e) {
    loginError.value = LOGIN_ERRORS[(e as ApiError).error] ?? 'Could not open the door. Try again.'
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
const searchRef = ref<{ focus: () => void } | null>(null)
const view = computed(() => (bundle.value && selected.value ? subjectView(bundle.value, checkins.value, selected.value, now.value.getTime()) : null))

const MODE_KEY = 'klubhub-door-mode'
const express = ref(false)
onMounted(() => {
  try {
    express.value = localStorage.getItem(MODE_KEY) === 'express'
  } catch {
    // Storage blocked: Standard mode.
  }
})
function setMode(x: boolean) {
  express.value = x
  try {
    localStorage.setItem(MODE_KEY, x ? 'express' : 'standard')
  } catch {
    // Not remembered; fine.
  }
}

const toast = ref<DoorToast | null>(null)
const announce = ref('')
let toastSeq = 0
function notify(text: string, nonce: string | null, tone: 'ok' | 'warn' = 'ok') {
  toast.value = { id: ++toastSeq, text, nonce, tone }
  announce.value = text
}

function backToSearch() {
  selected.value = null
  nextTick(() => searchRef.value?.focus())
}

async function admit(count: number) {
  const v = view.value
  if (!v) return
  const nonce = await store.checkIn(v.subject, count)
  notify(`${v.name} · ${count} IN`, nonce)
  q.value = ''
  backToSearch()
}

async function undo(nonce: string) {
  await store.undo(nonce)
  notify('UNDONE', null)
}

async function walkup() {
  notify('WALK-UP +1', await store.counter('walkup'))
}
async function out() {
  notify('OUT −1', await store.counter('out'))
}

async function onCode(code: string) {
  scanning.value = false
  const b = bundle.value
  if (!b) return
  const t = b.tickets.find(x => x.secret === code) ?? b.tickets.find(x => x.order_ref.toLowerCase() === code.toLowerCase())
  if (!t) {
    notify(`UNKNOWN CODE · ${code.slice(0, 24)}`, null, 'warn')
    return
  }
  const s: DoorSubject = { kind: 'ticket', id: t.id }
  const v = subjectView(b, checkins.value, s, now.value.getTime())
  if (express.value && v && !v.blocked && v.remaining > 0 && !v.warning && !v.conflict) {
    notify(`${v.name} · 1 IN`, await store.checkIn(s, 1))
    return
  }
  selected.value = s
}

function added(r: { nonce: string, name: string, count: number }) {
  adding.value = false
  notify(`${r.name} ADDED · ${r.count} IN`, r.nonce)
}

async function logout() {
  if (queued.value && !window.confirm(`${queued.value} door actions have not synced yet and will be lost. Log out anyway?`)) return
  await store.logout()
}

const eventLine = computed(() => {
  const e = bundle.value?.event
  return e ? dayLabel(e.starts_at, e.timezone) : device.value ? dayLabel(device.value.event.starts_at, Intl.DateTimeFormat().resolvedOptions().timeZone) : ''
})
const WIPED: Record<string, string> = {
  logout: 'Logged out. Nothing from the guest list is left on this device.',
  session: 'The door session ended, so this device forgot the guest list. Enter a new PIN to continue.',
  event: 'The night is over, so this device forgot the guest list.',
}
</script>

<template>
  <div class="door hud-bg">
    <p class="sr-only" aria-live="assertive" role="status" data-testid="door-announce">{{ announce }}</p>

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
      <p v-if="phase === 'loading'" role="status" class="section-lbl" style="text-align:center;">DOWNLOADING THE GUEST LIST…</p>
      <DoorPinPad v-else :busy="loginBusy" :error="loginError" @submit="login" />
      <p class="foot">Wrong event? Switch it in that event’s DOOR tab.</p>
    </main>

    <template v-else-if="phase === 'ready' && bundle">
      <header class="top">
        <div style="min-width:0;">
          <h1 class="title small">{{ bundle.event.title }}</h1>
          <p class="sub">{{ device?.label }}</p>
        </div>
        <DoorSyncBadge />
      </header>

      <main class="body">
        <section v-if="sessionEnded" class="glass panel accent-bar-failed" aria-labelledby="ended-h">
          <h2 id="ended-h" class="section-lbl" style="margin:0 0 6px;">DOOR SESSION ENDED</h2>
          <p style="margin:0 0 10px;font-size:14px;">
            The PIN window closed or this device was revoked. {{ queued }} door actions are kept and sync after you log in again.
          </p>
          <DoorPinPad v-if="relogin" :busy="loginBusy" :error="loginError" label="NEW DOOR PIN" @submit="login" />
          <button v-else type="button" class="btn-hud btn-hud-cta big" @click="relogin = true">LOG IN AGAIN</button>
        </section>

        <section v-if="rejections.length" class="glass panel accent-bar-archived" aria-labelledby="rej-h">
          <h2 id="rej-h" class="section-lbl" style="margin:0 0 6px;">NOT ACCEPTED BY THE SERVER</h2>
          <ul style="margin:0 0 8px;padding-left:18px;font-size:14px;">
            <li v-for="r in rejections" :key="r.nonce">{{ r.what }}: {{ rejectionText(r.error) }}</li>
          </ul>
          <button type="button" class="btn-hud btn-hud-ghost big" @click="store.dismissRejections()">DISMISS</button>
        </section>

        <DoorOccupancy :occ="occ" @walkup="walkup" @out="out" />

        <div class="modes" role="radiogroup" aria-label="Scan mode">
          <button type="button" role="radio" :aria-checked="!express" class="btn-hud big" :class="!express ? 'btn-hud-cta' : 'btn-hud-ghost'" @click="setMode(false)">
            STANDARD
          </button>
          <button type="button" role="radio" :aria-checked="express" class="btn-hud big" :class="express ? 'btn-hud-cta' : 'btn-hud-ghost'" @click="setMode(true)">
            EXPRESS
          </button>
        </div>

        <DoorCard v-if="view" :view="view" :timezone="bundle.event.timezone" @admit="admit" @back="backToSearch" />
        <DoorSearch v-show="!view" ref="searchRef" v-model="q" @pick="selected = $event" @scan="scanning = true" />

        <div class="actions">
          <button type="button" class="btn-hud btn-hud-ghost big" @click="adding = true">
            <UserPlus style="width:18px;height:18px;" aria-hidden="true" /> ADD GUEST
          </button>
          <button type="button" class="btn-hud btn-hud-ghost big" @click="logout">
            <LogOut style="width:18px;height:18px;" aria-hidden="true" /> LOG OUT
          </button>
        </div>
      </main>

      <DoorScanner v-if="scanning" :express="express" @code="onCode" @close="scanning = false" />
      <DoorAddSheet v-if="adding" @added="added" @close="adding = false" />
      <DoorUndoToast :toast="toast" @undo="undo" @expire="toast = null" />
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: calc(8px + env(safe-area-inset-top)) 16px 8px;
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-outline-variant);
}
.body {
  display: grid;
  gap: 12px;
  padding: 12px 16px 104px;
  max-width: 720px;
  margin: 0 auto;
  min-width: 0;
}
.modes,
.actions {
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
