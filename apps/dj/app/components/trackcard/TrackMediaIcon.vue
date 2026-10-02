<script setup lang="ts">
import { computed } from 'vue';
import { Disc3, FileAudio } from 'lucide-vue-next';
import { isDigitalMedia, type TrackMedia } from '../../types/tracklist';

// What a track was played from. Vinyl is a record, digital is an audio file, so
// the two read apart at a glance (and not by colour alone). Nothing renders
// when the media is not set. `label` adds the word next to the icon;
// `decorative` is for places that already print the word (a menu item), so
// screen readers do not hear it twice.
const props = withDefaults(defineProps<{
  media?: TrackMedia;
  size?: number;
  label?: boolean;
  decorative?: boolean;
}>(), { media: '', size: 16, label: false, decorative: false });

const digital = computed(() => isDigitalMedia(props.media));
const text = computed(() => (props.media ? props.media.toUpperCase() : ''));
const title = computed(() => (digital.value ? 'Digital' : 'Vinyl'));
</script>

<template>
  <span
    v-if="media"
    class="inline-flex items-center align-middle"
    style="gap: 0.4em; line-height: 1;"
    :title="decorative ? undefined : title"
    :aria-hidden="decorative ? 'true' : undefined"
    :data-media="media"
    data-testid="track-media"
  >
    <!-- Block-level and non-shrinking: an inline svg sits on the text baseline and
         drifts off the centre line of the label next to it. -->
    <FileAudio v-if="digital" :size="size" style="display: block; flex-shrink: 0;" aria-hidden="true" />
    <Disc3 v-else :size="size" style="display: block; flex-shrink: 0;" aria-hidden="true" />
    <span
      v-if="label"
      class="font-terminal tracking-terminal uppercase"
      style="font-size: 0.75em; line-height: 1; display: block;"
    >{{ text }}</span>
    <span v-else-if="!decorative" class="sr-only">{{ title }}</span>
  </span>
</template>
