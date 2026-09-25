<script setup lang="ts">
import { cn } from '#kui/lib/utils'
import { SelectLabel, type SelectLabelProps } from 'radix-vue'
import { computed } from 'vue'

const props = defineProps<SelectLabelProps & { class?: string }>()

const delegatedProps = computed(() => {
  const { class: _, ...delegated } = props
  return delegated
})
</script>

<template>
  <SelectLabel
    v-bind="delegatedProps"
    :class="cn('px-2 py-1.5 text-xs font-terminal tracking-terminal text-tertiary uppercase', props.class)"
  >
    <slot />
  </SelectLabel>
</template>
