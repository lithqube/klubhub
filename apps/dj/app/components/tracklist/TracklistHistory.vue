<script setup lang="ts">
import { onMounted } from 'vue';
import { listTracklists, getTracklist, deleteTracklist } from '@/composables/useTracklist';

const tracklistStore = useTracklistStore();
const uiStore = useUiStore();
const { pastTracklists } = storeToRefs(tracklistStore);
const { pastTracklistsLoading, pastTracklistsError } = storeToRefs(uiStore);

// Dialog state for delete confirmation
const showDeleteDialog = ref(false);
const pendingDeleteId = ref<string | null>(null);
const pendingDeleteTitle = ref<string>('');

const loadPastTracklists = async () => {
  uiStore.pastTracklistsLoading = true;
  uiStore.pastTracklistsError = null;
  try {
    const data = await listTracklists();
    tracklistStore.pastTracklists = data;
  } catch (err: unknown) {
    const error = err as { message?: string };
    uiStore.pastTracklistsError = error.message ?? 'Failed to load past tracklists';
  } finally {
    uiStore.pastTracklistsLoading = false;
  }
};

const openTracklist = async (id: string) => {
  try {
    const result = await getTracklist(id);
    tracklistStore.tracklist = result.tracklist;
    tracklistStore.tracks = result.tracks;
    tracklistStore.warnings = [];
    uiStore.step = 'edit';
  } catch (err: unknown) {
    const error = err as { message?: string };
    uiStore.pastTracklistsError = error.message ?? 'Failed to load tracklist';
  }
};

const confirmDelete = (id: string, title: string) => {
  pendingDeleteId.value = id;
  pendingDeleteTitle.value = title;
  showDeleteDialog.value = true;
};

const performDelete = async () => {
  if (!pendingDeleteId.value) return;
  try {
    await deleteTracklist(pendingDeleteId.value);
    await loadPastTracklists();
  } catch (err: unknown) {
    const error = err as { message?: string };
    uiStore.pastTracklistsError = error.message ?? 'Failed to delete tracklist';
  } finally {
    showDeleteDialog.value = false;
    pendingDeleteId.value = null;
    pendingDeleteTitle.value = '';
  }
};

const cancelDelete = () => {
  showDeleteDialog.value = false;
  pendingDeleteId.value = null;
  pendingDeleteTitle.value = '';
};

onMounted(() => {
  loadPastTracklists();
});
</script>

<template>
  <div class="space-y-4">
    <!-- Section header -->
    <div class="flex items-center justify-between">
      <h2 class="font-command tracking-command text-on-surface text-sm uppercase font-bold">
        PAST_TRACKLISTS
      </h2>
      <Button
        variant="ghost"
        size="sm"
        class="font-terminal tracking-terminal text-tertiary text-xs uppercase"
        @click="loadPastTracklists"
      >
        REFRESH
      </Button>
    </div>

    <!-- Loading state -->
    <div v-if="pastTracklistsLoading" class="text-center py-8">
      <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
        SYSTEM_PROCESS: LOADING_TRACKLISTS...
      </p>
    </div>

    <!-- Error state -->
    <div
      v-else-if="pastTracklistsError"
      class="p-3 ghost-border shadow-glow-error"
    >
      <p class="font-terminal tracking-terminal text-error text-xs uppercase">
        ERROR: {{ pastTracklistsError }}
      </p>
    </div>

    <!-- Empty state -->
    <div
      v-else-if="!pastTracklists || pastTracklists.length === 0"
      class="ghost-border p-8 text-center"
    >
      <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
        NO_TRACKLISTS_FOUND
      </p>
      <p class="font-data text-tertiary text-xs mt-1">
        Upload a tracklist to get started
      </p>
    </div>

    <!-- Tracklists table -->
    <div v-else class="overflow-hidden">
      <Table>
        <TableBody>
          <TableRow
            v-for="(item, index) in pastTracklists"
            :key="item.id"
            :class="[
              'accent-bar-ready cursor-pointer',
              index % 2 === 0 ? 'bg-surface-container' : 'bg-surface-container-low',
              'hover:bg-surface-bright transition-colors',
            ]"
          >
            <TableCell class="py-3">
              <div class="space-y-0.5">
                <p class="font-command text-on-surface text-sm font-medium">
                  {{ item.title || item.filename || 'Untitled' }}
                </p>
                <p class="font-terminal tracking-terminal text-tertiary text-xs uppercase">
                  {{ item.trackCount || item.track_count || 0 }} TRACKS
                  <span v-if="item.createdAt || item.created_at" class="ml-2">
                    &bull;
                    {{ new Date(item.createdAt || item.created_at).toLocaleDateString() }}
                  </span>
                </p>
              </div>
            </TableCell>
            <TableCell class="text-right py-3 space-x-2">
              <Button
                variant="ghost"
                size="sm"
                class="font-terminal tracking-terminal text-primary text-xs uppercase"
                @click="openTracklist(item.id)"
              >
                OPEN
              </Button>
              <Button
                variant="ghost"
                size="sm"
                class="font-terminal tracking-terminal text-error text-xs uppercase"
                @click="confirmDelete(item.id, item.title || item.filename || 'Untitled')"
              >
                DELETE
              </Button>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>

    <!-- Delete confirmation dialog -->
    <Dialog :open="showDeleteDialog" @update:open="(v) => { if (!v) cancelDelete() }">
      <DialogContent class="glass-panel-heavy">
        <DialogHeader>
          <DialogTitle class="font-command tracking-command text-on-surface uppercase">
            CONFIRM_DELETE
          </DialogTitle>
          <DialogDescription class="font-data text-tertiary text-sm">
            Delete "{{ pendingDeleteTitle }}"? This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter class="gap-2">
          <Button
            variant="ghost"
            class="font-terminal tracking-terminal text-tertiary text-xs uppercase"
            @click="cancelDelete"
          >
            CANCEL
          </Button>
          <Button
            variant="default"
            class="font-terminal tracking-terminal text-error text-xs uppercase ghost-border shadow-glow-error"
            @click="performDelete"
          >
            DELETE
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
