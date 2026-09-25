<script setup lang="ts">
import TrackcardPreviewPanel from '@/components/trackcard/TrackcardPreviewPanel.vue'
import TrackcardPreview from '@/components/TrackcardPreview.vue'

useHead({ title: 'Tracklist — KlubHub DJ' })

const settingsStore = useSettingsStore()
const tracklistStore = useTracklistStore()
const uiStore = useUiStore()

const { step } = storeToRefs(uiStore)
const { tracklist, tracks } = storeToRefs(tracklistStore)
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
} = storeToRefs(settingsStore)

onMounted(async () => {
  await settingsStore.loadFromApi()
})
</script>

<template>
  <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">

    <!-- Page header -->
    <div class="page-header">
      <div>
        <div class="page-title">TRACKLIST</div>
        <div class="page-sub">UPLOAD · PARSE · EXPORT</div>
      </div>
    </div>

    <!-- Upload step -->
    <template v-if="step === 'upload'">
      <div class="page-body" style="max-width:660px;">
        <TracklistUploadZone />
        <TracklistHistory />
      </div>
    </template>

    <!-- Edit step: editor left, preview right (desktop) -->
    <template v-else>
      <div class="page-body" style="flex-direction:row;gap:0;overflow:hidden;padding:0;">
        <!-- Left: editor -->
        <div style="flex:1;overflow-y:auto;padding:16px 20px;display:flex;flex-direction:column;gap:14px;">
          <TracklistEditor />
          <TracklistCustomizer />
          <TracklistExporter />
        </div>

        <!-- Right: desktop preview panel -->
        <div class="hidden lg:flex" style="width:380px;flex-shrink:0;border-left:1px solid color-mix(in srgb, var(--color-primary) 8%, transparent);overflow-y:auto;">
          <TrackcardPreviewPanel>
            <TrackcardPreview
              v-if="tracklist"
              :tracklist="tracklist"
              :tracks="tracks"
              :dj-name="djName"
              :logo-path="logoPath"
              :logo-position="logoPosition"
              :preset="preset"
              :bg-mode="bgMode"
              :bg-value="bgValue"
              :visible-fields="visibleFields"
              :max-tracks="maxTracks"
              :track-range-start="trackRangeStart"
              :track-range-end="trackRangeEnd"
              :custom-placeholder-path="customPlaceholderPath"
            />
          </TrackcardPreviewPanel>
        </div>
      </div>
    </template>

  </div>
</template>
