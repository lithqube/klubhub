<script setup lang="ts">
import { computed } from 'vue';
import { Gem } from 'lucide-vue-next';
import type { Track } from '../../types/tracklist';
import type { PresetColors } from '@/utils/design-tokens';
import TrackMediaIcon from './TrackMediaIcon.vue';

export type RowDensity = 'comfortable' | 'compact' | 'dense' | 'dense-2col';

// One row is 82px at "comfortable", 66px "compact", 50px "dense". The list picks
// the tier from the number of tracks so a long set still fits the 1920px card
// without running under the footer (see TrackCardTrackList).
const TOKENS: Record<RowDensity, {
  pad: number; pos: number; posW: number; art: number; artGap: number;
  title: number; artist: number; readout: number;
  gem: number; badge: number; mediaIcon: number; gap: number; mediaLabel: boolean;
  bpmW: number; keyW: number; mediaW: number;
}> = {
  comfortable: { pad: 14, pos: 22, posW: 60, art: 40, artGap: 16, title: 22, artist: 17, readout: 18, gem: 22, badge: 13, mediaIcon: 22, gap: 4, mediaLabel: true, bpmW: 96, keyW: 52, mediaW: 112 },
  compact: { pad: 9, pos: 20, posW: 52, art: 34, artGap: 14, title: 20, artist: 15, readout: 16, gem: 18, badge: 12, mediaIcon: 18, gap: 2, mediaLabel: true, bpmW: 84, keyW: 46, mediaW: 98 },
  dense: { pad: 5, pos: 17, posW: 44, art: 28, artGap: 12, title: 17, artist: 13, readout: 14, gem: 15, badge: 11, mediaIcon: 15, gap: 1, mediaLabel: true, bpmW: 72, keyW: 40, mediaW: 84 },
  'dense-2col': { pad: 5, pos: 16, posW: 36, art: 28, artGap: 10, title: 16, artist: 12, readout: 13, gem: 15, badge: 10, mediaIcon: 14, gap: 1, mediaLabel: false, bpmW: 62, keyW: 36, mediaW: 18 },
};

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
  density: {
    type: String as () => RowDensity,
    default: 'comfortable',
  },
});

const t = computed(() => TOKENS[props.density] ?? TOKENS.comfortable);

// BPM and key are readouts, not prose: terminal register, uppercase, tracked, tabular digits.
const readout = computed(() => ({
  textAlign: 'right',
  width: `${t.value.bpmW}px`,
  flexShrink: 0,
  fontSize: `${t.value.readout}px`,
  fontWeight: 600,
  letterSpacing: '0.05em',
  textTransform: 'uppercase',
  fontVariantNumeric: 'tabular-nums',
  fontFamily: 'var(--font-terminal)',
  whiteSpace: 'nowrap',
}) as const);

const getArtworkStyle = () => {
  // A hidden gem never shows its cover: the artwork would give the track away.
  if (props.track.hiddenGem) {
    return {
      backgroundColor: props.colors.accent,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
    };
  }

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
      padding: `${t.pad}px 0`,
      borderBottom: `1px dashed ${colors.text}33`,
      minWidth: 0,
    }"
  >
    <!-- Position: terminal-register readout in the signal colour -->
    <div
      :style="{
        width: `${t.posW}px`,
        textAlign: 'center',
        fontSize: `${t.pos}px`,
        fontWeight: 700,
        letterSpacing: '0.05em',
        fontVariantNumeric: 'tabular-nums',
        color: colors.primary,
        fontFamily: 'var(--font-terminal)',
        flexShrink: 0,
      }"
    >
      {{ index + 1 }}
    </div>

    <!-- Cover Art -->
    <div
      :style="[
        {
          width: `${t.art}px`,
          height: `${t.art}px`,
          marginRight: `${t.artGap}px`,
          flexShrink: 0,
          position: 'relative',
        },
        getArtworkStyle(),
      ]"
    >
      <Gem v-if="track.hiddenGem" :size="t.gem" :color="colors.primary" aria-hidden="true" />
      <template v-else-if="track.artworkStatus === 'manual'">
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
            :style="{
              background: colors.bg,
              color: colors.primary,
              border: `1px dashed ${colors.primary}`,
              fontFamily: 'var(--font-terminal)',
              fontSize: '10px',
              fontWeight: 700,
              letterSpacing: '0.08em',
              padding: '2px 4px',
            }"
            >LOCK</span
          >
        </div>
      </template>
    </div>

    <!-- Track info -->
    <div style="flex: 1; min-width: 0">
      <div
        :style="{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: '12px',
          marginBottom: `${t.gap}px`,
        }"
      >
        <div style="flex: 1; min-width: 0">
          <div style="display: flex; align-items: center; gap: 12px; min-width: 0">
            <!-- A hidden gem keeps its place in the order but not its name. -->
            <div
              v-if="track.hiddenGem"
              data-testid="card-hidden-title"
              :style="{
                fontFamily: 'var(--font-terminal)',
                fontWeight: 700,
                fontSize: `${Math.max(t.title - 2, 14)}px`,
                lineHeight: 1.25,
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                color: colors.primary,
              }"
            >
              HIDDEN GEM
            </div>
            <div
              v-else
              :style="{
                fontWeight: 600,
                fontSize: `${t.title}px`,
                lineHeight: 1.25,
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                color: colors.text,
              }"
            >
              {{ track.title || 'Unknown Title' }}
            </div>
            <span
              v-if="track.unreleased"
              data-testid="card-unreleased"
              :style="{
                flexShrink: 0,
                border: `1px dashed ${colors.primary}`,
                color: colors.primary,
                fontFamily: 'var(--font-terminal)',
                fontSize: `${t.badge}px`,
                fontWeight: 700,
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                padding: '1px 6px',
              }"
            >
              UNRELEASED
            </span>
          </div>
          <div
            v-if="track.hiddenGem"
            data-testid="card-hidden-artist"
            :style="{ width: '160px', height: `${Math.round(t.artist * 0.7)}px`, marginTop: '5px', backgroundColor: `${colors.text}4d` }"
          />
          <div
            v-else
            :style="{
              fontSize: `${t.artist}px`,
              lineHeight: 1.3,
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              color: `${colors.text}b3`,
            }"
          >
            {{ track.artist || 'Unknown Artist' }}
          </div>
        </div>

        <!-- What it was played from: a record for vinyl, an audio file for digital -->
        <div
          v-if="track.media"
          :style="{ ...readout, width: `${t.mediaW}px`, textAlign: 'left', color: `${colors.text}b3`, display: 'flex', alignItems: 'center', justifyContent: 'flex-start' }"
        >
          <TrackMediaIcon :media="track.media" :size="t.mediaIcon" :label="t.mediaLabel" />
        </div>

        <!-- Conditional fields based on visibleFields: terminal-register readouts -->
        <div
          v-if="visibleFields.includes('bpm') && track.bpm"
          :style="{ ...readout, color: colors.text }"
        >
          {{ track.bpm }} BPM
        </div>
        <div
          v-else-if="visibleFields.includes('bpm')"
          :style="{ ...readout, color: `${colors.text}66` }"
        >
          — BPM
        </div>

        <div
          v-if="visibleFields.includes('musical_key') && track.musicalKey"
          :style="{ ...readout, width: `${t.keyW}px`, color: colors.primary }"
        >
          {{ track.musicalKey }}
        </div>
        <div
          v-else-if="visibleFields.includes('musical_key')"
          :style="{ ...readout, width: `${t.keyW}px`, color: `${colors.text}66` }"
        >
          — Key
        </div>
      </div>
    </div>
  </div>
</template>
