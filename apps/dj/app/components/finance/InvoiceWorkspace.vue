<script setup lang="ts">
// Mount point for the invoice create dialog and detail sheet. Pages that
// offer invoice entry points (finance, gigs) render this once; the store
// holds which dialog is open, so buttons anywhere just call the store.
import { storeToRefs } from 'pinia'
import type { Invoice } from '../../types/finance'
import { useInvoiceStore } from '../../stores/invoice'
import { useToast } from '../ui/toast/use-toast'
import InvoiceCreateDialog from './InvoiceCreateDialog.vue'
import InvoiceDetailSheet from './InvoiceDetailSheet.vue'

const store = useInvoiceStore()
const { createOpen, createPresetGigId } = storeToRefs(store)
const { toast } = useToast()

function onCreated(inv: Invoice): void {
  toast({ title: 'Draft created', description: 'Review the customer and tax, then issue it. Nothing was sent.' })
  // Open after the create dialog has closed so focus lands in the sheet.
  setTimeout(() => { void store.openDetail(inv.id) }, 80)
}
</script>

<template>
  <InvoiceCreateDialog
    :open="createOpen"
    :preset-gig-id="createPresetGigId"
    @update:open="store.setCreateOpen"
    @created="onCreated"
  />
  <InvoiceDetailSheet />
</template>
