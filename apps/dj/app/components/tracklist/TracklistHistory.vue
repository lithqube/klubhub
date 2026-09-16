<script setup lang="ts">
import { onMounted } from 'vue';
import { listTracklists, getTracklist, deleteTracklist } from '@/composables/useTracklist';

const tracklistStore = useTracklistStore();
const uiStore = useUiStore();
const { pastTracklists } = storeToRefs(tracklistStore);
const { pastTracklistsLoading, pastTracklistsError } = storeToRefs(uiStore);

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
    <div style="display:flex;align-items:center;justify-content:space-between;">
      <div class="section-lbl">RECENT IMPORTS</div>
      <button class="btn-hud btn-hud-ghost btn-hud-xs" style="padding:0 10px;" @click="loadPastTracklists">
        REFRESH
      </button>
    </div>

    <!-- Loading state -->
    <div v-if="pastTracklistsLoading" class="glass" style="padding:14px 16px;">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px;">
        <span class="spost-meta">SYSTEM_PROCESS: LOADING_TRACKLISTS</span>
      </div>
      <div class="prog-track">
        <div class="prog-fill prog-fill-anim" style="width:60%;" />
      </div>
    </div>

    <!-- Error state -->
    <div
      v-else-if="pastTracklistsError"
      style="padding:12px 14px;border:1px dashed rgba(255,113,108,.3);box-shadow:0 0 16px rgba(255,113,108,.15);"
    >
      <span class="spost-meta" style="color:var(--color-error);">
        <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/></svg>
        ERROR: {{ pastTracklistsError }}
      </span>
    </div>

    <!-- Empty state -->
    <div
      v-else-if="!pastTracklists || pastTracklists.length === 0"
      class="glass"
      style="padding:32px 24px;text-align:center;border:1px dashed rgba(150,248,255,.15);"
    >
      <div style="font-family:var(--font-terminal);font-size:8px;letter-spacing:.06em;text-transform:uppercase;color:var(--color-tertiary);">
        NO_TRACKLISTS_FOUND
      </div>
      <div style="font-family:var(--font-data);font-size:11px;color:var(--color-tertiary);margin-top:4px;">
        Upload a tracklist to get started
      </div>
    </div>

    <!-- Tracklist list -->
    <div v-else class="glass" style="overflow:hidden;">
      <div
        v-for="item in pastTracklists"
        :key="item.id"
        class="accent-bar-ready"
        style="display:flex;align-items:center;gap:10px;padding:11px 14px;border-bottom:1px solid rgba(46,46,49,.15);cursor:pointer;"
        @click="openTracklist(item.id)"
      >
        <!-- Info -->
        <div style="flex:1;min-width:0;">
          <div style="font-family:var(--font-command);font-size:12px;font-weight:600;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">
            {{ item.title || item.filename || 'Untitled' }}
          </div>
          <div class="spost-meta" style="margin-top:2px;">
            <span class="data-frag" style="margin-right:4px;">{{ item.trackCount || item.track_count || 0 }} TRK</span>
            <span v-if="item.createdAt || item.created_at">
              {{ new Date(item.createdAt || item.created_at).toLocaleDateString('en-GB', { day:'2-digit', month:'short', year:'numeric' }).toUpperCase() }}
            </span>
          </div>
        </div>

        <!-- Actions -->
        <div style="display:flex;gap:6px;flex-shrink:0;" @click.stop>
          <button
            class="btn-hud btn-hud-ghost btn-hud-xs"
            @click="openTracklist(item.id)"
          >
            OPEN
          </button>
          <button
            class="btn-hud btn-hud-xs"
            style="border:1px dashed rgba(255,113,108,.4);color:var(--color-error);"
            @click="confirmDelete(item.id, item.title || item.filename || 'Untitled')"
          >
            DEL
          </button>
        </div>
      </div>
    </div>

    <!-- Delete confirmation overlay -->
    <Teleport to="body">
      <div
        v-if="showDeleteDialog"
        style="position:fixed;inset:0;z-index:9999;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.6);backdrop-filter:blur(4px);"
        @click.self="cancelDelete"
      >
        <div class="glass hud-card" style="width:360px;padding:24px 20px;">
          <div style="font-family:var(--font-command);font-size:13px;font-weight:700;color:var(--color-on-surface);text-transform:uppercase;letter-spacing:-.02em;margin-bottom:8px;">
            CONFIRM DELETE
          </div>
          <div style="font-family:var(--font-data);font-size:12px;color:var(--color-tertiary);margin-bottom:20px;">
            Delete "{{ pendingDeleteTitle }}"? This action cannot be undone.
          </div>
          <div style="display:flex;gap:8px;justify-content:flex-end;">
            <button class="btn-hud btn-hud-ghost" style="padding:0 14px;" @click="cancelDelete">
              CANCEL
            </button>
            <button
              class="btn-hud"
              style="padding:0 14px;border:1px dashed rgba(255,113,108,.4);color:var(--color-error);box-shadow:0 0 12px rgba(255,113,108,.15);"
              @click="performDelete"
            >
              DELETE
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
