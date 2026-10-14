<script setup lang="ts">
import { computed } from 'vue';
import type { Track } from '../../types/tracklist';
import type { PresetColors } from '@/utils/design-tokens';
import TrackCardTrackRow, { type RowDensity } from './TrackCardTrackRow.vue';

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

// The list has about 1500px between header and footer (measured). A comfortable
// row is 83px, compact 66, dense 50. Each tier stops at ~92% of that space, so a
// long set name that wraps the header onto a second line still fits. Past 28
// tracks the rows go into two columns that read down the left side, then down
// the right (room for about 60).
const density = computed<RowDensity>(() => {
  const n = visibleTracks.value.length;
  if (n <= 17) return 'comfortable';
  if (n <= 21) return 'compact';
  if (n <= 28) return 'dense';
  return 'dense-2col';
});
const twoColumns = computed(() => density.value === 'dense-2col');
</script>

<template>
  <div
    :data-density="density"
    :style="{
      flex: 1,
      minHeight: 0,
      marginBottom: '28px',
      overflow: 'hidden',
      ...(twoColumns
        ? {
            display: 'grid',
            gridAutoFlow: 'column',
            gridTemplateColumns: '1fr 1fr',
            gridTemplateRows: `repeat(${Math.ceil(visibleTracks.length / 2)}, auto)`,
            columnGap: '40px',
            alignContent: 'start',
          }
        : {}),
    }"
  >
    <!-- The list owns the space between header and footer and clips at a row edge
         rather than ever drawing under the footer. -->
    <TrackCardTrackRow
      v-for="(track, index) in visibleTracks"
      :key="track.id"
      :track="track"
      :index="index"
      :density="density"
      :visible-fields="visibleFields"
      :custom-placeholder-path="customPlaceholderPath"
      :colors="colors"
    />
  </div>
</template>
