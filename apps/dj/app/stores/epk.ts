import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import type { EPKContent, EPKExport, EPKExportCreateResult, PressQuote, SectionVisibility, UpdateEPKContentRequest } from '../types/epk';

export const useEpkStore = defineStore('epk', () => {
  // State
  const id = ref<string>('');
  const bioShort = ref<string>('');
  const bioLong = ref<string>('');
  const techRider = ref<string>('');
  const stagePlotPath = ref<string>('');
  const gigHighlights = ref<string[]>([]);
  const pressQuotes = ref<PressQuote[]>([]);
  const photoPaths = ref<string[]>([]);
  const sectionVisibility = ref<SectionVisibility>({});
  const exports = ref<EPKExport[]>([]);
  const saveStatus = ref<'idle' | 'saving' | 'saved' | 'error'>('idle');

  // Computed
  const photoCount = computed(() => photoPaths.value.length);
  const canAddPhoto = computed(() => photoPaths.value.length < 20);

  // Actions
  async function loadFromApi(): Promise<void> {
    const result = await $fetch<{ data: EPKContent }>('/api/v1/epk/content');
    const data = result.data;
    id.value = data.id;
    bioShort.value = data.bioShort;
    bioLong.value = data.bioLong;
    techRider.value = data.techRider;
    stagePlotPath.value = data.stagePlotPath;
    gigHighlights.value = data.gigHighlights;
    pressQuotes.value = data.pressQuotes;
    photoPaths.value = data.photoPaths;
    sectionVisibility.value = data.sectionVisibility;
  }

  async function updateContent(patch: UpdateEPKContentRequest): Promise<void> {
    saveStatus.value = 'saving';
    try {
      const result = await $fetch<{ data: EPKContent }>('/api/v1/epk/content', {
        method: 'PUT',
        body: patch,
      });
      const data = result.data;
      bioShort.value = data.bioShort;
      bioLong.value = data.bioLong;
      techRider.value = data.techRider;
      stagePlotPath.value = data.stagePlotPath;
      gigHighlights.value = data.gigHighlights;
      pressQuotes.value = data.pressQuotes;
      photoPaths.value = data.photoPaths;
      sectionVisibility.value = data.sectionVisibility;
      saveStatus.value = 'saved';
    } catch (error) {
      saveStatus.value = 'error';
      throw error;
    }
  }

  async function uploadPhoto(file: File): Promise<void> {
    const formData = new FormData();
    formData.append('photo', file);
    const result = await $fetch<{ path: string }>('/api/v1/epk/photos', {
      method: 'POST',
      body: formData,
    });
    photoPaths.value = [...photoPaths.value, result.path];
  }

  async function deletePhoto(path: string): Promise<void> {
    await $fetch(`/api/v1/epk/photos/${encodeURIComponent(path)}`, {
      method: 'DELETE',
    });
    photoPaths.value = photoPaths.value.filter((p) => p !== path);
  }

  async function uploadStagePlot(file: File): Promise<void> {
    const formData = new FormData();
    formData.append('stagePlot', file);
    const result = await $fetch<{ path: string }>('/api/v1/epk/stage-plot', {
      method: 'POST',
      body: formData,
    });
    stagePlotPath.value = result.path;
  }

  async function generateExport(): Promise<EPKExportCreateResult> {
    return await $fetch<EPKExportCreateResult>('/api/v1/epk/export', {
      method: 'POST',
    });
  }

  async function loadExports(): Promise<void> {
    const result = await $fetch<{ data: EPKExport[] }>('/api/v1/epk/exports');
    exports.value = result.data;
  }

  async function deleteExport(id: string): Promise<void> {
    await $fetch(`/api/v1/epk/exports/${id}`, { method: 'DELETE' });
    exports.value = exports.value.filter((e) => e.id !== id);
  }

  return {
    id,
    bioShort,
    bioLong,
    techRider,
    stagePlotPath,
    gigHighlights,
    pressQuotes,
    photoPaths,
    sectionVisibility,
    exports,
    saveStatus,
    photoCount,
    canAddPhoto,
    loadFromApi,
    updateContent,
    uploadPhoto,
    deletePhoto,
    uploadStagePlot,
    generateExport,
    loadExports,
    deleteExport,
  };
});
