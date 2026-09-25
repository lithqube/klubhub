<script setup lang="ts">
// Read-only view of a Party snapshot (issued invoices, credit notes).
import { computed } from 'vue'
import type { Party } from '../../types/finance'

const props = defineProps<{ party: Party; label: string }>()

const lines = computed(() => {
  const p = props.party
  const cityLine = [p.postal_code, p.city].filter(Boolean).join(' ')
  return [
    p.company && p.legal_name && p.company !== p.legal_name ? p.legal_name : '',
    p.address_line1,
    p.address_line2,
    [cityLine, p.region].filter(Boolean).join(', '),
    p.country,
    p.vat_id ? `VAT ID ${p.vat_id}` : '',
    p.tax_id ? `Tax ID ${p.tax_id}` : '',
    p.email,
  ].filter(Boolean)
})
</script>

<template>
  <section class="ps">
    <h3 class="section-lbl ps-lbl">{{ label }}</h3>
    <p class="ps-name">{{ party.company || party.legal_name || '—' }}</p>
    <p v-for="(l, i) in lines" :key="i" class="ps-line">{{ l }}</p>
  </section>
</template>

<style scoped>
.ps { display: flex; flex-direction: column; gap: 2px; }
.ps-lbl { margin: 0 0 4px; font-weight: 600; }
.ps-name { margin: 0; font-family: var(--font-command); font-size: 13px; font-weight: 700; letter-spacing: -.02em; text-transform: uppercase; color: var(--color-on-surface); }
.ps-line { margin: 0; font-family: var(--font-data); font-size: 12px; color: var(--color-on-surface-variant); }
</style>
