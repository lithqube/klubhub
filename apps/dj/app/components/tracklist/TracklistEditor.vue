<script setup lang="ts">
import { ref } from 'vue';
import { MoreVertical } from 'lucide-vue-next';
import { updateTrack, uploadTrackArtwork, pollArtworkStatus } from '@/composables/useTracklist';

const tracklistStore = useTracklistStore();
const uiStore = useUiStore();
const { tracklist, tracks, warnings } = storeToRefs(tracklistStore);
const { editLoading, editError } = storeToRefs(uiStore);

// Row selection state
const selectedTrackId = ref<string | null>(null);

// Inline edit state
const editingCell = ref<{ trackId: string; field: string } | null>(null);
const editingValue = ref<string>('');

// Artwork polling abort controller (private)
let artworkAbortController: AbortController | null = null;

// Status → accent-bar class mapping
const getAccentBarClass = (status?: string): string => {
  switch (status) {
    case 'draft': return 'accent-bar-draft';
    case 'pending': return 'accent-bar-pending';
    case 'archived': return 'accent-bar-archived';
    case 'ready':
    default: return 'accent-bar-ready';
  }
};

const selectRow = (trackId: string) => {
  selectedTrackId.value = selectedTrackId.value === trackId ? null : trackId;
};

const startEdit = (trackId: string, field: string, currentValue: unknown) => {
  editingCell.value = { trackId, field };
  editingValue.value = String(currentValue ?? '');
};

const commitEdit = async (trackId: string, field: string) => {
  if (!tracklist.value) return;
  editingCell.value = null;

  uiStore.editLoading = true;
  uiStore.editError = null;

  try {
    const result = await updateTrack(tracklist.value.id, trackId, {
      [field]: editingValue.value,
    });
    const index = tracks.value.findIndex((t) => t.id === trackId);
    if (index !== -1) {
      tracks.value[index] = { ...tracks.value[index], ...result };
    }
  } catch (err: unknown) {
    const error = err as { message?: string };
    uiStore.editError = error.message ?? 'Failed to save track';
  } finally {
    uiStore.editLoading = false;
  }
};

const cancelEdit = () => {
  editingCell.value = null;
  editingValue.value = '';
};

const handleArtworkUpload = async (trackId: string, e: Event) => {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file || !tracklist.value) return;

  uiStore.editLoading = true;
  uiStore.editError = null;

  try {
    const result = await uploadTrackArtwork(tracklist.value.id, trackId, file);
    const index = tracks.value.findIndex((t) => t.id === trackId);
    if (index !== -1) {
      tracks.value[index] = {
        ...tracks.value[index],
        artworkUrl: result.artworkUrl,
        artworkStatus: result.artworkStatus,
        artworkSource: result.artworkSource,
      };
    }
  } catch (err: unknown) {
    const error = err as { message?: string };
    uiStore.editError = error.message ?? 'Failed to upload artwork';
  } finally {
    uiStore.editLoading = false;
  }
};

const startArtworkPolling = () => {
  if (!tracklist.value) return;
  artworkAbortController = new AbortController();
  pollArtworkStatus(
    tracklist.value.id,
    2000,
    (updatedTracks) => {
      tracks.value = tracks.value.map((originalTrack) => {
        const updatedTrack = updatedTracks.find((t) => t.id === originalTrack.id);
        if (originalTrack.artworkStatus === 'manual') return originalTrack;
        return updatedTrack ?? originalTrack;
      });
    },
    artworkAbortController.signal,
  );
};

onMounted(() => {
  startArtworkPolling();
});

onBeforeUnmount(() => {
  artworkAbortController?.abort();
});
</script>

<template>
  <div class="space-y-4">
    <!-- Section header -->
    <div class="flex items-center justify-between">
      <h2 class="font-command tracking-command text-on-surface text-sm uppercase font-bold">
        TRACK_EDITOR
      </h2>
      <div v-if="editLoading" class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
        SAVING...
      </div>
    </div>

    <!-- Warnings -->
    <div
      v-if="warnings && warnings.length > 0"
      class="p-3 ghost-border shadow-glow-error space-y-1"
    >
      <p
        v-for="(warning, i) in warnings"
        :key="i"
        class="font-terminal tracking-terminal text-error text-xs uppercase"
      >
        WARN: {{ warning }}
      </p>
    </div>

    <!-- Edit error -->
    <div
      v-if="editError"
      class="p-3 ghost-border shadow-glow-error"
    >
      <p class="font-terminal tracking-terminal text-error text-xs uppercase">
        ERROR: {{ editError }}
      </p>
    </div>

    <!-- Track table -->
    <div class="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow class="hover:bg-transparent">
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-8">#</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-10">ART</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase">TITLE</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase">ARTIST</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-16">BPM</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-16">KEY</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-8 text-right">
              <MoreVertical class="w-4 h-4 ml-auto" />
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow
            v-for="(track, index) in tracks"
            :key="track.id"
            :class="[
              getAccentBarClass(track.status),
              selectedTrackId === track.id ? 'row-selected' : '',
              'cursor-pointer hover:bg-surface-bright transition-colors',
            ]"
            @click="selectRow(track.id)"
          >
            <!-- Position -->
            <TableCell class="font-data text-tertiary text-xs py-3 pl-4">
              {{ index + 1 }}
            </TableCell>

            <!-- Cover art -->
            <TableCell class="py-3">
              <div class="relative w-10 h-10 flex-shrink-0">
                <div
                  class="w-10 h-10 bg-surface-container-high flex items-center justify-center overflow-hidden"
                  :style="track.artworkUrl ? {
                    backgroundImage: `url(${track.artworkUrl})`,
                    backgroundSize: 'cover',
                    backgroundPosition: 'center',
                  } : {}"
                >
                  <span
                    v-if="!track.artworkUrl && track.artworkStatus === 'pending'"
                    class="font-terminal tracking-terminal text-tertiary text-xs uppercase"
                  >...</span>
                </div>
                <!-- Artwork upload overlay -->
                <label
                  class="absolute inset-0 cursor-pointer opacity-0 hover:opacity-100 bg-surface/60 flex items-center justify-center transition-opacity"
                  @click.stop
                >
                  <span class="font-terminal tracking-terminal text-primary text-xs uppercase">UPLOAD</span>
                  <input
                    type="file"
                    accept="image/*"
                    class="hidden"
                    @change="handleArtworkUpload(track.id, $event)"
                  />
                </label>
              </div>
            </TableCell>

            <!-- Title (inline edit) -->
            <TableCell class="py-3 max-w-xs">
              <template v-if="editingCell?.trackId === track.id && editingCell.field === 'title'">
                <Input
                  v-model="editingValue"
                  class="h-7 font-data text-on-surface text-xs"
                  autofocus
                  @blur="commitEdit(track.id, 'title')"
                  @keydown.enter="commitEdit(track.id, 'title')"
                  @keydown.esc="cancelEdit"
                  @click.stop
                />
              </template>
              <template v-else>
                <span
                  class="font-data text-on-surface text-sm block truncate"
                  @click.stop="startEdit(track.id, 'title', track.title)"
                >{{ track.title || '—' }}</span>
              </template>
            </TableCell>

            <!-- Artist (inline edit) -->
            <TableCell class="py-3 max-w-xs">
              <template v-if="editingCell?.trackId === track.id && editingCell.field === 'artist'">
                <Input
                  v-model="editingValue"
                  class="h-7 font-data text-on-surface text-xs"
                  autofocus
                  @blur="commitEdit(track.id, 'artist')"
                  @keydown.enter="commitEdit(track.id, 'artist')"
                  @keydown.esc="cancelEdit"
                  @click.stop
                />
              </template>
              <template v-else>
                <span
                  class="font-data text-tertiary text-sm block truncate"
                  @click.stop="startEdit(track.id, 'artist', track.artist)"
                >{{ track.artist || '—' }}</span>
              </template>
            </TableCell>

            <!-- BPM (inline edit) -->
            <TableCell class="py-3">
              <template v-if="editingCell?.trackId === track.id && editingCell.field === 'bpm'">
                <Input
                  v-model="editingValue"
                  type="number"
                  class="h-7 font-data text-primary text-xs w-16"
                  autofocus
                  @blur="commitEdit(track.id, 'bpm')"
                  @keydown.enter="commitEdit(track.id, 'bpm')"
                  @keydown.esc="cancelEdit"
                  @click.stop
                />
              </template>
              <template v-else>
                <span
                  class="font-data text-primary text-sm"
                  @click.stop="startEdit(track.id, 'bpm', track.bpm)"
                >{{ track.bpm || '—' }}</span>
              </template>
            </TableCell>

            <!-- Key -->
            <TableCell class="py-3">
              <span class="font-data text-on-surface-variant text-sm">
                {{ track.musicalKey || track.key || '—' }}
              </span>
            </TableCell>

            <!-- Actions (three-dot menu, right-aligned) -->
            <TableCell class="text-right py-3">
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <Button
                    variant="ghost"
                    size="icon"
                    class="text-tertiary hover:text-on-surface w-8 h-8"
                    @click.stop
                  >
                    <MoreVertical class="w-4 h-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent class="glass-panel-heavy">
                  <DropdownMenuItem
                    class="font-terminal tracking-terminal text-on-surface text-xs uppercase cursor-pointer"
                    @click="startEdit(track.id, 'title', track.title)"
                  >
                    EDIT_TITLE
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    class="font-terminal tracking-terminal text-on-surface text-xs uppercase cursor-pointer"
                    @click="startEdit(track.id, 'artist', track.artist)"
                  >
                    EDIT_ARTIST
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    class="font-terminal tracking-terminal text-on-surface text-xs uppercase cursor-pointer"
                    @click="startEdit(track.id, 'bpm', track.bpm)"
                  >
                    EDIT_BPM
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>
