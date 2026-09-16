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
    backgroundColor: props.colors.accent,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  };
};
</script>

<template>
  <div
    :style="{
      display: 'flex',
      alignItems: 'center',
      padding: '12px 0',
      borderBottom: `1px solid ${colors.text}1a`,
    }"
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
          marginRight: '16px',
          flexShrink: 0,
          position: 'relative',
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
            :style="{
              fontWeight: 600,
              fontSize: '20px',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              color: colors.text,
            }"
          >
            {{ track.title || 'Unknown Title' }}
          </div>
          <div :style="{ fontSize: '16px', opacity: 0.8, color: colors.text }">
            {{ track.artist || 'Unknown Artist' }}
          </div>
        </div>

        <!-- Conditional fields based on visibleFields -->
        <div
          v-if="visibleFields.includes('bpm') && track.bpm"
          :style="{ textAlign: 'right', fontSize: '18px', minWidth: '60px', color: colors.text }"
        >
          {{ track.bpm }} BPM
        </div>
        <div
          v-else-if="visibleFields.includes('bpm')"
          :style="{ textAlign: 'right', fontSize: '18px', minWidth: '60px', opacity: 0.5, color: colors.text }"
        >
          — BPM
        </div>

        <div
          v-if="visibleFields.includes('musical_key') && track.musicalKey"
          :style="{ textAlign: 'right', fontSize: '18px', minWidth: '60px', marginLeft: '12px', color: colors.text }"
        >
          {{ track.musicalKey }}
        </div>
        <div
          v-else-if="visibleFields.includes('musical_key')"
          :style="{ textAlign: 'right', fontSize: '18px', minWidth: '60px', marginLeft: '12px', opacity: 0.5, color: colors.text }"
        >
          — Key
        </div>
      </div>
    </div>
  </div>
</template>
