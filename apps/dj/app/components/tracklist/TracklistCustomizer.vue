<script setup lang="ts">
const settingsStore = useSettingsStore();
const {
  preset,
  bgMode,
  bgValue,
  visibleFields,
  maxTracks,
  trackRangeStart,
  trackRangeEnd,
  logoPath,
  logoPosition,
  customPlaceholderPath,
} = storeToRefs(settingsStore);

const PRESET_OPTIONS = [
  { value: 'default', label: 'DEFAULT' },
  { value: 'dark', label: 'DARK' },
  { value: 'light', label: 'LIGHT' },
  { value: 'neon', label: 'NEON' },
  { value: 'minimal', label: 'MINIMAL' },
] as const;

const BG_MODE_OPTIONS = [
  { value: 'solid', label: 'SOLID_COLOR' },
  { value: 'upload', label: 'UPLOAD_BG' },
  { value: 'mosaic', label: 'MOSAIC_ART' },
] as const;

const LOGO_POSITION_OPTIONS = [
  { value: 'top-left', label: 'TOP_LEFT' },
  { value: 'top-right', label: 'TOP_RIGHT' },
  { value: 'bottom-left', label: 'BOTTOM_LEFT' },
  { value: 'bottom-right', label: 'BOTTOM_RIGHT' },
] as const;

const FIELD_OPTIONS = [
  { value: 'title', label: 'TITLE' },
  { value: 'artist', label: 'ARTIST' },
  { value: 'bpm', label: 'BPM' },
  { value: 'key', label: 'KEY' },
  { value: 'durationSecs', label: 'DURATION' },
  { value: 'label', label: 'LABEL' },
  { value: 'genre', label: 'GENRE' },
] as const;

const saveError = ref<string | null>(null);

const saveSettings = async () => {
  saveError.value = null;
  try {
    await $fetch('/api/v1/settings', {
      method: 'PUT',
      body: {
        tracklist_preferences: {
          preset: preset.value,
          bg_mode: bgMode.value,
          visible_fields: visibleFields.value,
          max_tracks: maxTracks.value,
          track_range_start: trackRangeStart.value,
          track_range_end: trackRangeEnd.value,
          logo_position: logoPosition.value,
        },
        custom_placeholder_path: customPlaceholderPath.value,
        logo_path: logoPath.value,
        bg_value: bgMode.value === 'solid' ? bgValue.value : null,
      },
    });
  } catch (err: unknown) {
    const error = err as { message?: string };
    saveError.value = error.message ?? 'Failed to save settings';
  }
};

const onPresetChange = async (value: string) => {
  preset.value = value;
  await saveSettings();
};

const onBgModeChange = async (value: string) => {
  bgMode.value = value;
  if (value === 'solid') bgValue.value = null;
  else if (value === 'upload') bgValue.value = null;
  await saveSettings();
};

const onFieldToggle = async (field: string) => {
  const fields = [...visibleFields.value];
  const idx = fields.indexOf(field);
  if (idx === -1) {
    fields.push(field);
  } else {
    fields.splice(idx, 1);
  }
  visibleFields.value = fields;
  await saveSettings();
};

const onMaxTracksChange = async (e: Event) => {
  const val = parseInt((e.target as HTMLInputElement).value, 10);
  if (!isNaN(val)) {
    maxTracks.value = val;
    await saveSettings();
  }
};

const onLogoPositionChange = async (value: string) => {
  logoPosition.value = value;
  await saveSettings();
};

const onLogoUpload = async (e: Event) => {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;

  const formData = new FormData();
  formData.append('file', file);
  try {
    const result = await $fetch<{ path: string }>('/api/v1/settings/logo', {
      method: 'POST',
      body: formData,
    });
    logoPath.value = result.path;
    await saveSettings();
  } catch (err: unknown) {
    const error = err as { message?: string };
    saveError.value = error.message ?? 'Failed to upload logo';
  }
};

const isFieldVisible = (field: string) => visibleFields.value.includes(field);
</script>

<template>
  <div class="glass-panel p-4 space-y-4">
    <!-- Error display -->
    <div
      v-if="saveError"
      class="p-2 border border-dashed border-error/30 shadow-glow-error"
    >
      <p class="font-terminal tracking-terminal text-error text-xs uppercase">
        ERROR: {{ saveError }}
      </p>
    </div>

    <!-- ACTIVE_TEMPLATE section -->
    <div class="space-y-2">
      <h3 class="section-lbl border-b border-dashed border-outline-variant/40 pb-1">
        ACTIVE_TEMPLATE
      </h3>
      <div class="grid grid-cols-5 gap-1">
        <button
          v-for="opt in PRESET_OPTIONS"
          :key="opt.value"
          :class="[
            'py-2 px-1 text-center font-terminal tracking-terminal text-xs uppercase transition-colors',
            preset === opt.value
              ? 'bg-primary text-on-primary shadow-glow-primary'
              : 'ghost-border text-tertiary hover:text-on-surface',
          ]"
          @click="onPresetChange(opt.value)"
        >
          {{ opt.label }}
        </button>
      </div>
    </div>

    <!-- VISUAL_CONFIGURATION section -->
    <div class="space-y-3">
      <h3 class="section-lbl border-b border-dashed border-outline-variant/40 pb-1">
        VISUAL_CONFIGURATION
      </h3>

      <!-- Background mode -->
      <div class="space-y-1.5">
        <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase">BACKGROUND_MODE</p>
        <div class="flex gap-2">
          <button
            v-for="opt in BG_MODE_OPTIONS"
            :key="opt.value"
            :class="[
              'py-1.5 px-3 font-terminal tracking-terminal text-xs uppercase transition-colors',
              bgMode === opt.value
                ? 'bg-secondary text-on-primary'
                : 'ghost-border text-tertiary hover:text-on-surface',
            ]"
            @click="onBgModeChange(opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <!-- Max tracks -->
      <div class="space-y-1.5">
        <label class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
          MAX_TRACKS: <span class="text-primary">{{ maxTracks }}</span>
        </label>
        <Input
          type="number"
          :value="maxTracks"
          min="1"
          max="100"
          class="w-24 font-data text-on-surface text-sm"
          @change="onMaxTracksChange"
        />
      </div>
    </div>

    <!-- VISIBLE_FIELDS section -->
    <div class="space-y-3">
      <h3 class="section-lbl border-b border-dashed border-outline-variant/40 pb-1">
        VISIBLE_FIELDS
      </h3>
      <div class="space-y-2">
        <div
          v-for="field in FIELD_OPTIONS"
          :key="field.value"
          class="flex items-center justify-between"
        >
          <span class="font-terminal tracking-terminal text-on-surface-variant text-xs uppercase">
            {{ field.label }}
          </span>
          <Switch
            :checked="isFieldVisible(field.value)"
            @update:checked="onFieldToggle(field.value)"
          />
        </div>
      </div>
    </div>

    <!-- LOGO_UPLOAD section -->
    <div class="space-y-3">
      <h3 class="section-lbl border-b border-dashed border-outline-variant/40 pb-1">
        LOGO_UPLOAD
      </h3>

      <!-- Logo preview -->
      <div
        v-if="logoPath"
        class="w-20 h-20 bg-surface-container-high overflow-hidden"
      >
        <img :src="logoPath" alt="DJ Logo" class="w-full h-full object-contain" >
      </div>

      <!-- Logo file input -->
      <div>
        <label class="block font-terminal tracking-terminal text-tertiary text-xs uppercase mb-1">
          UPLOAD_LOGO
        </label>
        <input
          type="file"
          accept="image/*"
          class="font-terminal tracking-terminal text-tertiary text-xs uppercase ghost-border p-2 w-full cursor-pointer"
          @change="onLogoUpload"
        >
      </div>

      <!-- Logo position -->
      <div class="space-y-1.5">
        <label class="font-terminal tracking-terminal text-tertiary text-xs uppercase">LOGO_POSITION</label>
        <div class="grid grid-cols-2 gap-1">
          <button
            v-for="opt in LOGO_POSITION_OPTIONS"
            :key="opt.value"
            :class="[
              'py-1.5 px-2 font-terminal tracking-terminal text-xs uppercase transition-colors',
              logoPosition === opt.value
                ? 'bg-secondary text-on-primary'
                : 'ghost-border text-tertiary hover:text-on-surface',
            ]"
            @click="onLogoPositionChange(opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
