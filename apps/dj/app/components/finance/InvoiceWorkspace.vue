<script setup lang="ts">
// Mount point for the invoice detail sheet. Pages that open an invoice (finance,
// gigs) render this once; the store holds which invoice is open, so buttons
// anywhere just call the store. Creating an invoice is not here: it is an
// inline form on the finance page (InvoiceCreateForm), and the gig entry points
// send you there.
import InvoiceDetailSheet from './InvoiceDetailSheet.vue'
</script>

<template>
  <InvoiceDetailSheet />
</template>
