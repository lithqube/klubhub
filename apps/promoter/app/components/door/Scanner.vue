<script setup lang="ts">
import { X } from 'lucide-vue-next'
import { decodeFrame, type JsQR, loadJsQR, nativeQrDetector, SCAN_INTERVAL_MS, scaledSize } from '~/utils/qrDecode'

/**
 * QR scanner: the back camera, decoded with BarcodeDetector where the
 * browser has it, else jsQR (iOS Safari), throttled to ~9 fps. A typed
 * code always works — for a broken camera, a denied permission, a QR
 * reader that could not load offline, or a printed code the camera cannot
 * read.
 *
 * STANDARD: a code closes the scanner (the page opens the card).
 * EXPRESS: the camera stays open between scans; every code is emitted,
 * the page's answer shows over the video (`result`), and the same code is
 * ignored for 2 s. The page closes the scanner only when a card must open.
 * The mode switch lives in this header.
 */
const props = defineProps<{ result?: { id: number, text: string, tone: 'ok' | 'warn' } | null }>()
const express = defineModel<boolean>('express', { default: false })
const emit = defineEmits<{ code: [value: string], close: [] }>()

const REPEAT_MS = 2000
const RESULT_MS = 1600
const video = ref<HTMLVideoElement | null>(null)
const codeInput = ref<HTMLInputElement | null>(null)
const radios = ref<HTMLButtonElement[]>([])
const typed = ref('')
const status = ref<'starting' | 'scanning' | 'nocamera'>('starting')
const cameraError = ref('')
const shownResult = ref<{ text: string, tone: 'ok' | 'warn' } | null>(null)
let stream: MediaStream | null = null
let raf = 0
let stopped = false
let last: { value: string, at: number } | null = null
let resultTimer: ReturnType<typeof setTimeout> | undefined

watch(() => props.result?.id, () => {
  clearTimeout(resultTimer)
  shownResult.value = props.result ? { text: props.result.text, tone: props.result.tone } : null
  if (shownResult.value) resultTimer = setTimeout(() => (shownResult.value = null), RESULT_MS)
})

function stop() {
  stopped = true
  cancelAnimationFrame(raf)
  stream?.getTracks().forEach(t => t.stop())
  stream = null
}

function noCamera(msg: string) {
  stop()
  status.value = 'nocamera'
  cameraError.value = msg
  nextTick(() => codeInput.value?.focus())
}

function found(raw: string) {
  const value = raw.trim()
  if (stopped || !value) return
  if (!express.value) {
    stop()
    emit('code', value)
    return
  }
  const now = Date.now()
  if (last && last.value === value && now - last.at < REPEAT_MS) return
  last = { value, at: now }
  emit('code', value)
}

async function start() {
  if (!navigator.mediaDevices?.getUserMedia) {
    noCamera('This browser gives no camera access here. Type the code below.')
    return
  }
  try {
    stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' }, audio: false })
    if (stopped) return stop()
    const v = video.value!
    v.srcObject = stream
    await v.play()
  } catch {
    noCamera('No camera (permission denied or in use). Type the code below.')
    return
  }
  const detector = await nativeQrDetector()
  let jsqr: JsQR | null = null
  if (!detector) {
    jsqr = await loadJsQR().catch(() => null)
    if (!jsqr) {
      noCamera('The QR reader could not load: no signal, and it was not saved on this phone yet. Type the code below.')
      return
    }
  }
  if (stopped) return
  status.value = 'scanning'
  const canvas = document.createElement('canvas')
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  let lastFrame = 0
  let busy = false
  const loop = async (t: number) => {
    if (stopped) return
    raf = requestAnimationFrame(loop)
    const v = video.value
    if (busy || t - lastFrame < SCAN_INTERVAL_MS || !v || v.readyState < 2) return
    lastFrame = t
    busy = true
    try {
      if (detector) {
        const codes = await detector.detect(v)
        if (codes[0]?.rawValue) found(codes[0].rawValue)
      } else if (jsqr && ctx) {
        const { w, h } = scaledSize(v.videoWidth, v.videoHeight)
        if (!w) return
        canvas.width = w
        canvas.height = h
        ctx.drawImage(v, 0, 0, w, h)
        const value = decodeFrame(jsqr, ctx.getImageData(0, 0, w, h).data, w, h)
        if (value) found(value)
      }
    } catch {
      // A bad frame: try the next one.
    } finally {
      busy = false
    }
  }
  raf = requestAnimationFrame(loop)
}

function submitTyped() {
  const value = typed.value.trim()
  if (!value) return
  if (express.value) {
    // Typed codes are deliberate: no repeat guard, and the field is ready for the next one.
    last = null
    typed.value = ''
    emit('code', value)
    nextTick(() => codeInput.value?.focus())
    return
  }
  stop()
  emit('code', value)
}

// Radio group: arrow keys move the choice (and focus) between the two modes.
function onModeKey(e: KeyboardEvent) {
  if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) return
  e.preventDefault()
  express.value = !express.value
  nextTick(() => radios.value[express.value ? 1 : 0]?.focus())
}

onMounted(() => void start())
onBeforeUnmount(() => {
  stop()
  clearTimeout(resultTimer)
})
</script>

<template>
  <div class="scanner" role="dialog" aria-modal="true" aria-labelledby="scan-h" @keydown.esc="stop(); emit('close')">
    <div class="head">
      <h2 id="scan-h" class="section-lbl" style="margin:0;font-size:11px;">SCAN · {{ express ? 'EXPRESS: A SCAN CHECKS IN' : 'STANDARD: A SCAN OPENS THE CARD' }}</h2>
      <button type="button" class="btn-hud btn-hud-ghost close" aria-label="Close scanner" @click="stop(); emit('close')">
        <X style="width:22px;height:22px;" aria-hidden="true" />
      </button>
    </div>
    <div class="modes" role="radiogroup" aria-label="Scan mode" @keydown="onModeKey">
      <button
        v-for="(m, i) in (['STANDARD', 'EXPRESS'] as const)" :key="m" ref="radios" type="button" role="radio"
        :aria-checked="express === (i === 1)" :tabindex="express === (i === 1) ? 0 : -1"
        class="btn-hud big" :class="express === (i === 1) ? 'btn-hud-cta' : 'btn-hud-ghost'" @click="express = i === 1"
      >
        {{ m }}
      </button>
    </div>
    <div v-show="status !== 'nocamera'" class="frame">
      <video ref="video" playsinline muted autoplay aria-label="Camera view" />
      <p v-if="shownResult && status !== 'nocamera'" class="result" :class="shownResult.tone" data-testid="scan-result">{{ shownResult.text }}</p>
      <p class="hint" role="status">{{ status === 'starting' ? 'STARTING CAMERA…' : express ? 'EXPRESS · POINT AT THE NEXT CODE' : 'POINT AT THE QR CODE' }}</p>
    </div>
    <p v-if="cameraError" role="alert" class="err">{{ cameraError }}</p>
    <p v-if="status === 'nocamera' && shownResult" class="result inline" :class="shownResult.tone" data-testid="scan-result">{{ shownResult.text }}</p>
    <form class="typed" @submit.prevent="submitTyped">
      <label for="door-code" class="section-lbl">OR TYPE THE CODE</label>
      <div class="row">
        <input id="door-code" ref="codeInput" v-model="typed" class="hud-input" autocomplete="off" autocapitalize="characters" spellcheck="false" placeholder="TICKET CODE">
        <button type="submit" class="btn-hud btn-hud-cta go" :disabled="!typed.trim()">CHECK</button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.scanner {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px 16px calc(16px + env(safe-area-inset-bottom));
  background: var(--color-surface);
  overflow-y: auto;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.modes {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  width: 100%;
  max-width: 520px;
  margin: 0 auto;
}
.big {
  min-height: 56px;
  height: 56px;
  font-size: 11px;
}
.result {
  position: absolute;
  left: 8px;
  right: 8px;
  top: 8px;
  margin: 0;
  padding: 10px 12px;
  font-family: var(--font-command);
  font-size: 18px;
  font-weight: 700;
  text-align: center;
  color: var(--color-on-surface);
  background: rgba(0, 0, 0, .75);
  border: 2px solid var(--color-primary);
  overflow-wrap: anywhere;
}
.result.warn {
  border-color: var(--color-status-archived);
  color: var(--color-status-archived);
}
.result.inline {
  position: static;
}
.close {
  min-width: 56px;
  min-height: 56px;
  height: 56px;
}
.frame {
  position: relative;
  width: 100%;
  max-width: 520px;
  margin: 0 auto;
  aspect-ratio: 1;
  background: #000;
  border: 1px solid var(--color-primary-dim);
  overflow: hidden;
}
.frame video {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.hint {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 8px;
  margin: 0;
  text-align: center;
  font-family: var(--font-terminal);
  font-size: 11px;
  letter-spacing: .06em;
  color: var(--color-primary);
}
.err {
  margin: 0;
  font-size: 14px;
  color: var(--color-status-archived);
}
.typed {
  display: grid;
  gap: 6px;
  width: 100%;
  max-width: 520px;
  margin: 0 auto;
}
.row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
}
.row .hud-input {
  height: 56px;
  font-size: 18px;
}
.go {
  min-height: 56px;
  height: 56px;
  font-size: 12px;
}
</style>
