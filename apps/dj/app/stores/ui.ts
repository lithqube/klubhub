import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useUiStore = defineStore('ui', () => {
  const step = ref<string>('upload');

  const uploadLoading = ref<boolean>(false);
  const uploadError = ref<string | null>(null);

  const editLoading = ref<boolean>(false);
  const editError = ref<string | null>(null);

  const exportLoading = ref<boolean>(false);
  const exportError = ref<string | null>(null);

  const pastTracklistsLoading = ref<boolean>(false);
  const pastTracklistsError = ref<string | null>(null);

  function clearErrors(): void {
    uploadError.value = null;
    editError.value = null;
    exportError.value = null;
    pastTracklistsError.value = null;
  }

  return {
    step,
    uploadLoading,
    uploadError,
    editLoading,
    editError,
    exportLoading,
    exportError,
    pastTracklistsLoading,
    pastTracklistsError,
    clearErrors,
  };
});
