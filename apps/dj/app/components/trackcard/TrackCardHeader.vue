<script setup lang="ts">
import type { Tracklist } from '../../types/tracklist';
import type { PresetColors } from '@/utils/design-tokens';

defineProps({
  djName: {
    type: String,
    required: true,
  },
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
  <div :style="{ marginBottom: '40px', color: colors.text, fontFamily: 'var(--font-data)' }">
    <h1 :style="{ margin: '0 0 10px 0', fontSize: '48px', fontFamily: 'var(--font-command)' }">{{ djName }}</h1>
    <h2 :style="{ margin: '0', fontSize: '36px', opacity: 0.9, fontFamily: 'var(--font-command)' }">
      {{ tracklist.title }}
    </h2>
    <p :style="{ margin: '10px 0 0 0', fontSize: '24px', opacity: 0.7 }">
      {{ new Date(tracklist.createdAt).toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' }).toUpperCase() }}
    </p>
  </div>
</template>
