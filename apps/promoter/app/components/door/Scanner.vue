<script setup lang="ts">
import { X } from 'lucide-vue-next'
import { decodeFrame, loadJsQR, nativeQrDetector, SCAN_INTERVAL_MS, scaledSize } from '~/utils/qrDecode'

/**
 * QR scanner: the back camera, decoded with BarcodeDetector where the
 * browser has it, else jsQR (iOS Safari), throttled to ~9 fps. A typed
 * code always works — for a broken camera, a denied permission, or a
 * printed code the camera cannot read.
 */
defineProps<{ express: boolean }>()
const emit = defineEmits<{ code: [value: string], close: [] }>()

const video = ref<HTMLVideoElement | null>(null)
const codeInput = ref<HTMLInputElement | null>(null)
const typed = ref('')
const status = ref<'starting' | 'scanning' | 'nocamera'>('starting')
const cameraError = ref('')
let stream: MediaStream | null = null
let raf = 0
let stopped = false

function stop() {
  stopped = true
  cancelAnimationFrame(raf)
  stream?.getTracks().forEach(t => t.stop())
  stream = null
}

function noCamera(msg: string) {
  status.value = 'nocamera'
  cameraError.value = msg
  nextTick(() => codeInput.value?.focus())
}

function found(value: string) {
  if (stopped) return
  stop()
  emit('code', value.trim())
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
  status.value = 'scanning'
  const detector = await nativeQrDetector()
  const jsqr = detector ? null : await loadJsQR()
  const canvas = document.createElement('canvas')
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  let last = 0
  let busy = false
  const loop = async (t: number) => {
    if (stopped) return
    raf = requestAnimationFrame(loop)
    const v = video.value
    if (busy || t - last < SCAN_INTERVAL_MS || !v || v.readyState < 2) return
    last = t
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
  if (typed.value.trim()) found(typed.value)
}

onMounted(() => void start())
onBeforeUnmount(stop)
</script>

<template>
  <div class="scanner" role="dialog" aria-modal="true" aria-labelledby="scan-h">
    <div class="head">
      <h2 id="scan-h" class="section-lbl" style="margin:0;font-size:11px;">SCAN · {{ express ? 'EXPRESS (SCAN CHECKS IN)' : 'STANDARD (SCAN OPENS THE CARD)' }}</h2>
      <button type="button" class="btn-hud btn-hud-ghost close" aria-label="Close scanner" @click="stop(); emit('close')">
        <X style="width:22px;height:22px;" aria-hidden="true" />
      </button>
    </div>
    <div v-show="status !== 'nocamera'" class="frame">
      <video ref="video" playsinline muted autoplay aria-label="Camera view" />
      <p class="hint" role="status">{{ status === 'starting' ? 'STARTING CAMERA…' : 'POINT AT THE QR CODE' }}</p>
    </div>
    <p v-if="cameraError" role="alert" class="err">{{ cameraError }}</p>
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
