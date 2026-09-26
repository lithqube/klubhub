import { defineStore } from 'pinia';
import { ref } from 'vue';
import { useToast } from '#kui/components/ui/toast/use-toast';

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

  function showError(message: string): void {
    // Toasts are browser UI; skip during SSR so no timers leak on the server.
    if (!import.meta.client) return;
    useToast().toast({ title: 'Error', description: message, variant: 'destructive' });
  }

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
    showError,
    clearErrors,
  };
});
