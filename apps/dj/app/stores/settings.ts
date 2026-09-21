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

  // Profile fields shared with the EPK
  const socialLinks = ref<Record<string, string>>({});
  const contactInfo = ref<string>('');
  const saveStatus = ref<'idle' | 'saving' | 'saved' | 'error'>('idle');

  /** The subset of settings the UI is allowed to change through save(). */
  type SettingsPatch = Partial<{
    dj_name: string;
    social_links: Record<string, string>;
    contact_info: string;
  }>;

  // Every key of the API's UpdateSettingsRequest. PUT /settings replaces the
  // whole row, so all of them must be sent back or they are blanked.
  const WRITABLE_KEYS = [
    'dj_name', 'logo_path', 'default_colors', 'default_template', 'visible_fields',
    'social_links', 'bio_short', 'bio_long', 'contact_info', 'invoice_prefix',
  ] as const;

  function applyProfile(data: Record<string, any>): void {
    if (typeof data.dj_name === 'string') djName.value = data.dj_name;
    if (data.social_links && typeof data.social_links === 'object') {
      socialLinks.value = { ...data.social_links };
    }
    if (typeof data.contact_info === 'string') contactInfo.value = data.contact_info;
  }

  /**
   * Read-modify-write. PUT /settings is a full-row replace guarded by an
   * `updated_at` concurrency token: without the token it answers 409 (which
   * is why EPK settings writes never saved), and sending only the changed
   * field would wipe the rest. So fetch the current row, merge, send it all.
   */
  async function save(patch: SettingsPatch): Promise<void> {
    saveStatus.value = 'saving';
    try {
      const current = await $fetch<Record<string, any>>('/api/v1/settings');
      const body: Record<string, any> = { updated_at: current.updated_at };
      for (const key of WRITABLE_KEYS) body[key] = current[key];
      Object.assign(body, patch);
      const saved = await $fetch<Record<string, any>>('/api/v1/settings', { method: 'PUT', body });
      applyProfile(saved?.data ?? saved ?? body);
      saveStatus.value = 'saved';
    } catch (e) {
      saveStatus.value = 'error';
      throw e;
    }
  }

  async function loadFromApi(): Promise<void> {
    const data = await $fetch<Record<string, any>>('/api/v1/settings');
    applyProfile(data);
    if (data.dj_name !== undefined) djName.value = data.dj_name;
    if (data.logo_path !== undefined) logoPath.value = data.logo_path;
    if (data.logo_position !== undefined) logoPosition.value = data.logo_position;
    if (data.custom_placeholder_path !== undefined) customPlaceholderPath.value = data.custom_placeholder_path;
    if (data.preset !== undefined) preset.value = data.preset;
    if (data.bg_mode !== undefined) bgMode.value = data.bg_mode;
    if (data.bg_value !== undefined) bgValue.value = data.bg_value;
    // A fresh settings row carries `visible_fields: {}` (an empty object);
    // only a non-empty array may replace the default, or `.includes` breaks.
    if (Array.isArray(data.visible_fields) && data.visible_fields.length > 0) {
      visibleFields.value = data.visible_fields;
    }
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
    socialLinks,
    contactInfo,
    saveStatus,
    save,
    loadFromApi,
  };
});
