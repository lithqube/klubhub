<script setup lang="ts">
import { onClickOutside } from '@vueuse/core'
import type { CurvePoint } from '~/types/report'
import { timeLabel } from '~/utils/datetime'
import {
  barPath, bucketAt, bucketLabel, bucketRange, curveGeometry, defaultLayout, formatCount, layoutHeight, peakIndex, peakLabelPos,
} from '~/utils/report'

/**
 * Check-in curve (P2.4) as inline SVG, no chart library. Two panels share
 * one time axis: people inside (line + wash) above, heads per 15 minutes
 * below (arrived above the baseline, left below). A crosshair readout
 * follows the pointer and the arrow keys (only keyboard moves are announced);
 * the same numbers are always in the data table (visually hidden until
 * shown, never hidden from screen readers).
 */
const props = defineProps<{ curve: CurvePoint[], tz: string }>()
const uid = useId()
const tableId = `${uid}-table`

const wrap = ref<HTMLElement | null>(null)
const width = ref(640)
let ro: ResizeObserver | null = null
onMounted(() => {
  if (!wrap.value) return
  width.value = wrap.value.clientWidth || width.value
  ro = new ResizeObserver(([entry]) => {
    if (entry) width.value = entry.contentRect.width
  })
  ro.observe(wrap.value)
})
onBeforeUnmount(() => ro?.disconnect())

const layout = computed(() => defaultLayout(width.value))
const height = computed(() => layoutHeight(layout.value))
const geo = computed(() => curveGeometry(props.curve, props.tz, layout.value))
const n = computed(() => props.curve.length)

const peak = computed(() => peakIndex(props.curve))

const summary = computed(() => {
  if (!n.value) return 'Check-in curve: no activity.'
  const first = props.curve[0]!
  const last = props.curve.at(-1)!
  const p = props.curve[peak.value]!
  const arrivals = props.curve.reduce((s, c) => s + c.in + c.walkups, 0)
  const exits = props.curve.reduce((s, c) => s + c.out, 0)
  return `Check-in curve in 15-minute steps from ${bucketLabel(first.bucket_start, props.tz)} to ${timeLabel(new Date(Date.parse(last.bucket_start) + 900_000).toISOString(), props.tz)}: `
    + `${arrivals} arrived, ${exits} left, peak of ${p.occupancy} inside at ${bucketRange(p.bucket_start, props.tz)}. The data table below has every step.`
})

// ---------------------------------------------------------------- crosshair
const active = ref<number | null>(null)
/** Screen-reader line; set by keyboard moves only, never by pointer moves. */
const announce = ref('')

function onPointer(e: PointerEvent) {
  const svg = e.currentTarget as SVGSVGElement
  const rect = svg.getBoundingClientRect()
  if (!rect.width) return
  active.value = bucketAt((e.clientX - rect.left) * (layout.value.width / rect.width), n.value, layout.value)
}

function onKey(e: KeyboardEvent) {
  if (!n.value) return
  const cur = active.value ?? peak.value
  const next = ({ ArrowLeft: cur - 1, ArrowRight: cur + 1, Home: 0, End: n.value - 1 } as Record<string, number>)[e.key]
  if (next === undefined) return
  e.preventDefault()
  active.value = Math.min(n.value - 1, Math.max(0, next))
  const c = props.curve[active.value]!
  announce.value = `${bucketRange(c.bucket_start, props.tz)}: ${c.occupancy} inside, ${c.in} checked in, ${c.walkups} walk-ups, ${c.out} left`
}

// Touch has no pointerleave after a tap: a mouse leaving or a tap elsewhere clears the readout.
function onLeave(e: PointerEvent) {
  if (e.pointerType === 'mouse') active.value = null
}
onClickOutside(wrap, () => {
  active.value = null
})

const readout = computed(() => {
  if (active.value === null) return null
  const c = props.curve[active.value]
  const point = geo.value.points[active.value]
  if (!c || !point) return null
  // The crosshair sits on the occupancy point (the value at the bucket's end);
  // the readout is anchored there, flipped left of it past the middle. The
  // SVG scales down with its box, so place it in rendered pixels.
  const scale = width.value && layout.value.width ? Math.min(1, width.value / layout.value.width) : 1
  const x = point.x * scale
  const w = layout.value.width * scale
  return {
    c, x: point.x, y: point.y, range: bucketRange(c.bucket_start, props.tz),
    style: x > w / 2 ? { right: `${Math.max(0, w - x + 10)}px` } : { left: `${x + 10}px` },
  }
})

const showTable = ref(false)
const peakPoint = computed(() => (peak.value >= 0 ? geo.value.points[peak.value] : null))
const peakLabel = computed(() => (peakPoint.value ? peakLabelPos(peakPoint.value, layout.value) : null))
</script>

<template>
  <section class="glass curve" aria-labelledby="curve-h">
    <div class="head">
      <h2 id="curve-h" class="section-lbl" style="margin:0;">CHECK-IN CURVE · 15 MIN</h2>
      <ul class="legend" aria-label="Legend">
        <li><svg width="14" height="8" aria-hidden="true"><line x1="0" y1="4" x2="14" y2="4" class="k-line" /></svg> INSIDE</li>
        <li><svg width="10" height="10" aria-hidden="true"><rect width="10" height="10" rx="2" class="k-in" /></svg> ARRIVED (CHECK-INS + WALK-UPS)</li>
        <li><svg width="10" height="10" aria-hidden="true"><rect width="10" height="10" rx="2" class="k-out" /></svg> LEFT</li>
      </ul>
    </div>

    <div ref="wrap" class="plot">
      <svg
        :viewBox="`0 0 ${layout.width} ${height}`" :width="layout.width" :height="height" role="img" :aria-label="summary"
        tabindex="0" class="svg" data-testid="checkin-curve"
        @pointermove="onPointer" @pointerdown="onPointer" @pointerleave="onLeave" @keydown="onKey"
        @focus="active = active ?? peak" @blur="active = null"
      >
        <!-- occupancy panel -->
        <g aria-hidden="true">
          <text :x="layout.left" :y="layout.occTop + 2" class="panel-lbl" dominant-baseline="hanging" dx="4">INSIDE</text>
          <g v-for="t in geo.occTicks" :key="`o${t.value}`">
            <line :x1="layout.left" :x2="layout.width - layout.right" :y1="t.y" :y2="t.y" class="grid" />
            <text :x="layout.left - 6" :y="t.y" class="tick" text-anchor="end" dominant-baseline="middle">{{ formatCount(t.value) }}</text>
          </g>
          <path :d="geo.area" class="area" />
          <path :d="geo.line" class="line" />
          <g v-if="peakPoint && peakLabel">
            <circle :cx="peakPoint.x" :cy="peakPoint.y" r="4" class="dot" />
            <text :x="peakLabel.x" :y="peakLabel.y" class="direct" :text-anchor="peakLabel.anchor" data-testid="curve-peak-label">{{ curve[peak]!.occupancy }}</text>
          </g>
        </g>

        <!-- flow panel -->
        <g aria-hidden="true">
          <text :x="layout.left" :y="layout.flowTop - 12" class="panel-lbl" dx="4">HEADS PER 15 MIN</text>
          <g v-for="t in geo.flowTicks" :key="`f${t.value}`">
            <line :x1="layout.left" :x2="layout.width - layout.right" :y1="t.y" :y2="t.y" :class="t.value === 0 ? 'base' : 'grid'" />
            <text :x="layout.left - 6" :y="t.y" class="tick" text-anchor="end" dominant-baseline="middle">{{ t.value < 0 ? `−${formatCount(-t.value)}` : formatCount(t.value) }}</text>
          </g>
          <g v-for="b in geo.bars" :key="b.i" :class="{ dim: active !== null && active !== b.i }">
            <path :d="barPath(b.x, b.upY, b.w, b.upH, 'up')" class="bar-in" />
            <path :d="barPath(b.x, b.downY, b.w, b.downH, 'down')" class="bar-out" />
          </g>
          <text v-for="x in geo.xLabels" :key="x.label" :x="x.x" :y="layout.flowTop + layout.flowHeight + 14" class="tick" text-anchor="middle">{{ x.label }}</text>
        </g>

        <!-- crosshair -->
        <g v-if="readout" aria-hidden="true">
          <line :x1="readout.x" :x2="readout.x" :y1="layout.occTop" :y2="layout.flowTop + layout.flowHeight" class="cross" data-testid="curve-crosshair" />
          <circle :cx="readout.x" :cy="readout.y" r="4" class="dot" data-testid="curve-crosshair-dot" />
        </g>
      </svg>

      <div v-if="readout" class="tip" :style="readout.style" aria-hidden="true">
        <div class="tip-time">{{ readout.range }}</div>
        <div class="tip-row"><svg width="12" height="4"><line x1="0" y1="2" x2="12" y2="2" class="k-line" /></svg><strong>{{ readout.c.occupancy }}</strong> inside</div>
        <div class="tip-row"><svg width="12" height="4"><line x1="0" y1="2" x2="12" y2="2" class="k-in-line" /></svg><strong>{{ readout.c.in + readout.c.walkups }}</strong> arrived</div>
        <div class="tip-sub muted">{{ readout.c.in }} checked in · {{ readout.c.walkups }} {{ readout.c.walkups === 1 ? 'walk-up' : 'walk-ups' }}</div>
        <div class="tip-row"><svg width="12" height="4"><line x1="0" y1="2" x2="12" y2="2" class="k-out-line" /></svg><strong>{{ readout.c.out }}</strong> left</div>
      </div>
      <p class="sr-only" aria-live="polite" data-testid="curve-announce">{{ announce }}</p>
    </div>

    <div class="foot">
      <p class="hint">Tap or drag on the chart, or use ← → when it's focused.</p>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm" style="min-height:44px;" :aria-expanded="showTable" :aria-controls="tableId" @click="showTable = !showTable">
        {{ showTable ? 'HIDE TABLE' : 'SHOW AS TABLE' }}
      </button>
    </div>
    <div :id="tableId" :class="showTable ? 'table-wrap' : 'sr-only'">
      <table class="report-table" data-testid="curve-table">
        <caption class="sr-only">Check-ins per 15 minutes, event time ({{ tz }})</caption>
        <thead>
          <tr>
            <th scope="col">TIME</th>
            <th scope="col" class="num">CHECKED IN</th>
            <th scope="col" class="num">WALK-UPS</th>
            <th scope="col" class="num">LEFT</th>
            <th scope="col" class="num">INSIDE</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in curve" :key="c.bucket_start">
            <th scope="row" style="font-weight:400;font-size:12px;color:var(--color-on-surface);">{{ bucketRange(c.bucket_start, tz) }}</th>
            <td class="num">{{ c.in }}</td>
            <td class="num">{{ c.walkups }}</td>
            <td class="num">{{ c.out }}</td>
            <td class="num">{{ c.occupancy }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.curve {
  padding: 12px;
  display: grid;
  gap: 8px;
  min-width: 0;
}
.head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 6px 12px;
}
.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-family: var(--font-terminal);
  font-size: 10px;
  letter-spacing: .06em;
  color: var(--color-on-surface-variant);
}
.legend li { display: inline-flex; align-items: center; gap: 5px; }
.plot { position: relative; min-width: 0; overflow: hidden; }
.svg { display: block; max-width: 100%; height: auto; touch-action: pan-y; }
.svg:focus-visible { outline: 2px solid var(--color-primary); outline-offset: 3px; }
.grid { stroke: var(--color-outline-variant); stroke-width: 1; }
.base { stroke: var(--color-outline); stroke-width: 1; }
.tick {
  font-family: var(--font-terminal);
  font-size: 10px;
  fill: var(--color-tertiary);
  font-variant-numeric: tabular-nums;
}
.panel-lbl {
  font-family: var(--font-terminal);
  font-size: 10px;
  letter-spacing: .06em;
  fill: var(--color-tertiary);
}
.direct {
  font-family: var(--font-data);
  font-size: 11px;
  font-weight: 600;
  fill: var(--color-on-surface);
  /* a surface halo keeps the label legible where it sits beside the line */
  paint-order: stroke;
  stroke: var(--color-surface);
  stroke-width: 3px;
  stroke-linejoin: round;
}
.line, .k-line { fill: none; stroke: var(--color-primary); stroke-width: 2; stroke-linejoin: round; stroke-linecap: round; }
.area { fill: var(--color-primary); opacity: .1; }
.dot { fill: var(--color-primary); stroke: var(--color-surface); stroke-width: 2; }
.bar-in, .k-in { fill: var(--color-primary); }
.bar-out, .k-out { fill: var(--color-secondary); }
.k-in-line { stroke: var(--color-primary); stroke-width: 2; }
.k-out-line { stroke: var(--color-secondary); stroke-width: 2; }
.dim { opacity: .45; }
.cross { stroke: var(--color-on-surface-variant); stroke-width: 1; }
.tip {
  position: absolute;
  top: 8px;
  pointer-events: none;
  padding: 8px 10px;
  display: grid;
  gap: 3px;
  background: var(--color-surface-container-high);
  border: 1px solid var(--color-outline);
  font-family: var(--font-data);
  font-size: 12px;
  color: var(--color-on-surface-variant);
  max-width: calc(100% - 16px);
  z-index: 1;
}
.tip strong { color: var(--color-on-surface); font-variant-numeric: tabular-nums; margin-right: 3px; }
.tip-time { font-family: var(--font-terminal); font-size: 10px; letter-spacing: .06em; color: var(--color-tertiary); }
.tip-row { display: flex; align-items: center; gap: 6px; }
.muted { font-size: 11px; }
.tip-sub { padding-left: 18px; margin-top: -2px; }
.foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}
.hint { margin: 0; font-size: 11px; color: var(--color-on-surface-variant); }
.table-wrap { max-height: 360px; overflow: auto; }
@media (forced-colors: active) {
  .line, .k-line, .k-in-line, .k-out-line { stroke: CanvasText; }
  .bar-in, .k-in { fill: CanvasText; }
  .bar-out, .k-out { fill: GrayText; }
}
</style>
