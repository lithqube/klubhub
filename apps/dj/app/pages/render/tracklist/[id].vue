<script setup lang="ts">
import { useAsyncData, useRoute, useHead } from '#app';
import TrackcardPreview from '../../../components/TrackcardPreview.vue';

const route = useRoute();

// Fetch tracklist and settings data
const { data: tracklistData } = await useAsyncData(() =>
  $fetch(`/api/v1/tracklists/${route.params.id}`),
);

const { data: settingsData } = await useAsyncData(() =>
  $fetch(`/api/v1/settings`),
);

// Disable layout for pure white-canvas page for Playwright screenshots
definePageMeta({ layout: false });

// Set title for not found case
useHead({
  title: !tracklistData.value ? 'Not Found' : undefined,
});
</script>

<template>
  <div
    v-if="tracklistData.value && settingsData.value"
    style="margin: 0; padding: 0; overflow: hidden; background-color: white"
  >
    <!-- Only render if we have data -->
    <TrackcardPreview
      :tracklist="tracklistData.value.tracklist"
      :tracks="tracklistData.value.tracks"
      :dj-name="settingsData.value.djName ?? 'KlubHub DJ'"
      :logo-path="settingsData.value.logoPath"
      :logo-position="
        settingsData.value.tracklist_preferences?.logoPosition ?? 'top-left'
      "
      :preset="settingsData.value.tracklist_preferences?.preset ?? 'default'"
      :bg-mode="settingsData.value.tracklist_preferences?.bgMode ?? 'solid'"
      :bg-value="settingsData.value.tracklist_preferences?.bgValue"
      :visible-fields="
        settingsData.value.tracklist_preferences?.visibleFields ?? [
          'title',
          'artist',
          'bpm',
          'key',
          'durationSecs',
        ]
      "
      :max-tracks="settingsData.value.tracklist_preferences?.maxTracks ?? 50"
      :track-range-start="
        settingsData.value.tracklist_preferences?.trackRangeStart
      "
      :track-range-end="settingsData.value.tracklist_preferences?.trackRangeEnd"
      :custom-placeholder-path="settingsData.value.custom_placeholder_path"
    />
  </div>

  <div
    v-else
    style="margin: 0; padding: 0; overflow: hidden; background-color: white"
  >
    <!-- Fallback for not found -->
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
