import { ref, computed } from 'vue';
import { storeToRefs } from 'pinia';
import { useSettingsStore } from '../stores/settings';
import type { PostType } from '../types/social';

export function useSocialPostForm() {
  const settingsStore = useSettingsStore();
  const { djName } = storeToRefs(settingsStore);

  const postType = ref<PostType>('feed');
  const caption = ref('');
  const scheduledAt = ref('');       // datetime-local value
  const timezoneName = ref(Intl.DateTimeFormat().resolvedOptions().timeZone);  // browser default
  const timezoneSearch = ref('');
  const imageId = ref<string | null>(null);
  const imageFile = ref<File | null>(null);

  const charLimit = computed<number | null>(() => postType.value === 'feed' ? 2200 : null);
  const charCount = computed<number>(() => caption.value.length);
  const isOverLimit = computed<boolean>(() => charLimit.value !== null && charCount.value > charLimit.value);

  const timezoneOptions = computed<string[]>(() => {
    const all = Intl.supportedValuesOf('timeZone');
    if (!timezoneSearch.value) return all;
    return all.filter((tz) => tz.toLowerCase().includes(timezoneSearch.value.toLowerCase()));
  });

  const scheduledAtUTCPreview = computed<string>(() => {
    if (!scheduledAt.value || !timezoneName.value) return '';
    try {
      const [date, time] = scheduledAt.value.split('T');
      const dtStr = `${date}T${time}:00`;
      const formatter = new Intl.DateTimeFormat('en-US', {
        timeZone: timezoneName.value,
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      });
      return formatter.format(new Date(dtStr)) + ' (local)';
    } catch {
      return '';
    }
  });

  function generateCaption(tracklist: { title: string; trackCount: number; avgBpm: number }): void {
    caption.value = `${djName.value} @ ${tracklist.title} — ${tracklist.trackCount} tracks • avg ${Math.round(tracklist.avgBpm)} BPM\n#techno #djset`;
  }

  function resetForm(): void {
    postType.value = 'feed';
    caption.value = '';
    scheduledAt.value = '';
    timezoneName.value = Intl.DateTimeFormat().resolvedOptions().timeZone;
    timezoneSearch.value = '';
    imageId.value = null;
    imageFile.value = null;
  }

  return {
    postType,
    caption,
    scheduledAt,
    timezoneName,
    timezoneSearch,
    imageId,
    imageFile,
    charLimit,
    charCount,
    isOverLimit,
    timezoneOptions,
    scheduledAtUTCPreview,
    generateCaption,
    resetForm,
  };
}
