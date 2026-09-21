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

// Template note: refs are auto-unwrapped there, so the template must use
// `tracklistData`, not `tracklistData.value` — the latter reads a property
// named "value" on the payload (always undefined) and rendered "Not Found"
// for every tracklist. Settings arrive snake_case from the Go API.

// Disable layout for pure white-canvas page for Playwright screenshots
definePageMeta({ layout: false });

// Set title for not found case
useHead({
  title: !tracklistData?.value ? 'Not Found' : undefined,
});
</script>

<template>
  <div
    v-if="tracklistData && settingsData"
    style="margin: 0; padding: 0; overflow: hidden; background-color: white"
  >
    <!-- Only render if we have data -->
    <TrackcardPreview
      :tracklist="tracklistData.tracklist"
      :tracks="tracklistData.tracks"
      :dj-name="settingsData.dj_name || settingsData.djName || 'KlubHub DJ'"
      :logo-path="settingsData.logo_path || settingsData.logoPath"
      :logo-position="
        settingsData.tracklist_preferences?.logoPosition ?? 'top-left'
      "
      :preset="settingsData.tracklist_preferences?.preset ?? 'default'"
      :bg-mode="settingsData.tracklist_preferences?.bgMode ?? 'solid'"
      :bg-value="settingsData.tracklist_preferences?.bgValue"
      :visible-fields="
        settingsData.tracklist_preferences?.visibleFields ?? [
          'title',
          'artist',
          'bpm',
          'key',
          'durationSecs',
        ]
      "
      :max-tracks="settingsData.tracklist_preferences?.maxTracks ?? 50"
      :track-range-start="
        settingsData.tracklist_preferences?.trackRangeStart
      "
      :track-range-end="settingsData.tracklist_preferences?.trackRangeEnd"
      :custom-placeholder-path="settingsData.custom_placeholder_path"
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
