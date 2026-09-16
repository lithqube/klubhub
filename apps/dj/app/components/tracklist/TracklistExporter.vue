<script setup lang="ts">
import { ref } from 'vue';
import { generateImage } from '@/composables/useTracklist';

const tracklistStore = useTracklistStore();
const uiStore = useUiStore();

const { tracklist } = storeToRefs(tracklistStore);
const { exportLoading, exportError } = storeToRefs(uiStore);

// Track last export result for the "Schedule to Instagram" CTA
const lastExportStoryPath = ref<string | null>(null);
const lastExportSquarePath = ref<string | null>(null);

const exportImage = async (format: 'story' | 'square') => {
  if (!tracklist.value) return;

  uiStore.exportLoading = true;
  uiStore.exportError = null;
  lastExportStoryPath.value = null;
  lastExportSquarePath.value = null;

  try {
    const result = await generateImage(tracklist.value.id, format);
    if (result.story) {
      lastExportStoryPath.value = result.story;
      window.open(result.story, '_blank');
    }
    if (result.square) {
      lastExportSquarePath.value = result.square;
      window.open(result.square, '_blank');
    }
  } catch (err: unknown) {
    const error = err as { message?: string };
    uiStore.exportError = error.message ?? 'Failed to export image';
  } finally {
    uiStore.exportLoading = false;
  }
};

const exportBoth = async () => {
  if (!tracklist.value) return;

  uiStore.exportLoading = true;
  uiStore.exportError = null;
  lastExportStoryPath.value = null;
  lastExportSquarePath.value = null;

  try {
    const result = await generateImage(tracklist.value.id, 'both');
    if (result.story) {
      lastExportStoryPath.value = result.story;
      window.open(result.story, '_blank');
    }
    if (result.square) {
      lastExportSquarePath.value = result.square;
      window.open(result.square, '_blank');
    }
  } catch (err: unknown) {
    const error = err as { message?: string };
    uiStore.exportError = error.message ?? 'Failed to export images';
  } finally {
    uiStore.exportLoading = false;
  }
};

// Navigate to social page with the best available image path
function scheduleToInstagram() {
  // Prefer story format (1080×1920) as primary DJ post format; fall back to square
  const imageId = lastExportStoryPath.value ?? lastExportSquarePath.value;
  if (!imageId) return;
  navigateTo('/social?imageId=' + encodeURIComponent(imageId));
}
</script>

<template>
  <div class="space-y-4">
    <!-- Section header -->
    <h2 class="font-command tracking-command text-on-surface text-sm uppercase font-bold">
      EXPORT_IMAGES
    </h2>

    <!-- Export buttons -->
    <div class="flex flex-col gap-3 sm:flex-row">
      <!-- Story format -->
      <Button
        variant="default"
        :disabled="exportLoading || !tracklist"
        class="gradient-cta text-on-primary font-command tracking-command text-xs uppercase flex-1"
        @click="exportImage('story')"
      >
        <span v-if="exportLoading" class="font-terminal tracking-terminal text-xs uppercase">
          PROCESSING...
        </span>
        <span v-else>EXPORT_STORY (1080×1920)</span>
      </Button>

      <!-- Square format -->
      <Button
        variant="default"
        :disabled="exportLoading || !tracklist"
        class="gradient-cta text-on-primary font-command tracking-command text-xs uppercase flex-1"
        @click="exportImage('square')"
      >
        <span v-if="exportLoading" class="font-terminal tracking-terminal text-xs uppercase">
          PROCESSING...
        </span>
        <span v-else>EXPORT_SQUARE (1080×1080)</span>
      </Button>
    </div>

    <!-- Export both -->
    <Button
      variant="ghost"
      :disabled="exportLoading || !tracklist"
      class="w-full ghost-border font-command tracking-command text-on-surface text-xs uppercase"
      @click="exportBoth"
    >
      <span v-if="exportLoading" class="font-terminal tracking-terminal text-xs uppercase">
        GENERATING_BOTH...
      </span>
      <span v-else>EXPORT_BOTH</span>
    </Button>

    <!-- Loading indicator -->
    <div v-if="exportLoading" class="space-y-1">
      <div class="flex justify-between items-baseline">
        <span class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
          SYSTEM_PROCESS: IMAGE_GENERATION
        </span>
      </div>
      <Progress :model-value="undefined" class="animate-pulse" />
    </div>

    <!-- Error display -->
    <div
      v-if="exportError"
      class="p-3 ghost-border shadow-glow-error"
    >
      <p class="font-terminal tracking-terminal text-error text-xs uppercase">
        ERROR: {{ exportError }}
      </p>
    </div>

    <!-- Post-export CTA: Schedule to Instagram -->
    <div v-if="lastExportStoryPath || lastExportSquarePath" class="pt-1">
      <Button
        variant="ghost"
        class="w-full ghost-border font-command tracking-command text-primary text-xs uppercase"
        @click="scheduleToInstagram"
      >
        ⚡ SCHEDULE TO INSTAGRAM
      </Button>
    </div>

    <!-- No tracklist message -->
    <div
      v-if="!tracklist"
      class="text-center py-4"
    >
      <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
        NO_TRACKLIST_LOADED — upload a file first
      </p>
    </div>
  </div>
</template>
