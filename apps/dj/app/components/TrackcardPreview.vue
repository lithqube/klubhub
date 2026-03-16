<script setup lang="ts">
import { computed } from 'vue';
import { defineProps } from 'vue';
import type { Tracklist, Track } from '../../types/tracklist';

const props = defineProps({
  tracklist: {
    type: Object as () => Tracklist,
    required: true,
  },
  tracks: {
    type: Array as () => Track[],
    required: true,
  },
  djName: {
    type: String,
    required: true,
  },
  logoPath: {
    type: [String, null],
    default: null,
  },
  logoPosition: {
    type: String as () =>
      | 'top-left'
      | 'top-right'
      | 'bottom-left'
      | 'bottom-right',
    default: 'top-left',
  },
  preset: {
    type: String as () => 'default' | 'dark' | 'light' | 'neon' | 'minimal',
    default: 'default',
  },
  bgMode: {
    type: String as () => 'solid' | 'upload' | 'mosaic',
    default: 'solid',
  },
  bgValue: {
    type: [String, null],
    default: null,
  },
  visibleFields: {
    type: Array as () => string[],
    default: () => ['title', 'artist', 'bpm', 'key', 'durationSecs'],
  },
  maxTracks: {
    type: Number,
    default: 50,
  },
  trackRangeStart: {
    type: [Number, null],
    default: null,
  },
  trackRangeEnd: {
    type: [Number, null],
    default: null,
  },
  customPlaceholderPath: {
    type: [String, null],
    default: null,
  },
});

// Preset color maps
const presetColors = {
  default: {
    bg: '#1a1a2e',
    primary: '#e94560',
    accent: '#0f3460',
    text: '#ffffff',
  },
  dark: {
    bg: '#0a0a0a',
    primary: '#ffffff',
    accent: '#333333',
    text: '#cccccc',
  },
  light: {
    bg: '#f5f5f5',
    primary: '#1a1a2e',
    accent: '#e0e0e0',
    text: '#333333',
  },
  neon: {
    bg: '#0d0d0d',
    primary: '#00ff88',
    accent: '#ff0080',
    text: '#ffffff',
  },
  minimal: {
    bg: '#ffffff',
    primary: '#000000',
    accent: '#888888',
    text: '#000000',
  },
};

// Get colors based on preset
const colors = computed(() => presetColors[props.preset]);

// Background rendering logic
const getBackgroundStyle = computed(() => {
  const style: Record<string, any> = { position: 'absolute', inset: 0 };

  switch (props.bgMode) {
    case 'solid':
      style.backgroundColor = props.bgValue || colors.value.bg;
      break;
    case 'upload':
      if (props.bgValue) {
        style.backgroundImage = `url(${props.bgValue})`;
        style.backgroundSize = 'cover';
        style.backgroundPosition = 'center';
      } else {
        style.backgroundColor = colors.value.bg;
      }
      break;
    case 'mosaic':
      // For mosaic, we'll create a CSS grid of artwork thumbnails
      style.display = 'grid';
      style.gridTemplateColumns = 'repeat(3, 1fr)';
      style.gridTemplateRows = 'repeat(3, 1fr)';
      style.gap = '0';
      break;
  }

  return style;
});

// Logo positioning
const getLogoStyle = computed(() => {
  if (!props.logoPath) return { display: 'none' };

  const style: Record<string, any> = {
    position: 'absolute',
    width: '120px',
    height: 'auto',
    objectFit: 'contain',
  };

  switch (props.logoPosition) {
    case 'top-left':
      style.top = '20px';
      style.left = '20px';
      break;
    case 'top-right':
      style.top = '20px';
      style.right = '20px';
      break;
    case 'bottom-left':
      style.bottom = '20px';
      style.left = '20px';
      break;
    case 'bottom-right':
      style.bottom = '20px';
      style.right = '20px';
      break;
  }

  return style;
});

// Track range slicing
const slicedTracks = computed(() => {
  let tracksToShow = [...props.tracks];

  // Apply track range if specified
  if (props.trackRangeStart !== null && props.trackRangeEnd !== null) {
    tracksToShow = tracksToShow.slice(
      props.trackRangeStart,
      props.trackRangeEnd + 1,
    );
  }

  // Apply max tracks limit
  if (props.maxTracks > 0) {
    tracksToShow = tracksToShow.slice(0, props.maxTracks);
  }

  return tracksToShow;
});

// Artwork placeholder logic
const getArtworkStyle = (track: Track) => {
  if (track.artworkUrl) {
    return {
      backgroundImage: `url(${track.artworkUrl})`,
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

  // Default placeholder
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
    id="tracklist-card"
    :style="{
      width: '1080px',
      height: '1920px',
      overflow: 'hidden',
      position: 'relative',
    }"
  >
    <!-- Background layer -->
    <div :style="getBackgroundStyle" />

    <!-- Logo -->
    <img
      v-if="props.logoPath"
      :src="props.logoPath"
      :style="getLogoStyle"
      alt="Logo"
    />

    <!-- Content container -->
    <div
      style="
        position: relative;
        width: 100%;
        height: 100%;
        padding: 40px;
        box-sizing: border-box;
        color: #ffffff;
      "
    >
      <!-- Header section -->
      <div style="margin-bottom: 40px">
        <h1 style="margin: 0 0 10px 0; font-size: 48px">{{ props.djName }}</h1>
        <h2 style="margin: 0; font-size: 36px; opacity: 0.9">
          {{ props.tracklist.title }}
        </h2>
        <p style="margin: 10px 0 0 0; font-size: 24px; opacity: 0.7">
          {{ new Date(props.tracklist.createdAt).toLocaleDateString() }}
        </p>
      </div>

      <!-- Track list -->
      <div
        style="
          flex: 1;
          min-height: 200px;
          margin-bottom: 40px;
          overflow-y: auto;
        "
      >
        <div
          v-for="(track, index) in slicedTracks"
          :key="track.id"
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
              getArtworkStyle(track),
            ]"
          >
            <template v-if="track.artworkStatus === 'manual'">
              <div
                style="
                  position: absolute;
                  width: 100%;
                  height: 100%;
                  display: flex;
                  alignitems: center;
                  justifycontent: center;
                  pointerevents: none;
                "
              >
                <span
                  style="
                    background: rgba(0, 0, 0, 0.5);
                    color: white;
                    font-size: 10px;
                    padding: 2px 4px;
                    borderradius: 2px;
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
                v-if="props.visibleFields.includes('bpm') && track.bpm"
                style="text-align: right; font-size: 18px; min-width: 60px"
              >
                {{ track.bpm }} BPM
              </div>
              <div
                v-else-if="props.visibleFields.includes('bpm')"
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
                v-if="
                  props.visibleFields.includes('musical_key') &&
                  track.musicalKey
                "
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
                v-else-if="props.visibleFields.includes('musical_key')"
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
      </div>

      <!-- Footer -->
      <div
        style="
          position: absolute;
          bottom: 40px;
          left: 40px;
          right: 40px;
          text-align: center;
          font-size: 18px;
          opacity: 0.7;
        "
      >
        Generated by KlubHub DJ
      </div>
    </div>
  </div>
</template>

<style scoped>
@import url('https://fonts.googleapis.com/css2?family=Noto+Sans:wght@400;700&display=swap');

#tracklist-card {
  position: relative;
}

/* Ensure the font is applied */
* {
  font-family: 'Noto Sans', sans-serif;
}

/* Mosaic background specific styling */
.bg-mosaic div {
  aspect-ratio: 1;
  overflow: hidden;
  position: relative;
}

.bg-mosaic div img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* Manual artwork lock icon */
.manual-artwork-overlay {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}

.manual-artwork-overlay span {
  background: rgba(0, 0, 0, 0.5);
  color: white;
  font-size: 10px;
  padding: 2px 4px;
  border-radius: 2px;
}
</style>
