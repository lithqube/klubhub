<script setup lang="ts">
import type { Tracklist } from '../../types/tracklist';

defineProps({
  djName: {
    type: String,
    required: true,
  },
  tracklist: {
    type: Object as () => Tracklist,
    required: true,
  },
});
</script>

<template>
  <div style="margin-bottom: 40px">
    <h1 style="margin: 0 0 10px 0; font-size: 48px">{{ djName }}</h1>
    <h2 style="margin: 0; font-size: 36px; opacity: 0.9">
      {{ tracklist.title }}
    </h2>
    <p style="margin: 10px 0 0 0; font-size: 24px; opacity: 0.7">
      {{ new Date(tracklist.createdAt).toLocaleDateString() }}
    </p>
  </div>
</template>
