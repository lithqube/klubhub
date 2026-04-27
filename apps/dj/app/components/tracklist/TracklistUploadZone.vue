<script setup lang="ts">
import { ref, computed } from 'vue'
import { useDropZone } from '@vueuse/core'
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
    <!-- Drop zone — HUD design spec: dashed ghost border, glow on hover -->
    <div
      ref="dropZoneRef"
      class="upload-zone hud-card"
      :class="isOverDropZone ? 'luminous-threshold' : ''"
      @click="openFilePicker"
    >
      <CloudUpload
        style="width:36px;height:36px;stroke:var(--color-primary);stroke-width:1.5;opacity:.5;"
        aria-hidden="true"
      />

      <div style="font-family:var(--font-command);font-size:13px;font-weight:700;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;">
        DROP YOUR DJ HISTORY FILE
      </div>
      <div style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.08em;text-transform:uppercase;color:var(--color-tertiary);">
        REKORDBOX · SERATO · TRAKTOR
      </div>

      <!-- File info when selected -->
      <div v-if="hasFile" style="text-align:center;margin-top:4px;">
        <div style="font-family:var(--font-data);font-size:12px;color:var(--color-on-surface);">{{ uploadFilename }}</div>
        <div style="font-family:var(--font-terminal);font-size:8px;color:var(--color-tertiary);text-transform:uppercase;letter-spacing:.05em;">{{ uploadFileSize }}</div>
      </div>

      <div style="font-family:var(--font-terminal);font-size:7px;letter-spacing:.06em;text-transform:uppercase;color:rgba(150,248,255,.4);margin-top:2px;">
        OR CLICK TO BROWSE FILES
      </div>

      <input
        ref="fileInputRef"
        type="file"
        accept=".txt,.xml"
        class="hidden"
        @change="onFileSelect"
      />
    </div>

    <!-- Progress indicator -->
    <div v-if="uploadLoading" class="glass" style="padding:14px 16px;">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px;">
        <span class="spost-meta">SYSTEM_PROCESS: TRACKLIST_IMPORT</span>
        <span style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-primary);">
          {{ Math.round(uploadProgressPct) }}%
        </span>
      </div>
      <div class="prog-track">
        <div class="prog-fill prog-fill-anim" :style="{ width: uploadProgressPct + '%' }" />
      </div>
    </div>

    <!-- Error display -->
    <div
      v-if="uploadError"
      style="padding:12px 14px;border:1px dashed rgba(255,113,108,.3);box-shadow:0 0 16px rgba(255,113,108,.15);"
    >
      <span class="spost-meta" style="color:var(--color-error);">
        <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/></svg>
        ERROR: {{ uploadError }}
      </span>
    </div>
  </div>
</template>
