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
  <!-- Display register for the names, terminal register for the date readout.
       Dim text uses an alpha hex, not opacity, so the signal colour stays full strength. -->
  <div
    :style="{
      marginBottom: '40px',
      paddingBottom: '28px',
      borderBottom: `2px dashed ${colors.text}33`,
      color: colors.text,
      fontFamily: 'var(--font-data)',
    }"
  >
    <h1
      :style="{
        margin: '0 0 14px 0',
        fontSize: '72px',
        lineHeight: 1,
        fontWeight: 700,
        letterSpacing: '-0.02em',
        textTransform: 'uppercase',
        fontFamily: 'var(--font-command)',
      }"
    >
      {{ djName }}
    </h1>
    <h2
      :style="{
        margin: '0',
        fontSize: '30px',
        lineHeight: 1.2,
        fontWeight: 500,
        letterSpacing: '-0.01em',
        textTransform: 'uppercase',
        color: `${colors.text}cc`,
        overflowWrap: 'anywhere',
        fontFamily: 'var(--font-command)',
      }"
    >
      {{ tracklist.title }}
    </h2>
    <p
      :style="{
        margin: '18px 0 0 0',
        fontSize: '22px',
        fontWeight: 600,
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
        color: colors.primary,
        fontFamily: 'var(--font-terminal)',
      }"
    >
      {{ new Date(tracklist.createdAt).toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' }).toUpperCase() }}
    </p>
  </div>
</template>
