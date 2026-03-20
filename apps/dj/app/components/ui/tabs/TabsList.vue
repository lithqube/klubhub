<script setup lang="ts">
import { cn } from '@/lib/utils'
import { TabsList, type TabsListProps } from 'radix-vue'
import { computed } from 'vue'

const props = defineProps<TabsListProps & { class?: string }>()

const delegatedProps = computed(() => {
  const { class: _, ...delegated } = props
  return delegated
})
</script>

<template>
  <TabsList
    v-bind="delegatedProps"
    :class="cn(
      'inline-flex h-10 items-center gap-0 bg-surface-container-low',
      props.class,
    )"
  >
    <slot />
  </TabsList>
</template>
