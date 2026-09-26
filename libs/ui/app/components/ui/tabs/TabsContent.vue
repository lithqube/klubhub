<script setup lang="ts">
import { cn } from '#kui/lib/utils'
import { TabsContent, type TabsContentProps } from 'radix-vue'
import { computed } from 'vue'

const props = defineProps<TabsContentProps & { class?: string }>()

const delegatedProps = computed(() => {
  const { class: _, ...delegated } = props
  return delegated
})
</script>

<template>
  <TabsContent
    v-bind="delegatedProps"
    :class="cn(
      'mt-2 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary',
      props.class,
    )"
  >
    <slot />
  </TabsContent>
</template>
