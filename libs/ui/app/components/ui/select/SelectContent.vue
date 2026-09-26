<script setup lang="ts">
import { cn } from '#kui/lib/utils'
import { SelectContent, type SelectContentEmits, type SelectContentProps, SelectViewport, useForwardPropsEmits } from 'radix-vue'
import { computed } from 'vue'

const props = defineProps<SelectContentProps & { class?: string }>()
const emits = defineEmits<SelectContentEmits>()

const delegatedProps = computed(() => {
  const { class: _, ...delegated } = props
  return delegated
})

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <SelectContent
    v-bind="forwarded"
    :class="cn(
      'relative z-50 max-h-96 min-w-32 overflow-hidden rounded-none glass-panel-heavy text-on-surface',
      'data-[state=open]:animate-in data-[state=closed]:animate-out',
      'data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
      'data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
      'data-[side=bottom]:slide-in-from-top-2 data-[side=top]:slide-in-from-bottom-2',
      props.class,
    )"
  >
    <SelectViewport class="p-1">
      <slot />
    </SelectViewport>
  </SelectContent>
</template>
