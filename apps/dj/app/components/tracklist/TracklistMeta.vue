<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { Pencil, X } from 'lucide-vue-next';
import { renameTracklist, listLinkedGigs, type LinkedGig } from '@/composables/useTracklist';
import type { Gig } from '@/types/gig';

// Name and gig links of the open tracklist. Renaming goes through
// PUT /tracklists/{id}; linking uses the gig endpoints (a tracklist can be
// played at several gigs, so links are chips plus an "add" select).

const tracklistStore = useTracklistStore();
const gigStore = useGigStore();
const { tracklist } = storeToRefs(tracklistStore);
const { gigs } = storeToRefs(gigStore);

const reasonOf = (e: unknown, fallback: string): string => {
  const err = e as { data?: { error?: string; message?: string }; message?: string };
  return err?.data?.message ?? err?.data?.error ?? err?.message ?? fallback;
};

// ── rename ──
const editing = ref(false);
const draft = ref('');
const saving = ref(false);
const renameError = ref<string | null>(null);
const titleInput = ref<HTMLInputElement | null>(null);

const startEdit = async () => {
  if (!tracklist.value) return;
  draft.value = tracklist.value.title;
  renameError.value = null;
  editing.value = true;
  await nextTick();
  titleInput.value?.focus();
  titleInput.value?.select();
};

const cancelEdit = () => {
  editing.value = false;
  renameError.value = null;
};

const commitEdit = async () => {
  if (!editing.value || saving.value || !tracklist.value) return;
  const next = draft.value.trim();
  if (!next) {
    renameError.value = 'Name cannot be empty';
    return;
  }
  if (next === tracklist.value.title) {
    editing.value = false;
    return;
  }
  saving.value = true;
  renameError.value = null;
  try {
    const updated = await renameTracklist(tracklist.value.id, next);
    tracklist.value = { ...tracklist.value, title: updated.title, updatedAt: updated.updatedAt };
    // Keep the "recent imports" list in step so going back shows the new name.
    const row = tracklistStore.pastTracklists.find(t => t.id === updated.id);
    if (row) row.title = updated.title;
    editing.value = false;
  } catch (e) {
    renameError.value = reasonOf(e, 'Could not rename the tracklist');
  } finally {
    saving.value = false;
  }
};

// ── gig links ──
const linked = ref<LinkedGig[]>([]);
const linkError = ref<string | null>(null);
const linking = ref(false);
const selectedGig = ref('');

const loadLinked = async () => {
  const id = tracklist.value?.id;
  if (!id) {
    linked.value = [];
    return;
  }
  try {
    linked.value = await listLinkedGigs(id);
    linkError.value = null;
  } catch (e) {
    linked.value = [];
    linkError.value = reasonOf(e, 'Could not load linked gigs');
  }
};

watch(() => tracklist.value?.id, loadLinked, { immediate: true });
onMounted(() => {
  if (!gigs.value.length) void gigStore.fetchGigs();
});

const when = (iso: string) =>
  new Date(iso).toLocaleDateString('en-GB', { day: '2-digit', month: 'short', year: 'numeric' });

const linkedLabel = (g: LinkedGig) =>
  `${when(g.date)} · ${g.venue || g.eventName || 'Gig'}${g.city ? `, ${g.city}` : ''}`;
const gigLabel = (g: Gig) =>
  `${when(g.date)} · ${g.venue || g.event_name || 'Gig'}${g.city ? `, ${g.city}` : ''}`;

// Newest first; a gig that is already linked is not offered again.
const linkable = computed(() =>
  [...gigs.value]
    .filter(g => !linked.value.some(l => l.id === g.id))
    .sort((a, b) => b.date.localeCompare(a.date)),
);

const link = async () => {
  const gigId = selectedGig.value;
  const tracklistId = tracklist.value?.id;
  if (!gigId || !tracklistId) return;
  linking.value = true;
  linkError.value = null;
  const ok = await gigStore.linkTracklist(gigId, tracklistId);
  if (ok) await loadLinked();
  else linkError.value = 'Could not link this gig';
  selectedGig.value = '';
  linking.value = false;
};

const unlink = async (gigId: string) => {
  const tracklistId = tracklist.value?.id;
  if (!tracklistId) return;
  linking.value = true;
  linkError.value = null;
  const ok = await gigStore.unlinkTracklist(gigId, tracklistId);
  if (ok) await loadLinked();
  else linkError.value = 'Could not unlink this gig';
  linking.value = false;
};
</script>

<template>
  <div v-if="tracklist" class="space-y-4" data-testid="tracklist-meta">
    <!-- Name -->
    <div>
      <label class="section-lbl" style="display:block;margin-bottom:6px;" for="tracklist-name">NAME</label>
      <div v-if="!editing" class="flex items-center gap-3 min-w-0">
        <h1
          class="font-command tracking-command text-on-surface text-lg uppercase font-bold truncate"
          data-testid="tracklist-title"
        >
          {{ tracklist.title }}
        </h1>
        <button
          type="button"
          class="btn-hud btn-hud-ghost btn-hud-xs shrink-0"
          data-testid="tracklist-rename"
          aria-label="Rename tracklist"
          @click="startEdit"
        >
          <Pencil class="w-3 h-3" aria-hidden="true" /> RENAME
        </button>
      </div>
      <div v-else class="flex items-center gap-2">
        <input
          id="tracklist-name"
          ref="titleInput"
          v-model="draft"
          class="hud-input"
          style="flex:1;min-width:0;"
          maxlength="200"
          :disabled="saving"
          data-testid="tracklist-title-input"
          aria-label="Tracklist name"
          @keydown.enter.prevent="commitEdit"
          @keydown.esc.prevent="cancelEdit"
          @blur="commitEdit"
        >
        <button
          type="button"
          class="btn-hud btn-hud-cta btn-hud-xs"
          :disabled="saving"
          data-testid="tracklist-title-save"
          @mousedown.prevent
          @click="commitEdit"
        >
          {{ saving ? 'SAVING…' : 'SAVE' }}
        </button>
        <button
          type="button"
          class="btn-hud btn-hud-ghost btn-hud-xs"
          :disabled="saving"
          data-testid="tracklist-title-cancel"
          @mousedown.prevent
          @click="cancelEdit"
        >
          CANCEL
        </button>
      </div>
      <p
        v-if="renameError"
        role="alert"
        class="font-terminal tracking-terminal text-error text-xs uppercase mt-2"
        data-testid="tracklist-rename-error"
      >
        {{ renameError }}
      </p>
    </div>

    <!-- Gig links -->
    <div>
      <label class="section-lbl" style="display:block;margin-bottom:6px;" for="tracklist-link-gig">LINKED_GIGS</label>
      <div class="flex flex-wrap items-center gap-2">
        <span
          v-for="g in linked"
          :key="g.id"
          class="social-chip social-chip-cyan"
          data-testid="tracklist-linked-gig"
        >
          {{ linkedLabel(g) }}
          <button
            type="button"
            class="inline-flex"
            :disabled="linking"
            :aria-label="`Unlink ${linkedLabel(g)}`"
            data-testid="tracklist-unlink-gig"
            @click="unlink(g.id)"
          >
            <X class="w-3 h-3" aria-hidden="true" />
          </button>
        </span>
        <span
          v-if="!linked.length"
          class="font-terminal tracking-terminal text-tertiary text-xs uppercase"
          data-testid="tracklist-no-gig"
        >
          NOT LINKED TO A GIG
        </span>
        <select
          id="tracklist-link-gig"
          v-model="selectedGig"
          class="hud-input"
          style="width:auto;min-width:240px;max-width:100%;"
          :disabled="linking || !linkable.length"
          data-testid="tracklist-link-gig"
          @change="link"
        >
          <option value="">
            + LINK_TO_GIG
          </option>
          <option v-for="g in linkable" :key="g.id" :value="g.id">
            {{ gigLabel(g) }}
          </option>
        </select>
      </div>
      <p
        v-if="linkError"
        role="alert"
        class="font-terminal tracking-terminal text-error text-xs uppercase mt-2"
        data-testid="tracklist-link-error"
      >
        {{ linkError }}
      </p>
    </div>
  </div>
</template>
