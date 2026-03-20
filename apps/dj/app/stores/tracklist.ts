import { defineStore } from 'pinia';
import { ref, computed } from 'vue';

export const useTracklistStore = defineStore('tracklist', () => {
  const tracklist = ref<Record<string, any> | null>(null);
  const tracks = ref<Record<string, any>[]>([]);
  const warnings = ref<string[]>([]);
  const uploadFile = ref<File | null>(null);
  const uploadFilename = ref<string>('');
  const uploadFileSize = ref<string>('');
  const pastTracklists = ref<Record<string, any>[]>([]);

  // Internal state - not returned (private implementation detail)
  const abortController = ref<AbortController | null>(null);

  const trackCount = computed(() => tracks.value.length);

  function reset(): void {
    tracklist.value = null;
    tracks.value = [];
    warnings.value = [];
    uploadFile.value = null;
    uploadFilename.value = '';
    uploadFileSize.value = '';
    pastTracklists.value = [];
    abortController.value = null;
  }

  return {
    tracklist,
    tracks,
    warnings,
    uploadFile,
    uploadFilename,
    uploadFileSize,
    pastTracklists,
    trackCount,
    reset,
  };
});
