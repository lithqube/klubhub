<script setup lang="ts">
import { useAsyncData } from '#imports';
import TrackcardPreview from '@/app/components/TrackcardPreview.vue';

// Disable layout for pure white-canvas page for Playwright screenshots
definePageMeta({ layout: false });

// Fetch tracklist and settings data
const { data: tracklistData } = await useAsyncData(() =>
  $fetch(`/api/v1/tracklists/${route.params.id}`),
);

const { data: settingsData } = await useAsyncData(() =>
  $fetch(`/api/v1/settings`),
);

// Handle not found case
if (!tracklistData.value) {
  // Render plain text for Playwright to get a 200
  definePageMeta({
    head: {
      title: 'Not Found',
    },
  });
}
</script>

<template>
  <!-- Only render if we have data -->
  <div
    v-if="tracklistData.value && settingsData.value"
    style="margin: 0; padding: 0; overflow: hidden; background-color: white"
  >
    <TrackcardPreview
      :tracklist="tracklistData.value.tracklist"
      :tracks="tracklistData.value.tracks"
      :djName="settingsData.value.djName ?? 'KlubHub DJ'"
      :logoPath="settingsData.value.logoPath"
      :logoPosition="
        settingsData.value.tracklist_preferences?.logoPosition ?? 'top-left'
      "
      :preset="settingsData.value.tracklist_preferences?.preset ?? 'default'"
      :bgMode="settingsData.value.tracklist_preferences?.bgMode ?? 'solid'"
      :bgValue="settingsData.value.tracklist_preferences?.bgValue"
      :visibleFields="
        settingsData.value.tracklist_preferences?.visibleFields ?? [
          'title',
          'artist',
          'bpm',
          'key',
          'durationSecs',
        ]
      "
      :maxTracks="settingsData.value.tracklist_preferences?.maxTracks ?? 50"
      :trackRangeStart="
        settingsData.value.tracklist_preferences?.trackRangeStart
      "
      :trackRangeEnd="settingsData.value.tracklist_preferences?.trackRangeEnd"
      :customPlaceholderPath="settingsData.value.custom_placeholder_path"
    />
  </div>

  <!-- Fallback for not found -->
  <div
    v-else
    style="margin: 0; padding: 0; overflow: hidden; background-color: white"
  >
    Not Found
  </div>
</template>

<style scoped>
/* Ensure no margin/padding/scrollbar as specified */
body {
  margin: 0;
  overflow: hidden;
}

/* Ensure the container takes full viewport */
html,
body,
#__layout {
  height: 100%;
}
</style>
