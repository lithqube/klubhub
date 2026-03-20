<script setup lang="ts">
import { ref, computed } from 'vue';
import { useDropZone } from '@vueuse/core';
import { CloudUpload } from 'lucide-vue-next';
import { uploadTracklist } from '@/composables/useTracklist';

const tracklistStore = useTracklistStore();
const uiStore = useUiStore();
const { uploadLoading, uploadError } = storeToRefs(uiStore);
const { uploadFile, uploadFilename, uploadFileSize } = storeToRefs(tracklistStore);

// Progress simulation for UX feedback
const uploadProgressPct = ref(0);
let progressInterval: ReturnType<typeof setInterval> | null = null;

const startProgressSimulation = () => {
  uploadProgressPct.value = 0;
  progressInterval = setInterval(() => {
    if (uploadProgressPct.value < 85) {
      uploadProgressPct.value = Math.min(85, uploadProgressPct.value + Math.random() * 15);
    }
  }, 300);
};

const finishProgress = () => {
  if (progressInterval) {
    clearInterval(progressInterval);
    progressInterval = null;
  }
  uploadProgressPct.value = 100;
};

// Drop zone
const dropZoneRef = ref<HTMLDivElement | null>(null);
const { isOverDropZone } = useDropZone(dropZoneRef, {
  onDrop: (files) => {
    if (files && files.length > 0) handleFile(files[0]);
  },
});

const fileInputRef = ref<HTMLInputElement | null>(null);

const onFileSelect = (e: Event) => {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (file) handleFile(file);
};

const handleFile = async (file: File) => {
  const lower = file.name.toLowerCase();
  if (!lower.endsWith('.txt') && !lower.endsWith('.xml')) {
    uiStore.uploadError = 'Only .txt or .xml files are allowed';
    return;
  }

  tracklistStore.uploadFile = file;
  tracklistStore.uploadFilename = file.name;
  tracklistStore.uploadFileSize = `${(file.size / 1024).toFixed(1)} KB`;
  uiStore.uploadError = null;

  await parseFile(file);
};

const parseFile = async (file: File) => {
  uiStore.uploadLoading = true;
  uiStore.uploadError = null;
  startProgressSimulation();

  try {
    const result = await uploadTracklist(file);
    tracklistStore.tracklist = result.tracklist;
    tracklistStore.tracks = result.tracks;
    tracklistStore.warnings = result.warnings as string[];
    finishProgress();
    uiStore.step = 'edit';
  } catch (err: unknown) {
    const error = err as { message?: string };
    uiStore.uploadError = error.message ?? 'Failed to parse file';
  } finally {
    uiStore.uploadLoading = false;
  }
};

const openFilePicker = () => {
  fileInputRef.value?.click();
};

const hasFile = computed(() => !!uploadFile.value);
</script>

<template>
  <div class="space-y-4">
    <!-- Drop zone -->
    <div
      ref="dropZoneRef"
      :class="[
        'flex flex-col items-center justify-center gap-4 p-12 cursor-pointer transition-all duration-200',
        isOverDropZone
          ? 'luminous-threshold bg-surface-container-low'
          : 'ghost-border bg-transparent',
      ]"
      @click="openFilePicker"
    >
      <!-- Cloud upload icon -->
      <CloudUpload class="w-10 h-10 text-primary" />

      <!-- Labels -->
      <div class="text-center space-y-1">
        <p class="font-command tracking-command text-on-surface text-sm uppercase font-bold">
          DROP .XML OR .TXT
        </p>
        <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
          MAX_PAYLOAD: 50MB
        </p>
      </div>

      <!-- File info when selected -->
      <div v-if="hasFile" class="text-center mt-2">
        <p class="font-data text-on-surface text-sm">{{ uploadFilename }}</p>
        <p class="font-terminal tracking-terminal text-tertiary text-xs">{{ uploadFileSize }}</p>
      </div>

      <!-- OR divider -->
      <div class="flex items-center gap-4 w-full max-w-xs">
        <div class="flex-1 h-px bg-outline-variant"></div>
        <span class="font-terminal tracking-terminal text-tertiary text-xs uppercase">OR</span>
        <div class="flex-1 h-px bg-outline-variant"></div>
      </div>

      <!-- File picker button -->
      <Button
        variant="default"
        class="gradient-cta text-on-primary font-command tracking-command text-xs uppercase"
        @click.stop="openFilePicker"
      >
        SELECT FILE
      </Button>

      <input
        ref="fileInputRef"
        type="file"
        accept=".txt,.xml"
        class="hidden"
        @change="onFileSelect"
      />
    </div>

    <!-- Progress indicator -->
    <div v-if="uploadLoading" class="mt-4">
      <div class="flex justify-between items-baseline mb-1">
        <span class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
          SYSTEM_PROCESS: TRACKLIST_IMPORT
        </span>
        <span class="font-terminal tracking-terminal text-primary text-xs">
          {{ Math.round(uploadProgressPct) }}%
        </span>
      </div>
      <Progress :model-value="uploadProgressPct" />
    </div>

    <!-- Error display -->
    <div
      v-if="uploadError"
      class="p-3 ghost-border shadow-glow-error"
    >
      <p class="font-terminal tracking-terminal text-error text-xs uppercase">
        ERROR: {{ uploadError }}
      </p>
    </div>
  </div>
</template>
