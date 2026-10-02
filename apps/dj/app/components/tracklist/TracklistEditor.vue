<script setup lang="ts">
import { nextTick, reactive, ref } from 'vue';
import { Disc3, FlaskConical, Gem, GripVertical, MoreVertical, Plus } from 'lucide-vue-next';
import { addTrack, reorderTracks, updateTrack, uploadTrackArtwork, pollArtworkStatus } from '@/composables/useTracklist';
import { TRACK_MEDIA, type Track } from '@/types/tracklist';
import TrackMediaIcon from '@/components/trackcard/TrackMediaIcon.vue';

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
    case 'failed': return 'accent-bar-failed';
    case 'scheduled': return 'accent-bar-scheduled';
    case 'published': return 'accent-bar-published';
    case 'confirmed': return 'accent-bar-confirmed';
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

const reasonOf = (err: unknown, fallback: string): string => {
  const e = err as { data?: { error?: string }; message?: string };
  return e?.data?.error ?? e?.message ?? fallback;
};

const indexOfTrack = (id: string) => tracks.value.findIndex((t) => t.id === id);

// ── markers: hidden gem, unreleased, media ──
// Toggles apply at once (optimistically) and revert, with the reason, if the
// save fails.
const setMarker = async (
  track: Track,
  patch: Partial<Pick<Track, 'hiddenGem' | 'unreleased' | 'media'>>,
) => {
  if (!tracklist.value) return;
  const index = indexOfTrack(track.id);
  if (index === -1) return;
  // Only the fields this click changes are applied, confirmed and, on failure,
  // put back: another marker on the same track may be saving at the same time,
  // and restoring a whole-track snapshot would undo it.
  const keys = Object.keys(patch) as Array<keyof typeof patch>;
  const pick = (from: Track) => Object.fromEntries(keys.map((k) => [k, from[k]])) as typeof patch;
  const previous = pick(tracks.value[index]!);
  tracks.value[index] = { ...tracks.value[index]!, ...patch };
  uiStore.editError = null;
  try {
    const saved = await updateTrack(tracklist.value.id, track.id, patch);
    const i = indexOfTrack(track.id);
    if (i !== -1) tracks.value[i] = { ...tracks.value[i]!, ...pick(saved) };
  } catch (err) {
    const i = indexOfTrack(track.id);
    if (i !== -1) tracks.value[i] = { ...tracks.value[i]!, ...previous };
    uiStore.editError = reasonOf(err, 'Failed to save track');
  }
};

// ── add a track by hand ──
const adding = ref(false);
const draft = reactive({ title: '', artist: '', bpm: '', musicalKey: '' });
const addError = ref<string | null>(null);
const addSaving = ref(false);
const addTitleInput = ref<HTMLInputElement | null>(null);

const openAdd = async () => {
  adding.value = true;
  addError.value = null;
  await nextTick();
  addTitleInput.value?.focus();
};

const closeAdd = () => {
  adding.value = false;
  addError.value = null;
  Object.assign(draft, { title: '', artist: '', bpm: '', musicalKey: '' });
};

const submitAdd = async () => {
  if (!tracklist.value || addSaving.value) return;
  const title = draft.title.trim();
  if (!title) {
    addError.value = 'A track needs a title';
    return;
  }
  const bpm = draft.bpm.trim() === '' ? undefined : Number(draft.bpm);
  if (bpm !== undefined && (!Number.isFinite(bpm) || bpm < 0 || bpm > 999)) {
    addError.value = 'BPM must be a number up to 999';
    return;
  }
  addSaving.value = true;
  addError.value = null;
  try {
    const created = await addTrack(tracklist.value.id, {
      title,
      artist: draft.artist.trim(),
      bpm,
      musicalKey: draft.musicalKey.trim(),
    });
    tracks.value = [...tracks.value, created];
    // Stay open for the next one: DJs add several in a row.
    Object.assign(draft, { title: '', artist: '', bpm: '', musicalKey: '' });
    await nextTick();
    addTitleInput.value?.focus();
  } catch (err) {
    addError.value = reasonOf(err, 'Could not add the track');
  } finally {
    addSaving.value = false;
  }
};

// ── reorder: drag by the handle, or Alt+Up / Alt+Down on it ──
const draggingId = ref<string | null>(null);
const dropTarget = ref<{ id: string; after: boolean } | null>(null);
const reordering = ref(false);

const applyOrder = async (next: Track[]) => {
  if (!tracklist.value || reordering.value) return;
  const previous = tracks.value;
  tracks.value = next;
  uiStore.editError = null;
  reordering.value = true;
  try {
    const saved = await reorderTracks(tracklist.value.id, next.map((t) => t.id));
    const position = new Map(saved.map((t) => [t.id, t.position]));
    tracks.value = tracks.value.map((t) => (position.has(t.id) ? { ...t, position: position.get(t.id)! } : t));
  } catch (err) {
    tracks.value = previous;
    uiStore.editError = reasonOf(err, 'Could not save the new order');
  } finally {
    reordering.value = false;
  }
};

const moveTo = (fromId: string, targetId: string, after: boolean) => {
  if (fromId === targetId) return;
  const list = [...tracks.value];
  const from = list.findIndex((t) => t.id === fromId);
  if (from === -1) return;
  const [item] = list.splice(from, 1);
  let to = list.findIndex((t) => t.id === targetId);
  if (to === -1) return;
  if (after) to += 1;
  list.splice(to, 0, item!);
  if (list.every((t, i) => t.id === tracks.value[i]!.id)) return;
  return applyOrder(list);
};

const moveBy = async (id: string, delta: -1 | 1) => {
  const from = indexOfTrack(id);
  const to = from + delta;
  if (from === -1 || to < 0 || to >= tracks.value.length) return;
  const list = [...tracks.value];
  [list[from], list[to]] = [list[to]!, list[from]!];
  await applyOrder(list);
  await nextTick();
  document.querySelector<HTMLElement>(`[data-handle="${id}"]`)?.focus();
};

const clearDrag = () => {
  draggingId.value = null;
  dropTarget.value = null;
};

const onDragStart = (e: DragEvent, id: string) => {
  draggingId.value = id;
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', id);
    const row = (e.target as HTMLElement).closest('tr');
    if (row) e.dataTransfer.setDragImage(row, 24, 24);
  }
};

const onDragOver = (e: DragEvent, id: string) => {
  if (!draggingId.value) return;
  e.preventDefault();
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
  dropTarget.value = { id, after: e.clientY > rect.top + rect.height / 2 };
};

const onDrop = async (e: DragEvent) => {
  e.preventDefault();
  const target = dropTarget.value;
  const from = draggingId.value;
  clearDrag();
  if (target && from) await moveTo(from, target.id, target.after);
};

// A line on the edge the dragged track will land on.
const dropIndicator = (id: string): Record<string, string> => {
  const t = dropTarget.value;
  if (!t || t.id !== id || draggingId.value === id) return {};
  return { boxShadow: `inset 0 ${t.after ? '-2px' : '2px'} 0 0 var(--color-primary)` };
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
      <div class="flex items-center gap-3">
        <div
          v-if="editLoading || reordering"
          class="font-terminal tracking-terminal text-tertiary text-xs uppercase"
          role="status"
        >
          SAVING...
        </div>
        <button
          type="button"
          class="btn-hud btn-hud-ghost btn-hud-xs"
          :aria-expanded="adding"
          aria-controls="add-track-form"
          data-testid="add-track-toggle"
          @click="adding ? closeAdd() : openAdd()"
        >
          <Plus class="w-3.5 h-3.5" aria-hidden="true" /> ADD_TRACK
        </button>
      </div>
    </div>

    <!-- Add a track by hand: stays open for the next one -->
    <form
      v-if="adding"
      id="add-track-form"
      class="glass-panel p-3 space-y-3"
      data-testid="add-track-form"
      @submit.prevent="submitAdd"
    >
      <div class="grid gap-3" style="grid-template-columns: 2fr 2fr 1fr 1fr;">
        <div>
          <label class="section-lbl" style="display:block;margin-bottom:6px;" for="add-track-title">TITLE</label>
          <input
            id="add-track-title"
            ref="addTitleInput"
            v-model="draft.title"
            class="hud-input"
            style="width:100%;"
            maxlength="200"
            required
            data-testid="add-track-title"
          >
        </div>
        <div>
          <label class="section-lbl" style="display:block;margin-bottom:6px;" for="add-track-artist">ARTIST</label>
          <input
            id="add-track-artist"
            v-model="draft.artist"
            class="hud-input"
            style="width:100%;"
            maxlength="200"
            data-testid="add-track-artist"
          >
        </div>
        <div>
          <label class="section-lbl" style="display:block;margin-bottom:6px;" for="add-track-bpm">BPM</label>
          <input
            id="add-track-bpm"
            v-model="draft.bpm"
            class="hud-input"
            style="width:100%;"
            inputmode="decimal"
            data-testid="add-track-bpm"
          >
        </div>
        <div>
          <label class="section-lbl" style="display:block;margin-bottom:6px;" for="add-track-key">KEY</label>
          <input
            id="add-track-key"
            v-model="draft.musicalKey"
            class="hud-input"
            style="width:100%;"
            maxlength="10"
            data-testid="add-track-key"
          >
        </div>
      </div>
      <p
        v-if="addError"
        role="alert"
        class="font-terminal tracking-terminal text-error text-xs uppercase"
        data-testid="add-track-error"
      >
        {{ addError }}
      </p>
      <div class="flex items-center gap-2">
        <button type="submit" class="btn-hud btn-hud-cta btn-hud-xs" :disabled="addSaving" data-testid="add-track-submit">
          {{ addSaving ? 'ADDING…' : 'ADD TRACK' }}
        </button>
        <button type="button" class="btn-hud btn-hud-ghost btn-hud-xs" data-testid="add-track-done" @click="closeAdd">
          DONE
        </button>
      </div>
    </form>

    <!-- Warnings -->
    <div
      v-if="warnings && warnings.length > 0"
      class="p-3 border border-dashed border-error/30 shadow-glow-error space-y-1"
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
      class="p-3 border border-dashed border-error/30 shadow-glow-error"
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
            <TableHead class="w-8"><span class="sr-only">Reorder</span></TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-8">#</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-10">ART</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase px-2">TITLE · ARTIST</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-14 px-2">BPM</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-14 px-2">KEY</TableHead>
            <TableHead class="font-terminal tracking-terminal text-tertiary text-xs uppercase w-28 px-2">MARKS</TableHead>
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
              draggingId === track.id ? 'opacity-40' : '',
              'cursor-pointer transition-colors',
            ]"
            :style="dropIndicator(track.id)"
            @click="selectRow(track.id)"
            @dragover="onDragOver($event, track.id)"
            @drop="onDrop"
          >
            <!-- Drag handle: drag it, or focus it and press Alt+Up / Alt+Down -->
            <TableCell class="py-3 pl-2 pr-0">
              <button
                type="button"
                draggable="true"
                class="drag-handle"
                :data-handle="track.id"
                :aria-label="`Reorder ${track.title || 'track'}: drag, or press Alt and the up or down arrow`"
                title="Drag to reorder (Alt + Up / Down)"
                @click.stop
                @dragstart="onDragStart($event, track.id)"
                @dragend="clearDrag"
                @keydown.alt.up.prevent="moveBy(track.id, -1)"
                @keydown.alt.down.prevent="moveBy(track.id, 1)"
              >
                <GripVertical class="w-4 h-4" aria-hidden="true" />
              </button>
            </TableCell>

            <!-- Position -->
            <TableCell class="font-data text-tertiary text-xs py-3 px-2">
              {{ index + 1 }}
            </TableCell>

            <!-- Cover art -->
            <TableCell class="py-3 px-2">
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
                  >
                </label>
              </div>
            </TableCell>

            <!-- Title over artist, both inline-editable. One cell, so the table
                 keeps room for the marks at the width of the editor column. -->
            <TableCell class="py-3 px-2 w-full max-w-0">
              <template v-if="editingCell?.trackId === track.id && editingCell.field === 'title'">
                <Input
                  v-model="editingValue"
                  class="h-7 font-data text-on-surface text-sm"
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
              <template v-if="editingCell?.trackId === track.id && editingCell.field === 'artist'">
                <Input
                  v-model="editingValue"
                  class="h-7 font-data text-on-surface text-sm mt-1"
                  autofocus
                  @blur="commitEdit(track.id, 'artist')"
                  @keydown.enter="commitEdit(track.id, 'artist')"
                  @keydown.esc="cancelEdit"
                  @click.stop
                />
              </template>
              <template v-else>
                <span
                  class="font-data text-tertiary text-xs block truncate"
                  @click.stop="startEdit(track.id, 'artist', track.artist)"
                >{{ track.artist || '—' }}</span>
              </template>
            </TableCell>

            <!-- BPM (inline edit) -->
            <TableCell class="py-3 px-2">
              <template v-if="editingCell?.trackId === track.id && editingCell.field === 'bpm'">
                <Input
                  v-model="editingValue"
                  type="number"
                  class="h-7 font-data text-on-surface text-sm w-16"
                  autofocus
                  @blur="commitEdit(track.id, 'bpm')"
                  @keydown.enter="commitEdit(track.id, 'bpm')"
                  @keydown.esc="cancelEdit"
                  @click.stop
                />
              </template>
              <template v-else>
                <span
                  class="font-data text-on-surface text-sm"
                  @click.stop="startEdit(track.id, 'bpm', track.bpm)"
                >{{ track.bpm || '—' }}</span>
              </template>
            </TableCell>

            <!-- Key (inline edit) -->
            <TableCell class="py-3 px-2">
              <template v-if="editingCell?.trackId === track.id && editingCell.field === 'musicalKey'">
                <Input
                  v-model="editingValue"
                  class="h-7 font-data text-on-surface text-sm w-16"
                  maxlength="10"
                  autofocus
                  @blur="commitEdit(track.id, 'musicalKey')"
                  @keydown.enter="commitEdit(track.id, 'musicalKey')"
                  @keydown.esc="cancelEdit"
                  @click.stop
                />
              </template>
              <template v-else>
                <span
                  class="font-data text-on-surface-variant text-sm"
                  @click.stop="startEdit(track.id, 'musicalKey', track.musicalKey || track.key)"
                >{{ track.musicalKey || track.key || '—' }}</span>
              </template>
            </TableCell>

            <!-- Marks: hidden gem, unreleased, played-from (vinyl or digital) -->
            <TableCell class="py-3 px-2" @click.stop>
              <div class="flex items-center gap-1" role="group" aria-label="Track marks">
                <button
                  type="button"
                  class="mark-btn"
                  :aria-pressed="!!track.hiddenGem"
                  aria-label="Hidden gem"
                  title="Hidden gem: the card hides this track's title, artist and cover"
                  data-testid="mark-hidden-gem"
                  @click="setMarker(track, { hiddenGem: !track.hiddenGem })"
                >
                  <Gem class="w-4 h-4" aria-hidden="true" />
                </button>
                <button
                  type="button"
                  class="mark-btn"
                  :aria-pressed="!!track.unreleased"
                  aria-label="Unreleased"
                  title="Unreleased: the card tags this track UNRELEASED"
                  data-testid="mark-unreleased"
                  @click="setMarker(track, { unreleased: !track.unreleased })"
                >
                  <FlaskConical class="w-4 h-4" aria-hidden="true" />
                </button>
                <DropdownMenu>
                  <DropdownMenuTrigger as-child>
                    <button
                      type="button"
                      class="mark-btn"
                      :aria-pressed="!!track.media"
                      :aria-label="`Played from: ${track.media ? track.media.toUpperCase() : 'not set'}`"
                      title="Played from: vinyl or a digital file"
                      data-testid="mark-media"
                    >
                      <TrackMediaIcon v-if="track.media" :media="track.media" :size="16" decorative />
                      <Disc3 v-else class="w-4 h-4" aria-hidden="true" />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent class="glass-panel-heavy">
                    <DropdownMenuItem
                      v-for="m in TRACK_MEDIA"
                      :key="m.value"
                      class="font-terminal tracking-terminal text-on-surface text-xs uppercase cursor-pointer gap-2"
                      :data-testid="`media-${m.value}`"
                      @click="setMarker(track, { media: m.value })"
                    >
                      <TrackMediaIcon :media="m.value" :size="14" decorative /> {{ m.label }}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      class="font-terminal tracking-terminal text-tertiary text-xs uppercase cursor-pointer"
                      data-testid="media-none"
                      @click="setMarker(track, { media: '' })"
                    >
                      NOT_SET
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </TableCell>

            <!-- Actions (three-dot menu, right-aligned) -->
            <TableCell class="text-right py-3 px-2">
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
          <TableRow v-if="!tracks.length" class="hover:bg-transparent">
            <TableCell colspan="8" class="py-6 text-center">
              <span class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
                NO TRACKS YET · USE ADD_TRACK
              </span>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>

<style scoped>
/* Marks and the drag handle follow the HUD finish: dashed 1px hairlines, the
   signature (primary) only on what is switched on, a visible keyboard focus. */
.mark-btn,
.drag-handle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px dashed transparent;
  background: transparent;
  color: var(--color-tertiary);
  transition: color 0.15s, border-color 0.15s, background-color 0.15s;
}
.mark-btn:hover,
.drag-handle:hover {
  color: var(--color-on-surface);
  border-color: color-mix(in srgb, var(--color-outline-variant) 60%, transparent);
}
.mark-btn:active {
  background: color-mix(in srgb, var(--color-primary) 18%, transparent);
}
.mark-btn:focus-visible,
.drag-handle:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
.mark-btn[aria-pressed='true'] {
  color: var(--color-primary);
  border-color: color-mix(in srgb, var(--color-primary) 55%, transparent);
  background: color-mix(in srgb, var(--color-primary) 10%, transparent);
}
.drag-handle {
  cursor: grab;
}
.drag-handle:active {
  cursor: grabbing;
}
</style>
