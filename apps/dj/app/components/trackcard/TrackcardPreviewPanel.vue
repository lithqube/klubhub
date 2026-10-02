<script setup lang="ts">
// Layout wrapper with a slot. The slotted card is a fixed 1080x1920 canvas (the
// export size), so the panel scales it down to the width it is given instead of
// letting it overflow and clip.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';

const CARD_W = 1080;
const CARD_H = 1920;

const stage = ref<HTMLElement | null>(null);
const stageWidth = ref(0);
let observer: ResizeObserver | null = null;

onMounted(() => {
  if (!stage.value) return;
  stageWidth.value = stage.value.clientWidth;
  if (typeof ResizeObserver === 'undefined') return;
  observer = new ResizeObserver((entries) => {
    stageWidth.value = entries[0]?.contentRect.width ?? stageWidth.value;
  });
  observer.observe(stage.value);
});
onBeforeUnmount(() => observer?.disconnect());

// Never upscale past the real size; before the first measurement use a small
// safe scale so the card is never painted at full size and then jumps.
const scale = computed(() => (stageWidth.value > 0 ? Math.min(1, stageWidth.value / CARD_W) : 0.25));
const percent = computed(() => Math.round(scale.value * 100));
</script>

<template>
  <!-- Panel background: bg-surface-container (opaque, NOT glass per mockup spec) -->
  <div class="bg-surface-container w-full">
    <!-- Panel header: label left + live render indicator right -->
    <div class="flex justify-between items-center px-4 py-2 border-b border-dashed border-outline-variant/40">
      <span class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
        REALTIME_OUTPUT_PREVIEW
      </span>
      <span class="flex items-center gap-1.5">
        <span class="pulse-dot" aria-hidden="true" />
        <span class="font-terminal tracking-terminal text-primary text-xs uppercase">
          LIVE_RENDER
        </span>
      </span>
    </div>
    <!-- Preview stage: the card is scaled to the stage width; the sized wrapper keeps the
         scaled height in the layout (a CSS transform alone would leave 1920px of dead space). -->
    <div class="p-4">
      <div ref="stage" class="w-full">
        <div
          :style="{ width: `${CARD_W * scale}px`, height: `${CARD_H * scale}px` }"
          class="outline outline-1 outline-dashed outline-outline-variant/40"
          data-testid="trackcard-preview-stage"
        >
          <div
            :style="{
              width: `${CARD_W}px`,
              height: `${CARD_H}px`,
              transform: `scale(${scale})`,
              transformOrigin: 'top left',
            }"
          >
            <slot />
          </div>
        </div>
      </div>
      <!-- Readout: export size and the current preview scale -->
      <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase mt-3">
        {{ CARD_W }} × {{ CARD_H }} PX · 9:16 · {{ percent }}%
      </p>
    </div>
  </div>
</template>
