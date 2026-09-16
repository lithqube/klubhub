<script setup lang="ts">
import { DialogRoot, DialogPortal, DialogOverlay, DialogContent, DialogClose } from 'radix-vue'
import { cn } from '@/lib/utils'
import { X } from 'lucide-vue-next'

interface Props {
  side?: 'top' | 'right' | 'bottom' | 'left'
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  side: 'right',
})

const sideClasses = {
  top: 'inset-x-0 top-0 border-b border-surface-container-high',
  right: 'inset-y-0 right-0 h-full w-3/4 border-l border-surface-container-high sm:max-w-sm',
  bottom: 'inset-x-0 bottom-0 border-t border-surface-container-high',
  left: 'inset-y-0 left-0 h-full w-3/4 border-r border-surface-container-high sm:max-w-sm',
}
</script>

<template>
  <DialogRoot>
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-50 bg-surface/80 backdrop-blur-glass data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0" />
      <DialogContent
        :class="cn(
          'fixed z-50 gap-4 glass-panel-heavy p-6 shadow-glow-primary transition ease-in-out',
          'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:duration-300 data-[state=open]:duration-500',
          sideClasses[side],
          props.class
        )"
      >
        <slot />
        <DialogClose
          class="absolute right-4 top-4 rounded-none opacity-70 transition-opacity hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-primary disabled:pointer-events-none text-tertiary hover:text-on-surface"
        >
          <X class="h-4 w-4" />
          <span class="sr-only">Close</span>
        </DialogClose>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
