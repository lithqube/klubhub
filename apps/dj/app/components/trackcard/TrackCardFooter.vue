<script setup lang="ts">
import type { Tracklist } from '../../types/tracklist';
import type { PresetColors } from '@/utils/design-tokens';

defineProps({
  tracklist: {
    type: Object as () => Tracklist,
    required: true,
  },
  colors: {
    type: Object as () => PresetColors,
    required: true,
  },
});
</script>

<template>
  <div
    :style="{
      flexShrink: 0,
      textAlign: 'center',
      paddingTop: '20px',
      borderTop: `2px dashed ${colors.text}33`,
      fontSize: '18px',
      fontWeight: 600,
      letterSpacing: '0.12em',
      textTransform: 'uppercase',
      color: `${colors.text}99`,
      fontFamily: 'var(--font-terminal)',
    }"
  >
    Made with <span :style="{ color: colors.primary, fontWeight: 700 }">klubhub.io</span>
  </div>
</template>
