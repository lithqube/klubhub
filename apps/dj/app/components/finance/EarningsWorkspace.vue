<script setup lang="ts">
// Earnings workspace: the Entries section body. The entry composer is INLINE
// (not a modal): a new entry opens at the top, above the filter toolbar and the
// list, pushing the list down; EDIT opens the same form in place of that row
// while the other rows stay visible. Only one composer is open at a time. The
// earnings store owns open/close (openCreate / openEdit / setCreateOpen) and
// the saved entry upsert via createEntry / updateEntry; this component hosts
// the form, forwards `saved` so the page can refresh totals, and relays the
// list's void / delete requests to the page's confirmations.
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useEarningsStore } from '../../stores/earnings'
import EntryForm from './EntryForm.vue'
import EntryList from './EntryList.vue'
import type { Entry } from '../../types/finance'

const emit = defineEmits<{ saved: [entry: Entry]; void: [entry: Entry]; delete: [entry: Entry] }>()

const store = useEarningsStore()
const { createOpen, createKind, createPresetGigId, editId, composerSeq, filteredEntries, listLoaded, disabled } = storeToRefs(store)

const rowVisible = (id: string | null) =>
  !!id && listLoaded.value && !disabled.value && filteredEntries.value.some((e) => e.id === id)

// Where an edit opens is decided when it opens and then kept: the form takes
// its row's place when the list is showing that row, and otherwise (list not
// loaded yet, row filtered out) goes to the top so it is never invisible. A
// list refresh must not move it, because moving remounts the form and would
// drop the draft in it.
const placedInRow = ref(false)
watch([createOpen, editId], () => {
  placedInRow.value = createOpen.value && rowVisible(editId.value)
}, { immediate: true, flush: 'sync' })

// If the row disappears from under an open edit (a filter change), fall back to the top.
const inPlace = computed(() => placedInRow.value && rowVisible(editId.value))
const topOpen = computed(() => createOpen.value && !inPlace.value)
</script>

<template>
  <div class="ew">
    <EntryForm
      v-if="topOpen"
      :kind="createKind"
      :gig-id="createPresetGigId"
      :edit-id="editId"
      :focus-seq="composerSeq"
      @close="store.setCreateOpen(false)"
      @saved="emit('saved', $event)"
    />
    <EntryList
      :editing-id="inPlace ? editId : null"
      @edit="store.openEdit($event.id)"
      @void="emit('void', $event)"
      @delete="emit('delete', $event)"
    >
      <template #edit="{ entry }">
        <EntryForm
          :kind="entry.kind"
          :edit-id="entry.id"
          :focus-seq="composerSeq"
          @close="store.setCreateOpen(false)"
          @saved="emit('saved', $event)"
        />
      </template>
    </EntryList>
  </div>
</template>

<style scoped>
/* Same rhythm as the list on its own: 8px between the composer and the toolbar. */
.ew { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
</style>
