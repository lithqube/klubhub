<script setup lang="ts">
import { computed } from 'vue';
import type { Tracklist, Track } from '../types/tracklist';
import { PRESETS, type PresetKey } from '@/utils/design-tokens';
import TrackCard from './trackcard/TrackCard.vue';
import TrackCardHeader from './trackcard/TrackCardHeader.vue';
import TrackCardTrackList from './trackcard/TrackCardTrackList.vue';
import TrackCardLogo from './trackcard/TrackCardLogo.vue';
import TrackCardFooter from './trackcard/TrackCardFooter.vue';

type LogoPosition = 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right';

interface Props {
  tracklist: Tracklist;
  tracks: Track[];
  djName: string;
  logoPath?: string | null;
  logoPosition?: LogoPosition;
  preset?: PresetKey;
  bgMode?: 'solid' | 'upload' | 'mosaic';
  bgValue?: string | null;
  visibleFields?: string[];
  maxTracks?: number;
  trackRangeStart?: number | null;
  trackRangeEnd?: number | null;
  customPlaceholderPath?: string | null;
}

const props = withDefaults(defineProps<Props>(), {
  logoPath: null,
  logoPosition: 'top-left',
  preset: 'default',
  bgMode: 'solid',
  bgValue: null,
  visibleFields: () => ['title', 'artist', 'bpm', 'key', 'durationSecs'],
  maxTracks: 50,
  trackRangeStart: null,
  trackRangeEnd: null,
  customPlaceholderPath: null,
});

const colors = computed(() => PRESETS[props.preset]);
</script>

<template>
  <TrackCard :colors="colors" :bgMode="bgMode" :bgValue="bgValue" :tracks="tracks">
    <TrackCardLogo :logoPath="logoPath" :logoPosition="logoPosition" />
    <TrackCardHeader :djName="djName" :tracklist="tracklist" />
    <TrackCardTrackList
      :tracks="tracks"
      :trackRangeStart="trackRangeStart"
      :trackRangeEnd="trackRangeEnd"
      :maxTracks="maxTracks"
      :visibleFields="visibleFields"
      :customPlaceholderPath="customPlaceholderPath"
      :colors="colors"
    />
    <TrackCardFooter :tracklist="tracklist" :colors="colors" />
  </TrackCard>
</template>
