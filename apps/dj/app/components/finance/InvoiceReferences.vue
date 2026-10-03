<script setup lang="ts">
// Read-only EN 16931 references and payment terms of a non-draft invoice.
// Renders nothing when none is set.
import { computed } from 'vue'
import type { Invoice } from '../../types/finance'

const props = defineProps<{ invoice: Invoice }>()

const rows = computed(() => [
  { label: 'BUYER REFERENCE (LEITWEG-ID)', value: props.invoice.buyer_reference },
  { label: 'PURCHASE ORDER', value: props.invoice.purchase_order_ref },
  { label: 'CONTRACT REFERENCE', value: props.invoice.contract_ref },
  { label: 'PAYMENT TERMS', value: props.invoice.payment_terms },
].filter((r) => !!r.value))
</script>

<template>
  <section v-if="rows.length" class="refs" aria-labelledby="inv-refs-title">
    <h3 id="inv-refs-title" class="section-lbl refs-title">REFERENCES AND TERMS</h3>
    <dl class="refs-list">
      <div v-for="r in rows" :key="r.label" class="refs-row">
        <dt class="section-lbl">{{ r.label }}</dt>
        <dd>{{ r.value }}</dd>
      </div>
    </dl>
  </section>
</template>

<style scoped>
.refs { display: flex; flex-direction: column; gap: 6px; }
.refs-title { margin: 0; font-weight: 600; }
.refs-list { margin: 0; display: flex; flex-direction: column; gap: 6px; }
.refs-row { display: flex; flex-direction: column; gap: 2px; }
.refs-row dt { margin: 0; }
.refs-row dd { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface); overflow-wrap: anywhere; white-space: pre-wrap; }
</style>
