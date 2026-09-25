<script setup lang="ts">
import type { ExportData } from '~/types/event'
import { dayLabel, rangeLabel } from '~/utils/datetime'
import { accentOf } from '~/utils/exportPack'

const props = defineProps<{ data: ExportData }>()
const emit = defineEmits<{ download: [] }>()
const canvas = ref<HTMLCanvasElement | null>(null)
const W = 1200
const H = 630

function wrap(ctx: CanvasRenderingContext2D, text: string, max: number): string[] {
  const out: string[] = []
  let line = ''
  for (const word of text.split(/\s+/)) {
    const next = line ? `${line} ${word}` : word
    if (ctx.measureText(next).width > max && line) {
      out.push(line)
      line = word
    } else line = next
  }
  if (line) out.push(line)
  return out
}

async function draw() {
  const c = canvas.value
  const ctx = c?.getContext('2d')
  if (!c || !ctx) return
  await document.fonts?.ready
  const e = props.data.event
  const accent = accentOf(props.data)
  ctx.fillStyle = '#0e0e0f'
  ctx.fillRect(0, 0, W, H)
  // Scan lines + accent bar: Kinetic HUD, independent of the viewer's theme.
  ctx.fillStyle = 'rgba(150,248,255,0.04)'
  for (let y = 0; y < H; y += 4) ctx.fillRect(0, y, W, 1)
  ctx.fillStyle = accent
  ctx.fillRect(0, 0, 12, H)
  ctx.textBaseline = 'top'
  ctx.fillStyle = accent
  ctx.font = '600 26px Inter, system-ui, sans-serif'
  ctx.fillText(`${props.data.organizer.toUpperCase()} PRESENTS`, 72, 64)
  ctx.fillStyle = '#e0e0e2'
  ctx.font = '700 84px "Space Grotesk", system-ui, sans-serif'
  const title = wrap(ctx, e.title.toUpperCase(), W - 144).slice(0, 2)
  title.forEach((t, i) => ctx.fillText(t, 72, 110 + i * 92))
  let y = 110 + title.length * 92 + 20
  ctx.fillStyle = '#c3a9ff'
  ctx.font = '600 32px Inter, system-ui, sans-serif'
  ctx.fillText(`${dayLabel(e.starts_at, e.timezone)} · ${rangeLabel(e.starts_at, e.ends_at, e.timezone)} · ${props.data.location.name}`.toUpperCase(), 72, y)
  y += 64
  ctx.fillStyle = '#e0e0e2'
  ctx.font = '500 30px Inter, system-ui, sans-serif'
  const acts = props.data.lineup.map(l => l.display_name.toUpperCase()).join(' · ')
  wrap(ctx, acts, W - 144).slice(0, Math.max(1, Math.floor((H - 60 - y) / 42))).forEach((t, i) => ctx.fillText(t, 72, y + i * 42))
}

function toBlob(): Promise<Blob | null> {
  return new Promise(resolve => (canvas.value ? canvas.value.toBlob(resolve, 'image/png') : resolve(null)))
}
defineExpose({ toBlob })
onMounted(draw)
watch(() => props.data, draw)
</script>

<template>
  <section class="hud-card" style="padding:12px 14px;min-width:0;">
    <div style="display:flex;align-items:center;justify-content:space-between;gap:8px;">
      <h3 class="section-lbl" style="margin:0;">OG IMAGE 1200×630</h3>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" @click="emit('download')">DOWNLOAD PNG</button>
    </div>
    <canvas
      ref="canvas" :width="W" :height="H" role="img"
      :aria-label="`Share image: ${data.event.title}, ${data.location.name}`"
      style="display:block;width:100%;height:auto;aspect-ratio:1200/630;margin-top:8px;border:1px solid var(--color-outline-variant);"
    />
  </section>
</template>
