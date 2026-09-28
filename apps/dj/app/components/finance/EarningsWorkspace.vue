<script setup lang="ts">
// Earnings workspace mount point: the create/edit dialog. Mirrors
// `InvoiceWorkspace.vue` so the page just renders `<EarningsWorkspace />`
// once and uses the earnings store from anywhere. The store owns the saved
// entry upsert via createEntry / updateEntry; this component only forwards
// the emit so the page can decide whether to toast / refresh.
import { storeToRefs } from 'pinia'
import { useEarningsStore } from '../../stores/earnings'
import EntryFormDialog from './EntryFormDialog.vue'

const store = useEarningsStore()
const { createOpen, createKind, createPresetGigId, editId } = storeToRefs(store)
</script>

<template>
  <EntryFormDialog
    :open="createOpen"
    :kind="createKind"
    :gig-id="createPresetGigId"
    :edit-id="editId"
    @update:open="store.setCreateOpen"
  />
</template>
