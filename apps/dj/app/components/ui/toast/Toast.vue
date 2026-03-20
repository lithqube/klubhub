<script setup lang="ts">
import { cn } from '@/lib/utils'
import { X } from 'lucide-vue-next'
import type { Toast } from './use-toast'

const props = defineProps<{
  toast: Toast
  class?: string
}>()

const emits = defineEmits<{
  (e: 'dismiss', id: string): void
}>()

const variantClasses = {
  default: 'glass-panel-heavy text-on-surface luminous-threshold',
  destructive: 'bg-error-container text-error shadow-glow-error border-0',
}
</script>

<template>
  <div
    :class="cn(
      'flex w-full items-start gap-3 rounded-none p-4 transition-all duration-300',
      variantClasses[toast.variant ?? 'default'],
      props.class,
    )"
    role="alert"
    aria-live="assertive"
  >
    <div class="flex-1 space-y-1">
      <p v-if="toast.title" class="font-command tracking-command text-sm font-medium">
        {{ toast.title }}
      </p>
      <p v-if="toast.description" class="text-xs text-on-surface-variant">
        {{ toast.description }}
      </p>
    </div>
    <button
      class="ml-auto shrink-0 rounded-none opacity-70 hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-primary"
      @click="emits('dismiss', toast.id)"
    >
      <X class="h-4 w-4" />
      <span class="sr-only">Close</span>
    </button>
  </div>
</template>
