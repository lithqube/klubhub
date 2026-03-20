<script setup lang="ts">
import { cn } from '@/lib/utils'
import { TabsTrigger, type TabsTriggerProps } from 'radix-vue'
import { computed } from 'vue'

const props = defineProps<TabsTriggerProps & { class?: string }>()

const delegatedProps = computed(() => {
  const { class: _, ...delegated } = props
  return delegated
})
</script>

<template>
  <!-- Underline tab style: active gets cyan bottom border, no fill -->
  <TabsTrigger
    v-bind="delegatedProps"
    :class="cn(
      'inline-flex items-center justify-center px-4 py-2 text-sm font-command tracking-command',
      'border-b-2 border-transparent rounded-none',
      'text-tertiary hover:text-on-surface transition-colors duration-150',
      'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary',
      'disabled:pointer-events-none disabled:opacity-50',
      'data-[state=active]:border-primary data-[state=active]:text-primary data-[state=active]:bg-transparent',
      props.class,
    )"
  >
    <slot />
  </TabsTrigger>
</template>
