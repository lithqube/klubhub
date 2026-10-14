<script setup lang="ts">
// "Download PDF": a plain link to GET /api/v1/finance/invoices/{id}/pdf, which
// answers with an attachment, so the browser streams it to disk and the file
// never passes through JS memory. The one exception is the browser-only demo,
// whose API is a fetch interceptor that links cannot reach: there the PDF is
// fetched from it and opened as a blob.
import type { Invoice } from '../../types/finance'
import { invoicePdfUrl } from '../../stores/invoice'
import { invoiceNumberLabel } from '../../utils/invoiceDisplay'
import { openPdf } from '../../utils/openPdf'

const props = defineProps<{ invoice: Invoice }>()

function isDemo(): boolean {
  try {
    return !!(useRuntimeConfig().public as Record<string, unknown>).demo
  } catch {
    return false // outside a Nuxt app (unit tests)
  }
}

const filename = () => `${invoiceNumberLabel(props.invoice).replace(/[^A-Za-z0-9._-]+/g, '-')}.pdf`

async function onClick(e: MouseEvent): Promise<void> {
  if (!isDemo()) return
  e.preventDefault()
  const blob = await $fetch<Blob>(invoicePdfUrl(props.invoice.id), { responseType: 'blob' })
  const url = URL.createObjectURL(blob)
  openPdf(url, filename())
  setTimeout(() => URL.revokeObjectURL(url), 10_000)
}
</script>

<template>
  <a
    class="btn-hud btn-hud-ghost btn-hud-sm pdf-link"
    :href="invoicePdfUrl(invoice.id)"
    :download="filename()"
    rel="noopener"
    :aria-label="`Download PDF for ${invoiceNumberLabel(invoice)}`"
    @click="onClick"
  >
    DOWNLOAD PDF
  </a>
</template>

<style scoped>
.pdf-link { display: inline-flex; align-items: center; text-decoration: none; }
@media (max-width: 768px) { .pdf-link { min-height: 44px; } }
</style>
