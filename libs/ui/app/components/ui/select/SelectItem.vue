<script setup lang="ts">
import { cn } from '#kui/lib/utils'
import { SelectItem, SelectItemIndicator, type SelectItemProps, SelectItemText, useForwardProps } from 'radix-vue'
import { Check } from 'lucide-vue-next'
import { computed } from 'vue'

const props = defineProps<SelectItemProps & { class?: string }>()

const delegatedProps = computed(() => {
  const { class: _, ...delegated } = props
  return delegated
})

const forwarded = useForwardProps(delegatedProps)
</script>

<template>
  <SelectItem
    v-bind="forwarded"
    :class="cn(
      'relative flex w-full cursor-default select-none items-center rounded-none py-1.5 pl-8 pr-2 font-terminal tracking-terminal text-xs uppercase outline-none',
      'text-on-surface-variant focus:bg-surface-container-high focus:text-on-surface',
      'data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
      props.class,
    )"
  >
    <span class="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
      <SelectItemIndicator>
        <Check class="h-4 w-4 text-primary" />
      </SelectItemIndicator>
    </span>
    <SelectItemText>
      <slot />
    </SelectItemText>
  </SelectItem>
</template>
