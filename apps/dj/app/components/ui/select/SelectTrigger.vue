<script setup lang="ts">
import { cn } from '@/lib/utils'
import { SelectIcon, SelectTrigger, type SelectTriggerProps, useForwardProps } from 'radix-vue'
import { ChevronDown } from 'lucide-vue-next'
import { computed } from 'vue'

const props = defineProps<SelectTriggerProps & { class?: string }>()

const delegatedProps = computed(() => {
  const { class: _, ...delegated } = props
  return delegated
})

const forwarded = useForwardProps(delegatedProps)
</script>

<template>
  <SelectTrigger
    v-bind="forwarded"
    :class="cn(
      'flex h-9 w-full items-center justify-between gap-2 rounded-none bg-surface-container-high px-3 py-2 text-sm text-on-surface',
      'border-0 border-l-2 border-transparent focus:outline-none focus:border-primary disabled:cursor-not-allowed disabled:opacity-50',
      '[&>span]:line-clamp-1',
      props.class,
    )"
  >
    <slot />
    <SelectIcon as-child>
      <ChevronDown class="h-4 w-4 opacity-50 shrink-0" />
    </SelectIcon>
  </SelectTrigger>
</template>
