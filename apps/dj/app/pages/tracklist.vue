<script setup lang="ts">
import TrackcardPreviewPanel from '@/components/trackcard/TrackcardPreviewPanel.vue';
import TrackcardPreview from '@/components/TrackcardPreview.vue';

const settingsStore = useSettingsStore();
const tracklistStore = useTracklistStore();
const uiStore = useUiStore();

const { step } = storeToRefs(uiStore);
const { tracklist, tracks } = storeToRefs(tracklistStore);
const {
  djName,
  logoPath,
  logoPosition,
  preset,
  bgMode,
  bgValue,
  visibleFields,
  maxTracks,
  trackRangeStart,
  trackRangeEnd,
  customPlaceholderPath,
} = storeToRefs(settingsStore);

onMounted(async () => {
  await settingsStore.loadFromApi();
});
</script>

<template>
  <!-- min-h-dvh: avoids mobile-browser 100vh bug (UX law: viewport-units) -->
  <div class="min-h-dvh p-4 md:p-6 lg:p-8">
    <!-- Upload step -->
    <template v-if="step === 'upload'">
      <div class="max-w-2xl mx-auto">
        <TracklistUploadZone />
        <TracklistHistory class="mt-8" />
      </div>
    </template>

    <!-- Edit step: editor left, preview right (desktop) -->
    <template v-else>
      <div class="lg:grid lg:grid-cols-2 lg:gap-8">
        <!-- Left column: editor + customizer + exporter -->
        <div class="space-y-6">
          <TracklistEditor />
          <TracklistCustomizer />
          <TracklistExporter />
        </div>

        <!-- Desktop preview column — TrackcardPreviewPanel wraps TrackcardPreview -->
        <div class="hidden lg:block">
          <TrackcardPreviewPanel>
            <TrackcardPreview
              v-if="tracklist"
              :tracklist="tracklist"
              :tracks="tracks"
              :djName="djName"
              :logoPath="logoPath"
              :logoPosition="logoPosition"
              :preset="preset"
              :bgMode="bgMode"
              :bgValue="bgValue"
              :visibleFields="visibleFields"
              :maxTracks="maxTracks"
              :trackRangeStart="trackRangeStart"
              :trackRangeEnd="trackRangeEnd"
              :customPlaceholderPath="customPlaceholderPath"
            />
          </TrackcardPreviewPanel>
        </div>
      </div>
    </template>
  </div>
</template>
