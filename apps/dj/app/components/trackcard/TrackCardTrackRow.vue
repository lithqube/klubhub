<script setup lang="ts">
import type { Track } from '../../types/tracklist';
import type { PresetColors } from '@/utils/design-tokens';

const props = defineProps({
  track: {
    type: Object as () => Track,
    required: true,
  },
  index: {
    type: Number,
    required: true,
  },
  visibleFields: {
    type: Array as () => string[],
    required: true,
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

const getArtworkStyle = () => {
  if (props.track.artworkUrl) {
    return {
      backgroundImage: `url(${props.track.artworkUrl})`,
      backgroundSize: 'cover',
      backgroundPosition: 'center',
    };
  }

  if (props.customPlaceholderPath) {
    return {
      backgroundImage: `url(${props.customPlaceholderPath})`,
      backgroundSize: 'cover',
      backgroundPosition: 'center',
    };
  }

  return {
    backgroundColor: '#333333',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  };
};
</script>

<template>
  <div
    style="
      display: flex;
      align-items: center;
      padding: 12px 0;
      border-bottom: 1px solid rgba(255, 255, 255, 0.1);
    "
  >
    <!-- Position -->
    <div
      style="
        width: 60px;
        text-align: center;
        font-size: 24px;
        flex-shrink: 0;
      "
    >
      {{ index + 1 }}
    </div>

    <!-- Cover Art -->
    <div
      :style="[
        {
          width: '40px',
          height: '40px',
          borderRadius: '4px',
          marginRight: '16px',
          flexShrink: 0,
        },
        getArtworkStyle(),
      ]"
    >
      <template v-if="track.artworkStatus === 'manual'">
        <div
          style="
            position: absolute;
            width: 100%;
            height: 100%;
            display: flex;
            align-items: center;
            justify-content: center;
            pointer-events: none;
          "
        >
          <span
            style="
              background: rgba(0, 0, 0, 0.5);
              color: white;
              font-size: 10px;
              padding: 2px 4px;
              border-radius: 2px;
            "
            >LOCK</span
          >
        </div>
      </template>
    </div>

    <!-- Track info -->
    <div style="flex: 1; min-width: 0">
      <div
        style="
          display: flex;
          justify-content: space-between;
          margin-bottom: 4px;
        "
      >
        <div style="flex: 1">
          <div
            style="
              font-weight: 600;
              font-size: 20px;
              white-space: nowrap;
              overflow: hidden;
              text-overflow: ellipsis;
            "
          >
            {{ track.title || 'Unknown Title' }}
          </div>
          <div style="font-size: 16px; opacity: 0.8">
            {{ track.artist || 'Unknown Artist' }}
          </div>
        </div>

        <!-- Conditional fields based on visibleFields -->
        <div
          v-if="visibleFields.includes('bpm') && track.bpm"
          style="text-align: right; font-size: 18px; min-width: 60px"
        >
          {{ track.bpm }} BPM
        </div>
        <div
          v-else-if="visibleFields.includes('bpm')"
          style="
            text-align: right;
            font-size: 18px;
            min-width: 60px;
            opacity: 0.5;
          "
        >
          — BPM
        </div>

        <div
          v-if="visibleFields.includes('musical_key') && track.musicalKey"
          style="
            text-align: right;
            font-size: 18px;
            min-width: 60px;
            margin-left: 12px;
          "
        >
          {{ track.musicalKey }}
        </div>
        <div
          v-else-if="visibleFields.includes('musical_key')"
          style="
            text-align: right;
            font-size: 18px;
            min-width: 60px;
            margin-left: 12px;
            opacity: 0.5;
          "
        >
          — Key
        </div>
      </div>
    </div>
  </div>
</template>
