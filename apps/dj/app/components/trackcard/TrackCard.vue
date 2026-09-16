<script setup lang="ts">
import { computed } from 'vue';
import type { Track } from '../../types/tracklist';
import type { PresetColors } from '@/utils/design-tokens';

const props = defineProps({
  colors: {
    type: Object as () => PresetColors,
    required: true,
  },
  bgMode: {
    type: String as () => 'solid' | 'upload' | 'mosaic',
    default: 'solid',
  },
  bgValue: {
    type: [String, null] as unknown as () => string | null,
    default: null,
  },
  tracks: {
    type: Array as () => Track[],
    default: () => [],
  },
});

const backgroundStyle = computed(() => {
  const style: Record<string, string> = { position: 'absolute', inset: '0' };

  switch (props.bgMode) {
    case 'solid':
      style.backgroundColor = props.bgValue || props.colors.bg;
      break;
    case 'upload':
      if (props.bgValue) {
        style.backgroundImage = `url(${props.bgValue})`;
        style.backgroundSize = 'cover';
        style.backgroundPosition = 'center';
      } else {
        style.backgroundColor = props.colors.bg;
      }
      break;
    case 'mosaic':
      style.display = 'grid';
      style.gridTemplateColumns = 'repeat(3, 1fr)';
      style.gridTemplateRows = 'repeat(3, 1fr)';
      style.gap = '0';
      break;
  }

  return style;
});
</script>

<template>
  <div
    id="tracklist-card"
    :style="{
      width: '1080px',
      height: '1920px',
      overflow: 'hidden',
      position: 'relative',
    }"
  >
    <!-- Background layer -->
    <div :style="backgroundStyle" />

    <!-- Slot for logo, header, tracklist, footer -->
    <div
      :style="{
        position: 'relative',
        width: '100%',
        height: '100%',
        padding: '40px',
        boxSizing: 'border-box',
        color: colors.text,
        fontFamily: 'var(--font-data)',
      }"
    >
      <slot />
    </div>
  </div>
</template>
