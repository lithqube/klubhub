import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useSettingsStore = defineStore('settings', () => {
  // User preferences
  const djName = ref<string>('');
  const logoPath = ref<string | null>(null);
  const logoPosition = ref<string>('top-left');
  const customPlaceholderPath = ref<string | null>(null);
  const preset = ref<string>('default');
  const bgMode = ref<string>('solid');
  const bgValue = ref<string | null>(null);
  const visibleFields = ref<string[]>(['title', 'artist', 'bpm', 'key', 'durationSecs']);
  const maxTracks = ref<number>(50);
  const trackRangeStart = ref<number | null>(null);
  const trackRangeEnd = ref<number | null>(null);

  async function loadFromApi(): Promise<void> {
    const data = await $fetch<Record<string, any>>('/api/v1/settings');
    if (data.dj_name !== undefined) djName.value = data.dj_name;
    if (data.logo_path !== undefined) logoPath.value = data.logo_path;
    if (data.logo_position !== undefined) logoPosition.value = data.logo_position;
    if (data.custom_placeholder_path !== undefined) customPlaceholderPath.value = data.custom_placeholder_path;
    if (data.preset !== undefined) preset.value = data.preset;
    if (data.bg_mode !== undefined) bgMode.value = data.bg_mode;
    if (data.bg_value !== undefined) bgValue.value = data.bg_value;
    if (data.visible_fields !== undefined) visibleFields.value = data.visible_fields;
    if (data.max_tracks !== undefined) maxTracks.value = data.max_tracks;
    if (data.track_range_start !== undefined) trackRangeStart.value = data.track_range_start;
    if (data.track_range_end !== undefined) trackRangeEnd.value = data.track_range_end;
  }

  return {
    djName,
    logoPath,
    logoPosition,
    customPlaceholderPath,
    preset,
    bgMode,
    bgValue,
    visibleFields,
    maxTracks,
    trackRangeStart,
    trackRangeEnd,
    loadFromApi,
  };
});
