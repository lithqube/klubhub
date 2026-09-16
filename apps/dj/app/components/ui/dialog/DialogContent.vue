<script setup lang="ts">
import { cn } from '@/lib/utils'
import { DialogClose, DialogContent, type DialogContentEmits, type DialogContentProps, DialogOverlay, DialogPortal, useForwardPropsEmits } from 'radix-vue'
import { X } from 'lucide-vue-next'
import { computed } from 'vue'

const props = defineProps<DialogContentProps & { class?: string }>()
const emits = defineEmits<DialogContentEmits>()

const delegatedProps = computed(() => {
  const { class: _, ...delegated } = props
  return delegated
})

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <DialogPortal>
    <DialogOverlay class="fixed inset-0 z-50 bg-surface/80 backdrop-blur-glass data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0" />
    <DialogContent
      v-bind="forwarded"
      :class="cn(
        'fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2',
        'rounded-none glass-panel-heavy text-on-surface luminous-threshold p-6',
        'duration-200 data-[state=open]:animate-in data-[state=closed]:animate-out',
        'data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
        'data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
        'data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-48%',
        'data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-48%',
        props.class,
      )"
    >
      <slot />
      <DialogClose class="absolute right-4 top-4 rounded-none opacity-70 text-on-surface-variant transition-opacity hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-primary disabled:pointer-events-none">
        <X class="h-4 w-4" />
        <span class="sr-only">Close</span>
      </DialogClose>
    </DialogContent>
  </DialogPortal>
</template>
