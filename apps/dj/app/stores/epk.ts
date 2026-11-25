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
  // photoPaths are storage keys; the API signs a display URL for each one.
  const photoUrls = ref<Record<string, string>>({});
  const sectionVisibility = ref<SectionVisibility>({});
  const exports = ref<EPKExport[]>([]);
  const saveStatus = ref<'idle' | 'saving' | 'saved' | 'error'>('idle');

  // Computed
  const photoCount = computed(() => photoPaths.value.length);
  const canAddPhoto = computed(() => photoPaths.value.length < 20);

  // The Go API serializes empty slices/maps as JSON null (nil values) on a
  // fresh profile, so collections are coalesced to keep the refs usable.
  function applyContent(data: EPKContent, wants: (key: keyof UpdateEPKContentRequest) => boolean = () => true): void {
    if (wants('bioShort')) bioShort.value = data.bioShort ?? '';
    if (wants('bioLong')) bioLong.value = data.bioLong ?? '';
    if (wants('techRider')) techRider.value = data.techRider ?? '';
    if (wants('stagePlotPath' as keyof UpdateEPKContentRequest)) stagePlotPath.value = data.stagePlotPath ?? '';
    if (wants('gigHighlights')) gigHighlights.value = data.gigHighlights ?? [];
    if (wants('pressQuotes')) pressQuotes.value = data.pressQuotes ?? [];
    if (wants('photoPaths' as keyof UpdateEPKContentRequest)) photoPaths.value = data.photoPaths ?? [];
    if (wants('sectionVisibility')) sectionVisibility.value = data.sectionVisibility ?? {};
  }

  // Actions
  async function loadFromApi(): Promise<void> {
    const result = await $fetch<{ data: EPKContent }>('/api/v1/epk/content');
    id.value = result.data.id;
    applyContent(result.data);
    photoUrls.value = result.data.photoUrls ?? {};
  }

  async function updateContent(patch: UpdateEPKContentRequest): Promise<void> {
    saveStatus.value = 'saving';
    const fields: Record<string, { value: unknown }> = {
      bioShort, bioLong, techRider, stagePlotPath, gigHighlights, pressQuotes, photoPaths, sectionVisibility,
    };
    const sent = new Map(Object.keys(patch).map((key) => [key, fields[key]?.value]));
    try {
      const result = await $fetch<{ data: EPKContent }>('/api/v1/epk/content', {
        method: 'PUT',
        body: patch,
      });
      // Only the fields this save sent, and only if nothing was typed in them
      // since: the response holds old text for every other unsaved field.
      applyContent(result.data, (key) => sent.has(key) && fields[key]?.value === sent.get(key));
      if (result.data.photoUrls) photoUrls.value = result.data.photoUrls;
      saveStatus.value = 'saved';
    } catch (error) {
      saveStatus.value = 'error';
      throw error;
    }
  }

  async function uploadPhoto(file: File): Promise<void> {
    const formData = new FormData();
    formData.append('photo', file);
    const result = await $fetch<{ path: string; url?: string }>('/api/v1/epk/photos', {
      method: 'POST',
      body: formData,
    });
    photoPaths.value = [...photoPaths.value, result.path];
    // `path` is a storage key; `url` is the signed address the <img> needs.
    if (result.url) photoUrls.value = { ...photoUrls.value, [result.path]: result.url };
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
    photoUrls,
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
