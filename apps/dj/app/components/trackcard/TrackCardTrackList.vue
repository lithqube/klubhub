<script setup lang="ts">
import { computed } from 'vue';
import type { Track } from '../../types/tracklist';
import type { PresetColors } from '@/utils/design-tokens';
import TrackCardTrackRow from './TrackCardTrackRow.vue';

const props = defineProps({
  tracks: {
    type: Array as () => Track[],
    required: true,
  },
  trackRangeStart: {
    type: [Number, null] as unknown as () => number | null,
    default: null,
  },
  trackRangeEnd: {
    type: [Number, null] as unknown as () => number | null,
    default: null,
  },
  maxTracks: {
    type: Number,
    default: 50,
  },
  visibleFields: {
    type: Array as () => string[],
    default: () => ['title', 'artist', 'bpm', 'key', 'durationSecs'],
  },
  customPlaceholderPath: {
    type: [String, null] as unknown as () => string | null,
    default: null,
  },
  colors: {
    type: Object as () => PresetColors,
    required: true,
  },
});

const visibleTracks = computed(() => {
  let tracksToShow = [...props.tracks];

  if (props.trackRangeStart !== null && props.trackRangeEnd !== null) {
    tracksToShow = tracksToShow.slice(
      props.trackRangeStart,
      props.trackRangeEnd + 1,
    );
  }

  if (props.maxTracks > 0) {
    tracksToShow = tracksToShow.slice(0, props.maxTracks);
  }

  return tracksToShow;
});
</script>

<template>
  <div
    style="
      flex: 1;
      min-height: 200px;
      margin-bottom: 40px;
      overflow-y: auto;
    "
  >
    <TrackCardTrackRow
      v-for="(track, index) in visibleTracks"
      :key="track.id"
      :track="track"
      :index="index"
      :visible-fields="visibleFields"
      :custom-placeholder-path="customPlaceholderPath"
      :colors="colors"
    />
  </div>
</template>
