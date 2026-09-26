<script setup lang="ts">
// Overflow menu of state-changing actions for one invoice. Only actions the
// current status allows are listed; the menu is hidden when there are none.
import { computed } from 'vue'
import { MoreVertical } from 'lucide-vue-next'
import type { Invoice, InvoiceConfirmAction } from '../../types/finance'
import { invoiceNumberLabel } from '../../utils/invoiceDisplay'
import DropdownMenu from '#kui/components/ui/dropdown-menu/DropdownMenu.vue'
import DropdownMenuTrigger from '#kui/components/ui/dropdown-menu/DropdownMenuTrigger.vue'
import DropdownMenuContent from '#kui/components/ui/dropdown-menu/DropdownMenuContent.vue'
import DropdownMenuItem from '#kui/components/ui/dropdown-menu/DropdownMenuItem.vue'

const props = defineProps<{ invoice: Invoice }>()
const emit = defineEmits<{ action: [action: InvoiceConfirmAction] }>()

const items = computed<{ action: InvoiceConfirmAction; label: string; danger?: boolean }[]>(() => {
  const inv = props.invoice
  if (inv.kind !== 'invoice') return []
  if (inv.status === 'draft') return [{ action: 'cancel', label: 'CANCEL DRAFT', danger: true }]
  const out: { action: InvoiceConfirmAction; label: string; danger?: boolean }[] = []
  if (inv.status === 'issued') out.push({ action: 'pay', label: 'MARK AS PAID' })
  if (inv.status === 'issued' || inv.status === 'paid') {
    out.push({ action: 'correct', label: 'CORRECT (CREDIT + NEW DRAFT)' })
    out.push({ action: 'credit', label: 'ISSUE CREDIT NOTE', danger: true })
  }
  return out
})

const label = computed(() => `Actions for ${invoiceNumberLabel(props.invoice)}`)
</script>

<template>
  <DropdownMenu v-if="items.length" :modal="false">
    <DropdownMenuTrigger as-child>
      <button type="button" class="btn-hud btn-hud-ghost btn-hud-sm iam-trigger" :aria-label="label">
        <MoreVertical style="width:14px;height:14px;" aria-hidden="true" />
        <span class="iam-text">ACTIONS</span>
      </button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="min-w-[220px]">
      <DropdownMenuItem
        v-for="it in items"
        :key="it.action"
        :class="it.danger ? 'text-error focus:text-error min-h-[36px]' : 'min-h-[36px]'"
        @select="emit('action', it.action)"
      >
        {{ it.label }}
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>

<style scoped>
.iam-trigger { gap: 4px; }
@media (max-width: 768px) {
  .iam-trigger { min-height: 44px; height: 44px; min-width: 44px; }
  .iam-text { display: none; }
}
</style>
